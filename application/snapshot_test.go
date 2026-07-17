package application

import (
	"testing"
	"testing/fstest"
)

const snapshotEntry = `---
type: signal
kind: gap
layer: tactical
confidence: high
participants:
  - Christopher
topics:
  - portability/mcp
---

The public snapshot fixture has no filesystem dependency.`

func TestBuildSnapshotAndFilesystemLoaderConverge(t *testing.T) {
	document, err := parseEntryDocument("2026/07/13-010000-s-tac-api.md", []byte(snapshotEntry))
	if err != nil {
		t.Fatal(err)
	}
	structured, err := BuildSnapshot(t.Context(), SnapshotData{
		Project:  "example",
		Revision: "r1",
		Entries:  []EntryDocument{document},
	})
	if err != nil {
		t.Fatal(err)
	}
	filesystem, err := LoadSnapshotFS(t.Context(), "example", "r1", fstest.MapFS{
		"graph/2026/07/13-010000-s-tac-api.md": {Data: []byte(snapshotEntry)},
	}, "graph")
	if err != nil {
		t.Fatal(err)
	}
	const id = "20260713-010000-s-tac-api"
	if structured.graph.ByID[id] == nil || filesystem.graph.ByID[id] == nil {
		t.Fatalf("fixture missing: structured=%v filesystem=%v", structured.graph.ByID[id], filesystem.graph.ByID[id])
	}
	if len(structured.graph.ByID) != len(filesystem.graph.ByID) {
		t.Fatalf("construction paths diverged: structured=%d filesystem=%d", len(structured.graph.ByID), len(filesystem.graph.ByID))
	}
}

// TestBuildSnapshotMergesBaseFact locks the base-facts wiring on the
// application load path (AC7): a snapshot built with no on-disk facts still
// contains the embedded view-grammar fact, marked Embedded.
func TestBuildSnapshotMergesBaseFact(t *testing.T) {
	snapshot, err := BuildSnapshot(t.Context(), SnapshotData{Project: "example", Revision: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	const factID = "20260717-110000-s-prc-vwg"
	fact := snapshot.graph.ByID[factID]
	if fact == nil {
		t.Fatalf("snapshot graph missing base fact %s", factID)
	}
	if !fact.Embedded {
		t.Error("base fact is not marked Embedded")
	}
}

func TestBuildSnapshotRejectsInvalidCanonicalDocument(t *testing.T) {
	_, err := BuildSnapshot(t.Context(), SnapshotData{
		Project:  "example",
		Revision: "r1",
		Entries: []EntryDocument{{
			LogicalPath: "not-canonical.md",
			Frontmatter: map[string]any{"type": "signal", "layer": "tactical"},
		}},
	})
	if err == nil {
		t.Fatal("BuildSnapshot accepted a non-canonical logical path")
	}
}
