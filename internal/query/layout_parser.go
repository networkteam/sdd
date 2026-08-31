package query

import (
	"fmt"
	"strconv"

	"github.com/networkteam/sdd/internal/model"
)

// ParseLayout parses a `--layout=<spec>` string into a Layout AST.
//
// Slice 2 grammar (per d-tac-uww §3):
//
//	layout    := section ("," section)*
//	section   := func (":" func)*
//	func      := name ("(" arg-list? ")")?
//	arg-list  := arg ("," arg)*
//	arg       := func | number | identifier | string
//	name      := [a-zA-Z][a-zA-Z0-9_-]*
//	number    := -?[0-9]+ ("." [0-9]+)?
//	identifier:= same shape as name (leading letter required)
//	string    := "..." | '...'   (no escapes; cannot contain its own delimiter)
//
// Comma at paren depth 0 separates sections; comma at depth > 0 separates
// args. The parser is permissive on names: any well-formed name parses,
// and the executor rejects unknown ones at runtime with a clear listed-
// valid-set error. Errors point to a 0-based byte position.
func ParseLayout(s string) (model.Layout, error) {
	if s == "" {
		return model.Layout{}, fmt.Errorf("layout: empty layout")
	}
	p := &parser{src: s}
	layout, err := p.parseLayout()
	if err != nil {
		return model.Layout{}, err
	}
	if p.pos != len(p.src) {
		return model.Layout{}, p.errAtf(p.pos, "unexpected character %q", string(p.src[p.pos]))
	}
	return layout, nil
}

// parser is a position-tracking recursive-descent parser. Each parse
// method advances p.pos through src and returns errors carrying the byte
// position where parsing failed.
type parser struct {
	src string
	pos int
}

func (p *parser) errAtf(pos int, format string, args ...any) error {
	return fmt.Errorf("layout: at position %d: %s", pos, fmt.Sprintf(format, args...))
}

// parseLayout reads `section ("," section)*`. Trailing input that isn't a
// section separator surfaces in the caller via the post-parse position
// check.
func (p *parser) parseLayout() (model.Layout, error) {
	var layout model.Layout
	for {
		section, err := p.parseSection()
		if err != nil {
			return model.Layout{}, err
		}
		layout.Sections = append(layout.Sections, section)
		if p.pos == len(p.src) {
			return layout, nil
		}
		if p.src[p.pos] != ',' {
			// Caller (ParseLayout) reports the unexpected character.
			return layout, nil
		}
		p.pos++ // consume section separator ','
		if p.pos == len(p.src) {
			return model.Layout{}, p.errAtf(p.pos, "trailing comma (expected section)")
		}
	}
}

// parseSection reads `func (":" func)*` until it hits a section boundary
// (',' at depth 0, end-of-input, or unexpected character that the caller
// will report).
func (p *parser) parseSection() (model.Section, error) {
	var section model.Section
	start := p.pos
	for {
		fn, err := p.parseFunction()
		if err != nil {
			return model.Section{}, err
		}
		section.Functions = append(section.Functions, fn)
		if p.pos == len(p.src) || p.src[p.pos] != ':' {
			section.Source = p.src[start:p.pos]
			return section, nil
		}
		p.pos++ // consume ':'
		if p.pos == len(p.src) {
			return model.Section{}, p.errAtf(p.pos, "trailing colon (expected function name)")
		}
	}
}

// parseFunction reads `name ("(" arg-list? ")")?`. The arg-list is
// optional — bare names are functions with no Args.
func (p *parser) parseFunction() (model.Function, error) {
	if p.pos == len(p.src) {
		return model.Function{}, p.errAtf(p.pos, "expected function name")
	}
	if !isNameStart(p.src[p.pos]) {
		return model.Function{}, p.errAtf(p.pos, "expected function name (letter), got %q", string(p.src[p.pos]))
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.src) && isNameContinue(p.src[p.pos]) {
		p.pos++
	}
	fn := model.Function{Name: p.src[start:p.pos]}

	if p.pos < len(p.src) && p.src[p.pos] == '(' {
		p.pos++ // consume '('
		args, err := p.parseArgList()
		if err != nil {
			return model.Function{}, err
		}
		if p.pos == len(p.src) {
			return model.Function{}, p.errAtf(p.pos, "expected ')'")
		}
		if p.src[p.pos] != ')' {
			return model.Function{}, p.errAtf(p.pos, "expected ')', got %q%s", string(p.src[p.pos]), quotingHint(p.src[p.pos]))
		}
		p.pos++ // consume ')'
		fn.Args = args
	}
	return fn, nil
}

// parseArgList reads `arg ("," arg)*`. Empty arg list (immediate ')') is
// permitted; trailing/leading/double commas surface as positioned errors.
func (p *parser) parseArgList() ([]model.FunctionArg, error) {
	if p.pos < len(p.src) && p.src[p.pos] == ')' {
		return nil, nil
	}
	var args []model.FunctionArg
	for {
		arg, err := p.parseArg()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.pos == len(p.src) {
			return nil, p.errAtf(p.pos, "expected ')' or ','")
		}
		if p.src[p.pos] != ',' {
			return args, nil
		}
		p.pos++ // consume ','
		if p.pos == len(p.src) {
			return nil, p.errAtf(p.pos, "expected argument after ','")
		}
		if p.src[p.pos] == ')' || p.src[p.pos] == ',' {
			return nil, p.errAtf(p.pos, "expected argument, got %q", string(p.src[p.pos]))
		}
	}
}

// parseArg dispatches based on the leading character: quotes start a
// string, digits/'-' start a number, letters start an identifier or
// nested function call.
func (p *parser) parseArg() (model.FunctionArg, error) {
	if p.pos == len(p.src) {
		return model.FunctionArg{}, p.errAtf(p.pos, "expected argument")
	}
	c := p.src[p.pos]
	switch {
	case c == '"' || c == '\'':
		return p.parseString(c)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	case isNameStart(c):
		return p.parseIdentOrFunc()
	default:
		return model.FunctionArg{}, p.errAtf(p.pos, "unexpected character %q in argument", string(c))
	}
}

// parseIdentOrFunc reads a name-shaped token, then peeks for '(' to
// decide between a bare identifier and a nested function call. The
// rewind-and-recurse path keeps function parsing in one place.
func (p *parser) parseIdentOrFunc() (model.FunctionArg, error) {
	start := p.pos
	p.pos++
	for p.pos < len(p.src) && isNameContinue(p.src[p.pos]) {
		p.pos++
	}
	name := p.src[start:p.pos]
	if p.pos < len(p.src) && p.src[p.pos] == '(' {
		p.pos = start // rewind; parseFunction reads the name itself
		fn, err := p.parseFunction()
		if err != nil {
			return model.FunctionArg{}, err
		}
		nested := fn
		return model.FunctionArg{Kind: model.ArgKindFunc, Func: &nested}, nil
	}
	return model.FunctionArg{Kind: model.ArgKindIdent, String: name}, nil
}

// parseNumber reads `-?[0-9]+ ("." [0-9]+)?`. Parses to float64 — integer
// consumers (e.g. n(N)) validate integrality at execute time. Rejects
// scientific notation and multiple decimal points.
func (p *parser) parseNumber() (model.FunctionArg, error) {
	start := p.pos
	if p.src[p.pos] == '-' {
		p.pos++
		if p.pos == len(p.src) || p.src[p.pos] < '0' || p.src[p.pos] > '9' {
			return model.FunctionArg{}, p.errAtf(start, "expected digit after '-'")
		}
	}
	for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') {
		p.pos++
	}
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
		if p.pos == len(p.src) || p.src[p.pos] < '0' || p.src[p.pos] > '9' {
			return model.FunctionArg{}, p.errAtf(p.pos, "expected digit after '.'")
		}
		for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') {
			p.pos++
		}
	}
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		return model.FunctionArg{}, p.errAtf(p.pos, "unexpected '.' in number")
	}
	text := p.src[start:p.pos]
	n, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return model.FunctionArg{}, p.errAtf(start, "invalid number %q: %v", text, err)
	}
	return model.FunctionArg{Kind: model.ArgKindNumber, Number: n}, nil
}

// parseString reads a quote-delimited literal. No escape sequences in
// slice 2 — strings cannot contain their own delimiter; if a use case
// needs escapes later, they slot in here without grammar churn.
func (p *parser) parseString(delim byte) (model.FunctionArg, error) {
	openPos := p.pos
	p.pos++ // consume opening quote
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != delim {
		p.pos++
	}
	if p.pos == len(p.src) {
		return model.FunctionArg{}, fmt.Errorf("layout: unterminated string starting at position %d", openPos)
	}
	content := p.src[start:p.pos]
	p.pos++ // consume closing quote
	return model.FunctionArg{Kind: model.ArgKindString, String: content}, nil
}

// quotingHint returns an actionable suffix when the character that broke
// argument parsing looks like the middle of an unquoted value — a space
// (multi-word name), a hyphen (ISO date), or an alphanumeric run (duration
// like 7d). These are the arguments that must be quoted, and the bare
// "expected ')'" error left that undiscoverable. Any other character gets no
// hint, so genuine syntax mistakes aren't drowned in irrelevant advice.
//
// The character classes mirror the bare-token lexer (isNameStart/parseNumber):
// a bare token stops at the first char outside those classes, so the char we
// tripped on is exactly what a bare value would have continued into. If the
// bare-token rules change, revisit this set.
func quotingHint(c byte) string {
	if c == ' ' || c == '-' || (c >= '0' && c <= '9') || isNameStart(c) {
		return ` — quote multi-word names, dates, and durations, e.g. since("7d") or participant("Jonathan Philipp")`
	}
	return ""
}

func isNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameContinue(c byte) bool {
	return isNameStart(c) ||
		(c >= '0' && c <= '9') ||
		c == '_' ||
		c == '-'
}
