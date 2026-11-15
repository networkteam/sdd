package model_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestTitleSurvivesTheFrontmatterRoundTrip(t *testing.T) {
	for _, title := range []string{"Daemon project removal", "`sdd` info JSON output"} {
		t.Run(title, func(t *testing.T) {
			entry := &model.Entry{ID: "20260101-100000-s-tac-ttl", Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical, Title: title, Summary: "S.", Content: "Body."}
			written := model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"
			if !strings.Contains(written, "title: ") {
				t.Fatalf("the title should be written to the frontmatter:\n%s", written)
			}
			parsed, err := model.ParseEntry(entry.ID+".md", written)
			if err != nil {
				t.Fatalf("ParseEntry: %v", err)
			}
			if parsed.Title != entry.Title {
				t.Errorf("the title should read back unchanged, got %q", parsed.Title)
			}
		})
	}
}

func TestUntitledEntryWritesNoTitleKey(t *testing.T) {
	entry := &model.Entry{ID: "20260101-100000-s-tac-ttl", Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical, Summary: "S.", Content: "Body."}
	if written := model.FormatFrontmatter(entry); strings.Contains(written, "title:") {
		t.Errorf("an untitled entry should carry no title key:\n%s", written)
	}
}

func TestListingTitle(t *testing.T) {
	cases := []struct {
		name  string
		entry model.Entry
		want  string
	}{
		{name: "the title names a titled entry", entry: model.Entry{Title: "Daemon project removal", Summary: "The daemon project goes. It was unused."}, want: "Daemon project removal"},
		{name: "an untitled entry falls back to its first summary sentence", entry: model.Entry{Summary: "The daemon project goes. It was unused."}, want: "The daemon project goes."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.entry.ListingTitle(); got != tc.want {
				t.Errorf("listing title should name the entry, got %q", got)
			}
		})
	}
}

func TestNormalizeTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "daemon project removal", want: "Daemon project removal"},
		{in: "übergabe der projekte", want: "Übergabe der projekte"},
		{in: "3d printing setup", want: "3d printing setup"},
		{in: "`sdd` info JSON output", want: "`sdd` info JSON output"},
		{in: "`go.mod` replace directive", want: "`go.mod` replace directive"},
		{in: "daemon project removal.", want: "Daemon project removal"},
		{in: "  Daemon removal...  ", want: "Daemon removal"},
		{in: "Daemon removal …", want: "Daemon removal"},
		{in: "Daemon removal, ; : ! - – —", want: "Daemon removal"},
		{in: "Should the daemon go?", want: "Should the daemon go?"},
		{in: "Should the daemon go?!.", want: "Should the daemon go?"},
		{in: "Info output of `sdd`", want: "Info output of `sdd`"},
		{in: "Search index (beta).", want: "Search index (beta)"},
		{in: `The "daemon" runs`, want: "The daemon runs"},
		{in: "Die „Daemon“ Entfernung", want: "Die Daemon Entfernung"},
		{in: "Le « démon » part.", want: "Le démon part"},
		{in: "“Daemon removal”.", want: "Daemon removal"},
		{in: `"daemon removal"`, want: "Daemon removal"},
		{in: `"daemon removal" plan`, want: "Daemon removal plan"},
		{in: `"daemon removal plan`, want: "Daemon removal plan"},
		{in: `"sdd" output`, want: "`sdd` output"},
		{in: "„go.mod“ replace directive", want: "`go.mod` replace directive"},
		{in: `"sdd" info JSON output`, want: "`sdd` info JSON output"},
		{in: "\"a`b\" c", want: "A`b c"},
		{in: "\"`sdd`\" x", want: "`sdd` x"},
		{in: "The team's daemon’s exit", want: "The team's daemon’s exit"},
		{in: "Self-hosted runners", want: "Self-hosted runners"},
		{in: "...", want: ""},
		{in: "„…“", want: ""},
		{in: `"-" removal`, want: "- removal"},
		{in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := model.NormalizeTitle(tc.in)
			if got != tc.want {
				t.Errorf("double quotes and trailing punctuation but a question mark should go, a quoted leading word become a code span and a first letter outside a code span a capital, got %q", got)
			}
			// Every writer may normalize a title already normalized.
			if again := model.NormalizeTitle(got); again != got {
				t.Errorf("normalizing a normalized title should change nothing, got %q", again)
			}
		})
	}
}

func TestParseTitle(t *testing.T) {
	cases := []struct {
		given, want string
		wantErr     error
	}{
		{given: ` “daemon removal”. `, want: "Daemon removal"},
		{given: "...", wantErr: model.ErrEmptyTitle},
		{given: "  ", wantErr: model.ErrEmptyTitle},
		{given: `""`, wantErr: model.ErrEmptyTitle},
		{given: "Daemon\nremoval", wantErr: model.ErrMultiLineTitle},
	}
	for _, tc := range cases {
		t.Run(tc.given, func(t *testing.T) {
			got, err := model.ParseTitle(tc.given)
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Errorf("a given title should be stored normalized or refused, got %q, %v", got, err)
			}
		})
	}
}

func TestCheckTitle(t *testing.T) {
	cases := []struct {
		title   string
		refused bool
	}{
		{title: "Daemon project removal"},
		{title: "Daemon\nremoval", refused: true},
		{title: "Daemon\r\nremoval", refused: true},
		{title: "Daemon\rremoval", refused: true},
		{title: "Daemon\u2028removal", refused: true},
		{title: "Daemon\u2029removal", refused: true},
		{title: "Daemon\u0085removal", refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			if err := model.CheckTitle(tc.title); errors.Is(err, model.ErrMultiLineTitle) != tc.refused {
				t.Errorf("only a title spanning more than one line should be refused, got %v", err)
			}
		})
	}
}

func TestRepeatedTitle(t *testing.T) {
	entries := []*model.Entry{
		{ID: "20260101-100000-s-tac-aaa", Title: "Daemon shutdown order"},
		{ID: "20260101-100001-s-tac-bbb"},
	}
	cases := []struct {
		name  string
		title string
		id    string
		want  string
	}{
		{name: "another entry's title, in other case and spacing", title: " daemon SHUTDOWN order ", id: "20260101-100002-s-tac-ccc", want: "Daemon shutdown order"},
		{name: "the entry's own title", title: "Daemon shutdown order", id: "20260101-100000-s-tac-aaa"},
		{name: "a title no entry holds", title: "Daemon stop sequence", id: "20260101-100002-s-tac-ccc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := model.RepeatedTitle(entries, tc.title, tc.id)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("only another entry's title should count as repeated, got %q, %v", got, ok)
			}
		})
	}
}

func TestTitleBackfill(t *testing.T) {
	entries := []*model.Entry{
		{Summary: "S."},
		{Summary: "S.", Title: "Has both"},
		{},
		{Title: "Title without summary"},
		{Embedded: true, Summary: "S."},
		{Embedded: true},
	}
	cases := []struct {
		name                                     string
		fields                                   model.DescriptionFields
		wantBoth, wantSummaryOnly, wantTitleOnly int
	}{
		{name: "both fields", fields: model.AllDescriptionFields, wantBoth: 1, wantSummaryOnly: 1, wantTitleOnly: 1},
		{name: "titles only", fields: model.DescriptionFields{Title: true}, wantTitleOnly: 2},
		{name: "summaries only", fields: model.DescriptionFields{Summary: true}, wantSummaryOnly: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			both, summaryOnly, titleOnly := model.TitleBackfill(entries, tc.fields)
			if both != tc.wantBoth || summaryOnly != tc.wantSummaryOnly || titleOnly != tc.wantTitleOnly {
				t.Errorf("each entry should be counted by the fields it gets, got %d both, %d summary-only, %d title-only", both, summaryOnly, titleOnly)
			}
		})
	}
}

func TestParseDescriptionFields(t *testing.T) {
	cases := []struct {
		in      string
		want    model.DescriptionFields
		refused bool
	}{
		{in: "title", want: model.DescriptionFields{Title: true}},
		{in: "summary", want: model.DescriptionFields{Summary: true}},
		{in: "title,summary", want: model.AllDescriptionFields},
		{in: "summary, title", want: model.AllDescriptionFields},
		{in: "body", refused: true},
		{in: "", refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := model.ParseDescriptionFields(tc.in)
			if tc.refused {
				if err == nil || !strings.Contains(err.Error(), "title, summary or title,summary") {
					t.Errorf("an unknown value should be refused naming the allowed ones, got %v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("the value should name its fields, got %+v, %v", got, err)
			}
		})
	}
}
