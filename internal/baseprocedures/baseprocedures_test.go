package baseprocedures

import (
	"strings"
	"testing"
	"testing/fstest"
)

const validProcedure = `---
type: decision
layer: prc
kind: procedure
canonical: capture
confidence: medium
summary: The capture move.
---

The capture move: assemble, playback, write, verify.
`

const nonProcedure = `---
type: decision
layer: tac
kind: directive
intent: pending
confidence: medium
---

Not a procedure.
`

func TestLoad_ParsesAndMarksEmbedded(t *testing.T) {
	fsys := fstest.MapFS{
		"entries/README.md":                    {Data: []byte("# docs\n")},
		"entries/20260702-120000-d-prc-cap.md": {Data: []byte(validProcedure)},
	}

	entries, err := load(fsys)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (README skipped)", len(entries))
	}
	e := entries[0]
	if e.ID != "20260702-120000-d-prc-cap" {
		t.Errorf("id = %q", e.ID)
	}
	if !e.Embedded {
		t.Error("embedded flag not set")
	}
	if !e.IsProcedure() {
		t.Errorf("kind = %s, want procedure", e.Kind)
	}
	if e.Canonical != "capture" {
		t.Errorf("canonical = %q, want capture", e.Canonical)
	}
	if e.Summary == "" {
		t.Error("summary not carried from frontmatter")
	}
}

func TestLoad_RejectsNonProcedureEntry(t *testing.T) {
	fsys := fstest.MapFS{
		"entries/20260702-120000-d-tac-xyz.md": {Data: []byte(nonProcedure)},
	}

	_, err := load(fsys)
	if err == nil || !strings.Contains(err.Error(), "only kind: procedure") {
		t.Fatalf("expected non-procedure rejection, got %v", err)
	}
}

func TestLoad_RejectsUnparseableFilename(t *testing.T) {
	fsys := fstest.MapFS{
		"entries/notes.md": {Data: []byte(validProcedure)},
	}

	_, err := load(fsys)
	if err == nil {
		t.Fatal("expected error for non-ID filename")
	}
}

func TestEntries_EmbeddedSetLoads(t *testing.T) {
	// The real embedded set must always load cleanly — a failure here is a
	// broken build. The set may be empty (the capture procedure ships in a
	// later slice); every entry present must be a marked procedure.
	entries, err := Entries()
	if err != nil {
		t.Fatalf("embedded base procedures failed to load: %v", err)
	}
	for _, e := range entries {
		if !e.Embedded || !e.IsProcedure() {
			t.Errorf("embedded entry %s: Embedded=%v kind=%s", e.ID, e.Embedded, e.Kind)
		}
	}
}
