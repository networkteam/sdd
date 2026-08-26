// Package textpatch applies ordered exact search-replace pairs to a text
// atomically: each pair's Old must match exactly once in the text as it
// stands when that pair applies, pairs apply in sequence, and the first
// failing pair aborts the whole application (20260826-120330-d-tac-8f8).
package textpatch

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Pair is one exact search-replace edit.
type Pair struct {
	Old string
	New string
}

// Apply returns text with every pair applied in order, or an error naming the
// first failing pair — Old empty, absent, or matching more than once. On
// error the input text is unchanged by contract: nothing partial escapes.
func Apply(text string, pairs []Pair) (string, error) {
	out := text
	for i, p := range pairs {
		if p.Old == "" {
			return "", fmt.Errorf("pair %d: old text is empty", i+1)
		}
		switch n := strings.Count(out, p.Old); n {
		case 1:
			out = strings.Replace(out, p.Old, p.New, 1)
		case 0:
			return "", fmt.Errorf("pair %d: old text not found: %s", i+1, excerpt(p.Old))
		default:
			return "", fmt.Errorf("pair %d: old text matches %d times, must match exactly once: %s", i+1, n, excerpt(p.Old))
		}
	}
	return out, nil
}

// excerpt bounds a pair's Old text for error messages, cut at a rune boundary.
func excerpt(s string) string {
	const max = 80
	if len(s) <= max {
		return fmt.Sprintf("%q", s)
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%q…", s[:cut])
}
