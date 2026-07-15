package engine

import (
	"errors"
	"strings"
	"testing"
)

// Per-procedure table tests for the embedded implementation entry, driving
// the shipped base entry through the production loader (see
// engage_explore_test for the shared harness and the fake WIP commands).

var errFakeNoMarker = errors.New("fake wipDone: no marker set")

// startImplementationAtWork drives a fresh instance through contract and a
// tracked in-place setup to the working junction.
func startImplementationAtWork(t *testing.T, env *procEnv) *Serve {
	t.Helper()
	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"contract":    "AC1 remaining, AC2 covered by a partial done; ready to build",
		"widenReport": "searched constraints and prior attempts; nothing beyond the chain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "baseTarget" {
		t.Fatalf("after contract step = %s, want baseTarget", sv.Step)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "setup", "inPlace",
		map[string]any{"wipDescription": "implement the anchor"}, "in place, small scope")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "workTarget" {
		t.Fatalf("after setup step = %s, want workTarget", sv.Step)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"workBranch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "work" {
		t.Fatalf("after work target step = %s, want work", sv.Step)
	}
	return sv
}

func TestImplementation_HappyPathTracked(t *testing.T) {
	env := newProcEnv(t, "implementation")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "contract" {
		t.Fatalf("start step = %s, want contract", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "chains("+procAnchorID+")") {
		t.Errorf("contract unit should serve the anchor's chains, got %q", sv.Instructions)
	}

	sv = startImplementationAtWork(t, env)
	if len(env.wipMarkers) != 1 || env.wipMarkers[0] != "start:"+procAnchorID {
		t.Fatalf("tracked setup should create the marker, got %v", env.wipMarkers)
	}

	// One working-loop cycle: continue self-loops with the running notes.
	sv, err = env.session.Answer(sv.Instance, "work", "continue",
		map[string]any{"progressNotes": "slice 1 committed abc123; next: tests"}, "keep going")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "work" {
		t.Fatalf("continue should loop back to work, got %q", sv.Step)
	}

	sv, err = env.session.Answer(sv.Instance, "work", "conclude", nil, "contract met")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "record" {
		t.Fatalf("conclude should reach record, got %q", sv.Step)
	}

	// Recording the done holds the marker through the landing junction.
	sv, err = env.session.Report(sv.Instance, map[string]any{"doneEntry": procNeighborID})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "landing" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("record should route to the landing user chooser, got %q", sv.Step)
	}
	if len(env.wipMarkers) != 1 {
		t.Fatalf("landing must retain the marker, got %v", env.wipMarkers)
	}
	sv, err = env.session.Answer(sv.Instance, "landing", "landed", nil, "merged successfully")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "closeout" || len(env.wipMarkers) != 2 || env.wipMarkers[1] != "done:wip-"+procAnchorID {
		t.Fatalf("landed should remove marker and reach closeout, got step=%q markers=%v", sv.Step, env.wipMarkers)
	}

	sv, err = env.session.Answer(sv.Instance, "closeout", "finish", nil, "done for today")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("finish should complete the run, got %s at %q", sv.Status, sv.Step)
	}
}

func TestImplementation_QuickSkipsMarker(t *testing.T) {
	env := newProcEnv(t, "implementation")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"contract":    "one-line fix",
		"widenReport": "nothing bears on it",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "setup", "quick", nil, "too small to track")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "workTarget" {
		t.Fatalf("quick should reach workTarget, got %q", sv.Step)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"workBranch": "main"})
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Answer(sv.Instance, "work", "conclude", nil, "fixed")
	if err != nil {
		t.Fatal(err)
	}
	// No marker was created, so record must bypass closeMarker — a route
	// through wipDone would fail loudly on the missing marker.
	sv, err = env.session.Report(sv.Instance, map[string]any{"doneEntry": procNeighborID})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "closeout" {
		t.Fatalf("quick record should route straight to closeout, got %q", sv.Step)
	}
	if len(env.wipMarkers) != 0 {
		t.Fatalf("quick run should touch no markers, got %v", env.wipMarkers)
	}
}

func TestImplementation_HoldLoopsBackToSetup(t *testing.T) {
	env := newProcEnv(t, "implementation")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"contract":    "AC2 presumes an undecided output format",
		"widenReport": "no decision covers the format",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
	if err != nil {
		t.Fatal(err)
	}

	// Hold stashes the capture seed and re-serves setup: the missing decision
	// is captured as a sub-move, then the user picks a mode.
	sv, err = env.session.Answer(sv.Instance, "setup", "hold", nil, "decide the format first")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "setup" {
		t.Fatalf("hold should loop back to setup, got %q", sv.Step)
	}
	if len(env.wipMarkers) != 0 {
		t.Fatalf("hold must not create a marker, got %v", env.wipMarkers)
	}

	sv, err = env.session.Answer(sv.Instance, "setup", "inPlace",
		map[string]any{"wipDescription": "implement with the decided format"}, "decided, go")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "workTarget" {
		t.Fatalf("after hold resolution setup should advance to workTarget, got %q", sv.Step)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"workBranch": "main"})
	if err != nil || sv.Step != "work" {
		t.Fatalf("work branch should advance to work, got %q, %v", sv.Step, err)
	}
}

func TestImplementation_BlockedLoopsBackToWork(t *testing.T) {
	env := newProcEnv(t, "implementation")
	sv := startImplementationAtWork(t, env)

	sv, err := env.session.Answer(sv.Instance, "work", "blocked",
		map[string]any{"roadblock": "no decision defines staleness for markers"}, "stopping to dialogue")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "work" {
		t.Fatalf("blocked should loop back to work, got %q", sv.Step)
	}

	// The dialogue resolved it; the run continues in place.
	sv, err = env.session.Answer(sv.Instance, "work", "continue",
		map[string]any{"progressNotes": "staleness decided in dialogue; resuming slice 2"}, "resolved")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "work" {
		t.Fatalf("continue after blocked should stay in the loop, got %q", sv.Step)
	}
}

func TestImplementation_DoneEntryMustResolve(t *testing.T) {
	env := newProcEnv(t, "implementation")
	sv := startImplementationAtWork(t, env)

	sv, err := env.session.Answer(sv.Instance, "work", "conclude", nil, "wrapping up")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"doneEntry": procMissingID})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "record" {
		t.Fatalf("an unresolved doneEntry must hold record, got %q", sv.Step)
	}
	found := false
	for _, f := range sv.Failing {
		if f.Name == "doneEntryResolves" {
			found = true
		}
	}
	if !found {
		t.Fatalf("failing = %+v, want doneEntryResolves", sv.Failing)
	}
}

func TestImplementation_DispatchDeclarations(t *testing.T) {
	env := newProcEnv(t, "implementation")

	// The declared handoffs are the contract every sub-move rides: hold and
	// conclude are procedure-guarded to capture, blocked seeds any capture the
	// dialogue produces, and closeout's evaluate maps the fresh done signal
	// into the evaluation's anchor. Spec-level assertions so drift between the
	// shipped entry and the seeding machinery cannot pass unnoticed.
	seeds := map[string]struct {
		step      string
		choice    string
		procedure string
		seed      map[string]string
	}{
		"hold": {step: "setup", choice: "hold", procedure: "capture",
			seed: map[string]string{"widenReport": "widenReport", "anchor": "anchor", "captureBranch": "baseBranch"}},
		"blocked": {step: "work", choice: "blocked", procedure: "",
			seed: map[string]string{"widenReport": "widenReport", "anchor": "anchor", "captureBranch": "workBranch"}},
		"conclude": {step: "work", choice: "conclude", procedure: "capture",
			seed: map[string]string{"widenReport": "widenReport", "anchor": "anchor", "captureBranch": "workBranch"}},
		"evaluate": {step: "closeout", choice: "evaluate", procedure: "evaluate",
			seed: map[string]string{"anchor": "doneEntry", "widenReport": "widenReport"}},
	}
	for name, want := range seeds {
		var opt *Option
		for _, step := range env.spec.Steps {
			if step.ID != want.step {
				continue
			}
			for i := range step.Options {
				if step.Options[i].Choice == want.choice {
					opt = &step.Options[i]
				}
			}
		}
		if opt == nil {
			t.Fatalf("%s: option %s not found on step %s", name, want.choice, want.step)
		}
		if opt.Dispatch == nil {
			t.Fatalf("%s: option declares no dispatch", name)
		}
		if opt.Dispatch.Procedure != want.procedure {
			t.Errorf("%s: dispatch procedure = %q, want %q", name, opt.Dispatch.Procedure, want.procedure)
		}
		for child, parent := range want.seed {
			if got := opt.Dispatch.Seed[child]; got != parent {
				t.Errorf("%s: seed %s = %q, want %q", name, child, got, parent)
			}
		}
	}
}
