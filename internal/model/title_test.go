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
		{in: `"sdd" output`, want: "sdd output"},
		{in: "„go.mod“ replace directive", want: "go.mod replace directive"},
		{in: "The team's daemon’s exit", want: "The team's daemon’s exit"},
		{in: "Self-hosted runners", want: "Self-hosted runners"},
		{in: "...", want: ""},
		{in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := model.NormalizeTitle(tc.in); got != tc.want {
				t.Errorf("double quotes and trailing punctuation but a question mark should go and a first letter outside a code span become a capital, got %q", got)
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
	summaryAndTitle, summaryOnly, titleOnly := model.TitleBackfill(entries)
	if summaryAndTitle != 1 || summaryOnly != 1 || titleOnly != 1 {
		t.Errorf("every entry lacking a summary owes a paired call that keeps a stored title, and only the summarized untitled one a title call, got %d paired, %d summary-only, %d title-only", summaryAndTitle, summaryOnly, titleOnly)
	}
}
