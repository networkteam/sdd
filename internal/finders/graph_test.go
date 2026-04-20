package finders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestLoadGraph(t *testing.T) {
	dir := t.TempDir()

	writeGraphEntry(t, dir, "20260406-115516-s-stg-beh", `---
type: signal
layer: strategic
---

Signal one.`)

	writeGraphEntry(t, dir, "20260406-115540-d-stg-0gh", `---
type: decision
layer: strategic
kind: directive
refs:
  - 20260406-115516-s-stg-beh
closes:
  - 20260406-115516-s-stg-beh
---

Decision closing signal.`)

	writeGraphEntry(t, dir, "20260406-115559-s-cpt-f8v", `---
type: signal
layer: conceptual
kind: done
refs:
  - 20260406-115540-d-stg-0gh
closes:
  - 20260406-115540-d-stg-0gh
---

Done signal closing decision.`)

	f := New(nil)
	g, err := f.LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.Entries) != 3 {
		t.Fatalf("Entries = %d, want 3", len(g.Entries))
	}

	// Signal is closed by decision
	open := g.OpenSignals()
	if len(open) != 0 {
		t.Errorf("OpenSignals = %d, want 0", len(open))
	}

	// Decision is closed by done signal
	directives := g.Directives()
	if len(directives) != 0 {
		t.Errorf("Directives = %d, want 0", len(directives))
	}

	// Recent done signals
	done := g.RecentDone(10)
	if len(done) != 1 {
		t.Errorf("RecentDone = %d, want 1", len(done))
	}

	// Ref chain from done signal should include all three
	chain := g.RefChain("20260406-115559-s-cpt-f8v")
	if len(chain) != 3 {
		t.Errorf("RefChain = %d, want 3", len(chain))
	}
}

func TestLoadGraphWithAttachments(t *testing.T) {
	dir := t.TempDir()

	writeGraphEntry(t, dir, "20260406-115516-s-stg-beh", `---
type: signal
layer: strategic
---

See [design](./06-115516-s-stg-beh/design.md) for details.`)

	// Create attachment directory and file
	attachDir := filepath.Join(dir, "2026", "04", "06-115516-s-stg-beh")
	if err := os.MkdirAll(attachDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachDir, "design.md"), []byte("# Design"), 0644); err != nil {
		t.Fatal(err)
	}

	f := New(nil)
	g, err := f.LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(g.Entries))
	}
	e := g.Entries[0]
	if len(e.Attachments) != 1 {
		t.Fatalf("Attachments = %v, want 1 entry", e.Attachments)
	}
	wantPath := filepath.ToSlash(e.Attachments[0])
	if wantPath != "2026/04/06-115516-s-stg-beh/design.md" {
		t.Errorf("Attachments[0] = %q, want %q", wantPath, "2026/04/06-115516-s-stg-beh/design.md")
	}
}

// writeGraphEntry writes a graph entry file in the hierarchical YYYY/MM/ layout.
// The id is the full entry ID (e.g. "20260406-115516-s-stg-beh").
func writeGraphEntry(t *testing.T, dir, id, content string) {
	t.Helper()
	relPath, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatalf("IDToRelPath(%q): %v", id, err)
	}
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
