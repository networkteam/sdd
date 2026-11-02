package llmops_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The eval specimens check quotes against the quote contract
// (20261005-170145-d-tac-gch): a quote need not match its passage character
// for character, only locate it.

var quoteNormalizer = strings.NewReplacer(
	"„", "", "“", "", "”", "", "‚", "", "‘", "", "’", "", "«", "", "»", "",
	"‹", "", "›", "", `"`, "", "'", "", "`", "", "—", "-", "–", "-",
)

func normalizeQuoted(s string) string {
	return strings.Join(strings.Fields(quoteNormalizer.Replace(strings.ToLower(s))), " ")
}

// quoteParts splits a quote shortened with an ellipsis into the parts that
// must appear in order.
func quoteParts(quote string) []string {
	var parts []string
	for p := range strings.SplitSeq(strings.ReplaceAll(quote, "...", "…"), "…") {
		if n := normalizeQuoted(p); n != "" {
			parts = append(parts, n)
		}
	}
	return parts
}

// quoteLocatesIn reports whether quote locates a passage of text that overlaps
// passage; an empty passage accepts any location.
func quoteLocatesIn(text, quote, passage string) bool {
	nt := normalizeQuoted(text)
	from, to := 0, len(nt)
	if passage != "" {
		np := normalizeQuoted(passage)
		i := strings.Index(nt, np)
		if np == "" || i < 0 {
			return false
		}
		from, to = i, i+len(np)
	}
	parts := quoteParts(quote)
	if len(parts) == 0 {
		return false
	}
	for start := 0; ; {
		i := strings.Index(nt[start:], parts[0])
		if i < 0 {
			return false
		}
		first := start + i
		end := first + len(parts[0])
		for _, p := range parts[1:] {
			j := strings.Index(nt[end:], p)
			if j < 0 {
				return false
			}
			end += j + len(p)
		}
		if first < to && end > from {
			return true
		}
		start = first + 1
	}
}

var excerptPattern = regexp.MustCompile("“([^”]+)”|„([^“”]+)[“”]|\"([^\"]+)\"|«([^»]+)»|‚([^‘’]+)[‘’]|‘([^’]+)’|`([^`]+)`")

// citedExcerpts returns the excerpts an observation sets off in quotes or
// backticks.
func citedExcerpts(observation string) []string {
	var out []string
	for _, m := range excerptPattern.FindAllStringSubmatch(observation, -1) {
		for _, g := range m[1:] {
			if g != "" {
				out = append(out, g)
				break
			}
		}
	}
	return out
}

func TestQuoteLocatesIn(t *testing.T) {
	const draft = "Die Fehlermeldung nutzt jetzt die zweite Variante aus dem Call, nicht mehr „Einladungslink“.\n" +
		"The release procedure gains this as a fixed section of its notes template. It gains this again."
	tests := []struct {
		name, quote, passage string
		want                 bool
	}{
		{"exact words", "gains this as a fixed section", "", true},
		{"ASCII quotes for typographic ones", `nicht mehr "Einladungslink"`, "", true},
		{"other case and whitespace", "THE RELEASE   procedure\ngains", "", true},
		{"shortened with an ellipsis", "release procedure … notes template", "", true},
		{"shortened with three dots", "release procedure ... notes template", "", true},
		{"parts out of order", "notes template … release procedure", "", false},
		{"words not in the draft", "gains this as a permanent section", "", false},
		{"empty quote", " „“ ", "", false},
		{"inside the passage", "zweite Variante aus dem Call", "Die Fehlermeldung nutzt jetzt die zweite Variante aus dem Call, nicht mehr „Einladungslink“.", true},
		{"outside the passage", "fixed section", "Die Fehlermeldung nutzt jetzt die zweite Variante aus dem Call, nicht mehr „Einladungslink“.", false},
		{"second occurrence inside the passage", "gains this", "It gains this again.", true},
		{"passage not in the draft", "gains this", "A sentence the draft lacks.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteLocatesIn(draft, tt.quote, tt.passage); got != tt.want {
				t.Errorf("quoteLocatesIn(%q, passage %q) = %v, want %v", tt.quote, tt.passage, got, tt.want)
			}
		})
	}
}

func TestCitedExcerpts(t *testing.T) {
	got := citedExcerpts("The ref `grounded-in` says “extends the contract” while the body says \"retires the immutability contract\"; „Partner-Code“ and «Link» stay.")
	want := []string{"grounded-in", "extends the contract", "retires the immutability contract", "Partner-Code", "Link"}
	if !slices.Equal(got, want) {
		t.Errorf("citedExcerpts = %q, want %q", got, want)
	}
}
