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

func TestRenderView_AsGrouped(t *testing.T) {
	plan := entry("20260101-100000-d-tac-pln",
		withKind(model.KindPlan),
		withParticipants("Christopher"),
		withSummary("Plan entry"))
	dir1 := entry("20260101-110000-d-tac-da1",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("Directive one"))
	dir2 := entry("20260101-120000-d-tac-da2",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("Directive two"))
	g := model.NewGraph([]*model.Entry{plan, dir1, dir2})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-grouped",
			Data: model.Grouped{
				Field: "kind",
				Groups: []model.Group{
					{Key: "directive", Entries: []*model.Entry{dir1, dir2}},
					{Key: "plan", Entries: []*model.Entry{plan}},
				},
			},
		}},
	}

	got := renderView(result)
	// Per group: a `### <key>` header, then entry lines, then a blank
	// line. No `## ...` for the section as a whole — the `name(...)`
	// modifier (slice 6) is what supplies the section title.
	want := "" +
		"### directive\n" +
		"  20260101-110000-d-tac-da1 tactical directive decision (Christopher) {status: active} Directive one\n" +
		"  20260101-120000-d-tac-da2 tactical directive decision (Christopher) {status: active} Directive two\n" +
		"\n" +
		"### plan\n" +
		"  20260101-100000-d-tac-pln tactical plan decision (Christopher) {status: active} Plan entry\n"

	if got != want {
		t.Errorf("RenderView grouped mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderView_AsGroupedEmpty(t *testing.T) {
	// A grouped result with zero groups (e.g. all entries filtered away
	// before group()) renders as nothing — same as as-list with no entries.
	g := model.NewGraph(nil)
	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-grouped",
			Data:   model.Grouped{Field: "kind"},
		}},
	}
	got := renderView(result)
	if got != "" {
		t.Errorf("RenderView empty grouped: got %q, want empty string", got)
	}
}

func TestRenderView_NameModifier_EmitsHeader(t *testing.T) {
	// A non-empty SectionResult.Name renders as a `## <title>` line
	// before the section body. Used by the `name(string)` modifier and
	// (eventually) by auto-derived headers.
	a := entry("20260101-100000-d-tac-aaa",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("First"))
	g := model.NewGraph([]*model.Entry{a})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Name:   "Top entries",
			Data:   model.FlatList{Entries: []*model.Entry{a}},
		}},
	}
	got := renderView(result)
	if !strings.HasPrefix(got, "## Top entries\n\n") {
		t.Errorf("expected `## Top entries\\n\\n` prefix, got:\n%s", got)
	}
}

func TestRenderView_AsFocusBlock(t *testing.T) {
	// Smoke-shape test for the focus-block render: a `### <focus line>`
	// header followed by per-target lines with `{state: ...}` segments.
	target := entry("20260101-100000-d-tac-tgt",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("Target one"))
	pull := entry("20260101-110000-s-tac-pul",
		withKind(model.KindGap),
		withParticipants("Christopher"),
		withSummary("Pull-available target"))
	focus := entry("20260101-120000-d-prc-foc",
		withKind(model.KindFocus),
		withParticipants("Christopher"),
		withSummary("This week's focus"))
	g := model.NewGraph([]*model.Entry{target, pull, focus})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-focus-block",
			Data: model.FocusBlock{
				Focuses: []model.FocusGroup{{
					Focus:  focus,
					Actors: []string{"Christopher"},
					Targets: []model.FocusTarget{
						{
							Target:         target,
							ResolvedActors: []string{"Christopher"},
							ActorsExplicit: false,
							Score:          5.0,
							State:          model.FocusStateDriving,
						},
						{
							Target:         pull,
							ResolvedActors: nil,
							ActorsExplicit: true,
							Score:          0.0,
							State:          model.FocusStatePullAvailable,
						},
					},
				}},
			},
		}},
	}

	got := renderView(result)

	for _, want := range []string{
		"### 20260101-120000-d-prc-foc",
		"actors: Christopher\n",
		"{state: driving}",
		"20260101-100000-d-tac-tgt",
		"{state: pull-available}",
		"20260101-110000-s-tac-pul",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, got)
		}
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
