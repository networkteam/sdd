package finders

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// TestView_AsCounts checks the as-counts aggregation: per-topic entry counts
// ordered count-descending, with untagged entries contributing to no row.
func TestView_AsCounts(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective), withTopics("infra/cli"))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective), withTopics("infra/cli", "type-system"))
	c := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective), withTopics("infra/cli"))
	untagged := entry("20260101-130000-d-tac-ddd", withKind(model.KindDirective))

	g := model.NewGraph([]*model.Entry{a, b, c, untagged})

	layout := mustParseLayout(t, "as-counts")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	section := result.Sections[0]
	if section.Render != "as-counts" {
		t.Fatalf("render: got %q, want as-counts", section.Render)
	}
	counts, ok := section.Data.(model.Counts)
	if !ok {
		t.Fatalf("section data: got %T, want model.Counts", section.Data)
	}

	// Two distinct topics; untagged entry contributes nothing. infra/cli has
	// three members, type-system one — count-descending puts infra/cli first.
	if got, want := len(counts.Rows), 2; got != want {
		t.Fatalf("rows: got %d, want %d (%+v)", got, want, counts.Rows)
	}
	if counts.Rows[0].Label != "infra/cli" || counts.Rows[0].Count != 3 {
		t.Errorf("row 0: got %+v, want infra/cli count 3", counts.Rows[0])
	}
	if counts.Rows[1].Label != "type-system" || counts.Rows[1].Count != 1 {
		t.Errorf("row 1: got %+v, want type-system count 1", counts.Rows[1])
	}
}

// TestView_AsCounts_Empty confirms a graph with no tagged entries yields an
// empty (but valid) counts result — the presenter renders a "(no topics)"
// line rather than crashing.
func TestView_AsCounts_Empty(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, "as-counts")
	result, err := New(Options{}).OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	counts := result.Sections[0].Data.(model.Counts)
	if len(counts.Rows) != 0 {
		t.Errorf("rows: got %d, want 0", len(counts.Rows))
	}
}

// TestView_AsCounts_RejectsRankAndN confirms as-counts is mutually exclusive
// with rank()/n() — those operate on entries before aggregation and would
// produce wrong counts.
func TestView_AsCounts_RejectsRankAndN(t *testing.T) {
	g := model.NewGraph([]*model.Entry{entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))})
	f := New(Options{})

	for _, layout := range []string{"as-counts:rank(heat)", "as-counts:n(3)", "as-counts:group(by(kind))"} {
		_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, layout)})
		if err == nil {
			t.Errorf("%q: expected mutual-exclusion error, got nil", layout)
		}
	}
}

// TestView_Untagged keeps only entries whose effective topic set is empty,
// counting annotation membership (not just inline topics).
func TestView_Untagged(t *testing.T) {
	tagged := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective), withTopics("infra/cli"))
	bare := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective))
	annMember := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	ann := entry("20260101-130000-s-cpt-ann",
		withKind(model.KindAnnotation),
		withRefs(annMember.ID),
		withAnnotationTopics("infra/cli"))

	g := model.NewGraph([]*model.Entry{tagged, bare, annMember, ann})

	// Scope to directives so the annotation signal itself is out of frame —
	// this test asserts the untagged filter respects annotation *membership*
	// (the annotation's own self-tagging is AC3, exercised separately).
	layout := mustParseLayout(t, "kind(directive):untagged:as-list")
	result, err := New(Options{}).OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)

	// bare is the only directive with no effective topic. tagged has an inline
	// topic; annMember is tagged via the annotation that refs it.
	gotIDs := idsOf(flat.Entries)
	if len(gotIDs) != 1 || gotIDs[0] != bare.ID {
		t.Errorf("untagged: got %v, want [%s]", gotIDs, bare.ID)
	}
}

// TestView_IDFilter selects exactly the listed entries, resolving short IDs to
// full and ignoring IDs that match nothing.
func TestView_IDFilter(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective))
	c := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a, b, c})

	// Quoted full ID + bare short ID, mixed. Order follows the entry set.
	layout := mustParseLayout(t, `id("20260101-100000-d-tac-aaa",d-tac-ccc):as-list`)
	result, err := New(Options{}).OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	gotIDs := idsOf(flat.Entries)
	want := []string{a.ID, c.ID}
	if len(gotIDs) != len(want) || gotIDs[0] != want[0] || gotIDs[1] != want[1] {
		t.Errorf("id filter: got %v, want %v", gotIDs, want)
	}
}
