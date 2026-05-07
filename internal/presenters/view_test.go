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

func TestRenderView_RankedAsList(t *testing.T) {
	// When FlatList.Scores aligns with Entries, renderAsList emits a
	// `{score: X.XXX}` segment per entry. Verifies the scored vs
	// unscored branch in render_list.go.
	a := entry("20260101-100000-d-tac-aaa",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("First"))
	b := entry("20260101-110000-d-tac-bbb",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("Second"))
	g := model.NewGraph([]*model.Entry{a, b})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data: model.FlatList{
				Entries: []*model.Entry{a, b},
				Scores:  []float64{4.572, 1.230},
			},
		}},
	}
	got := renderView(result)
	want := "" +
		"  20260101-100000-d-tac-aaa tactical directive decision (Christopher) {status: active} {score: 4.572} First\n" +
		"  20260101-110000-d-tac-bbb tactical directive decision (Christopher) {status: active} {score: 1.230} Second\n"
	if got != want {
		t.Errorf("ranked render mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderView_UnrankedAsList_NoScoreSegment(t *testing.T) {
	// FlatList without Scores should render plain EntryLine output —
	// no score segment. Belt-and-suspenders against the scored branch
	// firing on by(date) results, which leave Scores nil.
	a := entry("20260101-100000-d-tac-aaa",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("Plain"))
	g := model.NewGraph([]*model.Entry{a})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data:   model.FlatList{Entries: []*model.Entry{a}}, // Scores nil
		}},
	}
	got := renderView(result)
	if strings.Contains(got, "score:") {
		t.Errorf("unranked output should not contain 'score:', got:\n%s", got)
	}
}
