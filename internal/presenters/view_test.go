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

func TestRenderView_AsListExpandRefs_SubLineShapes(t *testing.T) {
	// expand(refs) renders each entry's resolved refs as indented sub-lines:
	// kind-as-verb, then status, then optional desc. Covers the three shapes
	// from the AC — legacy bare-string (refs verb), object-form without desc,
	// object-form with desc — plus status surfacing on the parent's refs.
	parent := entry("20260101-100000-d-tac-par",
		withKind(model.KindPlan),
		withParticipants("Christopher"),
		withSummary("Parent plan"))
	g := model.NewGraph([]*model.Entry{parent})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data: model.FlatList{
				Entries: []*model.Entry{parent},
				RefExpansions: [][]model.RefExpansion{{
					{Kind: model.RefKindUnknown, ID: "20260101-090000-d-cpt-leg", Status: model.Status{Kind: model.StatusActive}},
					{Kind: model.RefKindGroundedIn, ID: "20260101-090100-d-cpt-grd", Status: model.Status{Kind: model.StatusActive}},
					{Kind: model.RefKindDependsOn, ID: "20260101-090200-d-tac-dep", Status: model.Status{Kind: model.StatusClosedBy, By: "20260101-091000-s-tac-don"}, Desc: "why this ref exists"},
				}},
			},
		}},
	}

	got := renderView(result)
	want := "" +
		"  20260101-100000-d-tac-par tactical plan decision (Christopher) {status: active} Parent plan\n" +
		"    → refs 20260101-090000-d-cpt-leg {status: active}\n" +
		"    → grounded-in 20260101-090100-d-cpt-grd {status: active}\n" +
		"    → depends-on 20260101-090200-d-tac-dep {status: closed-by 20260101-091000-s-tac-don}: \"why this ref exists\"\n"

	if got != want {
		t.Errorf("expand(refs) render mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderView_AsListExpandRefs_CrossRepoUnresolved(t *testing.T) {
	// A cross-repo ref sub-line carries the full prefixed ID and the
	// bracketed unresolved marker in place of a status segment.
	parent := entry("20260101-100000-d-tac-par",
		withKind(model.KindPlan),
		withParticipants("Christopher"),
		withSummary("Parent plan"))
	g := model.NewGraph([]*model.Entry{parent})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data: model.FlatList{
				Entries: []*model.Entry{parent},
				RefExpansions: [][]model.RefExpansion{{
					{Kind: model.RefKindGroundedIn, ID: "github.com/networkteam/other:20260101-090000-d-cpt-rem", UnresolvedRepo: "github.com/networkteam/other", Desc: "remote basis"},
				}},
			},
		}},
	}

	got := renderView(result)
	want := "" +
		"  20260101-100000-d-tac-par tactical plan decision (Christopher) {status: active} Parent plan\n" +
		"    → grounded-in github.com/networkteam/other:20260101-090000-d-cpt-rem [unresolved: repo github.com/networkteam/other]: \"remote basis\"\n"

	if got != want {
		t.Errorf("expand(refs) cross-repo render mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderView_AsListExpandRefs_DoneTargetOmitsStatus(t *testing.T) {
	// A ref pointing at a done signal (terminal, StatusNone) renders without
	// a status segment — matching how done signals render on every surface.
	parent := entry("20260101-100000-d-tac-par",
		withKind(model.KindPlan),
		withParticipants("Christopher"),
		withSummary("Parent"))
	g := model.NewGraph([]*model.Entry{parent})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data: model.FlatList{
				Entries: []*model.Entry{parent},
				RefExpansions: [][]model.RefExpansion{{
					{Kind: model.RefKindGroundedIn, ID: "20260101-090000-s-tac-evd", Status: model.Status{Kind: model.StatusNone}},
				}},
			},
		}},
	}

	got := renderView(result)
	want := "" +
		"  20260101-100000-d-tac-par tactical plan decision (Christopher) {status: active} Parent\n" +
		"    → grounded-in 20260101-090000-s-tac-evd\n"
	if got != want {
		t.Errorf("done-target render mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderView_AsListExpandRefs_SupersededPath(t *testing.T) {
	// A ref pointing at a multiply-superseded target renders the supersede
	// trail through to the live head. The origin (the sub-line's own id) is
	// dropped; the rendered hops are the superseders ending at the head, so a
	// reader who meets an intermediate id elsewhere can connect it.
	parent := entry("20260101-100000-d-tac-par",
		withKind(model.KindPlan),
		withParticipants("Christopher"),
		withSummary("Parent plan"))
	g := model.NewGraph([]*model.Entry{parent})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data: model.FlatList{
				Entries: []*model.Entry{parent},
				RefExpansions: [][]model.RefExpansion{{
					{
						Kind:   model.RefKindGroundedIn,
						ID:     "20260101-090000-d-cpt-old",
						Status: model.Status{Kind: model.StatusSupersededBy, By: "20260101-092000-d-cpt-new"},
						SupersedePath: []string{
							"20260101-090000-d-cpt-old",
							"20260101-091000-d-cpt-mid",
							"20260101-092000-d-cpt-new",
						},
					},
				}},
			},
		}},
	}

	got := renderView(result)
	want := "" +
		"  20260101-100000-d-tac-par tactical plan decision (Christopher) {status: active} Parent plan\n" +
		"    → grounded-in 20260101-090000-d-cpt-old {status: superseded-by 20260101-091000-d-cpt-mid → 20260101-092000-d-cpt-new}\n"
	if got != want {
		t.Errorf("superseded-path render mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderView_AsListExpandRefs_ComposesWithScore(t *testing.T) {
	// Ranked + expanded: the entry line keeps its {score: ...} segment and
	// the ref sub-lines render beneath it.
	parent := entry("20260101-100000-d-tac-par",
		withKind(model.KindPlan),
		withParticipants("Christopher"),
		withSummary("Parent"))
	g := model.NewGraph([]*model.Entry{parent})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Data: model.FlatList{
				Entries:       []*model.Entry{parent},
				Scores:        []float64{2.500},
				RefExpansions: [][]model.RefExpansion{{{Kind: model.RefKindGroundedIn, ID: "20260101-090000-d-cpt-grd", Status: model.Status{Kind: model.StatusActive}}}},
			},
		}},
	}

	got := renderView(result)
	want := "" +
		"  20260101-100000-d-tac-par tactical plan decision (Christopher) {status: active} {score: 2.500} Parent\n" +
		"    → grounded-in 20260101-090000-d-cpt-grd {status: active}\n"
	if got != want {
		t.Errorf("scored+expanded render mismatch:\n  got:  %q\n  want: %q", got, want)
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
