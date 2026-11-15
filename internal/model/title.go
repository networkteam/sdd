package model

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ListingTitle names the entry on a one-line listing: its title, or the
// first summary sentence for an entry that has none yet.
func (e *Entry) ListingTitle() string {
	if e.Title != "" {
		return e.Title
	}
	return e.FirstSummarySentence()
}

// TitleRule is the guidance every title writer follows, the title model and
// an agent writing a title alike, rendered from here into each surface that
// states it.
const TitleRule = "A title names the entry's subject for a reader scanning a list of entries: a noun phrase of 3 to 8 plain words, at most 50 characters, starting with a capital letter, like \"Daemon project removal\". Write code terms (a command, identifier, file or package name) and proper nouns (a product, tool, service or company name) as backtick code spans in their own casing wherever they appear; a title naming one first opens with the code span, like \"`sdd` info JSON output\", \"`macOS` signing failure\" or \"`GitHub` token rotation\". A title uses no quotation marks: a code term or proper noun is a code span instead."

// titleTrailers are trimmed from a title's end: a title names a subject and
// closes on none of them. A question mark stays, as do a closing code span or
// bracket.
const titleTrailers = ".,;:!…-–—"

// titleQuotes are removed wherever they stand (TitleRule). Single quotes stay:
// they double as apostrophes.
const titleQuotes = "\"“”„«»"

// NormalizeTitle is the form every stored title takes, whatever wrote it:
// double quotes removed, whitespace trimmed from both ends, trailing
// punctuation but a question mark trimmed from the end, and the first letter
// capitalized, since listings show titles in a column and a mix of cases reads
// as noise there. A title opening with a code span keeps its casing, a
// backtick having no capital.
func NormalizeTitle(title string) string {
	title = strings.Map(func(r rune) rune {
		if strings.ContainsRune(titleQuotes, r) {
			return -1
		}
		return r
	}, title)
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}
	title = strings.TrimRightFunc(strings.TrimSpace(title), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(titleTrailers, r)
	})
	first, size := utf8.DecodeRuneInString(title)
	if size == 0 {
		return title
	}
	return string(unicode.ToTitle(first)) + title[size:]
}
