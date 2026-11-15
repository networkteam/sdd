package model

import (
	"errors"
	"fmt"
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
// backtick having no capital; a quoted word opening it is a code term quoted
// rather than set as a code span, and becomes one. Normalizing a normalized
// title changes nothing.
func NormalizeTitle(title string) string {
	title = quotedOpenerAsCodeSpan(strings.TrimSpace(title))
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

// quotedOpenerAsCodeSpan sets a single quoted word that opens title and holds
// a letter or digit but no backtick as a code span; any other title is
// returned as it is.
func quotedOpenerAsCodeSpan(title string) string {
	isQuote := func(r rune) bool { return strings.ContainsRune(titleQuotes, r) }
	first, size := utf8.DecodeRuneInString(title)
	if size == 0 || !isQuote(first) {
		return title
	}
	rest := title[size:]
	end := strings.IndexFunc(rest, isQuote)
	word := rest[:max(end, 0)]
	if strings.ContainsFunc(word, unicode.IsSpace) || strings.Contains(word, "`") || !strings.ContainsFunc(word, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return title
	}
	_, closing := utf8.DecodeRuneInString(rest[end:])
	return "`" + word + "`" + rest[end+closing:]
}

// ErrMultiLineTitle refuses a title no one-line listing can show.
var ErrMultiLineTitle = errors.New("the title spans more than one line")

// ErrEmptyTitle refuses a given title NormalizeTitle leaves nothing of.
var ErrEmptyTitle = errors.New("the title is empty once whitespace, quotes and trailing punctuation are removed")

// CheckTitle refuses a title every writer must refuse before storing it.
func CheckTitle(title string) error {
	if strings.ContainsAny(title, "\r\n\u0085\u2028\u2029") {
		return fmt.Errorf("%w: %q", ErrMultiLineTitle, title)
	}
	return nil
}

// ParseTitle is the stored form of a title a caller gives: normalized, and
// refused when empty or spanning more than one line.
func ParseTitle(given string) (string, error) {
	title := NormalizeTitle(given)
	if title == "" {
		return "", ErrEmptyTitle
	}
	if err := CheckTitle(title); err != nil {
		return "", err
	}
	return title, nil
}

// SimilarTitleCount is how many titles of the entries nearest an entry's body
// a title call is shown, so the title it writes stands apart from them.
const SimilarTitleCount = 5

// RepeatedTitle returns the title of an entry other than id that title
// repeats, compared trimmed and case-insensitively.
func RepeatedTitle(entries []*Entry, title, id string) (string, bool) {
	title = strings.TrimSpace(title)
	for _, e := range entries {
		if e.ID != id && e.Title != "" && strings.EqualFold(strings.TrimSpace(e.Title), title) {
			return e.Title, true
		}
	}
	return "", false
}

// DescriptionFields names the fields of an entry's description a summarize
// run writes (`sdd summarize --only`).
type DescriptionFields struct {
	Title, Summary bool
}

// AllDescriptionFields is a summarize run's default: title and summary.
var AllDescriptionFields = DescriptionFields{Title: true, Summary: true}

// OrAll reads the zero value as AllDescriptionFields.
func (f DescriptionFields) OrAll() DescriptionFields {
	if f == (DescriptionFields{}) {
		return AllDescriptionFields
	}
	return f
}

// ParseDescriptionFields reads an --only value: title, summary or both,
// comma-separated.
func ParseDescriptionFields(value string) (DescriptionFields, error) {
	var fields DescriptionFields
	for _, name := range strings.Split(value, ",") {
		switch strings.TrimSpace(name) {
		case "title":
			fields.Title = true
		case "summary":
			fields.Summary = true
		default:
			return DescriptionFields{}, fmt.Errorf("--only takes title, summary or title,summary, got %q", value)
		}
	}
	return fields, nil
}

// Unfilled reports whether `sdd summarize --all` makes a call for the entry:
// it lacks one of the fields and has a file to write.
func (e *Entry) Unfilled(fields DescriptionFields) bool {
	return !e.Embedded && (fields.Title && e.Title == "" || fields.Summary && e.Summary == "")
}

// TitleBackfill counts the model calls `sdd summarize --all` makes, by the
// fields each writes: an entry lacking both gets both, a field it holds is
// kept, and a field outside fields is never written. Embedded entries have no
// file to write and are not counted.
func TitleBackfill(entries []*Entry, fields DescriptionFields) (summaryAndTitle, summaryOnly, titleOnly int) {
	for _, e := range entries {
		if !e.Unfilled(fields) {
			continue
		}
		writesTitle := fields.Title && e.Title == ""
		writesSummary := fields.Summary && e.Summary == ""
		switch {
		case writesTitle && writesSummary:
			summaryAndTitle++
		case writesSummary:
			summaryOnly++
		default:
			titleOnly++
		}
	}
	return summaryAndTitle, summaryOnly, titleOnly
}
