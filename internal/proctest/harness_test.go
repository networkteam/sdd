package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/proctest"
)

const smokeRefID = "20260601-120000-d-tac-ref"

// The harness smoke: a world opens over real stores, a capture runs the full
// real path — assemble, guide, playback, write, summary — and the entry lands
// on disk. Procedure behavior beyond this lives in the per-procedure suites.
func TestHarnessCaptureRunsEndToEnd(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(proctest.Entry{
		ID: smokeRefID, Type: "decision", Kind: "directive", Layer: "tactical", Intent: "pending",
		Summary: "A directive the smoke capture refs.", Body: "A directive the smoke capture refs.",
	}))
	session := world.Open(t, "harness-smoke")

	serve := session.Start(t, "capture", nil)
	proctest.RequireStep(t, serve, "assemble")
	instance := serve.Instance

	session.LogRead(t, "show", []string{smokeRefID}, nil)
	serve = session.Report(t, instance, map[string]any{
		"body":        "A tactical gap: the smoke fixture observes something.",
		"entryKind":   "gap",
		"layer":       "tactical",
		"refs":        []any{map[string]any{"id": smokeRefID, "kind": "addresses"}},
		"topics":      []any{"testing/fixture"},
		"confidence":  "medium",
		"widenReport": "smoke: inspected the fixture directive in full",
	})
	proctest.RequireStep(t, serve, "playback")

	serve = session.Answer(t, instance, "playback", "confirm", nil, "capture it")
	proctest.RequireStep(t, serve, "verifySummary")
	if world.LLM.Calls("writing-guide") != 1 {
		t.Fatalf("writing guide ran %d times, want 1", world.LLM.Calls("writing-guide"))
	}

	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if !strings.Contains(entry.Content, "smoke fixture observes") {
		t.Fatalf("persisted entry body = %q", entry.Content)
	}
}
