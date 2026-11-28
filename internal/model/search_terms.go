package model

import (
	"fmt"
	"regexp"
	"strings"
)

// CompileSearchTerms compiles each search term into a case-insensitive regex
// (opt out with (?-i) in the term). Blank terms are dropped so a stray empty
// term never degrades to match-all.
func CompileSearchTerms(terms []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		re, err := regexp.Compile("(?i)" + t)
		if err != nil {
			return nil, fmt.Errorf("compiling term %q: %w", t, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// MatchesSearchTerms reports whether every term matches the entry's summary
// or body — text search's AND semantics over the in-memory entry text.
func (e *Entry) MatchesSearchTerms(terms []*regexp.Regexp) bool {
	text := e.Summary + "\n" + e.Content
	for _, re := range terms {
		if !re.MatchString(text) {
			return false
		}
	}
	return true
}
