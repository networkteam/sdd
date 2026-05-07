package presenters_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

func TestRenderView_AsListSingleSection(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa",
		withKind(model.KindDirective),
		withConfidence("medium"),
		withParticipants("Christopher", "Claude"),
		withSummary("First entry"))
	b := entry("20260101-110000-s-tac-bbb",
		withKind(model.KindGap),
		withParticipants("Christopher"),
		withSummary("Second entry"))
	g := model.NewGraph([]*model.Entry{a, b})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data:   model.FlatList{Entries: []*model.Entry{a, b}},
		}},
	}

	got := renderView(result)
	want := "" +
		"  20260101-100000-d-tac-aaa tactical directive decision [confidence: medium] (Christopher, Claude) {status: active} First entry\n" +
		"  20260101-110000-s-tac-bbb tactical gap signal (Christopher) {status: open} Second entry\n"

	if got != want {
		t.Errorf("RenderView mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderView_AsListEmpty(t *testing.T) {
	g := model.NewGraph(nil)
	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data:   model.FlatList{Entries: nil},
		}},
	}
	got := renderView(result)
	if got != "" {
		t.Errorf("RenderView empty: got %q, want empty string", got)
	}
}

func TestRenderView_MultipleSectionsSeparatedByBlankLine(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("Entry A"))
	b := entry("20260101-110000-s-tac-bbb",
		withKind(model.KindGap),
		withParticipants("Christopher"),
		withSummary("Entry B"))
	g := model.NewGraph([]*model.Entry{a, b})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{
			{Render: "as-list", Data: model.FlatList{Entries: []*model.Entry{a}}},
			{Render: "as-list", Data: model.FlatList{Entries: []*model.Entry{b}}},
		},
	}
	got := renderView(result)

	// Sections are separated by a single blank line so the visual cluster
	// is obvious without imposing headers (slice 5 will add `name(...)`
	// headers when that primitive lands).
	if !strings.Contains(got, "Entry A\n\n  ") {
		t.Errorf("expected blank-line separator between sections, got:\n%s", got)
	}
	if !strings.Contains(got, "Entry B") {
		t.Errorf("expected second section in output, got:\n%s", got)
	}
}

func renderView(r *query.ViewResult) string {
	var buf bytes.Buffer
	presenters.RenderView(&buf, r)
	return buf.String()
}
