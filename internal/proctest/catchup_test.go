package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

// catchupEntries populates the graph so every asserted lane has real content:
// a closed directive with its closing done (Recent done), an active pending
// directive (Active and hot / Open loops), and an open gap (Open and warm).
func catchupEntries() []*model.Entry {
	return []*model.Entry{
		{
			ID: "20260815-090000-d-tac-old", Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
			Summary: "An earlier engine slice awaits closure.",
			Content: "An earlier engine slice awaits closure.",
		},
		{
			ID: "20260816-110000-s-tac-don", Type: model.TypeSignal, Kind: model.KindDone, Layer: model.LayerTactical,
			Summary: "The earlier engine slice landed.",
			Content: "The earlier engine slice landed; its closure is recorded here.",
			Closes:  []string{"20260815-090000-d-tac-old"},
		},
		{
			ID: "20260815-100000-d-tac-nxt", Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
			Summary: "Ship the next engine slice.",
			Content: "Ship the next engine slice.",
		},
		{
			ID: "20260816-120000-s-tac-gap", Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical,
			Summary: "The catch-up lanes need real fixture coverage.",
			Content: "The catch-up lanes need real fixture coverage.",
		},
	}
}

func TestCatchupHappyPath(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(catchupEntries()...))
	session := world.Open(t, "catchup-happy")

	serve := session.Start(t, "catch-up", nil)
	proctest.RequireStep(t, serve, "compose")
	instance := serve.Instance

	// The injected multi-section view carries the real lanes over the real
	// graph — named sections with the fixture entries in them.
	for _, lane := range []string{"## Recent done", "## Active and hot", "## Open loops", "## Open and warm"} {
		if !strings.Contains(serve.Instructions, lane) {
			t.Errorf("compose unit should inject the %q lane, got %q", lane, serve.Instructions)
		}
	}
	for _, content := range []string{"engine slice landed", "Ship the next engine slice", "real fixture coverage", "No active WIP markers."} {
		if !strings.Contains(serve.Instructions, content) {
			t.Errorf("compose lanes should carry the real graph content %q, got %q", content, serve.Instructions)
		}
	}
	if !strings.Contains(serve.Instructions, "explicit recovery choice is available") {
		t.Fatalf("catch-up instructions do not project actionable recovery notices: %q", serve.Instructions)
	}
	if got := strings.Join(serve.Missing, ","); got != "briefing" {
		t.Fatalf("missing = %q, want briefing", got)
	}

	serve = session.Report(t, instance, map[string]any{
		"briefing": "*Current focus: ship the engine.*\n\n**Slice work**\n\n1. Continue (`d-tac-nxt`).\n\n**What do you want to move forward?**",
	})
	proctest.RequireStep(t, serve, "junction")
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("briefing should reach the junction user chooser, got %+v", serve.PendingChooser)
	}

	serve = session.Answer(t, instance, "junction", "pursue",
		map[string]any{"selectedThread": "continue the engine plan"}, "let's continue the engine work")
	proctest.RequireStatus(t, serve, "completed")
}

func TestCatchupConcludeEndsQuiet(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(catchupEntries()...))
	session := world.Open(t, "catchup-conclude")

	serve := session.Start(t, "catch-up", nil)
	instance := serve.Instance
	serve = session.Report(t, instance, map[string]any{
		"briefing": "**Quiet week.**\n\n**What do you want to move forward?**",
	})
	proctest.RequireStep(t, serve, "junction")

	serve = session.Answer(t, instance, "junction", "conclude", nil, "nothing right now")
	proctest.RequireStatus(t, serve, "completed")
}
