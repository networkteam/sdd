package engine

import (
	"fmt"
	"strings"
	"unicode"
)

// Guard grammar — deliberately poor, per the surface spec:
//
//	expr := predicateName
//	      | expr "and" expr
//	      | expr "or" expr
//	      | "not" expr
//	      | "(" expr ")"
//
// Boolean combinations of named Go predicates. Nothing else — no
// comparisons, no literals, no field access, no assignment. Logic that
// doesn't fit is either a new registry function (Go, unit-tested) or a
// chooser (judgment). Precedence: not > and > or.

// GuardExpr is a parsed guard expression.
type GuardExpr struct {
	root guardNode
	src  string
}

// String returns the original expression source.
func (g *GuardExpr) String() string { return g.src }

// Predicates returns the distinct predicate names the expression references,
// in first-appearance order. Used by spec validation (every name must exist
// in the registry) and by stall diagnostics.
func (g *GuardExpr) Predicates() []string {
	seen := make(map[string]bool)
	var names []string
	var walk func(n guardNode)
	walk = func(n guardNode) {
		switch node := n.(type) {
		case *predNode:
			if !seen[node.name] {
				seen[node.name] = true
				names = append(names, node.name)
			}
		case *notNode:
			walk(node.expr)
		case *binNode:
			walk(node.left)
			walk(node.right)
		}
	}
	walk(g.root)
	return names
}

// Eval evaluates the expression against a predicate evaluator. Predicates
// are pure, so full evaluation (no short-circuit) is safe and keeps stall
// diagnostics complete.
func (g *GuardExpr) Eval(eval func(name string) (bool, error)) (bool, error) {
	return g.root.eval(eval)
}

type guardNode interface {
	eval(func(string) (bool, error)) (bool, error)
}

type predNode struct{ name string }

func (n *predNode) eval(eval func(string) (bool, error)) (bool, error) {
	return eval(n.name)
}

type notNode struct{ expr guardNode }

func (n *notNode) eval(eval func(string) (bool, error)) (bool, error) {
	v, err := n.expr.eval(eval)
	return !v, err
}

type binNode struct {
	op          string // "and" | "or"
	left, right guardNode
}

func (n *binNode) eval(eval func(string) (bool, error)) (bool, error) {
	l, err := n.left.eval(eval)
	if err != nil {
		return false, err
	}
	r, err := n.right.eval(eval)
	if err != nil {
		return false, err
	}
	if n.op == "and" {
		return l && r, nil
	}
	return l || r, nil
}

// ParseGuard parses a guard expression. Whitespace (including newlines from
// folded YAML scalars) separates tokens.
func ParseGuard(src string) (*GuardExpr, error) {
	tokens, err := tokenizeGuard(src)
	if err != nil {
		return nil, err
	}
	p := &guardParser{tokens: tokens, src: src}
	root, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("guard %q: unexpected token %q", src, p.tokens[p.pos])
	}
	return &GuardExpr{root: root, src: src}, nil
}

func tokenizeGuard(src string) ([]string, error) {
	var tokens []string
	i := 0
	for i < len(src) {
		c := rune(src[i])
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(' || c == ')':
			tokens = append(tokens, string(c))
			i++
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(src) {
				r := rune(src[j])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
					break
				}
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		default:
			return nil, fmt.Errorf("guard %q: unexpected character %q — guards are boolean combinations of predicate names only (no comparisons, literals, or field access)", src, string(c))
		}
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty guard expression")
	}
	return tokens, nil
}

type guardParser struct {
	tokens []string
	pos    int
	src    string
}

func (p *guardParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *guardParser) parseOr() (guardNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "or" {
		p.pos++
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &binNode{op: "or", left: left, right: right}
	}
	return left, nil
}

func (p *guardParser) parseAnd() (guardNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek() == "and" {
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &binNode{op: "and", left: left, right: right}
	}
	return left, nil
}

func (p *guardParser) parseUnary() (guardNode, error) {
	switch tok := p.peek(); tok {
	case "not":
		p.pos++
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &notNode{expr: expr}, nil
	case "(":
		p.pos++
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("guard %q: missing closing parenthesis", p.src)
		}
		p.pos++
		return expr, nil
	case "", ")", "and", "or":
		return nil, fmt.Errorf("guard %q: expected predicate name, got %q", p.src, tok)
	default:
		if !isValidFuncName(tok) {
			return nil, fmt.Errorf("guard %q: invalid predicate name %q", p.src, tok)
		}
		p.pos++
		return &predNode{name: tok}, nil
	}
}

// isValidFuncName reports whether s is a plausible registry function name:
// a Go-style identifier. Reserved words are handled by the parser before
// this check.
func isValidFuncName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if unicode.IsLetter(r) || r == '_' {
			continue
		}
		if i > 0 && unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return !strings.Contains(" and or not ", " "+s+" ")
}
