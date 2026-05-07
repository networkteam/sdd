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
