package mcpapp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpserver "github.com/networkteam/sdd/mcpapp"
)

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
}
