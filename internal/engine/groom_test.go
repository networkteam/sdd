package engine

import (
	"strings"
	"testing"
)

// Per-procedure table tests for the embedded groom entry, driving the
// shipped base entry through the production loader (see engage_explore_test
// for the shared harness).

func TestGroom_SweepThenWalk(t *testing.T) {
	env := newProcEnv(t, "groom")

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "sweep" {
		t.Fatalf("start step = %s, want sweep", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "lanes for wip") {
		t.Errorf("sweep unit should serve the WIP lane, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"candidates": "1. " + procNeighborID + " resolved but open — downstream done covers it; propose closing done",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "walk" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("a reported table should reach the walk user chooser, got %q", sv.Step)
	}

	// One cleanup loops back for the next candidate.
	sv, err = env.session.Answer(sv.Instance, "walk", "cleanup",
		map[string]any{"selectedCleanup": "close candidate 1 with a done"}, "yes, close it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "walk" {
		t.Fatalf("cleanup should loop back to walk, got %q", sv.Step)
	}

	sv, err = env.session.Answer(sv.Instance, "walk", "conclude", nil, "that's all")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("conclude should complete, got %s at %q", sv.Status, sv.Step)
	}
}

func TestGroom_FocusHintRendered(t *testing.T) {
	env := newProcEnv(t, "groom")

	sv, err := env.session.Start(env.spec, map[string]any{"focusHint": "the type-system cluster"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.Instructions, "the type-system cluster") {
		t.Errorf("sweep should render the focus hint verbatim, got %q", sv.Instructions)
	}
}

func TestGroom_CleanupDeclaresCaptureHandoff(t *testing.T) {
	env := newProcEnv(t, "groom")

	// The cleanup dispatch seeds the sweep's evidence into each capture's
	// grounding — the candidate table is the widen record.
	var cleanup *Option
	for _, step := range env.spec.Steps {
		if step.ID != "walk" {
			continue
		}
		for i := range step.Options {
			if step.Options[i].Choice == "cleanup" {
				cleanup = &step.Options[i]
			}
		}
	}
	if cleanup == nil {
		t.Fatal("walk has no cleanup option")
	}
	if cleanup.Dispatch == nil || cleanup.Dispatch.Procedure != "capture" {
		t.Fatalf("cleanup must dispatch capture, got %+v", cleanup.Dispatch)
	}
	if got := cleanup.Dispatch.Seed["widenReport"]; got != "candidates" {
		t.Errorf("cleanup seed widenReport = %q, want candidates", got)
	}
}

// A stale WIP marker is cleared through the wipRemove transition, not a
// capture: the agent supplies the marker ID as staleMarker, the engine
// removes it, and the walk loops back for the next candidate. No CLI.
func TestGroom_RemoveMarkerClearsViaTransition(t *testing.T) {
	env := newProcEnv(t, "groom")

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"candidates": "1. stale WIP marker 20260706-114441-christopher on d-tac-v5f — closed; propose removal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "walk" {
		t.Fatalf("reported candidates should reach walk, got %q", sv.Step)
	}

	sv, err = env.session.Answer(sv.Instance, "walk", "removeMarker",
		map[string]any{"staleMarker": "20260706-114441-christopher"}, "yes, clear it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "walk" {
		t.Fatalf("removeMarker should loop back to walk, got %q", sv.Step)
	}

	found := false
	for _, m := range env.wipMarkers {
		if m == "remove:20260706-114441-christopher" {
			found = true
		}
	}
	if !found {
		t.Fatalf("wipRemove should have removed the marker, recorded %v", env.wipMarkers)
	}

	sv, err = env.session.Answer(sv.Instance, "walk", "conclude", nil, "that's all")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("conclude should complete, got %s at %q", sv.Status, sv.Step)
	}
}
