package finders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestLoadWIPMarkers(t *testing.T) {
	dir := t.TempDir()
	wipDir := filepath.Join(dir, "wip")
	if err := os.MkdirAll(wipDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeWIPMarker(t, dir, "20260409-100000-alice", `---
entry: 20260408-171831-d-cpt-axa
participant: Alice
exclusive: true
---

First task.
`)
	writeWIPMarker(t, dir, "20260409-110000-bob", `---
entry: 20260408-171831-d-cpt-axa
participant: Bob
---

Second task.
`)

	f := New(Options{})
	markers, err := f.LoadWIPMarkers(dir)
	if err != nil {
		t.Fatalf("LoadWIPMarkers: %v", err)
	}

	if len(markers) != 2 {
		t.Fatalf("got %d markers, want 2", len(markers))
	}

	// Should be sorted by time
	if markers[0].ID != "20260409-100000-alice" {
		t.Errorf("first marker = %q, want alice (earlier)", markers[0].ID)
	}
	if markers[1].ID != "20260409-110000-bob" {
		t.Errorf("second marker = %q, want bob (later)", markers[1].ID)
	}
}

func TestLoadWIPMarkersNoDir(t *testing.T) {
	dir := t.TempDir()
	f := New(Options{})
	markers, err := f.LoadWIPMarkers(dir)
	if err != nil {
		t.Fatalf("LoadWIPMarkers: %v", err)
	}
	if len(markers) != 0 {
		t.Errorf("got %d markers, want 0", len(markers))
	}
}

func TestLoadGraphSkipsWIPDir(t *testing.T) {
	dir := t.TempDir()

	// Write a normal graph entry
	writeGraphEntry(t, dir, "20260406-115516-s-stg-beh", `---
type: s
layer: stg
---

A signal.
`)

	// Write a WIP marker (should be ignored by LoadGraph)
	wipDir := filepath.Join(dir, "wip")
	if err := os.MkdirAll(wipDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeWIPMarker(t, dir, "20260409-100000-alice", `---
entry: 20260406-115516-s-stg-beh
participant: Alice
---

Working on it.
`)

	f := New(Options{})
	g, err := f.LoadGraph(dir)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	if len(g.Entries) != 1 {
		t.Errorf("got %d entries, want 1 (WIP marker should be skipped)", len(g.Entries))
	}
}

// writeWIPMarker writes a WIP marker file under graphDir/wip/.
func writeWIPMarker(t *testing.T, graphDir, markerID, content string) {
	t.Helper()
	markerPath := filepath.Join(graphDir, model.WIPMarkerPath(markerID))
	if err := os.MkdirAll(filepath.Dir(markerPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
