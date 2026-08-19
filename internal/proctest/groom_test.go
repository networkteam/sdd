package proctest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/proctest"
)

const (
	groomGapID    = "20260601-131500-s-tac-gap"
	groomMarkerID = "20260706-114441-christopher"
)

func groomGapEntry() proctest.Entry {
	return proctest.Entry{
		ID: groomGapID, Type: "signal", Kind: "gap", Layer: "tactical",
		Summary: "A resolved gap still deriving as open.",
		Body:    "A resolved gap still deriving as open.",
	}
}

// writeWIPMarker writes a real marker file into the graph dir's wip/
// directory, as sdd's own wip-start does. File-local helper: the harness has
// no marker fixture support, and groom's stale-marker path needs one that no
// live instance holds.
func writeWIPMarker(t *testing.T, graphDir, markerID, entryID string) string {
	t.Helper()
	path := filepath.Join(graphDir, "wip", markerID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nentry: " + entryID + "\nparticipant: Christopher\nexclusive: true\n---\n\nStale work from a long-gone session.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGroomSweepThenWalk(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(groomGapEntry()))
	writeWIPMarker(t, world.GraphDir, groomMarkerID, groomGapID)
	session := world.Open(t, "groom-sweep")

	serve := session.Start(t, "groom", nil)
	proctest.RequireStep(t, serve, "sweep")
	instance := serve.Instance
	if !strings.Contains(serve.Instructions, groomMarkerID) {
		t.Errorf("sweep unit should serve the live WIP lane with the marker, got %q", serve.Instructions)
	}

	serve = session.Report(t, instance, map[string]any{
		"candidates": "1. " + groomGapID + " resolved but open — downstream work covers it; propose closing done",
	})
	proctest.RequireStep(t, serve, "walk")
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("a reported table should reach the walk user chooser, got %+v", serve.PendingChooser)
	}

	// One cleanup loops back for the next candidate.
	serve = session.Answer(t, instance, "walk", "cleanup",
		map[string]any{"selectedCleanup": "close candidate 1 with a done"}, "yes, close it")
	proctest.RequireStep(t, serve, "walk")

	serve = session.Answer(t, instance, "walk", "conclude", nil, "that's all")
	proctest.RequireStatus(t, serve, "completed")
}

func TestGroomFocusHintRendered(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(groomGapEntry()))
	session := world.Open(t, "groom-focus-hint")

	serve := session.Start(t, "groom", map[string]any{"focusHint": "the type-system cluster"})
	proctest.RequireStep(t, serve, "sweep")
	if !strings.Contains(serve.Instructions, "the type-system cluster") {
		t.Errorf("sweep should render the focus hint verbatim, got %q", serve.Instructions)
	}
}

// The cleanup option's declared capture handoff, exercised behaviorally: the
// answered dispatch seeds the sweep's candidate table into the capture child
// as its widenReport, and the child is the real capture procedure — the
// closing done passes the real construction boundary and retires the
// candidate on disk. Replaces the old spec-level Dispatch inspection.
func TestGroomCleanupDispatchesRealCapture(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(groomGapEntry()))
	session := world.Open(t, "groom-cleanup-capture")

	serve := session.Start(t, "groom", nil)
	instance := serve.Instance
	candidates := "1. " + groomGapID + " resolved but open — downstream work covers it; propose closing done"
	serve = session.Report(t, instance, map[string]any{"candidates": candidates})
	proctest.RequireStep(t, serve, "walk")
	serve = session.Answer(t, instance, "walk", "cleanup",
		map[string]any{"selectedCleanup": "close candidate 1 with a done"}, "yes, close it")
	proctest.RequireStep(t, serve, "walk")

	capture := startChild(t, session, "capture", instance, nil)
	proctest.RequireStep(t, capture, "assemble")
	if missingContains(capture, "widenReport") {
		t.Fatalf("widenReport should be seeded from the sweep's candidates, missing %v", capture.Missing)
	}
	if !strings.Contains(capture.Instructions, candidates) {
		t.Errorf("assemble should render the seeded candidate table, got %q", capture.Instructions)
	}

	// The closes target must have been served in full to this session.
	session.LogRead(t, "show", []string{groomGapID}, nil)
	serve = session.Report(t, capture.Instance, map[string]any{
		"body":       "The gap is retired because the downstream work already covers it; grooming records the closure the graph implied.",
		"entryKind":  "done",
		"layer":      "tactical",
		"closes":     []any{groomGapID},
		"topics":     []any{"grooming"},
		"confidence": "medium",
	})
	proctest.RequireStep(t, serve, "playback")
	serve = session.Answer(t, capture.Instance, "playback", "confirm", nil, "close it")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, capture.Instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")

	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("cleanup capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if len(entry.Closes) != 1 || entry.Closes[0] != groomGapID {
		t.Errorf("persisted closes = %v, want the groomed gap", entry.Closes)
	}

	serve = session.Answer(t, instance, "walk", "conclude", nil, "that's all")
	proctest.RequireStatus(t, serve, "completed")
}

// A stale WIP marker is cleared through the wipRemove transition, not a
// capture: the agent supplies the marker ID as staleMarker, the real engine
// removes the marker file from the graph dir, and the walk loops back.
func TestGroomRemoveMarkerClearsViaTransition(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(groomGapEntry()))
	markerPath := writeWIPMarker(t, world.GraphDir, groomMarkerID, groomGapID)
	session := world.Open(t, "groom-remove-marker")

	serve := session.Start(t, "groom", nil)
	instance := serve.Instance
	serve = session.Report(t, instance, map[string]any{
		"candidates": "1. stale WIP marker " + groomMarkerID + " on " + groomGapID + " — closed; propose removal",
	})
	proctest.RequireStep(t, serve, "walk")

	serve = session.Answer(t, instance, "walk", "removeMarker",
		map[string]any{"staleMarker": groomMarkerID}, "yes, clear it")
	proctest.RequireStep(t, serve, "walk")

	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("wipRemove should have removed the marker file, stat err = %v", err)
	}

	serve = session.Answer(t, instance, "walk", "conclude", nil, "that's all")
	proctest.RequireStatus(t, serve, "completed")
}
