package finders

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func TestView_ActiveAsList(t *testing.T) {
	// Fixture spans the full status surface so the active filter is
	// exercised against every lifecycle state. Times are spread out so the
	// graph's chronological sort produces a deterministic result order.
	activeDecision := entry("20260101-100000-d-tac-act",
		withKind(model.KindDirective))
	closedDecision := entry("20260101-110000-d-tac-cld",
		withKind(model.KindDirective))
	closer := entry("20260101-120000-s-tac-clo",
		withKind(model.KindDone),
		withCloses(closedDecision.ID))
	supersededDecision := entry("20260101-130000-d-tac-sup",
		withKind(model.KindDirective))
	superseder := entry("20260101-140000-d-tac-spr",
		withKind(model.KindDirective),
		withSupersedes(supersededDecision.ID))
	openSignal := entry("20260101-150000-s-tac-opn",
		withKind(model.KindGap))

	g := model.NewGraph([]*model.Entry{
		activeDecision, closedDecision, closer,
		supersededDecision, superseder, openSignal,
	})

	layout := mustParseLayout(t, "active:as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	if got, want := len(result.Sections), 1; got != want {
		t.Fatalf("sections: got %d, want %d", got, want)
	}
	section := result.Sections[0]
	if section.Render != "as-list" {
		t.Errorf("render: got %q, want %q", section.Render, "as-list")
	}

	flat, ok := section.Data.(model.FlatList)
	if !ok {
		t.Fatalf("section data: got %T, want model.FlatList", section.Data)
	}

	// active excludes closed and superseded entries. Done signals are
	// terminal (StatusNone, not closed) so they pass through — matches
	// d-tac-uww's literal definition: "not closed and not superseded".
	wantIDs := []string{
		activeDecision.ID,
		closer.ID,
		superseder.ID,
		openSignal.ID,
	}
	gotIDs := idsOf(flat.Entries)
	if !equalIDs(gotIDs, wantIDs) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", gotIDs, wantIDs)
	}
}

func TestView_AsListAlone_IncludesAll(t *testing.T) {
	// Without `active`, the as-list section returns every entry — closed
	// and superseded too. Verifies the filter is genuinely opt-in, not the
	// default behaviour.
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective))
	closer := entry("20260101-120000-s-tac-clo",
		withKind(model.KindDone),
		withCloses(b.ID))

	g := model.NewGraph([]*model.Entry{a, b, closer})

	layout := mustParseLayout(t, "as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	want := []string{a.ID, b.ID, closer.ID}
	got := idsOf(flat.Entries)
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_MultipleSections(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-s-tac-bbb", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{a, b})

	layout := mustParseLayout(t, "active:as-list,as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got := len(result.Sections); got != 2 {
		t.Fatalf("sections: got %d, want 2", got)
	}
}

func TestView_UnknownFunction(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "futurefn:as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for unknown function, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"unknown function", "futurefn", "active", "as-list"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing substring %q", msg, want)
		}
	}
}

func TestView_MissingRender(t *testing.T) {
	g := model.NewGraph(nil)
	// Section has no render terminator — every section must end in a render.
	layout := mustParseLayout(t, "active")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for missing render, got nil")
	}
	if !strings.Contains(err.Error(), "render") {
		t.Errorf("error %q does not mention render", err.Error())
	}
}

func TestView_RenderNotLast(t *testing.T) {
	g := model.NewGraph(nil)
	// as-list appears mid-section instead of at the end — render is
	// always the terminus per the design.
	layout := mustParseLayout(t, "as-list:active")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for render not last, got nil")
	}
	if !strings.Contains(err.Error(), "render") {
		t.Errorf("error %q does not mention render", err.Error())
	}
}

func TestView_NilGraph(t *testing.T) {
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: nil, Layout: mustParseLayout(t, "as-list")})
	if err == nil {
		t.Fatalf("expected error for nil graph, got nil")
	}
}

// --- Slice 2: kind() and n() primitives ---

func TestView_KindFilter_Single(t *testing.T) {
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	gap := entry("20260101-120000-s-tac-gap", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{plan, directive, gap})

	layout := mustParseLayout(t, "kind(plan):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{plan.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_KindFilter_Disjunction(t *testing.T) {
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	activity := entry("20260101-120000-d-tac-act", withKind(model.KindActivity))
	gap := entry("20260101-130000-s-tac-gap", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{plan, directive, activity, gap})

	layout := mustParseLayout(t, "kind(plan,directive,activity):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{plan.ID, directive.ID, activity.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_KindFilter_StringArg(t *testing.T) {
	// kind() should accept string-quoted args interchangeably with idents
	// so users can write either kind(plan) or kind("plan").
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{plan, directive})

	layout := mustParseLayout(t, `kind("plan"):as-list`)
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	if len(got) != 1 || got[0] != plan.ID {
		t.Errorf("entries: got %v, want [%s]", got, plan.ID)
	}
}

func TestView_NPagination(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective))
	c := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	d := entry("20260101-130000-d-tac-ddd", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a, b, c, d})

	layout := mustParseLayout(t, "n(2):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{a.ID, b.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_NLargerThanResult(t *testing.T) {
	// n(N) with N greater than the available result returns everything,
	// not an error — pagination caps at result length.
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a, b})

	layout := mustParseLayout(t, "n(100):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if len(flat.Entries) != 2 {
		t.Errorf("entries: got %d, want 2", len(flat.Entries))
	}
}

func TestView_NZero(t *testing.T) {
	// n(0) returns empty — degenerate but valid (no negative page sizes).
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, "n(0):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if len(flat.Entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(flat.Entries))
	}
}

func TestView_KindThenN_Compose(t *testing.T) {
	plan1 := entry("20260101-100000-d-tac-pl1", withKind(model.KindPlan))
	plan2 := entry("20260101-110000-d-tac-pl2", withKind(model.KindPlan))
	plan3 := entry("20260101-120000-d-tac-pl3", withKind(model.KindPlan))
	directive := entry("20260101-130000-d-tac-dir", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{plan1, plan2, plan3, directive})

	// kind first, then n: filter to plans, page first 2.
	layout := mustParseLayout(t, "kind(plan):n(2):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{plan1.ID, plan2.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_NThenKind_SameAsKindThenN(t *testing.T) {
	// Pipeline order in source doesn't change the canonical
	// filter→page→render order — the executor accumulates intent and
	// applies in canonical sequence so n(2):kind(plan) == kind(plan):n(2).
	plan1 := entry("20260101-100000-d-tac-pl1", withKind(model.KindPlan))
	plan2 := entry("20260101-110000-d-tac-pl2", withKind(model.KindPlan))
	plan3 := entry("20260101-120000-d-tac-pl3", withKind(model.KindPlan))
	directive := entry("20260101-130000-d-tac-dir", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{plan1, plan2, plan3, directive})

	layout := mustParseLayout(t, "n(2):kind(plan):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{plan1.ID, plan2.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_ActiveKindNCompose(t *testing.T) {
	// active + kind + n composed end-to-end on a representative graph.
	activePlan1 := entry("20260101-100000-d-tac-ap1", withKind(model.KindPlan))
	activePlan2 := entry("20260101-110000-d-tac-ap2", withKind(model.KindPlan))
	closedPlan := entry("20260101-120000-d-tac-clp", withKind(model.KindPlan))
	closer := entry("20260101-130000-s-tac-clo",
		withKind(model.KindDone),
		withCloses(closedPlan.ID))
	directive := entry("20260101-140000-d-tac-dir", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{activePlan1, activePlan2, closedPlan, closer, directive})

	layout := mustParseLayout(t, "active:kind(plan):n(5):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{activePlan1.ID, activePlan2.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

// Error paths — argument validation lives in the executor (the parser is
// permissive). Each case checks the error mentions the offending function
// so users can locate the issue in their layout string.

func TestView_ActiveTakesNoArgs(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "active(plan):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for active with args, got nil")
	}
	if !strings.Contains(err.Error(), "active") {
		t.Errorf("error %q does not mention 'active'", err.Error())
	}
}

func TestView_KindRequiresArgs(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "kind():as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for kind with no args, got nil")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("error %q does not mention 'kind'", err.Error())
	}
}

func TestView_KindRejectsNonIdentifier(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "kind(10):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for kind with numeric arg, got nil")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("error %q does not mention 'kind'", err.Error())
	}
}

func TestView_NRequiresExactlyOneArg(t *testing.T) {
	g := model.NewGraph(nil)
	cases := []string{
		"n():as-list",
		"n(1,2):as-list",
	}
	for _, layoutStr := range cases {
		t.Run(layoutStr, func(t *testing.T) {
			layout := mustParseLayout(t, layoutStr)
			f := New(Options{})
			_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "n") {
				t.Errorf("error %q does not mention 'n'", err.Error())
			}
		})
	}
}

func TestView_NRejectsNonNumber(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "n(abc):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for n with identifier arg, got nil")
	}
	if !strings.Contains(err.Error(), "n") {
		t.Errorf("error %q does not mention 'n'", err.Error())
	}
}

func TestView_NRejectsNonInteger(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "n(2.5):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for non-integer n, got nil")
	}
	if !strings.Contains(err.Error(), "integer") {
		t.Errorf("error %q does not mention 'integer'", err.Error())
	}
}

func TestView_NRejectsNegative(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "n(-1):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for negative n, got nil")
	}
	if !strings.Contains(err.Error(), "negative") &&
		!strings.Contains(err.Error(), "non-negative") {
		t.Errorf("error %q does not mention non-negativity", err.Error())
	}
}

// --- Slice 3: rank() primitive ---

// TestView_RankInDegree builds a graph with known in-degrees and
// verifies rank(in-degree) sorts entries descending by reference count.
// Uses in-degree (not heat) so the test is independent of wall-clock
// time — the score math is decay-insensitive.
func TestView_RankInDegree(t *testing.T) {
	target := entry("20260101-100000-d-tac-trg", withKind(model.KindDirective))
	popular := entry("20260101-110000-d-tac-pop", withKind(model.KindDirective))
	// Three refs pointing at popular, one ref pointing at target.
	r1 := entry("20260101-120000-d-tac-rf1", withKind(model.KindDirective), withRefs(popular.ID))
	r2 := entry("20260101-130000-d-tac-rf2", withKind(model.KindDirective), withRefs(popular.ID))
	r3 := entry("20260101-140000-d-tac-rf3", withKind(model.KindDirective), withRefs(popular.ID))
	r4 := entry("20260101-150000-d-tac-rf4", withKind(model.KindDirective), withRefs(target.ID))

	g := model.NewGraph([]*model.Entry{target, popular, r1, r2, r3, r4})

	layout := mustParseLayout(t, "rank(in-degree):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)

	// Expect popular (in-degree 3) first, target (in-degree 1) second,
	// then the four refs (in-degree 0) in stable order.
	if len(flat.Entries) == 0 {
		t.Fatalf("expected non-empty result")
	}
	if flat.Entries[0].ID != popular.ID {
		t.Errorf("first entry: got %s, want %s (highest in-degree)", flat.Entries[0].ID, popular.ID)
	}
	if flat.Entries[1].ID != target.ID {
		t.Errorf("second entry: got %s, want %s (in-degree 1)", flat.Entries[1].ID, target.ID)
	}
	// Scores parallel to entries.
	if len(flat.Scores) != len(flat.Entries) {
		t.Errorf("scores length: got %d, want %d", len(flat.Scores), len(flat.Entries))
	}
	if flat.Scores[0] != 3 {
		t.Errorf("first score: got %v, want 3", flat.Scores[0])
	}
	if flat.Scores[1] != 1 {
		t.Errorf("second score: got %v, want 1", flat.Scores[1])
	}
}

// TestView_RankByDate verifies by(date) sorts entries by Time descending
// and leaves Scores nil — by(date) is a sort, not a ranking.
func TestView_RankByDate(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260102-100000-d-tac-bbb", withKind(model.KindDirective))
	c := entry("20260103-100000-d-tac-ccc", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a, b, c})

	layout := mustParseLayout(t, "rank(by(date)):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)

	want := []string{c.ID, b.ID, a.ID}
	got := idsOf(flat.Entries)
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
	if flat.Scores != nil {
		t.Errorf("by(date) should leave Scores nil, got %v", flat.Scores)
	}
}

// TestView_RankHeatDefaultDecay smoke-tests rank(heat) — verifies it
// runs without error and produces a sorted result with scores. Score
// math itself is tested in model.HeatScore tests.
func TestView_RankHeatDefaultDecay(t *testing.T) {
	target := entry("20260101-100000-d-tac-trg", withKind(model.KindDirective))
	ref := entry("20260101-110000-d-tac-ref", withKind(model.KindDirective), withRefs(target.ID))
	g := model.NewGraph([]*model.Entry{target, ref})

	layout := mustParseLayout(t, "rank(heat):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if len(flat.Scores) != len(flat.Entries) {
		t.Errorf("scores length: got %d, want %d", len(flat.Scores), len(flat.Entries))
	}
	// target (1 incoming ref) should rank above ref (0 incoming refs).
	if flat.Entries[0].ID != target.ID {
		t.Errorf("first entry: got %s, want %s (has incoming ref)", flat.Entries[0].ID, target.ID)
	}
}

// TestView_RankHeatExplicitDecay confirms rank(heat(exp-7d)) parses and
// executes without error — the model's decay tests cover the math.
func TestView_RankHeatExplicitDecay(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, "rank(heat(exp-7d)):as-list")
	f := New(Options{})
	if _, err := f.View(query.ViewQuery{Graph: g, Layout: layout}); err != nil {
		t.Fatalf("View: %v", err)
	}
}

// TestView_RankHeatNoneDecay covers heat(none) — heat with no decay
// collapses to weighted in-degree, useful for "structurally central
// regardless of recency" per the design's sample invocations.
func TestView_RankHeatNoneDecay(t *testing.T) {
	target := entry("20260101-100000-d-tac-trg", withKind(model.KindDirective))
	r1 := entry("20260101-110000-d-tac-rf1", withKind(model.KindDirective), withRefs(target.ID))
	r2 := entry("20260101-120000-d-tac-rf2", withKind(model.KindDirective), withRefs(target.ID))
	g := model.NewGraph([]*model.Entry{target, r1, r2})

	layout := mustParseLayout(t, "rank(heat(none)):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if flat.Entries[0].ID != target.ID {
		t.Errorf("first entry: got %s, want %s (highest weighted in-degree)", flat.Entries[0].ID, target.ID)
	}
	// With none decay, heat == in-degree exactly.
	if flat.Scores[0] != 2 {
		t.Errorf("first score: got %v, want 2 (heat=in-degree under none decay)", flat.Scores[0])
	}
}

// TestView_RankComposesWithFilterAndPage verifies the canonical
// filter→rank→page order. Pagination is applied AFTER ranking so the
// "top N by score" semantics work.
func TestView_RankComposesWithFilterAndPage(t *testing.T) {
	target := entry("20260101-100000-d-tac-trg", withKind(model.KindPlan))
	popular := entry("20260101-110000-d-tac-pop", withKind(model.KindPlan))
	other := entry("20260101-120000-d-tac-oth", withKind(model.KindPlan))
	directive := entry("20260101-130000-d-tac-dir", withKind(model.KindDirective))
	r1 := entry("20260101-140000-d-tac-rf1", withKind(model.KindDirective), withRefs(popular.ID))
	r2 := entry("20260101-150000-d-tac-rf2", withKind(model.KindDirective), withRefs(popular.ID))
	r3 := entry("20260101-160000-d-tac-rf3", withKind(model.KindDirective), withRefs(target.ID))

	g := model.NewGraph([]*model.Entry{target, popular, other, directive, r1, r2, r3})

	// kind(plan) limits to 3 plans; rank(in-degree) orders by in-degree;
	// n(2) takes top 2 → popular (2 refs), target (1 ref). 'other' and
	// 'directive' are filtered out.
	layout := mustParseLayout(t, "kind(plan):rank(in-degree):n(2):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	want := []string{popular.ID, target.ID}
	got := idsOf(flat.Entries)
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
	if len(flat.Scores) != 2 {
		t.Errorf("scores length: got %d, want 2", len(flat.Scores))
	}
}

// Error paths — argument validation and unknown algorithms/decays.

func TestView_RankUnknownAlgorithm(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "rank(future-algo):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for unknown algorithm, got nil")
	}
	for _, want := range []string{"unknown rank algorithm", "future-algo", "heat", "in-degree"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing substring %q", err.Error(), want)
		}
	}
}

func TestView_RankUnknownDecay(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "rank(heat(exp-99d)):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for unknown decay, got nil")
	}
	if !strings.Contains(err.Error(), "exp-99d") {
		t.Errorf("error %q does not mention 'exp-99d'", err.Error())
	}
}

func TestView_RankBareIdentifierAcceptedAsShorthand(t *testing.T) {
	// rank(heat) is shorthand for rank(heat()) — the algorithm with no
	// decay, picking up the default. Verifies the bare-identifier path
	// reaches the same algorithm dispatch as the function-call path.
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, "rank(in-degree):as-list")
	f := New(Options{})
	if _, err := f.View(query.ViewQuery{Graph: g, Layout: layout}); err != nil {
		t.Fatalf("View: %v (rank(in-degree) shorthand should work)", err)
	}
}

func TestView_RankNoArgs(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "rank():as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for rank with no args, got nil")
	}
}

func TestView_RankByOnlyDate(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "rank(by(name)):as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for by(name), got nil")
	}
	if !strings.Contains(err.Error(), "by(date)") {
		t.Errorf("error %q does not mention 'by(date)'", err.Error())
	}
}

func TestView_RankInDegreeIgnoresDecayArg(t *testing.T) {
	// in-degree silently ignores a decay arg per the design — it has no
	// recency component to weight, so the grammar accepts but the
	// executor discards.
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "rank(in-degree(exp-7d)):as-list")
	f := New(Options{})
	if _, err := f.View(query.ViewQuery{Graph: g, Layout: layout}); err != nil {
		t.Fatalf("View: %v (in-degree should ignore decay)", err)
	}
}

// helpers

func mustParseLayout(t *testing.T, s string) model.Layout {
	t.Helper()
	l, err := query.ParseLayout(s)
	if err != nil {
		t.Fatalf("ParseLayout(%q): %v", s, err)
	}
	return l
}

func idsOf(entries []*model.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
