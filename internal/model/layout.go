package model

// Layout is the parsed root of a `--layout=...` argument to `sdd view`.
// Each Section corresponds to one comma-separated entry in the source
// string; sections are rendered in source order.
type Layout struct {
	Sections []Section
}

// Section is one colon-chained pipeline within a Layout. Functions execute
// left-to-right: filters and transforms accumulate, the section terminates
// in a render function (e.g. as-list).
type Section struct {
	Functions []Function
}

// Function is one step in a Section's pipeline — a name plus optional
// positional arguments captured in source order.
type Function struct {
	Name string
	Args []FunctionArg
}

// ArgKind tags the variant of a FunctionArg. Args are either nested
// function calls (e.g. `heat(exp-14d)` inside `rank(...)`) or literal
// values (numbers, bare identifiers, quoted strings).
type ArgKind string

const (
	// ArgKindFunc is a nested function call; FunctionArg.Func is populated.
	ArgKindFunc ArgKind = "func"
	// ArgKindNumber is a numeric literal; FunctionArg.Number is populated.
	// Floats are accepted at parse time so future modifiers like
	// stalled(0.5) don't churn the type; integer-only consumers (e.g. n(N))
	// validate integrality at execute time.
	ArgKindNumber ArgKind = "number"
	// ArgKindIdent is a bare identifier (unquoted, e.g. `plan`, `exp-14d`);
	// FunctionArg.String holds the raw text.
	ArgKindIdent ArgKind = "ident"
	// ArgKindString is a quoted string literal; FunctionArg.String holds
	// the unquoted content.
	ArgKindString ArgKind = "string"
)

// FunctionArg is one positional argument to a Function. Inspect Kind to
// dispatch — exactly one of Func / Number / String is meaningful.
type FunctionArg struct {
	Kind   ArgKind
	Func   *Function
	Number float64
	String string
}

// UsesFunction reports whether any section calls a function with the given
// name. Callers inspect the expanded layout — so a filter injected by a macro
// counts the same as one written by hand — to tailor feedback, e.g. naming
// known participants when a participant() filter matched nothing.
func (l Layout) UsesFunction(name string) bool {
	for _, section := range l.Sections {
		for _, fn := range section.Functions {
			if fn.Name == name {
				return true
			}
		}
	}
	return false
}
