package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/baseprocedures"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

const (
	exploreTargetID   = "20260601-150000-d-tac-tg1"
	exploreNeighborID = "20260601-160000-s-tac-tg2"
	// exploreMissingID is well-formed but absent from the fixture graph.
	exploreMissingID = "20260601-170000-d-tac-gon"

	exploreTargetBody   = "A directive the exploration targets."
	exploreNeighborBody = "A connected gap explored alongside."
)

func exploreWorld(t *testing.T) *proctest.World {
	t.Helper()
	return proctest.NewWorld(t, proctest.WithEntries(
		&model.Entry{
			ID: exploreTargetID, Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
			Summary: exploreTargetBody, Content: exploreTargetBody,
		},
		&model.Entry{
			ID: exploreNeighborID, Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical,
			Summary: exploreNeighborBody, Content: exploreNeighborBody,
		},
	))
}

func exploreParams() map[string]any {
	return map[string]any{
		"targets": []any{exploreTargetID, exploreNeighborID},
		"goal":    "overview: how do these connect",
	}
}

func TestExplore_HappyPath(t *testing.T) {
	world := exploreWorld(t)
	session := world.Open(t, "explore-happy")

	serve := session.Start(t, "explore", exploreParams())
	proctest.RequireStep(t, serve, "inspect")
	for _, want := range []string{exploreTargetID, exploreTargetBody, exploreNeighborID, exploreNeighborBody} {
		if !strings.Contains(serve.Instructions, want) {
			t.Errorf("inspect unit should serve the target chains in full, missing %q in %q", want, serve.Instructions)
		}
	}
	if !strings.Contains(serve.Instructions, "overview: how do these connect") {
		t.Errorf("inspect unit should render the goal verbatim, got %q", serve.Instructions)
	}
	instance := serve.Instance

	serve = session.Report(t, instance, map[string]any{
		"widenReport":  "angle 1: goal phrase; angle 2: target concept — one neighbor kept",
		"inspectedIds": []any{exploreTargetID, exploreNeighborID},
	})
	proctest.RequireStep(t, serve, "compress")
	if !strings.Contains(serve.Instructions, "## Goal") {
		t.Errorf("compress unit should carry the briefing structure, got %q", serve.Instructions)
	}

	serve = session.Report(t, instance, map[string]any{
		"briefing": "## Goal\noverview: how do these connect\n## Targets\n…",
	})
	proctest.RequireStatus(t, serve, "completed")
}

func TestExplore_ParamsRequiredAtStart(t *testing.T) {
	world := exploreWorld(t)
	session := world.Open(t, "explore-params")

	if _, err := session.StartErr(t, "explore", map[string]any{"targets": []any{exploreTargetID}}); err == nil ||
		!strings.Contains(err.Error(), "goal") {
		t.Errorf("start without goal must fail naming it, got %v", err)
	}
	if _, err := session.StartErr(t, "explore", map[string]any{"goal": "overview"}); err == nil ||
		!strings.Contains(err.Error(), "targets") {
		t.Errorf("start without targets must fail naming them, got %v", err)
	}
}

func TestExplore_InspectedIdsMustResolve(t *testing.T) {
	world := exploreWorld(t)
	session := world.Open(t, "explore-unresolved")

	serve := session.Start(t, "explore", exploreParams())
	serve = session.Report(t, serve.Instance, map[string]any{
		"widenReport":  "widened",
		"inspectedIds": []any{exploreTargetID, exploreMissingID},
	})
	proctest.RequireStep(t, serve, "inspect")
	requireDiagnostic(t, serve, "an inspected ID does not resolve")
}

func TestExplore_OneShotBatchCompletes(t *testing.T) {
	world := exploreWorld(t)
	session := world.Open(t, "explore-batch")

	serve := session.Start(t, "explore", exploreParams())
	serve = session.Report(t, serve.Instance, map[string]any{
		"widenReport":  "one pass saturated the goal",
		"inspectedIds": []any{exploreTargetID},
		"briefing":     "## Goal\noverview…",
	})
	proctest.RequireStatus(t, serve, "completed")
}

// The shipped explore entry is task-class — it enters through dispatch, not
// the session's move list.
func TestExplore_ShippedEntryIsTaskClass(t *testing.T) {
	entries, err := baseprocedures.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Canonical == "explore" {
			if !entry.IsTaskProcedure() {
				t.Fatalf("explore entry class = %q, want task", entry.Class)
			}
			return
		}
	}
	t.Fatal("embedded base entries carry no explore procedure")
}
