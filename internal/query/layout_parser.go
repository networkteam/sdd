package query

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model"
)

// ParseLayout parses a `--layout=<spec>` string into a Layout AST.
//
// Slice 1 grammar (subset of d-tac-uww §3 — paren args arrive in slice 2):
//
//	layout  := section ("," section)*
//	section := func (":" func)*
//	func    := name
//	name    := [a-zA-Z][a-zA-Z0-9_-]*
//
// The parser is permissive on names: any well-formed name parses, and the
// executor rejects unknown ones at runtime with a clear listed-valid-set
// error. This keeps the parser stable as the function vocabulary grows.
//
// Errors point to a 0-based byte position so users can locate the issue in
// the source string.
func ParseLayout(s string) (model.Layout, error) {
	if s == "" {
		return model.Layout{}, fmt.Errorf("layout: empty layout")
	}

	var layout model.Layout
	var section model.Section

	pos := 0
	for pos < len(s) {
		// Read a function name. The current byte must start a valid name.
		if !isNameStart(s[pos]) {
			return model.Layout{}, fmt.Errorf("layout: at position %d: expected function name (letter), got %q", pos, s[pos])
		}
		start := pos
		pos++
		for pos < len(s) && isNameContinue(s[pos]) {
			pos++
		}
		section.Functions = append(section.Functions, model.Function{Name: s[start:pos]})

		if pos == len(s) {
			break
		}

		switch s[pos] {
		case ':':
			pos++
			if pos == len(s) {
				return model.Layout{}, fmt.Errorf("layout: at position %d: trailing colon (expected function name)", pos)
			}
			// Loop continues; next iteration parses the next function in this section.
		case ',':
			layout.Sections = append(layout.Sections, section)
			section = model.Section{}
			pos++
			if pos == len(s) {
				return model.Layout{}, fmt.Errorf("layout: at position %d: trailing comma (expected section)", pos)
			}
		default:
			return model.Layout{}, fmt.Errorf("layout: at position %d: unexpected character %q", pos, s[pos])
		}
	}

	// Append the final accumulating section. Reachable only when the loop
	// exits via end-of-input after reading a function name (not via ',' or ':').
	if len(section.Functions) > 0 {
		layout.Sections = append(layout.Sections, section)
	}

	return layout, nil
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
