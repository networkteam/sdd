package engine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// Per-procedure table tests for the embedded implementation entry, driving
// the shipped base entry through the production loader (see
// engage_explore_test for the shared harness and the fake WIP commands).

var errFakeNoMarker = errors.New("fake wipDone: no marker set")

type implementationBranchGraphs struct {
	base *model.Graph
	work *model.Graph
}

func (g implementationBranchGraphs) Current() (*model.Graph, error) { return g.base, nil }
func (g implementationBranchGraphs) Invalidate()                    {}
func (g implementationBranchGraphs) CurrentFor(store *Store) (*model.Graph, error) {
	if branch, ok := store.Get("workBranch"); ok && branch == "feature" {
		return g.work, nil
	}
	return g.base, nil
}

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

func assertBindingClearSelfGuard(t *testing.T, instructions string) {
	t.Helper()
	for _, want := range []string{
		"Only if this run actually entered a worktree",
		"only after the host has reported a successful landing",
		"clear the session branch binding",
		"If this run did not enter a worktree or its landing was not successful, make no session branch-binding change",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("closeout instructions missing binding guard %q:\n%s", want, instructions)
		}
	}
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
	assertBindingClearSelfGuard(t, sv.Instructions)

	sv, err = env.session.Answer(sv.Instance, "closeout", "finish", nil, "done for today")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("finish should complete the run, got %s at %q", sv.Status, sv.Step)
	}
}

func TestImplementation_RoutesBaseAndWorkBranchesInEveryMode(t *testing.T) {
	tests := []struct {
		mode       string
		workBranch string
	}{
		{mode: "inPlace", workBranch: "main"},
		{mode: "branch", workBranch: "feature"},
		{mode: "worktree", workBranch: "feature"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			env := newProcEnv(t, "implementation")
			sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{
				"contract": "ready", "widenReport": "constraints checked",
			})
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "baseTarget" {
				t.Fatalf("contract step = %q, want baseTarget", sv.Step)
			}
			baseBranchMissing := false
			for _, field := range sv.Missing {
				if field == "baseBranch" {
					baseBranchMissing = true
				}
			}
			if !baseBranchMissing {
				t.Fatalf("baseTarget must require an explicit baseBranch report: missing=%v", sv.Missing)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
			if err != nil {
				t.Fatal(err)
			}
			setupFields := map[string]any{"wipDescription": "route targets"}
			if test.mode == "worktree" {
				setupFields["worktreeMode"] = "worktree"
			}
			sv, err = env.session.Answer(sv.Instance, "setup", test.mode, setupFields, test.mode)
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "workTarget" {
				t.Fatalf("setup step = %q, want workTarget", sv.Step)
			}
			workBranchMissing := false
			for _, field := range sv.Missing {
				if field == "workBranch" {
					workBranchMissing = true
				}
			}
			if !workBranchMissing {
				t.Fatalf("workTarget must still require an explicit workBranch report: missing=%v", sv.Missing)
			}
			for _, want := range []string{
				"current session binding",
				"natural candidate and default suggestion",
				"report `workBranch` explicitly",
				"engine never copies or adopts the binding",
			} {
				if !strings.Contains(sv.Instructions, want) {
					t.Fatalf("workTarget instructions missing %q:\n%s", want, sv.Instructions)
				}
			}
			if test.mode == "worktree" {
				for _, want := range []string{
					"after the host has entered that worktree",
					"session branch-binding capability",
					"does not fill this procedure's state automatically",
				} {
					if !strings.Contains(sv.Instructions, want) {
						t.Fatalf("worktree instructions missing %q:\n%s", want, sv.Instructions)
					}
				}
			} else if strings.Contains(sv.Instructions, "after the host has entered that worktree") {
				t.Fatalf("%s workTarget rendered worktree-only declaration:\n%s", test.mode, sv.Instructions)
			}
			if len(env.wipBranches) != 1 || env.wipBranches[0] != "main" {
				t.Fatalf("WIP branches = %v, want base main", env.wipBranches)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{"workBranch": test.workBranch})
			if err != nil || sv.Step != "work" {
				t.Fatalf("work target = %q, %v", sv.Step, err)
			}
			if got, ok := env.session.Instance(sv.Instance); !ok {
				t.Fatal("implementation instance disappeared")
			} else if branch, ok := got.Store.Get("workBranch"); !ok || branch != test.workBranch {
				t.Fatalf("workBranch = %v, %v", branch, ok)
			} else if worktreeMode, set := got.Store.Get("worktreeMode"); test.mode == "worktree" && (!set || worktreeMode != "worktree") {
				t.Fatalf("worktreeMode = %v, %v; want worktree", worktreeMode, set)
			} else if test.mode != "worktree" && set {
				t.Fatalf("%s unexpectedly persisted worktreeMode=%v", test.mode, worktreeMode)
			}

			sv, err = env.session.Answer(sv.Instance, "work", "conclude", nil, "mode routing verified")
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{"doneEntry": procNeighborID})
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "landing" {
				t.Fatalf("%s record step = %q, want landing", test.mode, sv.Step)
			}
			if test.mode == "worktree" {
				sv, err = env.session.Answer(sv.Instance, "landing", "defer", nil, "not landed yet")
				if err != nil {
					t.Fatal(err)
				}
				if sv.Step != "landing" {
					t.Fatalf("deferred worktree landing step = %q, want landing", sv.Step)
				}
				if strings.Contains(sv.Instructions, "clear the session branch binding") {
					t.Fatalf("deferred landing rendered clear guidance before landing:\n%s", sv.Instructions)
				}
			}
			sv, err = env.session.Answer(sv.Instance, "landing", "landed", nil, "landed successfully")
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "closeout" {
				t.Fatalf("%s landed step = %q, want closeout", test.mode, sv.Step)
			}
			assertBindingClearSelfGuard(t, sv.Instructions)
		})
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
	assertBindingClearSelfGuard(t, sv.Instructions)
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

func TestImplementation_DoneEntryResolvesAgainstWorkBranch(t *testing.T) {
	env := newProcEnv(t, "implementation")
	work := procGraph(t)
	env.session.engine.Graphs = implementationBranchGraphs{
		base: model.NewGraph([]*model.Entry{work.ByID[procAnchorID]}),
		work: work,
	}

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"contract": "ready", "widenReport": "constraints checked",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "setup", "worktree",
		map[string]any{"wipDescription": "target-aware reads", "worktreeMode": "worktree"}, "use a worktree")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"workBranch": "feature"})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "work", "conclude", nil, "contract met")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"doneEntry": procNeighborID})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "landing" {
		t.Fatalf("work-branch-only done should reach landing, got %q with failures %+v", sv.Step, sv.Failing)
	}
	sv, err = env.session.Answer(sv.Instance, "landing", "landed", nil, "landed successfully")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "closeout" {
		t.Fatalf("landed worktree should reach closeout, got %q", sv.Step)
	}
	assertBindingClearSelfGuard(t, sv.Instructions)
	for _, want := range []string{
		"session branch-binding capability",
		"does not delete the branch or worktree",
	} {
		if !strings.Contains(sv.Instructions, want) {
			t.Fatalf("worktree closeout instructions missing %q:\n%s", want, sv.Instructions)
		}
	}

	sink, ok := env.session.sink.(*memorySink)
	if !ok {
		t.Fatalf("implementation test sink = %T, want *memorySink", env.session.sink)
	}
	replayed, err := env.session.engine.ReplaySession(env.session.ID, env.session.Participant, sink.events,
		func(canonical string) (*Spec, error) {
			if canonical != "implementation" {
				t.Fatalf("replay resolved unexpected procedure %q", canonical)
			}
			return env.spec, nil
		}, nil)
	if err != nil {
		t.Fatal(err)
	}
	replayedInstance, ok := replayed.Instance(sv.Instance)
	if !ok {
		t.Fatal("replayed implementation instance missing")
	}
	if mode, set := replayedInstance.Store.Get("worktreeMode"); !set || mode != "worktree" {
		t.Fatalf("replayed worktreeMode = %v, %v; want worktree", mode, set)
	}
	replayedServe, err := replayed.Serve(sv.Instance)
	if err != nil {
		t.Fatal(err)
	}
	if replayedServe.Step != "closeout" || !strings.Contains(replayedServe.Instructions, "clear the session branch binding") {
		t.Fatalf("replayed worktree closeout lost clear guidance: step=%q\n%s", replayedServe.Step, replayedServe.Instructions)
	}
	assertBindingClearSelfGuard(t, replayedServe.Instructions)
}

func TestImplementation_WorktreeModeIsScopedToWorktreeChoice(t *testing.T) {
	env := newProcEnv(t, "implementation")
	setup := env.spec.StepByID["setup"]
	if setup == nil {
		t.Fatal("implementation spec has no setup step")
	}
	for _, option := range setup.Options {
		hasWorktreeMode := false
		required := false
		for _, field := range option.Collect {
			if field.Name == "worktreeMode" {
				hasWorktreeMode = true
				required = !field.Optional
			}
		}
		if option.Choice == "worktree" {
			if !hasWorktreeMode || !required {
				t.Fatalf("worktree collect = %+v; worktreeMode must be required", option.Collect)
			}
		} else if hasWorktreeMode {
			t.Fatalf("%s option can write worktreeMode: %+v", option.Choice, option.Collect)
		}
	}

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"contract": "ready", "widenReport": "constraints checked",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Answer(sv.Instance, "setup", "branch", map[string]any{
		"wipDescription": "branch run", "worktreeMode": "worktree",
	}, "use a branch"); err == nil || !strings.Contains(err.Error(), `field "worktreeMode" is not collected by option "branch"`) {
		t.Fatalf("branch worktreeMode rejection = %v", err)
	}
	if _, err := env.session.Answer(sv.Instance, "setup", "worktree", map[string]any{
		"wipDescription": "worktree run",
	}, "use a worktree"); err == nil || !strings.Contains(err.Error(), `option "worktree" requires field "worktreeMode"`) {
		t.Fatalf("missing worktreeMode rejection = %v", err)
	}
	if _, err := env.session.Answer(sv.Instance, "setup", "worktree", map[string]any{
		"wipDescription": "worktree run", "worktreeMode": "",
	}, "use a worktree"); err == nil {
		t.Fatal("empty worktreeMode marker was accepted")
	}
}

func TestImplementation_PreseededWorktreeModeIsSafeForNonWorktreeModes(t *testing.T) {
	tests := []struct {
		mode       string
		setup      map[string]any
		workBranch string
	}{
		{
			mode:       "inPlace",
			setup:      map[string]any{"wipDescription": "in-place run"},
			workBranch: "main",
		},
		{
			mode:       "branch",
			setup:      map[string]any{"wipDescription": "branch run"},
			workBranch: "feature",
		},
		{
			mode:       "quick",
			workBranch: "main",
		},
	}

	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			env := newProcEnv(t, "implementation")
			sv, err := env.session.Start(env.spec, map[string]any{
				"anchor":       procAnchorID,
				"worktreeMode": "worktree",
			}, "")
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{
				"contract": "ready", "widenReport": "constraints checked",
			})
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Answer(sv.Instance, "setup", test.mode, test.setup, test.mode)
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "workTarget" {
				t.Fatalf("setup step = %q, want workTarget", sv.Step)
			}
			if !slices.Contains(sv.Missing, "workBranch") {
				t.Fatalf("preseeded %s run must still require an explicit workBranch report: missing=%v", test.mode, sv.Missing)
			}
			for _, want := range []string{
				"Only if this run actually enters a worktree",
				"If this run does not enter a worktree, make no session branch-binding change",
				"report `workBranch` explicitly",
				"engine never copies or adopts the binding",
			} {
				if !strings.Contains(sv.Instructions, want) {
					t.Fatalf("preseeded %s workTarget instructions missing %q:\n%s", test.mode, want, sv.Instructions)
				}
			}

			sv, err = env.session.Report(sv.Instance, map[string]any{"workBranch": test.workBranch})
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "work" {
				t.Fatalf("work branch report step = %q, want work", sv.Step)
			}
			sv, err = env.session.Answer(sv.Instance, "work", "conclude", nil, "mode routing verified")
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{"doneEntry": procNeighborID})
			if err != nil {
				t.Fatal(err)
			}
			if test.mode != "quick" {
				if sv.Step != "landing" {
					t.Fatalf("%s record step = %q, want landing", test.mode, sv.Step)
				}
				sv, err = env.session.Answer(sv.Instance, "landing", "landed", nil, "landed successfully")
				if err != nil {
					t.Fatal(err)
				}
			}
			if sv.Step != "closeout" {
				t.Fatalf("%s completion step = %q, want closeout", test.mode, sv.Step)
			}
			assertBindingClearSelfGuard(t, sv.Instructions)
		})
	}
}

func TestImplementation_WorktreeClearGuidanceSurvivesMarkerSuppression(t *testing.T) {
	tests := []struct {
		name   string
		marker any
	}{
		{name: "nil", marker: nil},
		{name: "empty", marker: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newProcEnv(t, "implementation")
			sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{
				"contract": "ready", "widenReport": "constraints checked",
			})
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{"baseBranch": "main"})
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Answer(sv.Instance, "setup", "worktree", map[string]any{
				"wipDescription": "marker suppression regression",
				"worktreeMode":   "worktree",
			}, "use a worktree")
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{
				"workBranch":   "feature",
				"worktreeMode": test.marker,
			})
			if err != nil {
				t.Fatal(err)
			}
			inst, ok := env.session.Instance(sv.Instance)
			if !ok {
				t.Fatal("implementation instance disappeared")
			}
			mode, set := inst.Store.Get("worktreeMode")
			if test.marker == nil {
				if set {
					t.Fatalf("nil report left worktreeMode=%v", mode)
				}
			} else if !set || mode != "" {
				t.Fatalf("empty report left worktreeMode=%v, %v; want empty stored marker", mode, set)
			}

			sv, err = env.session.Answer(sv.Instance, "work", "conclude", nil, "contract met")
			if err != nil {
				t.Fatal(err)
			}
			sv, err = env.session.Report(sv.Instance, map[string]any{"doneEntry": procNeighborID})
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "landing" {
				t.Fatalf("suppressed-marker worktree record step = %q, want landing", sv.Step)
			}
			sv, err = env.session.Answer(sv.Instance, "landing", "landed", nil, "landed successfully")
			if err != nil {
				t.Fatal(err)
			}
			if sv.Step != "closeout" {
				t.Fatalf("suppressed-marker worktree landing step = %q, want closeout", sv.Step)
			}
			assertBindingClearSelfGuard(t, sv.Instructions)

			sink, ok := env.session.sink.(*memorySink)
			if !ok {
				t.Fatalf("implementation test sink = %T, want *memorySink", env.session.sink)
			}
			replayed, err := env.session.engine.ReplaySession(env.session.ID, env.session.Participant, sink.events,
				func(canonical string) (*Spec, error) {
					if canonical != "implementation" {
						t.Fatalf("replay resolved unexpected procedure %q", canonical)
					}
					return env.spec, nil
				}, nil)
			if err != nil {
				t.Fatal(err)
			}
			replayedServe, err := replayed.Serve(sv.Instance)
			if err != nil {
				t.Fatal(err)
			}
			if replayedServe.Step != "closeout" {
				t.Fatalf("replayed suppressed-marker worktree step = %q, want closeout", replayedServe.Step)
			}
			assertBindingClearSelfGuard(t, replayedServe.Instructions)
		})
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
