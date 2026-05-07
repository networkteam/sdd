package finders

import (
	"strings"
	"testing"
	"time"

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

// --- Slice 4: layer() / since() / topic() filter primitives ---

func TestView_LayerFilterAbbrev(t *testing.T) {
	stg := entry("20260101-100000-d-stg-aaa", withKind(model.KindAspiration))
	cpt := entry("20260101-110000-d-cpt-bbb", withKind(model.KindContract))
	tac := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{stg, cpt, tac})

	layout := mustParseLayout(t, "layer(tac):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{tac.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_LayerFilterFullName(t *testing.T) {
	// layer(tactical) should resolve to the same set as layer(tac).
	stg := entry("20260101-100000-d-stg-aaa", withKind(model.KindAspiration))
	tac := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{stg, tac})

	layout := mustParseLayout(t, "layer(tactical):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	if len(got) != 1 || got[0] != tac.ID {
		t.Errorf("layer(tactical): got %v, want [%s]", got, tac.ID)
	}
}

func TestView_LayerFilterStringForm(t *testing.T) {
	// Quoted form should work too: layer("tac").
	tac := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{tac})

	layout := mustParseLayout(t, `layer("tac"):as-list`)
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if len(flat.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(flat.Entries))
	}
}

func TestView_LayerWrongArgs(t *testing.T) {
	g := model.NewGraph(nil)
	cases := []string{
		"layer():as-list",
		"layer(tac, ops):as-list",
	}
	for _, layoutStr := range cases {
		t.Run(layoutStr, func(t *testing.T) {
			// "tac, ops" has whitespace which the parser rejects, so use
			// no-whitespace form. Just test arg-count errors.
			ls := layoutStr
			if ls == "layer(tac, ops):as-list" {
				ls = "layer(tac,ops):as-list"
			}
			layout := mustParseLayout(t, ls)
			f := New(Options{})
			if _, err := f.View(query.ViewQuery{Graph: g, Layout: layout}); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

// TestView_SinceDuration uses synthetic entries with deliberate times so
// the duration cutoff produces deterministic results without needing to
// inject a clock — the cutoff is computed from time.Now() at execution,
// and the fixture sets entry times relative to a fixed past anchor with
// large enough offsets that the test stays stable regardless of when
// it runs.
func TestView_SinceDuration(t *testing.T) {
	// Far past: 2020-01-01 — well outside any reasonable since() window.
	old := entry("20200101-100000-d-tac-old", withKind(model.KindDirective))
	// Recent past: 2 days ago, which 7d will include.
	recentTime := time.Now().Add(-2 * 24 * time.Hour)
	recent := &model.Entry{
		ID:    "20260101-100000-d-tac-rec",
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Time:  recentTime,
	}
	g := model.NewGraph([]*model.Entry{old, recent})

	layout := mustParseLayout(t, `since("7d"):as-list`)
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	if len(got) != 1 || got[0] != recent.ID {
		t.Errorf("since(7d): got %v, want [%s]", got, recent.ID)
	}
}

func TestView_SinceISODate(t *testing.T) {
	// since("2026-01-01") includes entries dated 2026-01-01 onward.
	a := entry("20251215-100000-d-tac-old", withKind(model.KindDirective))
	b := entry("20260201-100000-d-tac-new", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a, b})

	layout := mustParseLayout(t, `since("2026-01-01"):as-list`)
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	if len(got) != 1 || got[0] != b.ID {
		t.Errorf("since(2026-01-01): got %v, want [%s]", got, b.ID)
	}
}

func TestView_SinceMalformedSpec(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, `since("not-a-spec"):as-list`)
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error for malformed since spec, got nil")
	}
	if !strings.Contains(err.Error(), "since") {
		t.Errorf("error %q does not mention 'since'", err.Error())
	}
}

func TestView_TopicFilterInline(t *testing.T) {
	// Entry with inline topic 'catch-up-scaling' matches topic(catch-up-scaling).
	tagged := &model.Entry{
		ID:     "20260101-100000-s-cpt-aaa",
		Type:   model.TypeSignal,
		Kind:   model.KindGap,
		Layer:  model.LayerConceptual,
		Topics: []model.TopicPath{mustParseTopic(t, "catch-up-scaling")},
	}
	other := &model.Entry{
		ID:    "20260101-110000-s-cpt-bbb",
		Type:  model.TypeSignal,
		Kind:  model.KindGap,
		Layer: model.LayerConceptual,
	}
	g := model.NewGraph([]*model.Entry{tagged, other})

	layout := mustParseLayout(t, "topic(catch-up-scaling):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	if len(got) != 1 || got[0] != tagged.ID {
		t.Errorf("topic(catch-up-scaling): got %v, want [%s]", got, tagged.ID)
	}
}

func TestView_TopicFilterStringForm(t *testing.T) {
	// Topic paths with `/` need string form.
	tagged := &model.Entry{
		ID:     "20260101-100000-s-cpt-aaa",
		Type:   model.TypeSignal,
		Kind:   model.KindGap,
		Layer:  model.LayerConceptual,
		Topics: []model.TopicPath{mustParseTopic(t, "infrastructure/cli")},
	}
	g := model.NewGraph([]*model.Entry{tagged})

	layout := mustParseLayout(t, `topic("infrastructure/cli"):as-list`)
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if len(flat.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(flat.Entries))
	}
}

func TestView_TopicFilterPrefixComponent(t *testing.T) {
	// Component-wise prefix: topic("UX") matches "UX/CLI" but not "UXTesting".
	uxCli := &model.Entry{
		ID:     "20260101-100000-s-cpt-aaa",
		Type:   model.TypeSignal,
		Kind:   model.KindGap,
		Layer:  model.LayerConceptual,
		Topics: []model.TopicPath{mustParseTopic(t, "UX/CLI")},
	}
	uxTesting := &model.Entry{
		ID:     "20260101-110000-s-cpt-bbb",
		Type:   model.TypeSignal,
		Kind:   model.KindGap,
		Layer:  model.LayerConceptual,
		Topics: []model.TopicPath{mustParseTopic(t, "UXTesting")},
	}
	g := model.NewGraph([]*model.Entry{uxCli, uxTesting})

	layout := mustParseLayout(t, "topic(UX):as-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	if len(got) != 1 || got[0] != uxCli.ID {
		t.Errorf("topic(UX): got %v, want [%s] (component-wise prefix)", got, uxCli.ID)
	}
}

func TestView_AllFiltersCompose(t *testing.T) {
	// Full slice-4 composition: active + kind + layer + since + topic + rank + page.
	// Verifies every primitive is reachable in the same pipeline.
	plan := &model.Entry{
		ID:     "20260420-100000-d-tac-pln",
		Type:   model.TypeDecision,
		Layer:  model.LayerTactical,
		Kind:   model.KindPlan,
		Time:   time.Now().Add(-3 * 24 * time.Hour),
		Topics: []model.TopicPath{mustParseTopic(t, "infrastructure")},
	}
	otherKind := &model.Entry{
		ID:     "20260420-110000-d-tac-dir",
		Type:   model.TypeDecision,
		Layer:  model.LayerTactical,
		Kind:   model.KindDirective,
		Time:   time.Now().Add(-3 * 24 * time.Hour),
		Topics: []model.TopicPath{mustParseTopic(t, "infrastructure")},
	}
	wrongLayer := &model.Entry{
		ID:     "20260420-120000-d-stg-pln",
		Type:   model.TypeDecision,
		Layer:  model.LayerStrategic,
		Kind:   model.KindPlan,
		Time:   time.Now().Add(-3 * 24 * time.Hour),
		Topics: []model.TopicPath{mustParseTopic(t, "infrastructure")},
	}
	tooOld := &model.Entry{
		ID:     "20200101-100000-d-tac-old",
		Type:   model.TypeDecision,
		Layer:  model.LayerTactical,
		Kind:   model.KindPlan,
		Time:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Topics: []model.TopicPath{mustParseTopic(t, "infrastructure")},
	}
	wrongTopic := &model.Entry{
		ID:    "20260420-130000-d-tac-pl2",
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindPlan,
		Time:  time.Now().Add(-3 * 24 * time.Hour),
	}
	g := model.NewGraph([]*model.Entry{plan, otherKind, wrongLayer, tooOld, wrongTopic})

	layout := mustParseLayout(t, `active:kind(plan):layer(tac):since("7d"):topic(infrastructure):n(10):as-list`)
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{plan.ID}
	if !equalIDs(got, want) {
		t.Errorf("composed filter:\n  got:  %v\n  want: %v", got, want)
	}
}

// --- Slice 5: group(by(field)) + as-grouped ---

func TestView_GroupByKind_AsGrouped(t *testing.T) {
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	dir1 := entry("20260101-110000-d-tac-da1", withKind(model.KindDirective))
	dir2 := entry("20260101-120000-d-tac-da2", withKind(model.KindDirective))
	gap := entry("20260101-130000-s-tac-gap", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{plan, dir1, dir2, gap})

	layout := mustParseLayout(t, "group(by(kind)):as-grouped")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	section := result.Sections[0]
	if section.Render != "as-grouped" {
		t.Errorf("render: got %q, want as-grouped", section.Render)
	}
	grouped, ok := section.Data.(model.Grouped)
	if !ok {
		t.Fatalf("section data: got %T, want model.Grouped", section.Data)
	}
	if grouped.Field != "kind" {
		t.Errorf("Field: got %q, want kind", grouped.Field)
	}

	// Alphabetical group order: directive, gap, plan.
	wantKeys := []string{"directive", "gap", "plan"}
	gotKeys := make([]string, len(grouped.Groups))
	for i, gr := range grouped.Groups {
		gotKeys[i] = gr.Key
	}
	if !equalStrings(gotKeys, wantKeys) {
		t.Errorf("group keys:\n  got:  %v\n  want: %v", gotKeys, wantKeys)
	}

	// Within-group order is input order — dir1 before dir2.
	directiveGroup := grouped.Groups[0]
	if got, want := idsOf(directiveGroup.Entries), []string{dir1.ID, dir2.ID}; !equalIDs(got, want) {
		t.Errorf("directive entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_GroupByKind_ComposesWithKindFilter(t *testing.T) {
	// The decisions macro shape: filter to specific kinds, then group by kind.
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	gap := entry("20260101-120000-s-tac-gap", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{plan, directive, gap})

	layout := mustParseLayout(t, "kind(plan,directive):group(by(kind)):as-grouped")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	grouped := result.Sections[0].Data.(model.Grouped)
	wantKeys := []string{"directive", "plan"}
	gotKeys := make([]string, len(grouped.Groups))
	for i, gr := range grouped.Groups {
		gotKeys[i] = gr.Key
	}
	if !equalStrings(gotKeys, wantKeys) {
		t.Errorf("group keys:\n  got:  %v\n  want: %v", gotKeys, wantKeys)
	}
}

func TestView_GroupRequiresByMarker(t *testing.T) {
	// Per d-tac-3pq: group's argument must be the nested marker call by(<field>),
	// not a bare identifier or string. Slice 5 is strict about this so the
	// nested-form contract surfaces clearly to users.
	g := model.NewGraph(nil)
	cases := []string{
		"group():as-grouped",
		"group(kind):as-grouped",
		`group("kind"):as-grouped`,
	}
	for _, layoutSrc := range cases {
		t.Run(layoutSrc, func(t *testing.T) {
			f := New(Options{})
			_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, layoutSrc)})
			if err == nil {
				t.Fatalf("expected error for %q, got nil", layoutSrc)
			}
			if !strings.Contains(err.Error(), "by(") {
				t.Errorf("error %q must mention by(<field>)", err.Error())
			}
		})
	}
}

func TestView_GroupRequiresFieldArg(t *testing.T) {
	// by() with no inner argument is a clear user mistake — error rather
	// than silently grouping into a single empty-key bucket.
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, "group(by()):as-grouped")})
	if err == nil {
		t.Fatalf("expected error for group(by()), got nil")
	}
}

func TestView_GroupRejectsUnknownField(t *testing.T) {
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, "group(by(summary)):as-grouped")})
	if err == nil {
		t.Fatalf("expected error for unknown field 'summary', got nil")
	}
	for _, want := range []string{"summary", "kind", "layer", "type"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestView_AsGroupedWithoutGroup_ShapeMismatch(t *testing.T) {
	// as-grouped expects a Grouped result; without a group() call the
	// section produces a flat list — this is the AC 16 render-shape
	// mismatch error fired for the first time.
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, "as-grouped")})
	if err == nil {
		t.Fatalf("expected render-shape mismatch error, got nil")
	}
	for _, want := range []string{"as-grouped", "flat", "group("} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestView_GroupWithAsList_ShapeMismatch(t *testing.T) {
	// Symmetric mismatch: group(...) followed by as-list — grouped result,
	// flat-shape render. Same listed-valid-set guidance.
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, "group(by(kind)):as-list")})
	if err == nil {
		t.Fatalf("expected render-shape mismatch error, got nil")
	}
	for _, want := range []string{"as-list", "grouped"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestView_GroupExclusiveWithRank(t *testing.T) {
	// Slice 5 doesn't sort within groups — combining group() with rank()
	// errors clearly. Per-group ranking is reserved for a future slice.
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, "group(by(kind)):rank(in-degree):as-grouped")})
	if err == nil {
		t.Fatalf("expected error for group + rank, got nil")
	}
	if !strings.Contains(err.Error(), "rank") || !strings.Contains(err.Error(), "group") {
		t.Errorf("error %q must mention both group and rank", err.Error())
	}
}

func TestView_GroupExclusiveWithN(t *testing.T) {
	// Same reasoning for n() — the meaning of "first N entries" across
	// groups is ambiguous in slice 5; clear error rather than guessing.
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, "group(by(kind)):n(5):as-grouped")})
	if err == nil {
		t.Fatalf("expected error for group + n, got nil")
	}
	if !strings.Contains(err.Error(), "n") || !strings.Contains(err.Error(), "group") {
		t.Errorf("error %q must mention both group and n", err.Error())
	}
}

func TestView_GroupAsGroupedKnownInUnknownErr(t *testing.T) {
	// Unknown function error must list group and as-grouped after slice 5
	// so the listed-valid-set message stays accurate as the vocabulary
	// grows.
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "futurefn:as-list")
	f := New(Options{})
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	for _, want := range []string{"group", "as-grouped"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
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

func mustParseTopic(t *testing.T, s string) model.TopicPath {
	t.Helper()
	p, err := model.ParseTopicPath(s)
	if err != nil {
		t.Fatalf("ParseTopicPath(%q): %v", s, err)
	}
	return p
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
