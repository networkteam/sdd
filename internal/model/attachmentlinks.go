package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateAttachmentLinks checks that references to the entry's attachment
// directory point to files that exist in the entry's Attachments list — both
// the pre-resolution {{attachments}}/ placeholder (the only form authorable
// before an ID is minted) and the resolved ./<shortname>/ form. References
// inside code are documentation of the syntax, not a claim to carry the file,
// so the scan reads past them (s-tac-ujs).
func validateAttachmentLinks(e *Entry) {
	prefixes := []string{"{{attachments}}/"}
	if len(e.ID) >= 8 {
		prefixes = append(prefixes, "./"+e.ID[6:]+"/") // DD-HHmmss-type-layer-suffix
	}

	knownFiles := make(map[string]bool)
	for _, a := range e.Attachments {
		knownFiles[filepath.Base(a)] = true
	}

	scan := maskCodeRegions(e.Content)
	for _, prefix := range prefixes {
		for off := 0; off < len(scan); {
			idx := strings.Index(scan[off:], prefix)
			if idx < 0 {
				break
			}
			start := off + idx + len(prefix)
			// The prefix matched outside code, so the filename that follows is
			// unmasked too — read it from the original.
			after := e.Content[start:]
			filename := after
			if end := strings.IndexAny(after, ") \n\t\"'"); end >= 0 {
				filename = after[:end]
			}
			if filename != "" && !knownFiles[filename] {
				e.Warnings = append(e.Warnings, Warning{
					Field:   "attachments",
					Value:   prefix + filename,
					Message: fmt.Sprintf("broken attachment link: %s%s (file not found in attachment directory)", prefix, filename),
				})
			}
			off = start
		}
	}
}

// maskCodeRegions blanks every byte inside a fenced code block, an indented
// code block, or an inline code span, preserving newlines so offsets and line
// structure carry over to the original. Masking errs towards skipping: a
// missed warning is recoverable, a wrong one blocks a write with no override
// path (d-cpt-20r).
func maskCodeRegions(content string) string {
	out := []byte(content)

	var (
		fenceChar byte
		fenceLen  int
		inFence   bool
		inIndent  bool
	)
	prevBlank := true

	for start := 0; start < len(content); {
		end := strings.IndexByte(content[start:], '\n')
		if end < 0 {
			end = len(content)
		} else {
			end += start
		}
		line := content[start:end]
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		blank := trimmed == ""

		switch char, run := fenceRun(trimmed); {
		case inFence:
			blankRange(out, start, end)
			if char == fenceChar && run >= fenceLen && indent < 4 && strings.TrimSpace(trimmed[run:]) == "" {
				inFence = false
			}
		case inIndent && (blank || indent >= 4):
			blankRange(out, start, end)
		case run > 0 && indent < 4:
			fenceChar, fenceLen, inFence, inIndent = char, run, true, false
			blankRange(out, start, end)
		case prevBlank && indent >= 4 && !blank:
			inIndent = true
			blankRange(out, start, end)
		default:
			inIndent = false
		}

		prevBlank = blank
		start = end + 1
	}

	maskSpans(out)
	return string(out)
}

// fenceRun reports the fence character and length of a line's opening run,
// or a zero run when the line does not start a CommonMark fence.
func fenceRun(trimmed string) (byte, int) {
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0
	}
	char := trimmed[0]
	run := 0
	for run < len(trimmed) && trimmed[run] == char {
		run++
	}
	if run < 3 {
		return 0, 0
	}
	return char, run
}

// maskSpans blanks inline code spans in already block-masked bytes: a backtick
// run opens a span that the next run of exactly equal length closes. An
// unmatched run is a literal backtick and masks nothing.
func maskSpans(b []byte) {
	for i := 0; i < len(b); {
		if b[i] != '`' {
			i++
			continue
		}
		openEnd := i
		for openEnd < len(b) && b[openEnd] == '`' {
			openEnd++
		}
		closeEnd := closingRun(b, openEnd, openEnd-i)
		if closeEnd < 0 {
			i = openEnd
			continue
		}
		blankRange(b, i, closeEnd)
		i = closeEnd
	}
}

// closingRun returns the end offset of the next backtick run of exactly n
// bytes, or -1 when none arrives before a blank line — a code span never
// spans one.
func closingRun(b []byte, from, n int) int {
	for i := from; i < len(b); {
		switch b[i] {
		case '`':
			runStart := i
			for i < len(b) && b[i] == '`' {
				i++
			}
			if i-runStart == n {
				return i
			}
		case '\n':
			i++
			for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
				i++
			}
			if i >= len(b) || b[i] == '\n' {
				return -1
			}
		default:
			i++
		}
	}
	return -1
}

func blankRange(b []byte, start, end int) {
	for i := start; i < end; i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}
