package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWIPMarker(t *testing.T) {
	content := `---
entry: 20260408-171831-d-cpt-axa
participant: Christopher
exclusive: true
---

Prototyping widget layout for dashboard.
`
	m, err := ParseWIPMarker("20260409-103000-christopher.md", content)
	if err != nil {
		t.Fatalf("ParseWIPMarker: %v", err)
	}

	if m.ID != "20260409-103000-christopher" {
		t.Errorf("ID = %q, want 20260409-103000-christopher", m.ID)
	}
	if m.Entry != "20260408-171831-d-cpt-axa" {
		t.Errorf("Entry = %q, want 20260408-171831-d-cpt-axa", m.Entry)
	}
	if m.Participant != "Christopher" {
		t.Errorf("Participant = %q, want Christopher", m.Participant)
	}
	if !m.Exclusive {
		t.Error("Exclusive = false, want true")
	}
	if m.Content != "Prototyping widget layout for dashboard." {
		t.Errorf("Content = %q, want 'Prototyping widget layout for dashboard.'", m.Content)
	}
	if m.Time.Format("20060102-150405") != "20260409-103000" {
		t.Errorf("Time = %v, want 20260409-103000", m.Time)
	}
}

func TestParseWIPMarkerNonExclusive(t *testing.T) {
	content := `---
entry: 20260408-171831-d-cpt-axa
participant: Alice
---

Reviewing the decision.
`
	m, err := ParseWIPMarker("20260409-110000-alice.md", content)
	if err != nil {
		t.Fatalf("ParseWIPMarker: %v", err)
	}

	if m.Exclusive {
		t.Error("Exclusive = true, want false")
	}
	if m.Participant != "Alice" {
		t.Errorf("Participant = %q, want Alice", m.Participant)
	}
}

func TestParseWIPMarkerMissingEntry(t *testing.T) {
	content := `---
participant: Christopher
---

Some work.
`
	_, err := ParseWIPMarker("20260409-103000-christopher.md", content)
	if err == nil {
		t.Fatal("expected error for missing entry field")
	}
}

func TestParseWIPMarkerMissingParticipant(t *testing.T) {
	content := `---
entry: 20260408-171831-d-cpt-axa
---

Some work.
`
	_, err := ParseWIPMarker("20260409-103000-christopher.md", content)
	if err == nil {
		t.Fatal("expected error for missing participant field")
	}
}

func TestParseWIPMarkerInvalidEntryID(t *testing.T) {
	content := `---
entry: not-a-valid-id
participant: Christopher
---

Some work.
`
	_, err := ParseWIPMarker("20260409-103000-christopher.md", content)
	if err == nil {
		t.Fatal("expected error for invalid entry ID")
	}
}

func TestParseWIPMarkerShortID(t *testing.T) {
	_, err := ParseWIPMarker("short.md", "---\nentry: x\n---\n")
	if err == nil {
		t.Fatal("expected error for short marker ID")
	}
}

func TestFormatWIPMarker(t *testing.T) {
	m := &WIPMarker{
		ID:          "20260409-103000-christopher",
		Entry:       "20260408-171831-d-cpt-axa",
		Participant: "Christopher",
		Exclusive:   true,
		Content:     "Prototyping widget layout.",
	}

	got := FormatWIPMarker(m)

	// Should be parseable back
	parsed, err := ParseWIPMarker(m.ID+".md", got)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Entry != m.Entry {
		t.Errorf("Entry = %q, want %q", parsed.Entry, m.Entry)
	}
	if parsed.Participant != m.Participant {
		t.Errorf("Participant = %q, want %q", parsed.Participant, m.Participant)
	}
	if parsed.Exclusive != m.Exclusive {
		t.Errorf("Exclusive = %v, want %v", parsed.Exclusive, m.Exclusive)
	}
	if parsed.Content != m.Content {
		t.Errorf("Content = %q, want %q", parsed.Content, m.Content)
	}
}

func TestFormatWIPMarkerNoContent(t *testing.T) {
	m := &WIPMarker{
		Entry:       "20260408-171831-d-cpt-axa",
		Participant: "Alice",
	}

	got := FormatWIPMarker(m)
	parsed, err := ParseWIPMarker("20260409-110000-alice.md", got)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Content != "" {
		t.Errorf("Content = %q, want empty", parsed.Content)
	}
	if parsed.Exclusive {
		t.Error("Exclusive = true, want false")
	}
}

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

	markers, err := LoadWIPMarkers(dir)
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
	markers, err := LoadWIPMarkers(dir)
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

	g, err := LoadGraph(dir)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	if len(g.Entries) != 1 {
		t.Errorf("got %d entries, want 1 (WIP marker should be skipped)", len(g.Entries))
	}
}

func TestMarkersForEntry(t *testing.T) {
	markers := []*WIPMarker{
		{ID: "20260409-100000-alice", Entry: "20260408-171831-d-cpt-axa"},
		{ID: "20260409-110000-bob", Entry: "20260408-171831-d-cpt-axa"},
		{ID: "20260409-120000-carol", Entry: "20260409-095950-d-prc-jzp"},
	}

	got := MarkersForEntry(markers, "20260408-171831-d-cpt-axa")
	if len(got) != 2 {
		t.Errorf("got %d markers for axa, want 2", len(got))
	}

	got = MarkersForEntry(markers, "nonexistent")
	if len(got) != 0 {
		t.Errorf("got %d markers for nonexistent, want 0", len(got))
	}
}

func TestHasExclusiveMarker(t *testing.T) {
	markers := []*WIPMarker{
		{ID: "20260409-100000-alice", Entry: "20260408-171831-d-cpt-axa", Exclusive: true},
		{ID: "20260409-110000-bob", Entry: "20260408-171831-d-cpt-axa", Exclusive: false},
		{ID: "20260409-120000-carol", Entry: "20260409-095950-d-prc-jzp", Exclusive: false},
	}

	m, ok := HasExclusiveMarker(markers, "20260408-171831-d-cpt-axa")
	if !ok {
		t.Fatal("expected exclusive marker for axa")
	}
	if m.ID != "20260409-100000-alice" {
		t.Errorf("exclusive marker = %q, want alice", m.ID)
	}

	_, ok = HasExclusiveMarker(markers, "20260409-095950-d-prc-jzp")
	if ok {
		t.Error("expected no exclusive marker for jzp")
	}
}

func TestWIPMarkerShortContent(t *testing.T) {
	m := &WIPMarker{Content: "A short description"}
	if got := m.ShortContent(100); got != "A short description" {
		t.Errorf("ShortContent(100) = %q", got)
	}

	m.Content = "First line\nSecond line"
	if got := m.ShortContent(100); got != "First line" {
		t.Errorf("ShortContent multiline = %q, want 'First line'", got)
	}

	m.Content = "A very long description that exceeds the limit"
	if got := m.ShortContent(20); got != "A very long desc ..." {
		t.Errorf("ShortContent(20) = %q", got)
	}
}

func TestParseWIPMarkerWithBranch(t *testing.T) {
	content := `---
entry: 20260411-115135-d-tac-3xw
participant: Christopher
exclusive: true
branch: sdd/3xw-branching-implementation
---

Implementing branching support.
`
	m, err := ParseWIPMarker("20260411-193515-christopher.md", content)
	if err != nil {
		t.Fatalf("ParseWIPMarker: %v", err)
	}

	if m.Branch != "sdd/3xw-branching-implementation" {
		t.Errorf("Branch = %q, want sdd/3xw-branching-implementation", m.Branch)
	}
}

func TestFormatWIPMarkerWithBranch(t *testing.T) {
	m := &WIPMarker{
		ID:          "20260411-193515-christopher",
		Entry:       "20260411-115135-d-tac-3xw",
		Participant: "Christopher",
		Exclusive:   true,
		Branch:      "sdd/3xw-branching-implementation",
		Content:     "Implementing branching support.",
	}

	got := FormatWIPMarker(m)
	parsed, err := ParseWIPMarker(m.ID+".md", got)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Branch != m.Branch {
		t.Errorf("Branch = %q, want %q", parsed.Branch, m.Branch)
	}
}

func TestFormatWIPMarkerWithoutBranch(t *testing.T) {
	m := &WIPMarker{
		ID:          "20260409-103000-christopher",
		Entry:       "20260408-171831-d-cpt-axa",
		Participant: "Christopher",
		Exclusive:   true,
		Content:     "No branch.",
	}

	got := FormatWIPMarker(m)
	if strings.Contains(got, "branch:") {
		t.Errorf("expected no branch field in output, got:\n%s", got)
	}
	parsed, err := ParseWIPMarker(m.ID+".md", got)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Branch != "" {
		t.Errorf("Branch = %q, want empty", parsed.Branch)
	}
}

func TestDeriveBranchName(t *testing.T) {
	tests := []struct {
		entryID     string
		description string
		want        string
	}{
		{"20260411-115135-d-tac-3xw", "Branching implementation", "sdd/3xw-branching-implementation"},
		{"20260410-211553-d-tac-s6w", "Pre-flight validator", "sdd/s6w-pre-flight-validator"},
		{"20260410-211553-d-tac-s6w", "", "sdd/s6w"},
		{"20260410-211553-d-tac-s6w", "  Spaces & special! chars  ", "sdd/s6w-spaces-special-chars"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := DeriveBranchName(tt.entryID, tt.description)
			if got != tt.want {
				t.Errorf("DeriveBranchName(%q, %q) = %q, want %q", tt.entryID, tt.description, got, tt.want)
			}
		})
	}
}

func TestDeriveWorktreePath(t *testing.T) {
	got := DeriveWorktreePath("/Users/hlubek/Dev/AI/Claude/resonance", "sdd/3xw-branching")
	want := "/Users/hlubek/Dev/AI/Claude/sdd-3xw-branching"
	if got != want {
		t.Errorf("DeriveWorktreePath = %q, want %q", got, want)
	}
}

// --- test helpers ---

func writeWIPMarker(t *testing.T, graphDir, markerID, content string) {
	t.Helper()
	markerPath := filepath.Join(graphDir, WIPMarkerPath(markerID))
	if err := os.MkdirAll(filepath.Dir(markerPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
