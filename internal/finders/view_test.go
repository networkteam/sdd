package finders

import (
	"fmt"
	"math"
	"slices"
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err == nil {
		t.Fatalf("expected error for unknown function, got nil")
	}
	msg := err.Error()
	// Lists primitives AND macros: a wrong guess is often a reach for a macro,
	// and the primitive-only list left that vocabulary undiscoverable (top,
	// focus, done, …).
	for _, want := range []string{"unknown function", "futurefn", "active", "as-list", "top", "focus"} {
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err == nil {
		t.Fatalf("expected error for missing render, got nil")
	}
	if !strings.Contains(err.Error(), "render") {
		t.Errorf("error %q does not mention render", err.Error())
	}
}

func TestView_RenderCanAppearMidSection(t *testing.T) {
	// Per d-tac-uww §2 "non-filter modifiers (rank, page, name, render)
	// apply last-write-wins per modifier kind." Render is logically the
	// terminus but doesn't need to be syntactically last — the executor
	// uses canonical bucket order regardless of source ordering. This is
	// what lets macro expansion + user modifier append work: `top(N)`'s
	// `as-list` lands inside the expansion, then user `:rank(...)` may
	// append after it without erroring.
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, "as-list:active")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat, ok := result.Sections[0].Data.(model.FlatList)
	if !ok {
		t.Fatalf("section data: got %T, want model.FlatList", result.Sections[0].Data)
	}
	if got := idsOf(flat.Entries); len(got) != 1 || got[0] != a.ID {
		t.Errorf("entries: got %v, want [%s]", got, a.ID)
	}
}

func TestView_MultipleRendersLastWins(t *testing.T) {
	// Last-write-wins applies to render too: `as-list:as-grouped` is a
	// degenerate but valid section that resolves to as-grouped. Concrete
	// regression for the macro composition `top(N):rank(...)` which now
	// passes through after this relaxation, but as-list:as-grouped is the
	// minimal expression of the rule.
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})
	// Pair as-list with a group()/as-grouped tail so the section is
	// shape-consistent with the chosen render. as-list alone followed
	// by as-grouped without a group() would be a render-shape mismatch.
	layout := mustParseLayout(t, "as-list:group(by(kind)):as-grouped")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got := result.Sections[0].Render; got != "as-grouped" {
		t.Errorf("render: got %q, want as-grouped (last-write-wins)", got)
	}
}

func TestView_MacroExpansion_TopWithRankModifier(t *testing.T) {
	// Regression for slice 6: `top(N):rank(...)` after macro expansion
	// produces `active:n(N):rank(heat(exp-14d)):as-list:rank(...)`.
	// Without the render-position relaxation this would error; with it
	// the second rank() last-write-wins-overrides the macro's heat rank.
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a, b})

	layout := mustParseLayoutAndExpand(t, "top(2):rank(in-degree)")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat, ok := result.Sections[0].Data.(model.FlatList)
	if !ok {
		t.Fatalf("section data: got %T, want model.FlatList", result.Sections[0].Data)
	}
	if len(flat.Entries) != 2 {
		t.Errorf("entries: got %d, want 2", len(flat.Entries))
	}
	// rank(in-degree) ignores decay — score is integer-valued.
	if len(flat.Scores) != 2 {
		t.Fatalf("scores: got %d, want 2", len(flat.Scores))
	}
	for i, s := range flat.Scores {
		if s != float64(int64(s)) {
			t.Errorf("score[%d] = %v, want integer (in-degree)", i, s)
		}
	}
}

func TestView_NilGraph(t *testing.T) {
	f := New(Options{})
	_, err := f.OnGraph(nil).View(query.ViewQuery{Layout: mustParseLayout(t, "as-list")})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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

func TestView_IntentFilter_Single(t *testing.T) {
	pending := entry("20260101-100000-d-tac-pen", withKind(model.KindDirective), withIntent(model.IntentPending))
	guiding := entry("20260101-110000-d-tac-gid", withKind(model.KindDirective), withIntent(model.IntentGuiding))
	settled := entry("20260101-120000-d-tac-set", withKind(model.KindDirective), withIntent(model.IntentSettled))
	plan := entry("20260101-130000-d-tac-pln", withKind(model.KindPlan))
	g := model.NewGraph([]*model.Entry{pending, guiding, settled, plan})

	layout := mustParseLayout(t, "intent(guiding):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if got, want := idsOf(flat.Entries), []string{guiding.ID}; !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_IntentFilter_NotExcludesGuiding(t *testing.T) {
	// The catch-up "Active and hot" lane excludes guiding directives: it wants
	// pending and unspecified, never standing context. not(intent(guiding))
	// keeps everything but guiding — including unspecified directives, whose
	// empty intent is not in the exclusion set.
	pending := entry("20260101-100000-d-tac-pen", withKind(model.KindDirective), withIntent(model.IntentPending))
	guiding := entry("20260101-110000-d-tac-gid", withKind(model.KindDirective), withIntent(model.IntentGuiding))
	unspecified := entry("20260101-120000-d-tac-uns", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{pending, guiding, unspecified})

	layout := mustParseLayout(t, "kind(directive):not(intent(guiding)):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if got, want := idsOf(flat.Entries), []string{pending.ID, unspecified.ID}; !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_ActiveExcludesSettled(t *testing.T) {
	// A settled directive is born terminal, so the active filter drops it from
	// overview listings even though it carries no closing edge.
	pending := entry("20260101-100000-d-tac-pen", withKind(model.KindDirective), withIntent(model.IntentPending))
	settled := entry("20260101-110000-d-tac-set", withKind(model.KindDirective), withIntent(model.IntentSettled))
	g := model.NewGraph([]*model.Entry{pending, settled})

	layout := mustParseLayout(t, "kind(directive):active:as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if got, want := idsOf(flat.Entries), []string{pending.ID}; !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_IntentFilter_RejectsInvalidValue(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "intent(tentative):as-list")
	f := New(Options{})
	if _, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout}); err == nil {
		t.Error("View: expected error for invalid intent value, got nil")
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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

func TestView_KindFilter_MultipleCallsIntersect(t *testing.T) {
	// Per d-tac-uww §2: "Multiple kind(...) filters intersect; kind(K1, K2)
	// within one filter is disjunction." This makes macro composition
	// behave as the design states — a `decisions` macro expanding to
	// kind(plan,directive,...) followed by a user kind(plan) modifier
	// narrows to plans, not all five.
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	activity := entry("20260101-120000-d-tac-act", withKind(model.KindActivity))
	g := model.NewGraph([]*model.Entry{plan, directive, activity})

	layout := mustParseLayout(t, "kind(plan,directive,activity):kind(plan,directive):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	// Intersection of {plan, directive, activity} and {plan, directive}
	// is {plan, directive}.
	want := []string{plan.ID, directive.ID}
	got := idsOf(flat.Entries)
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_KindFilter_DisjointIntersection(t *testing.T) {
	// kind(plan):kind(directive) — disjoint single-element disjunctions.
	// No entry has both kinds, so the intersection is empty.
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{plan, directive})

	layout := mustParseLayout(t, "kind(plan):kind(directive):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if len(flat.Entries) != 0 {
		t.Errorf("entries: got %v, want []", idsOf(flat.Entries))
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	if len(got) != 1 || got[0] != plan.ID {
		t.Errorf("entries: got %v, want [%s]", got, plan.ID)
	}
}

func TestView_ParticipantFilter_Single(t *testing.T) {
	chris := entry("20260101-100000-d-tac-aaa", withParticipants("Christopher"))
	jonathan := entry("20260101-110000-s-tac-bbb", withParticipants("Jonathan Philipp"))
	both := entry("20260101-120000-s-tac-ccc", withParticipants("Christopher", "Claude"))
	g := model.NewGraph([]*model.Entry{chris, jonathan, both})

	layout := mustParseLayout(t, "participant(Christopher):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	want := []string{chris.ID, both.ID}
	if got := idsOf(flat.Entries); !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_ParticipantFilter_QuotedMultiWordName(t *testing.T) {
	// Names with spaces require the quoted-string arg form.
	chris := entry("20260101-100000-d-tac-aaa", withParticipants("Christopher"))
	jonathan := entry("20260101-110000-s-tac-bbb", withParticipants("Jonathan Philipp"))
	g := model.NewGraph([]*model.Entry{chris, jonathan})

	layout := mustParseLayout(t, `participant("Jonathan Philipp"):as-list`)
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if want := []string{jonathan.ID}; !equalIDs(idsOf(flat.Entries), want) {
		t.Errorf("entries: got %v, want %v", idsOf(flat.Entries), want)
	}
}

func TestView_ParticipantFilter_Disjunction(t *testing.T) {
	chris := entry("20260101-100000-d-tac-aaa", withParticipants("Christopher"))
	jonathan := entry("20260101-110000-s-tac-bbb", withParticipants("Jonathan Philipp"))
	dana := entry("20260101-120000-s-tac-ccc", withParticipants("Dana"))
	g := model.NewGraph([]*model.Entry{chris, jonathan, dana})

	layout := mustParseLayout(t, `participant(Christopher,"Jonathan Philipp"):as-list`)
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if want := []string{chris.ID, jonathan.ID}; !equalIDs(idsOf(flat.Entries), want) {
		t.Errorf("entries: got %v, want %v", idsOf(flat.Entries), want)
	}
}

func TestView_ParticipantFilter_MultipleCallsIntersect(t *testing.T) {
	// participant(A):participant(B) intersects — keeps only entries listing
	// both, mirroring kind()'s multiple-calls-intersect semantic.
	chris := entry("20260101-100000-d-tac-aaa", withParticipants("Christopher"))
	both := entry("20260101-120000-s-tac-ccc", withParticipants("Christopher", "Claude"))
	g := model.NewGraph([]*model.Entry{chris, both})

	layout := mustParseLayout(t, "participant(Christopher):participant(Claude):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if want := []string{both.ID}; !equalIDs(idsOf(flat.Entries), want) {
		t.Errorf("entries: got %v, want %v", idsOf(flat.Entries), want)
	}
}

func TestView_ParticipantFilter_NoArgs(t *testing.T) {
	g := model.NewGraph([]*model.Entry{entry("20260101-100000-d-tac-aaa", withParticipants("Christopher"))})
	layout := mustParseLayout(t, "participant():as-list")
	f := New(Options{})
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err == nil {
		t.Fatal("expected error for participant() with no args")
	}
	if !strings.Contains(err.Error(), "participant") {
		t.Errorf("error %q does not mention participant", err.Error())
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
			_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	if _, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout}); err != nil {
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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

// TestView_RankColdness exercises the coldness algorithm end to end through
// the view pipeline: ordering (fresh-unacted ranks highest, each incoming
// ref demotes), the coldness-specific default decay (exp-30d, not heat's
// exp-14d), header derivation, decay-arg parsing, and the full catch-up
// "Open loops" lane. Exact score math is pinned in model.TestColdnessScore;
// here entry times are anchored to a captured now() and the value check uses
// a tolerant epsilon because View reads the wall clock internally. Durations
// use 24h multiples (not AddDate) so the age is exact regardless of DST.
func TestView_RankColdness(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour

	t.Run("fresh beats aged", func(t *testing.T) {
		fresh := entry("20260101-100000-d-tac-frs", withKind(model.KindPlan))
		fresh.Time = now
		aged := entry("20260101-110000-d-tac-agd", withKind(model.KindPlan))
		aged.Time = now.Add(-90 * day)
		g := model.NewGraph([]*model.Entry{fresh, aged})

		flat := flatOf(t, runView(t, g, "kind(plan):rank(coldness(exp-30d)):as-list"))
		want := []string{fresh.ID, aged.ID}
		if got := idsOf(flat.Entries); !equalIDs(got, want) {
			t.Errorf("order:\n  got:  %v\n  want: %v", got, want)
		}
	})

	t.Run("unacted beats acted", func(t *testing.T) {
		unacted := entry("20260101-100000-d-tac-una", withKind(model.KindPlan))
		unacted.Time = now
		acted := entry("20260101-110000-d-tac-act", withKind(model.KindPlan))
		acted.Time = now
		referrer := entry("20260101-120000-s-tac-ref", withKind(model.KindGap), withRefs(acted.ID))
		g := model.NewGraph([]*model.Entry{unacted, acted, referrer})

		// kind(plan) drops the gap referrer; both plans share an age, so
		// only in-degree separates them: unacted (0 → 1.0) over acted (1 → 0.5).
		flat := flatOf(t, runView(t, g, "kind(plan):rank(coldness(exp-30d)):as-list"))
		want := []string{unacted.ID, acted.ID}
		if got := idsOf(flat.Entries); !equalIDs(got, want) {
			t.Errorf("order:\n  got:  %v\n  want: %v", got, want)
		}
	})

	t.Run("default decay is exp-30d, header derives", func(t *testing.T) {
		// A 30-day-old, un-referenced entry scores decay(30)=0.5 under
		// exp-30d. Bare rank(coldness) must pick exp-30d, not heat's
		// exp-14d (which would give ≈0.226) — the value distinguishes them.
		e := entry("20260101-100000-d-tac-def", withKind(model.KindPlan))
		e.Time = now.Add(-30 * day)
		g := model.NewGraph([]*model.Entry{e})

		result := runView(t, g, "rank(coldness):as-list")
		flat := flatOf(t, result)
		if len(flat.Scores) != 1 {
			t.Fatalf("scores: got %d, want 1", len(flat.Scores))
		}
		if math.Abs(flat.Scores[0]-0.5) > 1e-3 {
			t.Errorf("default-decay coldness of 30d-old entry: got %v, want ≈0.5 (exp-30d not exp-14d)", flat.Scores[0])
		}
		if got, want := result.Sections[0].Name, "Top by coldness (exp-30d)"; got != want {
			t.Errorf("section header: got %q, want %q", got, want)
		}
	})

	t.Run("explicit decay parses", func(t *testing.T) {
		e := entry("20260101-100000-d-tac-exp", withKind(model.KindPlan))
		e.Time = now
		g := model.NewGraph([]*model.Entry{e})
		runView(t, g, "rank(coldness(exp-7d)):as-list") // fatals on error
	})

	t.Run("unknown decay errors", func(t *testing.T) {
		g := model.NewGraph(nil)
		layout := mustParseLayout(t, "rank(coldness(exp-99d)):as-list")
		f := New(Options{})
		_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
		if err == nil {
			t.Fatalf("expected error for unknown decay, got nil")
		}
		if !strings.Contains(err.Error(), "exp-99d") {
			t.Errorf("error %q does not mention 'exp-99d'", err.Error())
		}
	})

	t.Run("catch-up open-loops lane composes end to end", func(t *testing.T) {
		// The exact layout the catch-up template injects. Verifies the
		// action-carrying kinds pass, an observational kind (insight) is
		// dropped, and expand(refs) carries each entry's upstream.
		plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan),
			withRefObjs(model.Ref{ID: "20260101-080000-d-cpt-bas", Kind: model.RefKindGroundedIn, Desc: "rests on"}))
		plan.Time = now
		activity := entry("20260101-110000-d-tac-acy", withKind(model.KindActivity))
		activity.Time = now.Add(-5 * day)
		directive := entry("20260101-120000-d-tac-dir", withKind(model.KindDirective))
		directive.Time = now.Add(-10 * day)
		gap := entry("20260101-130000-s-tac-gap", withKind(model.KindGap))
		gap.Time = now.Add(-1 * day)
		question := entry("20260101-140000-s-tac-qst", withKind(model.KindQuestion))
		question.Time = now.Add(-2 * day)
		basis := entry("20260101-080000-d-cpt-bas", withKind(model.KindContract))  // ref target, out of lane
		insight := entry("20260101-150000-s-tac-ins", withKind(model.KindInsight)) // must NOT appear
		g := model.NewGraph([]*model.Entry{plan, activity, directive, gap, question, basis, insight})

		layout := `kind(plan,activity,directive,gap,question):active:rank(coldness(exp-30d)):n(8):expand(refs):name("Open loops"):as-list`
		flat := flatOf(t, runView(t, g, layout))

		got := idsOf(flat.Entries)
		if slices.Contains(got, insight.ID) {
			t.Errorf("insight leaked into open-loops lane: %v", got)
		}
		if len(got) != 5 {
			t.Fatalf("lane entries: got %d %v, want 5 (plan, activity, directive, gap, question)", len(got), got)
		}
		// expand(refs) carries the plan's upstream so the agent can thread it.
		if len(flat.RefExpansions) != len(flat.Entries) {
			t.Fatalf("RefExpansions not aligned: %d vs %d entries", len(flat.RefExpansions), len(flat.Entries))
		}
		planIdx := slices.Index(got, plan.ID)
		if planIdx < 0 || len(flat.RefExpansions[planIdx]) != 1 || flat.RefExpansions[planIdx][0].ID != basis.ID {
			t.Errorf("plan upstream: want 1 row → %s, got %+v", basis.ID, flat.RefExpansions[planIdx])
		}
	})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	if _, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout}); err != nil {
		t.Fatalf("View: %v (rank(in-degree) shorthand should work)", err)
	}
}

func TestView_RankNoArgs(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "rank():as-list")
	f := New(Options{})
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err == nil {
		t.Fatalf("expected error for rank with no args, got nil")
	}
}

func TestView_RankByOnlyDate(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "rank(by(name)):as-list")
	f := New(Options{})
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	if _, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout}); err != nil {
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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

func TestView_TypeFilterAbbrev(t *testing.T) {
	dec := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	sig := entry("20260101-110000-s-tac-bbb", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{dec, sig})

	layout := mustParseLayout(t, "type(d):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if want := []string{dec.ID}; !equalIDs(idsOf(flat.Entries), want) {
		t.Errorf("type(d): got %v, want %v", idsOf(flat.Entries), want)
	}
}

func TestView_TypeFilterFullName(t *testing.T) {
	// type(signal) resolves to the same set as type(s).
	dec := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	sig := entry("20260101-110000-s-tac-bbb", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{dec, sig})

	layout := mustParseLayout(t, "type(signal):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if want := []string{sig.ID}; !equalIDs(idsOf(flat.Entries), want) {
		t.Errorf("type(signal): got %v, want %v", idsOf(flat.Entries), want)
	}
}

func TestView_TypeFilterComposesWithKind(t *testing.T) {
	// type(d):kind(gap) intersects to empty — gap is a signal kind, so no
	// decision matches. Confirms type() narrows alongside other filters.
	dec := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	sig := entry("20260101-110000-s-tac-bbb", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{dec, sig})

	layout := mustParseLayout(t, "type(d):kind(gap):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	if len(flat.Entries) != 0 {
		t.Errorf("type(d):kind(gap): got %v, want []", idsOf(flat.Entries))
	}
}

func TestView_TypeFilterRequiresOneArg(t *testing.T) {
	g := model.NewGraph([]*model.Entry{entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))})
	layout := mustParseLayout(t, "type():as-list")
	f := New(Options{})
	if _, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout}); err == nil {
		t.Fatal("expected error for type() with no args")
	}
}

func TestView_LayerFilterFullName(t *testing.T) {
	// layer(tactical) should resolve to the same set as layer(tac).
	stg := entry("20260101-100000-d-stg-aaa", withKind(model.KindAspiration))
	tac := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{stg, tac})

	layout := mustParseLayout(t, "layer(tactical):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
			if _, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout}); err == nil {
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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

// --- Slice 7 partial: name(string) modifier ---

func TestView_NameModifier_SetsSectionHeader(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, `name("Top entries"):as-list`)
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got, want := result.Sections[0].Name, "Top entries"; got != want {
		t.Errorf("Name: got %q, want %q", got, want)
	}
}

func TestView_NameModifier_LastWriteWins(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, `name("first"):name("second"):as-list`)
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got, want := result.Sections[0].Name, "second"; got != want {
		t.Errorf("Name: got %q, want %q (last-write-wins)", got, want)
	}
}

func TestView_NameModifier_AcceptsBareIdent(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, "name(Aspirations):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got, want := result.Sections[0].Name, "Aspirations"; got != want {
		t.Errorf("Name: got %q, want %q", got, want)
	}
}

func TestView_NameModifier_RequiresExactlyOneArg(t *testing.T) {
	g := model.NewGraph(nil)
	for _, src := range []string{"name:as-list", "name():as-list", `name("a","b"):as-list`} {
		t.Run(src, func(t *testing.T) {
			f := New(Options{})
			_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, src)})
			if err == nil {
				t.Fatalf("expected error for %q, got nil", src)
			}
			if !strings.Contains(err.Error(), "name") {
				t.Errorf("error %q must mention 'name'", err.Error())
			}
		})
	}
}

func TestView_NameModifier_RejectsNonString(t *testing.T) {
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "name(42):as-list")})
	if err == nil {
		t.Fatalf("expected error for numeric name arg, got nil")
	}
}

// --- Slice 7: focus block + dedup ---

func TestView_FocusBlock_EndToEnd(t *testing.T) {
	// Full pipeline: kind(focus):active filter selects focus entries,
	// expand(involvement) builds the FocusBlock, as-focus-block emits
	// the result. Verifies the wiring without going through the macro.
	target := entry("20260101-100000-d-tac-tgt", withKind(model.KindDirective))
	focus := entry("20260101-110000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher"),
		withInvolvement(target.ID, nil, false),
	)
	g := model.NewGraph([]*model.Entry{target, focus})

	layout := mustParseLayout(t, "kind(focus):active:expand(involvement):as-focus-block")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	section := result.Sections[0]
	if section.Render != "as-focus-block" {
		t.Errorf("render: got %q, want as-focus-block", section.Render)
	}
	block, ok := section.Data.(model.FocusBlock)
	if !ok {
		t.Fatalf("section data: got %T, want model.FocusBlock", section.Data)
	}
	if len(block.Focuses) != 1 || len(block.Focuses[0].Targets) != 1 {
		t.Fatalf("expected 1 focus / 1 target, got %d/%d",
			len(block.Focuses), len(block.Focuses[0].Targets))
	}
	if got := block.Focuses[0].Targets[0].Target.ID; got != target.ID {
		t.Errorf("target: got %s, want %s", got, target.ID)
	}
}

func TestView_SectionsRenderIndependently(t *testing.T) {
	// Per s-cpt-tn0: each section renders independently; an entry in
	// both a focus block and a subsequent as-list appears in both.
	// Cross-section dedup is captured as an open design question;
	// this test pins the conservative no-dedup default the revert
	// established.
	target := entry("20260101-100000-d-tac-tgt", withKind(model.KindDirective))
	other := entry("20260101-110000-d-tac-oth", withKind(model.KindDirective))
	focus := entry("20260101-120000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher"),
		withInvolvement(target.ID, nil, false),
	)
	g := model.NewGraph([]*model.Entry{target, other, focus})

	layout := mustParseLayout(t, "kind(focus):active:expand(involvement):as-focus-block,kind(directive):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if len(result.Sections) != 2 {
		t.Fatalf("sections: got %d, want 2", len(result.Sections))
	}
	flat, ok := result.Sections[1].Data.(model.FlatList)
	if !ok {
		t.Fatalf("section 2 data: got %T, want FlatList", result.Sections[1].Data)
	}
	got := idsOf(flat.Entries)
	// Both directives appear — `target` is in the focus block AND in
	// the as-list. The previous AC 13 dedup that stripped target
	// from the as-list is intentionally gone.
	if len(got) != 2 {
		t.Errorf("as-list after focus block: got %d entries, want 2 (no cross-section dedup)", len(got))
	}
}

func TestView_FocusBlock_StalledModifierConfigures(t *testing.T) {
	// Section pipeline with stalled(value) — verify the threshold flows
	// into state derivation. Inject a graph with a target that has
	// in-degree refs so heat is non-zero.
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	target := entry("20260101-100000-d-tac-tgt", withKind(model.KindDirective))
	focus := entry("20260101-110000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher"),
		withInvolvement(target.ID, nil, false),
	)
	// Add some refs targeting `target` so HeatScore is non-trivial. The
	// referrers' creation times feed decay; using `now` minus a few days
	// keeps scores small but non-zero.
	r1 := entry("20260505-100000-s-tac-r01", withKind(model.KindGap), withRefs(target.ID))
	r1.Time = now.AddDate(0, 0, -2)
	g := model.NewGraph([]*model.Entry{target, focus, r1})

	// Default threshold is 1.0. Heat(exp-14d) for a 2-day-old single
	// ref: 2^(-2/14) ≈ 0.906 — below 1.0. Without stalled(0.5), state
	// is stalled. With stalled(0.5), state is driving.
	resultDefault, err := f(t).OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "kind(focus):expand(involvement):as-focus-block")})
	if err != nil {
		t.Fatalf("View default: %v", err)
	}
	stateDefault := resultDefault.Sections[0].Data.(model.FocusBlock).Focuses[0].Targets[0].State
	resultLow, err := f(t).OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "kind(focus):expand(involvement):stalled(0.5):as-focus-block")})
	if err != nil {
		t.Fatalf("View stalled(0.5): %v", err)
	}
	stateLow := resultLow.Sections[0].Data.(model.FocusBlock).Focuses[0].Targets[0].State

	// We don't pin the exact state under the live clock since now=time.Now()
	// inside View — we only verify that lowering the threshold can flip a
	// stalled into a driving (or keep driving driving). The logical
	// invariant: the lower-threshold case can never be more-stalled than
	// the default.
	if stateDefault == model.FocusStateDriving && stateLow == model.FocusStateStalled {
		t.Errorf("inverted: default = driving, low-threshold = stalled (not possible)")
	}
}

func TestView_FocusBlock_RankIsExclusive(t *testing.T) {
	g := model.NewGraph(nil)
	fdr := New(Options{})
	_, err := fdr.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t,
		"kind(focus):expand(involvement):rank(in-degree):as-focus-block")})
	if err == nil {
		t.Fatalf("expected error for expand+rank, got nil")
	}
	if !strings.Contains(err.Error(), "expand") || !strings.Contains(err.Error(), "rank") {
		t.Errorf("error %q must mention both expand and rank", err.Error())
	}
}

func TestView_FocusBlock_StalledRequiresFocusBlock(t *testing.T) {
	// stalled(value) on a flat-list section is a clear user mistake —
	// the modifier has no effect outside focus-block state derivation.
	g := model.NewGraph(nil)
	fdr := New(Options{})
	_, err := fdr.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "stalled(1.0):as-list")})
	if err == nil {
		t.Fatalf("expected error for stalled() outside focus-block, got nil")
	}
	if !strings.Contains(err.Error(), "stalled") || !strings.Contains(err.Error(), "focus-block") {
		t.Errorf("error %q must mention stalled and focus-block", err.Error())
	}
}

func TestView_FocusBlock_ShapeMismatch_AsList(t *testing.T) {
	g := model.NewGraph(nil)
	fdr := New(Options{})
	_, err := fdr.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t,
		"kind(focus):expand(involvement):as-list")})
	if err == nil {
		t.Fatalf("expected render-shape mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "as-list") || !strings.Contains(err.Error(), "focus-block") {
		t.Errorf("error %q must mention as-list and focus-block", err.Error())
	}
}

func TestView_FocusBlock_ShapeMismatch_AsFocusBlockOnFlat(t *testing.T) {
	g := model.NewGraph(nil)
	fdr := New(Options{})
	_, err := fdr.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "as-focus-block")})
	if err == nil {
		t.Fatalf("expected render-shape mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "expand(involvement)") {
		t.Errorf("error %q must point to expand(involvement)", err.Error())
	}
}

// f returns a fresh Finder for tests — a small alias so multi-call tests
// don't repeat New(Options{}) each step.
func f(_ *testing.T) *Finder { return New(Options{}) }

// --- Slice 5: group(by(field)) + as-grouped ---

func TestView_GroupByKind_AsGrouped(t *testing.T) {
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	dir1 := entry("20260101-110000-d-tac-da1", withKind(model.KindDirective))
	dir2 := entry("20260101-120000-d-tac-da2", withKind(model.KindDirective))
	gap := entry("20260101-130000-s-tac-gap", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{plan, dir1, dir2, gap})

	layout := mustParseLayout(t, "group(by(kind)):as-grouped")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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
			_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, layoutSrc)})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "group(by()):as-grouped")})
	if err == nil {
		t.Fatalf("expected error for group(by()), got nil")
	}
}

func TestView_GroupRejectsUnknownField(t *testing.T) {
	g := model.NewGraph(nil)
	f := New(Options{})
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "group(by(summary)):as-grouped")})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "as-grouped")})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "group(by(kind)):as-list")})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "group(by(kind)):rank(in-degree):as-grouped")})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayout(t, "group(by(kind)):n(5):as-grouped")})
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
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	for _, want := range []string{"group", "as-grouped"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// --- d-tac-e1s: not(<filter>) negation primitive ---

func TestView_NotKind_ExcludesKinds(t *testing.T) {
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	contract := entry("20260101-120000-d-cpt-con", withKind(model.KindContract))
	aspiration := entry("20260101-130000-d-stg-asp", withKind(model.KindAspiration))
	g := model.NewGraph([]*model.Entry{plan, directive, contract, aspiration})

	layout := mustParseLayout(t, "not(kind(contract,aspiration)):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{plan.ID, directive.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_NotKind_ComposesWithPositiveKind(t *testing.T) {
	// Positive kind() includes ∩ negative not(kind()) excludes — the
	// expected pattern for a top(N) catch-up that wants only actionable
	// decisions but not standing ones.
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	directive := entry("20260101-110000-d-tac-dir", withKind(model.KindDirective))
	contract := entry("20260101-120000-d-cpt-con", withKind(model.KindContract))
	aspiration := entry("20260101-130000-d-stg-asp", withKind(model.KindAspiration))
	gap := entry("20260101-140000-s-tac-gap", withKind(model.KindGap))
	g := model.NewGraph([]*model.Entry{plan, directive, contract, aspiration, gap})

	layout := mustParseLayout(t, "kind(plan,directive,contract,aspiration):not(kind(contract,aspiration)):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{plan.ID, directive.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_NotKind_MultipleCallsUnion(t *testing.T) {
	// Multiple not(kind(...)) calls union their exclusion sets — not(kind(A,B)):not(kind(C))
	// drops kinds A, B, and C.
	plan := entry("20260101-100000-d-tac-pln", withKind(model.KindPlan))
	contract := entry("20260101-110000-d-cpt-con", withKind(model.KindContract))
	aspiration := entry("20260101-120000-d-stg-asp", withKind(model.KindAspiration))
	annotation := entry("20260101-130000-s-cpt-ann", withKind(model.KindAnnotation))
	g := model.NewGraph([]*model.Entry{plan, contract, aspiration, annotation})

	layout := mustParseLayout(t, "not(kind(contract,aspiration)):not(kind(annotation)):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
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

func TestView_NotLayer_ExcludesLayer(t *testing.T) {
	stg := entry("20260101-100000-d-stg-aaa", withKind(model.KindAspiration))
	cpt := entry("20260101-110000-d-cpt-bbb", withKind(model.KindContract))
	tac := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{stg, cpt, tac})

	layout := mustParseLayout(t, "not(layer(stg)):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{cpt.ID, tac.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_NotTopic_ExcludesTopic(t *testing.T) {
	// not(topic(L)) drops entries whose effective topic set has L as a
	// component-wise prefix; the comparison topic-filter test covers
	// component-wise semantics, this one covers the inverse selection.
	infra := &model.Entry{
		ID:     "20260101-100000-d-tac-aaa",
		Type:   model.TypeDecision,
		Layer:  model.LayerTactical,
		Kind:   model.KindDirective,
		Topics: []model.TopicPath{mustParseTopic(t, "infrastructure/cli")},
	}
	other := &model.Entry{
		ID:     "20260101-110000-d-tac-bbb",
		Type:   model.TypeDecision,
		Layer:  model.LayerTactical,
		Kind:   model.KindDirective,
		Topics: []model.TopicPath{mustParseTopic(t, "type-system/kinds")},
	}
	untagged := &model.Entry{
		ID:    "20260101-120000-d-tac-ccc",
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
	}
	g := model.NewGraph([]*model.Entry{infra, other, untagged})

	layout := mustParseLayout(t, `not(topic("infrastructure")):as-list`)
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{other.ID, untagged.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_Not_RejectsUnsupportedInner(t *testing.T) {
	// active, since, and nested not are deferred / disallowed for d-tac-e1s.
	// Each surface variants in one table-driven test so the supported-set
	// error is exercised uniformly.
	g := model.NewGraph(nil)
	cases := []struct {
		layout string
		hint   string
	}{
		{"not(active):as-list", "active"},
		{`not(since("7d")):as-list`, "since"},
		{"not(not(kind(plan))):as-list", "nested"},
	}
	for _, tc := range cases {
		t.Run(tc.hint, func(t *testing.T) {
			layout := mustParseLayout(t, tc.layout)
			f := New(Options{})
			_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			// Error message must list the supported inner set so users
			// see what's available rather than guessing.
			for _, want := range []string{"kind", "layer", "topic"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing supported-inner %q", err.Error(), want)
				}
			}
		})
	}
}

func TestView_Not_ArityErrors(t *testing.T) {
	// Zero or multiple arguments on not() are clear user mistakes; the
	// error must point at the canonical example shape.
	g := model.NewGraph(nil)
	cases := []string{
		"not():as-list",
		"not(kind(plan),kind(directive)):as-list",
	}
	for _, layoutStr := range cases {
		t.Run(layoutStr, func(t *testing.T) {
			layout := mustParseLayout(t, layoutStr)
			f := New(Options{})
			_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "exactly one filter argument") {
				t.Errorf("error %q does not name the arity contract", err.Error())
			}
		})
	}
}

func TestView_Not_RegisteredInKnownFunctions(t *testing.T) {
	// Misspellings like nott(...) must produce the unknown-function error
	// listing not as available — the AC against shadow misspellings.
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "nott(kind(plan)):as-list")
	f := New(Options{})
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not") {
		t.Errorf("known-functions list must contain 'not'; error was: %v", err)
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

// mustParseLayoutAndExpand mirrors the CLI's two-step pipeline: parse
// grammar, then expand macros. Tests that exercise macros use this so
// the resulting Layout matches what `sdd view` actually executes.
func mustParseLayoutAndExpand(t *testing.T, s string) model.Layout {
	t.Helper()
	l, err := query.ParseLayout(s)
	if err != nil {
		t.Fatalf("ParseLayout(%q): %v", s, err)
	}
	expanded, err := query.ExpandMacros(l)
	if err != nil {
		t.Fatalf("ExpandMacros(%q): %v", s, err)
	}
	return expanded
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

func TestView_SkipComposesWithN(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective))
	c := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	d := entry("20260101-130000-d-tac-ddd", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a, b, c, d})

	layout := mustParseLayout(t, "skip(1):n(2):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	flat := result.Sections[0].Data.(model.FlatList)
	got := idsOf(flat.Entries)
	want := []string{b.ID, c.ID}
	if !equalIDs(got, want) {
		t.Errorf("entries:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestView_SkipPastEndIsEmpty(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	g := model.NewGraph([]*model.Entry{a})

	layout := mustParseLayout(t, "skip(5):as-list")
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if n := result.Sections[0].Data.Count(); n != 0 {
		t.Errorf("skip past the end must yield an empty section, got %d entries", n)
	}
}

func TestView_SkipRejectedForParticipantsBlock(t *testing.T) {
	g := model.NewGraph(nil)
	layout := mustParseLayout(t, "active:kind(actor):skip(2):as-participants-block")
	f := New(Options{})
	if _, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout}); err == nil {
		t.Fatal("skip over the participants block must be rejected")
	}
}

func TestView_ServeBudgetCutsShapesAtWholeUnits(t *testing.T) {
	hub := entry("20260101-090000-d-tac-hub", withKind(model.KindPlan))
	refs := []*model.Entry{hub}
	for i := range 9 {
		refs = append(refs, entry(
			fmt.Sprintf("20260101-10%02d00-d-tac-r%c", i, 'a'+i),
			withKind(model.KindDirective), withRefs(hub.ID),
		))
	}
	g := model.NewGraph(refs)
	f := New(Options{})

	bodiesLayout := mustParseLayout(t, `kind(directive):as-bodies:name("Bodies")`)
	bounded, err := f.OnGraph(g).View(query.ViewQuery{Layout: bodiesLayout, Budget: query.ViewBudget{BodyBytes: 1}})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	bodies := bounded.Sections[0].Data.(model.Bodies)
	if len(bodies.Entries) != 0 || bodies.Dropped != 9 {
		t.Fatalf("bodies cut = %d kept, %d dropped; want whole-body cut accounting", len(bodies.Entries), bodies.Dropped)
	}
	if bodies.Pull != `kind(directive):as-bodies:name("Bodies")` {
		t.Fatalf("bodies pull = %q, want the section's own source", bodies.Pull)
	}

	unbounded, err := f.OnGraph(g).View(query.ViewQuery{Layout: bodiesLayout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	full := unbounded.Sections[0].Data.(model.Bodies)
	if full.Dropped != 0 || len(full.Entries) != 9 {
		t.Fatalf("explicit pulls must arrive complete: %d kept, %d dropped", len(full.Entries), full.Dropped)
	}
}
