// Package mdcompose composes Markdown fragments into one coherent document.
// An embedded fragment — an entry body, a generated section — is demoted
// beneath the headings of the structure serving it, so no embedded heading
// outranks its container (d-cpt-5wv).
//
// Headings are recognized in ATX form (`#` … `######`) outside fenced code
// blocks. Setext underlining is left untouched: graph entry bodies and served
// fragments are authored ATX throughout, and rewriting an underline would mean
// rewriting two lines on a guess about which one is prose.
package mdcompose

import "strings"

// MaxHeadingLevel is Markdown's deepest ATX heading. Demotion clamps here: a
// seventh `#` stops being a heading at all, so a fragment pushed past the floor
// collapses its deepest levels rather than degrading into paragraph text.
const MaxHeadingLevel = 6

// DemoteTo shifts every heading in fragment by one constant amount, so that its
// shallowest heading lands at level and the relative hierarchy is preserved. A
// fragment whose headings already sit at or below level is returned unchanged —
// the rule demotes into place, it never promotes a fragment toward the surface.
func DemoteTo(fragment string, level int) string {
	top := TopHeadingLevel(fragment)
	if top == 0 || top >= level {
		return fragment
	}
	by := level - top
	lines := strings.Split(fragment, "\n")
	scan(fragment, func(i int, _ string, h heading, isHeading bool) {
		if isHeading {
			lines[i] = h.render(h.level + by)
		}
	})
	return strings.Join(lines, "\n")
}

// SplitLeadingHeading splits a leading heading off fragment: when its first
// non-blank line is a heading, that heading's text is returned separately from
// the remaining body, so a composer can lift it into the container's own
// structure instead of nesting a title under a title. Otherwise title is empty
// and rest is fragment unchanged.
func SplitLeadingHeading(fragment string) (title, rest string) {
	lines := strings.Split(fragment, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if _, ok := parseFence(line); ok {
			return "", fragment
		}
		h, ok := parseHeading(line)
		if !ok {
			return "", fragment
		}
		return h.text, strings.TrimLeft(strings.Join(lines[i+1:], "\n"), "\n")
	}
	return "", fragment
}

// TopHeadingLevel reports the shallowest heading level in fragment, or 0 when it
// carries no heading. Callers composing their own structure use it to work out
// how deep an embedded fragment already sits.
func TopHeadingLevel(fragment string) int {
	top := 0
	scan(fragment, func(_ int, _ string, h heading, isHeading bool) {
		if isHeading && (top == 0 || h.level < top) {
			top = h.level
		}
	})
	return top
}

// heading is one parsed ATX heading line.
type heading struct {
	indent string
	level  int
	text   string
}

// render writes the heading back out at the given level, clamped to the ATX
// floor. A heading with no text keeps its bare-marker form.
func (h heading) render(level int) string {
	if level > MaxHeadingLevel {
		level = MaxHeadingLevel
	}
	if level < 1 {
		level = 1
	}
	out := h.indent + strings.Repeat("#", level)
	if h.text != "" {
		out += " " + h.text
	}
	return out
}

// parseHeading recognizes an ATX heading line: up to three spaces of
// indentation (four makes it an indented code block), one to six `#`, then
// either the end of the line or whitespace separating the text. A trailing
// closing sequence (`## Title ##`) is dropped, matching how Markdown renders it.
func parseHeading(line string) (heading, bool) {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]
	if len(indent) > 3 {
		return heading{}, false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > MaxHeadingLevel {
		return heading{}, false
	}
	rest := trimmed[level:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return heading{}, false
	}
	return heading{indent: indent, level: level, text: stripClosingSequence(rest)}, true
}

// stripClosingSequence trims a heading's text, dropping the optional closing run
// of `#` that Markdown allows (`## Title ##`). A run fused to the text
// (`## C#`) is part of the text and stays.
func stripClosingSequence(rest string) string {
	text := strings.TrimSpace(rest)
	stripped := strings.TrimRight(text, "#")
	if stripped == text {
		return text
	}
	if stripped == "" {
		return ""
	}
	if trimmed := strings.TrimRight(stripped, " \t"); trimmed != stripped {
		return trimmed
	}
	return text
}

// scan walks fragment line by line, handing each line to fn along with its
// parsed heading when the line is one — never inside a fenced code block, where
// a `#` is content and rewriting it would corrupt the sample.
func scan(fragment string, fn func(i int, line string, h heading, isHeading bool)) {
	var fence fenceDelimiter
	for i, line := range strings.Split(fragment, "\n") {
		if delim, ok := parseFence(line); ok {
			switch {
			case fence.length == 0:
				fence = delim
			case delim.char == fence.char && delim.length >= fence.length && delim.info == "":
				fence = fenceDelimiter{}
			}
			fn(i, line, heading{}, false)
			continue
		}
		if fence.length > 0 {
			fn(i, line, heading{}, false)
			continue
		}
		h, ok := parseHeading(line)
		fn(i, line, h, ok)
	}
}

// fenceDelimiter is one parsed code-fence line. info carries whatever follows
// the run — an opening fence may name a language, a closing fence may not.
type fenceDelimiter struct {
	char   byte
	length int
	info   string
}

// parseFence recognizes a code-fence delimiter: at least three backticks or
// tildes, indented no more than three spaces.
func parseFence(line string) (fenceDelimiter, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || trimmed == "" {
		return fenceDelimiter{}, false
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return fenceDelimiter{}, false
	}
	length := 0
	for length < len(trimmed) && trimmed[length] == char {
		length++
	}
	if length < 3 {
		return fenceDelimiter{}, false
	}
	return fenceDelimiter{char: char, length: length, info: strings.TrimSpace(trimmed[length:])}, true
}
