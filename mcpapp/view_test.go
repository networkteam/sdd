package mcpapp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	mcpserver "github.com/networkteam/sdd/mcpapp"
)

// TestViewFirstHitHint covers the four (bound × first-view) cells of the
// view-tool breadcrumb (AC8). The first-view pointer is keyed to the
// connection, so two fresh connections realize every cell: an unbound reader
// (the s-prc-3kh cohort) and a bound session.
func TestViewFirstHitHint(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	const doorMark = "start_session is the door"
	const factMark = "view layout grammar"

	// Connection A: never opens a session (unbound).
	unbound := connect(t, env.srv)
	var a1, a2 mcpserver.ViewResult
	call(t, unbound, "view", map[string]any{"layout": "active:as-list"}, &a1)
	call(t, unbound, "view", map[string]any{"layout": "active:as-list"}, &a2)

	// unbound + first view: door AND fact, door first.
	if !strings.Contains(a1.Hint, doorMark) || !strings.Contains(a1.Hint, factMark) {
		t.Errorf("unbound first view should join both breadcrumbs: %q", a1.Hint)
	}
	if !strings.Contains(a1.Hint, basefacts.ViewGrammarFactID) {
		t.Errorf("first-view hint should point at the fact ID: %q", a1.Hint)
	}
	if strings.Index(a1.Hint, doorMark) > strings.Index(a1.Hint, factMark) {
		t.Errorf("door breadcrumb should come first: %q", a1.Hint)
	}
	// unbound + subsequent view: door only.
	if !strings.Contains(a2.Hint, doorMark) || strings.Contains(a2.Hint, factMark) {
		t.Errorf("unbound second view should carry the door breadcrumb only: %q", a2.Hint)
	}

	// Connection B: opens a session (bound).
	bound := connect(t, env.srv)
	openSession(t, bound)
	var b1, b2 mcpserver.ViewResult
	call(t, bound, "view", map[string]any{"layout": "active:as-list"}, &b1)
	call(t, bound, "view", map[string]any{"layout": "active:as-list"}, &b2)

	// bound + first view: fact only.
	if strings.Contains(b1.Hint, doorMark) || !strings.Contains(b1.Hint, factMark) {
		t.Errorf("bound first view should carry the fact breadcrumb only: %q", b1.Hint)
	}
	// bound + subsequent view: no breadcrumb.
	if b2.Hint != "" {
		t.Errorf("bound second view should carry no breadcrumb, got %q", b2.Hint)
	}
}

// TestViewEmptyResultNamesParticipants exercises the AC3 path end to end: a
// participant filter that matches nothing must not return a blank string, and
// because participant() is an exact match, the miss names the participants the
// graph actually knows so a wrong spelling is obvious.
func TestViewEmptyResultNamesParticipants(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/06/02-100000-s-tac-aaa.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	entry := `---
type: signal
layer: tactical
kind: gap
participants:
  - Christopher
summary: A fixture gap authored by Christopher.
---

A fixture gap authored by Christopher.
`
	if err := os.WriteFile(path, []byte(entry), 0644); err != nil {
		t.Fatal(err)
	}

	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)

	var empty mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"layout": "participant(Nobody):as-list"}, &empty)
	if !strings.Contains(empty.Sections, "0 entries matched") {
		t.Errorf("empty participant view missing explicit statement: %q", empty.Sections)
	}
	if !strings.Contains(empty.Sections, "Christopher") {
		t.Errorf("empty participant view should name known participants: %q", empty.Sections)
	}

	var hit mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"layout": "participant(Christopher):as-list"}, &hit)
	if strings.Contains(hit.Sections, "0 entries matched") {
		t.Errorf("matching participant view should not report an empty result: %q", hit.Sections)
	}
	if !strings.Contains(hit.Sections, "s-tac-aaa") {
		t.Errorf("matching participant view should list the entry: %q", hit.Sections)
	}

	// as-counts aggregates by topic and the fixture entry is untagged: it
	// produces zero rows but did match the pipeline. The response must not
	// claim "0 entries matched" (nor name participants as if the filter
	// missed) — the honest render is the presenter's "(no topics)" line.
	var counts mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"layout": "participant(Christopher):as-counts"}, &counts)
	if strings.Contains(counts.Sections, "0 entries matched") {
		t.Errorf("untagged as-counts falsely reported an empty result: %q", counts.Sections)
	}
	if !strings.Contains(counts.Sections, "(no topics)") {
		t.Errorf("untagged as-counts should render the no-topics line: %q", counts.Sections)
	}
}
