package proctest_test

import (
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/proctest"
)

const (
	engageAnchorID   = "20260601-120000-d-tac-anc"
	engageNeighborID = "20260601-130000-s-tac-nbr"
	// engageMissingID is well-formed but absent from the fixture graph.
	engageMissingID = "20260601-140000-d-tac-gon"

	engageAnchorBody   = "A directive the engagement anchors on."
	engageNeighborBody = "A connected gap engaged alongside."
)

func engageWorld(t *testing.T) *proctest.World {
	t.Helper()
	return proctest.NewWorld(t, proctest.WithEntries(
		proctest.Entry{
			ID: engageAnchorID, Type: "decision", Kind: "directive", Layer: "tactical", Intent: "pending",
			Summary: engageAnchorBody, Body: engageAnchorBody,
		},
		proctest.Entry{
			ID: engageNeighborID, Type: "signal", Kind: "gap", Layer: "tactical",
			Summary: engageNeighborBody, Body: engageNeighborBody,
		},
	))
}

// requireDiagnostic fails unless one of the serve's gate diagnostics carries
// the fragment — the application-layer surface of a failing predicate.
// Candidate for the proctest harness once a third suite needs it.
func requireDiagnostic(t *testing.T, serve *sdd.WorkflowServe, fragment string) {
	t.Helper()
	for _, d := range serve.Diagnostics {
		if strings.Contains(d, fragment) {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want one containing %q", serve.Diagnostics, fragment)
}

func TestEngage_HappyPath(t *testing.T) {
	world := engageWorld(t)
	session := world.Open(t, "engage-happy")

	serve := session.Start(t, "engage", nil)
	proctest.RequireStep(t, serve, "anchor")
	if got := strings.Join(serve.Missing, ","); got != "anchor" {
		t.Fatalf("missing = %q, want anchor (targets and goal are optional)", got)
	}
	instance := serve.Instance

	serve = session.Report(t, instance, map[string]any{
		"anchor": engageAnchorID,
		"goal":   "implement it",
	})
	proctest.RequireStep(t, serve, "brief")
	if !strings.Contains(serve.Instructions, engageAnchorID) {
		t.Errorf("brief unit should serve the anchor's chain with its ID, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, engageAnchorBody) {
		t.Errorf("brief unit should serve the anchor in full, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "implement it") {
		t.Errorf("brief unit should render the goal, got %q", serve.Instructions)
	}

	serve = session.Report(t, instance, map[string]any{
		"brief":       "AC-status: 1 done, 2 remaining.",
		"widenReport": "searched three angles, nothing beyond the chain",
	})
	proctest.RequireStep(t, serve, "moves")
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("moves must serve a user chooser, got %+v", serve.PendingChooser)
	}
	if !strings.Contains(serve.Instructions, engageAnchorID) {
		t.Errorf("moves unit should name the anchor, got %q", serve.Instructions)
	}

	serve = session.Answer(t, instance, "moves", "move",
		map[string]any{"selectedMove": "implement slice 5"}, "let's build it")
	proctest.RequireStatus(t, serve, "completed")
}

func TestEngage_AnchorMustResolve(t *testing.T) {
	world := engageWorld(t)
	session := world.Open(t, "engage-unresolved")

	serve := session.Start(t, "engage", nil)
	serve = session.Report(t, serve.Instance, map[string]any{"anchor": engageMissingID})
	// Stalling on anchor means brief's entryChains injection never ran on the
	// unresolved anchor — the inject belongs to the step never reached.
	proctest.RequireStep(t, serve, "anchor")
	requireDiagnostic(t, serve, "the anchor or a target does not resolve")
}

func TestEngage_TargetsServedAlongside(t *testing.T) {
	world := engageWorld(t)
	session := world.Open(t, "engage-targets")

	serve := session.Start(t, "engage", nil)
	serve = session.Report(t, serve.Instance, map[string]any{
		"anchor":  engageAnchorID,
		"targets": []any{engageNeighborID},
	})
	proctest.RequireStep(t, serve, "brief")
	for _, want := range []string{engageAnchorBody, engageNeighborID, engageNeighborBody} {
		if !strings.Contains(serve.Instructions, want) {
			t.Errorf("brief should serve anchor and target chains, missing %q in %q", want, serve.Instructions)
		}
	}

	// A target that doesn't resolve holds the anchor gate the same way.
	world2 := engageWorld(t)
	session2 := world2.Open(t, "engage-target-unresolved")
	serve2 := session2.Start(t, "engage", nil)
	serve2 = session2.Report(t, serve2.Instance, map[string]any{
		"anchor":  engageAnchorID,
		"targets": []any{engageMissingID},
	})
	proctest.RequireStep(t, serve2, "anchor")
	requireDiagnostic(t, serve2, "the anchor or a target does not resolve")
}

func TestEngage_OneShotBatchCascades(t *testing.T) {
	world := engageWorld(t)
	session := world.Open(t, "engage-batch")

	serve := session.Start(t, "engage", nil)
	serve = session.Report(t, serve.Instance, map[string]any{
		"anchor":      engageAnchorID,
		"brief":       "narrative brief",
		"widenReport": "widened from two angles",
	})
	proctest.RequireStep(t, serve, "moves")
}

func TestEngage_ConcludeAndAbort(t *testing.T) {
	for choice, want := range map[string]string{
		"conclude": "completed",
		"abort":    "abandoned",
	} {
		world := engageWorld(t)
		session := world.Open(t, "engage-"+choice)

		serve := session.Start(t, "engage", nil)
		serve = session.Report(t, serve.Instance, map[string]any{
			"anchor":      engageAnchorID,
			"brief":       "brief",
			"widenReport": "widened",
		})
		proctest.RequireStep(t, serve, "moves")
		serve = session.Answer(t, serve.Instance, "moves", choice, nil, "user said so")
		if serve.Status != want {
			t.Errorf("%s: status = %s, want %s", choice, serve.Status, want)
		}
	}
}
