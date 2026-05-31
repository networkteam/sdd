package finders

import (
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// refExpansionFixture builds a graph with one source plan whose refs span
// the status surface (active / closed / superseded) and ref-kind surface
// (object-form kinds plus a legacy bare-string ref → unknown). Returns the
// graph and the source entry's ID for layout assertions.
func refExpansionFixture() (*model.Graph, string) {
	targetActive := entry("20260101-090000-d-cpt-act", withKind(model.KindDirective))
	targetClosed := entry("20260101-090100-d-cpt-clo", withKind(model.KindDirective))
	closer := entry("20260101-091000-s-cpt-cls", withKind(model.KindDone), withCloses(targetClosed.ID))
	targetSuperseded := entry("20260101-090200-d-cpt-sup", withKind(model.KindDirective))
	superseder := entry("20260101-092000-d-cpt-spr", withKind(model.KindDirective), withSupersedes(targetSuperseded.ID))

	source := entry("20260101-100000-d-tac-src",
		withKind(model.KindPlan),
		withRefObjs(
			model.Ref{ID: targetActive.ID, Kind: model.RefKindGroundedIn},
			model.Ref{ID: targetClosed.ID, Kind: model.RefKindDependsOn, Desc: "blocked on this"},
			model.Ref{ID: targetSuperseded.ID, Kind: model.RefKindBuildsOn},
			model.Ref{ID: "20260101-090300-d-cpt-leg", Kind: model.RefKindUnknown}, // legacy bare-string ref
		),
	)
	legacyTarget := entry("20260101-090300-d-cpt-leg", withKind(model.KindDirective))

	g := model.NewGraph([]*model.Entry{
		targetActive, targetClosed, closer, targetSuperseded, superseder, source, legacyTarget,
	})
	return g, source.ID
}

func runView(t *testing.T, g *model.Graph, layout string) *query.ViewResult {
	t.Helper()
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, layout)})
	if err != nil {
		t.Fatalf("View(%q): %v", layout, err)
	}
	return result
}

func flatOf(t *testing.T, result *query.ViewResult) model.FlatList {
	t.Helper()
	if len(result.Sections) != 1 {
		t.Fatalf("sections: got %d, want 1", len(result.Sections))
	}
	flat, ok := result.Sections[0].Data.(model.FlatList)
	if !ok {
		t.Fatalf("section data: got %T, want model.FlatList", result.Sections[0].Data)
	}
	return flat
}

func TestView_ExpandRefs_AllRefsWithStatusAndKind(t *testing.T) {
	g, sourceID := refExpansionFixture()
	// Filter to the source plan alone, then expand all its refs.
	flat := flatOf(t, runView(t, g, "kind(plan):expand(refs):as-list"))

	if got := idsOf(flat.Entries); len(got) != 1 || got[0] != sourceID {
		t.Fatalf("entries: got %v, want [%s]", got, sourceID)
	}
	if len(flat.RefExpansions) != len(flat.Entries) {
		t.Fatalf("RefExpansions not aligned: %d expansions, %d entries", len(flat.RefExpansions), len(flat.Entries))
	}

	rows := flat.RefExpansions[0]
	if len(rows) != 4 {
		t.Fatalf("ref rows: got %d, want 4", len(rows))
	}

	// Rows render in stored ref order. Verify kind, status, and desc on each.
	want := []model.RefExpansion{
		{Kind: model.RefKindGroundedIn, ID: "20260101-090000-d-cpt-act", Status: model.Status{Kind: model.StatusActive}},
		{Kind: model.RefKindDependsOn, ID: "20260101-090100-d-cpt-clo", Status: model.Status{Kind: model.StatusClosedBy, By: "20260101-091000-s-cpt-cls"}, Desc: "blocked on this"},
		{Kind: model.RefKindBuildsOn, ID: "20260101-090200-d-cpt-sup", Status: model.Status{Kind: model.StatusSupersededBy, By: "20260101-092000-d-cpt-spr"}},
		{Kind: model.RefKindUnknown, ID: "20260101-090300-d-cpt-leg", Status: model.Status{Kind: model.StatusActive}},
	}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d:\n  got:  %+v\n  want: %+v", i, rows[i], w)
		}
	}
}

func TestView_ExpandRefs_InactiveFiltersToClosedAndSuperseded(t *testing.T) {
	g, _ := refExpansionFixture()
	flat := flatOf(t, runView(t, g, "kind(plan):expand(refs(inactive)):as-list"))

	rows := flat.RefExpansions[0]
	if len(rows) != 2 {
		t.Fatalf("inactive rows: got %d, want 2 (closed + superseded)", len(rows))
	}
	if rows[0].ID != "20260101-090100-d-cpt-clo" || rows[0].Status.Kind != model.StatusClosedBy {
		t.Errorf("row 0: got %+v, want the closed target", rows[0])
	}
	if rows[1].ID != "20260101-090200-d-cpt-sup" || rows[1].Status.Kind != model.StatusSupersededBy {
		t.Errorf("row 1: got %+v, want the superseded target", rows[1])
	}
}

func TestView_ExpandRefs_InactiveIncludesCascadeClosedRole(t *testing.T) {
	// inactive is the inverse of the `active` filter, which drops roles whose
	// bound actor chain is retired (cascade-closed). A ref to such a role must
	// therefore surface under expand(refs(inactive)) — otherwise it would be
	// invisible in both the active and inactive views.
	actorHead := entry("20260101-080000-s-prc-act", withKind(model.KindActor), withCanonical("Dana"))
	retire := entry("20260101-085000-d-prc-ret", withKind(model.KindDirective), withCloses(actorHead.ID))
	role := entry("20260101-081000-d-prc-rol", withKind(model.KindRole), withActor("Dana"))
	source := entry("20260101-100000-d-tac-src",
		withKind(model.KindPlan),
		withRefObjs(model.Ref{ID: role.ID, Kind: model.RefKindGroundedIn}))
	g := model.NewGraph([]*model.Entry{actorHead, retire, role, source})

	// Sanity: the active filter drops the cascade-closed role.
	if active := g.Filter(model.GraphFilter{OpenOnly: true}); slices.Contains(idsOf(active), role.ID) {
		t.Fatalf("precondition: active filter should drop cascade-closed role %s", role.ID)
	}

	flat := flatOf(t, runView(t, g, "kind(plan):expand(refs(inactive)):as-list"))
	rows := flat.RefExpansions[0]
	if len(rows) != 1 {
		t.Fatalf("inactive rows: got %d, want 1 (the cascade-closed role)", len(rows))
	}
	if rows[0].ID != role.ID || rows[0].Status.Kind != model.StatusCascadeClosedBy {
		t.Errorf("row: got %+v, want cascade-closed role %s", rows[0], role.ID)
	}
}

func TestView_ExpandRefs_ZeroRefsEntry(t *testing.T) {
	norefs := entry("20260101-100000-d-tac-nor", withKind(model.KindPlan))
	g := model.NewGraph([]*model.Entry{norefs})
	flat := flatOf(t, runView(t, g, "kind(plan):expand(refs):as-list"))

	if len(flat.RefExpansions) != 1 {
		t.Fatalf("RefExpansions: got %d, want 1 (aligned to single entry)", len(flat.RefExpansions))
	}
	if len(flat.RefExpansions[0]) != 0 {
		t.Errorf("entry with no refs: got %d rows, want 0", len(flat.RefExpansions[0]))
	}
}

func TestView_ExpandRefs_ComposesWithFiltersRankAndPage(t *testing.T) {
	// active + kind + rank(in-degree) + n(1) all narrow the entry set; then
	// expand(refs) attaches sub-lines to whatever survived. Proves the
	// expansion runs after ranking and pagination (the catch-up layout shape).
	target := entry("20260101-080000-d-cpt-tgt", withKind(model.KindDirective))
	planHot := entry("20260101-100000-d-tac-hot",
		withKind(model.KindPlan),
		withRefObjs(model.Ref{ID: target.ID, Kind: model.RefKindGroundedIn}))
	planCold := entry("20260101-110000-d-tac-cld",
		withKind(model.KindPlan),
		withRefObjs(model.Ref{ID: target.ID, Kind: model.RefKindRelated}))
	// referrer pushes planHot's in-degree above planCold's so rank order is
	// deterministic; it's a directive, filtered out by kind(plan).
	referrer := entry("20260101-120000-d-tac-ref",
		withKind(model.KindDirective),
		withRefObjs(model.Ref{ID: planHot.ID, Kind: model.RefKindRelated}))
	g := model.NewGraph([]*model.Entry{target, planHot, planCold, referrer})

	flat := flatOf(t, runView(t, g, "active:kind(plan):rank(in-degree):n(1):expand(refs):as-list"))

	if got := idsOf(flat.Entries); len(got) != 1 || got[0] != planHot.ID {
		t.Fatalf("entries: got %v, want [%s] (top by in-degree, paged to 1)", got, planHot.ID)
	}
	if len(flat.Scores) != 1 {
		t.Errorf("scores not aligned after rank: got %d, want 1", len(flat.Scores))
	}
	if len(flat.RefExpansions) != 1 || len(flat.RefExpansions[0]) != 1 {
		t.Fatalf("RefExpansions: got %v, want one entry with one ref", flat.RefExpansions)
	}
	if flat.RefExpansions[0][0].ID != target.ID {
		t.Errorf("expanded ref: got %s, want %s", flat.RefExpansions[0][0].ID, target.ID)
	}
}

func TestView_ExpandRefs_RenderShapeAndExclusionErrors(t *testing.T) {
	g, _ := refExpansionFixture()
	f := New(Options{})

	cases := []struct {
		name    string
		layout  string
		wantErr string
	}{
		{"refs requires as-list", "kind(plan):expand(refs):as-focus-block", "render-shape mismatch"},
		{"refs excludes group", "kind(plan):expand(refs):group(by(kind)):as-grouped", "mutually exclusive with group"},
		{"involvement still requires focus-block", "kind(focus):expand(involvement):as-list", "render-shape mismatch"},
		{"involvement still excludes rank", "kind(focus):expand(involvement):rank(in-degree):as-focus-block", "expand(involvement) is mutually exclusive with rank"},
		{"unknown expand field", "kind(plan):expand(bogus):as-list", `unknown field "bogus"`},
		{"bad nested filter", "kind(plan):expand(refs(bogus)):as-list", "filter must be inactive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, tc.layout)})
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error mismatch:\n  got:  %v\n  want substring: %q", err, tc.wantErr)
			}
		})
	}
}
