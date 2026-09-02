package mcpapp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
)

// TestViewFirstHitHint covers the view-tool breadcrumb (AC8): the first view
// call on a connection carries the pointer to the view-grammar fact, later
// calls carry nothing. The pointer is keyed to the connection, so a second
// connection pays it again.
func TestViewFirstHitHint(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	const factMark = "view layout grammar"

	for _, name := range []string{"first connection", "second connection"} {
		cs := connect(t, env.srv)
		session := openSession(t, cs).Session
		var v1, v2 mcpserver.ViewResult
		call(t, cs, "view", map[string]any{"session": session, "layout": "active:as-list"}, &v1)
		call(t, cs, "view", map[string]any{"session": session, "layout": "active:as-list"}, &v2)

		if !strings.Contains(v1.Hint, factMark) || !strings.Contains(v1.Hint, basefacts.ViewGrammarFactID) {
			t.Errorf("%s: first view should carry the fact breadcrumb naming the fact ID: %q", name, v1.Hint)
		}
		if v2.Hint != "" {
			t.Errorf("%s: second view should carry no breadcrumb, got %q", name, v2.Hint)
		}
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
	session := openSession(t, cs).Session

	var empty mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"session": session, "layout": "participant(Nobody):as-list"}, &empty)
	if !strings.Contains(empty.Sections, "0 entries matched") {
		t.Errorf("empty participant view missing explicit statement: %q", empty.Sections)
	}
	if !strings.Contains(empty.Sections, "Christopher") {
		t.Errorf("empty participant view should name known participants: %q", empty.Sections)
	}

	var hit mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"session": session, "layout": "participant(Christopher):as-list"}, &hit)
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
	call(t, cs, "view", map[string]any{"session": session, "layout": "participant(Christopher):as-counts"}, &counts)
	if strings.Contains(counts.Sections, "0 entries matched") {
		t.Errorf("untagged as-counts falsely reported an empty result: %q", counts.Sections)
	}
	if !strings.Contains(counts.Sections, "(no topics)") {
		t.Errorf("untagged as-counts should render the no-topics line: %q", counts.Sections)
	}
}
