package sdd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEntry(t *testing.T) {
	content := `---
type: decision
layer: strategic
refs:
  - 20260406-115516-s-stg-beh
participants:
  - Christopher
confidence: medium
---

Explore a novel process framework.`

	e, err := ParseEntry("20260406-115540-d-stg-0gh.md", content)
	if err != nil {
		t.Fatal(err)
	}

	if e.ID != "20260406-115540-d-stg-0gh" {
		t.Errorf("ID = %q, want %q", e.ID, "20260406-115540-d-stg-0gh")
	}
	if e.Type != TypeDecision {
		t.Errorf("Type = %q, want %q", e.Type, TypeDecision)
	}
	if e.Layer != LayerStrategic {
		t.Errorf("Layer = %q, want %q", e.Layer, LayerStrategic)
	}
	if len(e.Refs) != 1 || e.Refs[0] != "20260406-115516-s-stg-beh" {
		t.Errorf("Refs = %v, want [20260406-115516-s-stg-beh]", e.Refs)
	}
	if e.Confidence != "medium" {
		t.Errorf("Confidence = %q, want %q", e.Confidence, "medium")
	}
	if e.Content != "Explore a novel process framework." {
		t.Errorf("Content = %q, want %q", e.Content, "Explore a novel process framework.")
	}
	if e.Time.IsZero() {
		t.Error("Time should not be zero")
	}
}

func TestParseEntrySignal(t *testing.T) {
	content := `---
type: signal
layer: strategic
participants:
  - Christopher
---

Current practices produce overhead.`

	e, err := ParseEntry("20260406-115516-s-stg-beh.md", content)
	if err != nil {
		t.Fatal(err)
	}

	if e.Type != TypeSignal {
		t.Errorf("Type = %q, want %q", e.Type, TypeSignal)
	}
	if e.Confidence != "" {
		t.Errorf("Confidence = %q, want empty", e.Confidence)
	}
}

func TestLoadGraph(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "20260406-115516-s-stg-beh.md", `---
type: signal
layer: strategic
---

Signal one.`)

	writeFile(t, dir, "20260406-115540-d-stg-0gh.md", `---
type: decision
layer: strategic
refs:
  - 20260406-115516-s-stg-beh
---

Decision referencing signal.`)

	writeFile(t, dir, "20260406-115559-a-cpt-f8v.md", `---
type: action
layer: conceptual
refs:
  - 20260406-115540-d-stg-0gh
---

Action referencing decision.`)

	g, err := LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.Entries) != 3 {
		t.Fatalf("Entries = %d, want 3", len(g.Entries))
	}

	// Signal is referenced by decision, so it's not open
	open := g.OpenSignals()
	if len(open) != 0 {
		t.Errorf("OpenSignals = %d, want 0", len(open))
	}

	// Decision is referenced by action (not a decision), so it's active
	active := g.ActiveDecisions()
	if len(active) != 1 {
		t.Errorf("ActiveDecisions = %d, want 1", len(active))
	}

	// Recent actions
	actions := g.RecentActions(10)
	if len(actions) != 1 {
		t.Errorf("RecentActions = %d, want 1", len(actions))
	}

	// Ref chain from action should include all three
	chain := g.RefChain("20260406-115559-a-cpt-f8v")
	if len(chain) != 3 {
		t.Errorf("RefChain = %d, want 3", len(chain))
	}
}

func TestActiveDecisionsSuperseded(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "20260406-100000-d-tac-aaa.md", `---
type: decision
layer: tactical
---

Original decision.`)

	writeFile(t, dir, "20260406-110000-d-tac-bbb.md", `---
type: decision
layer: tactical
supersedes:
  - 20260406-100000-d-tac-aaa
---

Superseding decision.`)

	g, err := LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	active := g.ActiveDecisions()
	if len(active) != 1 {
		t.Fatalf("ActiveDecisions = %d, want 1", len(active))
	}
	if active[0].ID != "20260406-110000-d-tac-bbb" {
		t.Errorf("Active decision = %q, want the superseding one", active[0].ID)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
