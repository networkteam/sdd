package proctest_test

// Ported from internal/engine/implementation_test.go: the same shipped
// implementation entry, driven through the real application. Fake WIP markers
// became real marker files under each store's wip/ directory, the fake branch
// router became WithBranchDir stores, and the closing done is recorded through
// the real dispatched capture procedure.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

const (
	implAnchorID = "20260601-120000-d-tac-ref"
	implDoneID   = "20260601-130000-s-tac-don"
	// implMissingID is well-formed but absent from every fixture store.
	implMissingID = "20260601-140000-d-tac-gon"
)

func implAnchorEntry() *model.Entry {
	return &model.Entry{
		ID: implAnchorID, Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
		Summary: "A directive the implementation anchors on.",
		Content: "A directive the implementation anchors on.",
	}
}

// implDoneEntry is a pre-recorded closing done fixture for scenarios that
// exercise routing rather than the capture sub-move itself.
func implDoneEntry() *model.Entry {
	return &model.Entry{
		ID: implDoneID, Type: model.TypeSignal, Kind: model.KindDone, Layer: model.LayerTactical,
		Closes:  []string{implAnchorID},
		Summary: "The anchor directive was delivered in commit abc1234.",
		Content: "The anchor directive was delivered in commit abc1234.",
	}
}

// startImplChild starts a procedure under a parent instance, riding the
// parent's dispatch seed — harness Session.Start always parents on the shell.
func startImplChild(t *testing.T, session *proctest.Session, canonical, parent string) *sdd.WorkflowServe {
	t.Helper()
	serve, err := session.WF.Start(t.Context(), session.World.Identity, sdd.WorkflowStartRequest{Canonical: canonical, Parent: parent})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

func wipMarkerIDs(t *testing.T, graphDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(model.WIPDir(graphDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			ids = append(ids, strings.TrimSuffix(entry.Name(), ".md"))
		}
	}
	return ids
}

func requireNoMarkers(t *testing.T, graphDir string) {
	t.Helper()
	if ids := wipMarkerIDs(t, graphDir); len(ids) != 0 {
		t.Fatalf("wip markers in %s = %v, want none", graphDir, ids)
	}
}

func requireSingleMarker(t *testing.T, graphDir string) *model.WIPMarker {
	t.Helper()
	ids := wipMarkerIDs(t, graphDir)
	if len(ids) != 1 {
		t.Fatalf("wip markers in %s = %v, want exactly one", graphDir, ids)
	}
	content, err := os.ReadFile(filepath.Join(model.WIPDir(graphDir), ids[0]+".md"))
	if err != nil {
		t.Fatal(err)
	}
	marker, err := model.ParseWIPMarker(ids[0]+".md", string(content))
	if err != nil {
		t.Fatal(err)
	}
	if marker.Entry != implAnchorID || !marker.Exclusive {
		t.Fatalf("marker = %+v, want an exclusive marker on %s", marker, implAnchorID)
	}
	return marker
}

func entryOnDisk(t *testing.T, graphDir, id string) bool {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(graphDir, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

// implToSetup drives a fresh instance through contract and baseTarget,
// logging the anchor read the contract step requires.
func implToSetup(t *testing.T, session *proctest.Session, params map[string]any, baseBranch string) *sdd.WorkflowServe {
	t.Helper()
	serve := session.Start(t, "implementation", params)
	proctest.RequireStep(t, serve, "contract")
	session.LogRead(t, "show", []string{implAnchorID}, nil)
	serve = session.Report(t, serve.Instance, map[string]any{
		"contract":    "AC1 remaining, AC2 covered by a partial done; ready to build",
		"widenReport": "searched constraints and prior attempts; nothing beyond the chain",
	})
	proctest.RequireStep(t, serve, "baseTarget")
	serve = session.Report(t, serve.Instance, map[string]any{"baseBranch": baseBranch})
	proctest.RequireStep(t, serve, "setup")
	return serve
}

// startImplementationAtWork drives a fresh instance through contract and a
// tracked in-place setup to the working junction.
func startImplementationAtWork(t *testing.T, session *proctest.Session) *sdd.WorkflowServe {
	t.Helper()
	serve := implToSetup(t, session, map[string]any{"anchor": implAnchorID}, "main")
	serve = session.Answer(t, serve.Instance, "setup", "inPlace",
		map[string]any{"wipDescription": "implement the anchor"}, "in place, small scope")
	proctest.RequireStep(t, serve, "workTarget")
	serve = session.Report(t, serve.Instance, map[string]any{"workBranch": "main"})
	proctest.RequireStep(t, serve, "work")
	return serve
}

// driveCapture runs an already-started capture child through playback and
// summary verification, returning the written entry's ID.
func driveCapture(t *testing.T, session *proctest.Session, instance string, fields map[string]any) string {
	t.Helper()
	serve := session.Report(t, instance, fields)
	proctest.RequireStep(t, serve, "playback")
	serve = session.Answer(t, instance, "playback", "confirm", nil, "capture it")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches the body"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}
	return entryID
}

// captureDone records the closing done as the real dispatched capture
// sub-move: the child inherits the conclude answer's seed, and the done draft
// satisfies the construction boundary by closing the anchor.
func captureDone(t *testing.T, session *proctest.Session, implInstance string) string {
	t.Helper()
	child := startImplChild(t, session, "capture", implInstance)
	proctest.RequireStep(t, child, "assemble")
	if slices.Contains(child.Missing, "widenReport") {
		t.Fatalf("dispatched capture should inherit the seeded widenReport, missing = %v", child.Missing)
	}
	return driveCapture(t, session, child.Instance, map[string]any{
		"body":       "Implemented the anchor directive; delivered in commit abc1234 with all acceptance criteria addressed.",
		"entryKind":  "done",
		"layer":      "tactical",
		"closes":     []any{implAnchorID},
		"topics":     []any{"implementation/engine"},
		"confidence": "high",
	})
}

// resumedInstanceServe re-attaches from a fresh connection — the application's
// real replay path — and returns the instance's re-served position.
func resumedInstanceServe(t *testing.T, world *proctest.World, sessionID sdd.SessionID, connID, instance string) *sdd.WorkflowServe {
	t.Helper()
	_, result := world.Resume(t, sessionID, connID)
	for i := range result.Open {
		if result.Open[i].Instance == instance {
			return &result.Open[i]
		}
	}
	t.Fatalf("resumed session lost instance %s: %+v", instance, result.Open)
	return nil
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
	world := proctest.NewWorld(t, proctest.WithEntries(implAnchorEntry()))
	session := world.Open(t, "impl-happy")

	serve := session.Start(t, "implementation", map[string]any{"anchor": implAnchorID})
	proctest.RequireStep(t, serve, "contract")
	if !strings.Contains(serve.Instructions, implAnchorID) {
		t.Errorf("contract unit should serve the anchor's chains, got %q", serve.Instructions)
	}
	instance := serve.Instance

	session.LogRead(t, "show", []string{implAnchorID}, nil)
	serve = session.Report(t, instance, map[string]any{
		"contract":    "AC1 remaining, AC2 covered by a partial done; ready to build",
		"widenReport": "searched constraints and prior attempts; nothing beyond the chain",
	})
	proctest.RequireStep(t, serve, "baseTarget")
	serve = session.Report(t, instance, map[string]any{"baseBranch": "main"})
	proctest.RequireStep(t, serve, "setup")
	serve = session.Answer(t, instance, "setup", "inPlace",
		map[string]any{"wipDescription": "implement the anchor"}, "in place, small scope")
	proctest.RequireStep(t, serve, "workTarget")
	marker := requireSingleMarker(t, world.GraphDir)
	if marker.Content != "implement the anchor" {
		t.Fatalf("marker content = %q, want the wipDescription", marker.Content)
	}
	serve = session.Report(t, instance, map[string]any{"workBranch": "main"})
	proctest.RequireStep(t, serve, "work")

	// One working-loop cycle: continue self-loops with the running notes.
	serve = session.Answer(t, instance, "work", "continue",
		map[string]any{"progressNotes": "slice 1 committed abc123; next: tests"}, "keep going")
	proctest.RequireStep(t, serve, "work")

	serve = session.Answer(t, instance, "work", "conclude", nil, "contract met")
	proctest.RequireStep(t, serve, "record")

	// Recording the done holds the marker through the landing junction.
	doneID := captureDone(t, session, instance)
	serve = session.Report(t, instance, map[string]any{"doneEntry": doneID})
	proctest.RequireStep(t, serve, "landing")
	if serve.PendingChooser == nil || string(serve.PendingChooser.Kind) != "user" {
		t.Fatalf("record should route to the landing user chooser, got %+v", serve.PendingChooser)
	}
	requireSingleMarker(t, world.GraphDir)

	serve = session.Answer(t, instance, "landing", "landed", nil, "merged successfully")
	proctest.RequireStep(t, serve, "closeout")
	requireNoMarkers(t, world.GraphDir)
	assertBindingClearSelfGuard(t, serve.Instructions)

	serve = session.Answer(t, instance, "closeout", "finish", nil, "done for today")
	proctest.RequireStatus(t, serve, "completed")
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
			featureDir := t.TempDir()
			proctest.WriteEntry(t, featureDir, implAnchorEntry())
			proctest.WriteEntry(t, featureDir, implDoneEntry())
			world := proctest.NewWorld(t,
				proctest.WithEntries(implAnchorEntry(), implDoneEntry()),
				proctest.WithBranchDir("feature", featureDir),
			)
			session := world.Open(t, "impl-routes-"+test.mode)

			serve := session.Start(t, "implementation", map[string]any{"anchor": implAnchorID})
			proctest.RequireStep(t, serve, "contract")
			instance := serve.Instance
			session.LogRead(t, "show", []string{implAnchorID}, nil)
			serve = session.Report(t, instance, map[string]any{
				"contract": "ready", "widenReport": "constraints checked",
			})
			proctest.RequireStep(t, serve, "baseTarget")
			if !slices.Contains(serve.Missing, "baseBranch") {
				t.Fatalf("baseTarget must require an explicit baseBranch report: missing=%v", serve.Missing)
			}
			serve = session.Report(t, instance, map[string]any{"baseBranch": "main"})
			proctest.RequireStep(t, serve, "setup")
			setupFields := map[string]any{"wipDescription": "route targets"}
			if test.mode == "worktree" {
				setupFields["worktreeMode"] = "worktree"
			}
			serve = session.Answer(t, instance, "setup", test.mode, setupFields, test.mode)
			proctest.RequireStep(t, serve, "workTarget")
			if !slices.Contains(serve.Missing, "workBranch") {
				t.Fatalf("workTarget must still require an explicit workBranch report: missing=%v", serve.Missing)
			}
			for _, want := range []string{
				"current session binding",
				"natural candidate and default suggestion",
				"report `workBranch` explicitly",
				"engine never copies or adopts the binding",
			} {
				if !strings.Contains(serve.Instructions, want) {
					t.Fatalf("workTarget instructions missing %q:\n%s", want, serve.Instructions)
				}
			}
			if test.mode == "worktree" {
				for _, want := range []string{
					"after the host has entered that worktree",
					"session branch-binding capability",
					"does not fill this procedure's state automatically",
				} {
					if !strings.Contains(serve.Instructions, want) {
						t.Fatalf("worktree instructions missing %q:\n%s", want, serve.Instructions)
					}
				}
			} else if strings.Contains(serve.Instructions, "after the host has entered that worktree") {
				t.Fatalf("%s workTarget rendered worktree-only declaration:\n%s", test.mode, serve.Instructions)
			}
			// The marker lives on the explicit base branch, never the work branch.
			requireSingleMarker(t, world.GraphDir)
			requireNoMarkers(t, featureDir)
			serve = session.Report(t, instance, map[string]any{"workBranch": test.workBranch})
			proctest.RequireStep(t, serve, "work")

			serve = session.Answer(t, instance, "work", "conclude", nil, "mode routing verified")
			proctest.RequireStep(t, serve, "record")
			serve = session.Report(t, instance, map[string]any{"doneEntry": implDoneID})
			proctest.RequireStep(t, serve, "landing")
			if test.mode == "worktree" {
				serve = session.Answer(t, instance, "landing", "defer", nil, "not landed yet")
				proctest.RequireStep(t, serve, "landing")
				if strings.Contains(serve.Instructions, "clear the session branch binding") {
					t.Fatalf("deferred landing rendered clear guidance before landing:\n%s", serve.Instructions)
				}
				requireSingleMarker(t, world.GraphDir)
			}
			serve = session.Answer(t, instance, "landing", "landed", nil, "landed successfully")
			proctest.RequireStep(t, serve, "closeout")
			requireNoMarkers(t, world.GraphDir)
			assertBindingClearSelfGuard(t, serve.Instructions)
		})
	}
}

func TestImplementation_QuickSkipsMarker(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(implAnchorEntry(), implDoneEntry()))
	session := world.Open(t, "impl-quick")

	serve := session.Start(t, "implementation", map[string]any{"anchor": implAnchorID})
	session.LogRead(t, "show", []string{implAnchorID}, nil)
	instance := serve.Instance
	session.Report(t, instance, map[string]any{
		"contract":    "one-line fix",
		"widenReport": "nothing bears on it",
	})
	session.Report(t, instance, map[string]any{"baseBranch": "main"})
	serve = session.Answer(t, instance, "setup", "quick", nil, "too small to track")
	proctest.RequireStep(t, serve, "workTarget")
	requireNoMarkers(t, world.GraphDir)
	serve = session.Report(t, instance, map[string]any{"workBranch": "main"})
	proctest.RequireStep(t, serve, "work")

	serve = session.Answer(t, instance, "work", "conclude", nil, "fixed")
	proctest.RequireStep(t, serve, "record")
	// No marker was created, so record must bypass the landing junction — a
	// route through wipDone would fail loudly on the unset wipMarker.
	serve = session.Report(t, instance, map[string]any{"doneEntry": implDoneID})
	proctest.RequireStep(t, serve, "closeout")
	assertBindingClearSelfGuard(t, serve.Instructions)
	requireNoMarkers(t, world.GraphDir)
}

func TestImplementation_HoldLoopsBackToSetup(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(implAnchorEntry()))
	session := world.Open(t, "impl-hold")

	serve := session.Start(t, "implementation", map[string]any{"anchor": implAnchorID})
	session.LogRead(t, "show", []string{implAnchorID}, nil)
	instance := serve.Instance
	session.Report(t, instance, map[string]any{
		"contract":    "AC2 presumes an undecided output format",
		"widenReport": "no decision covers the format",
	})
	serve = session.Report(t, instance, map[string]any{"baseBranch": "main"})
	proctest.RequireStep(t, serve, "setup")

	// Hold stashes the capture seed and re-serves setup: the missing decision
	// is captured as a sub-move, then the user picks a mode.
	serve = session.Answer(t, instance, "setup", "hold", nil, "decide the format first")
	proctest.RequireStep(t, serve, "setup")
	requireNoMarkers(t, world.GraphDir)

	serve = session.Answer(t, instance, "setup", "inPlace",
		map[string]any{"wipDescription": "implement with the decided format"}, "decided, go")
	proctest.RequireStep(t, serve, "workTarget")
	serve = session.Report(t, instance, map[string]any{"workBranch": "main"})
	proctest.RequireStep(t, serve, "work")
}

// TestImplementation_HoldSeedsCaptureOnBaseBranch is the behavioral half of
// the old dispatch-declaration check for hold: the dispatched capture inherits
// widenReport and captureBranch from baseBranch, so the captured decision
// lands on the base store even before any work branch exists.
func TestImplementation_HoldSeedsCaptureOnBaseBranch(t *testing.T) {
	featureDir := t.TempDir()
	proctest.WriteEntry(t, featureDir, implAnchorEntry())
	world := proctest.NewWorld(t,
		proctest.WithEntries(implAnchorEntry()),
		proctest.WithBranchDir("feature", featureDir),
	)
	session := world.Open(t, "impl-hold-seed")

	serve := implToSetup(t, session, map[string]any{"anchor": implAnchorID}, "feature")
	instance := serve.Instance
	serve = session.Answer(t, instance, "setup", "hold", nil, "decide the format first")
	proctest.RequireStep(t, serve, "setup")

	child := startImplChild(t, session, "capture", instance)
	proctest.RequireStep(t, child, "assemble")
	if slices.Contains(child.Missing, "widenReport") {
		t.Fatalf("hold dispatch should seed widenReport, missing = %v", child.Missing)
	}
	entryID := driveCapture(t, session, child.Instance, map[string]any{
		"body":       "The output format is JSON lines, one record per entry.",
		"entryKind":  "directive",
		"layer":      "tactical",
		"intent":     "pending",
		"refs":       []any{map[string]any{"id": implAnchorID, "kind": "addresses"}},
		"confidence": "medium",
	})
	if !entryOnDisk(t, featureDir, entryID) {
		t.Fatalf("hold capture %s should land on the seeded baseBranch store", entryID)
	}
	if entryOnDisk(t, world.GraphDir, entryID) {
		t.Fatalf("hold capture %s leaked onto the default store", entryID)
	}
}

func TestImplementation_BlockedLoopsBackToWork(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(implAnchorEntry()))
	session := world.Open(t, "impl-blocked")
	serve := startImplementationAtWork(t, session)
	instance := serve.Instance

	serve = session.Answer(t, instance, "work", "blocked",
		map[string]any{"roadblock": "no decision defines staleness for markers"}, "stopping to dialogue")
	proctest.RequireStep(t, serve, "work")

	// The blocked answer seeds any capture the dialogue produces.
	child := startImplChild(t, session, "capture", instance)
	proctest.RequireStep(t, child, "assemble")
	if slices.Contains(child.Missing, "widenReport") {
		t.Fatalf("blocked dispatch should seed widenReport, missing = %v", child.Missing)
	}

	// The dialogue resolved it; the run continues in place.
	serve = session.Answer(t, instance, "work", "continue",
		map[string]any{"progressNotes": "staleness decided in dialogue; resuming slice 2"}, "resolved")
	proctest.RequireStep(t, serve, "work")
}

func TestImplementation_DoneEntryMustResolve(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(implAnchorEntry()))
	session := world.Open(t, "impl-unresolved-done")
	serve := startImplementationAtWork(t, session)
	instance := serve.Instance

	serve = session.Answer(t, instance, "work", "conclude", nil, "wrapping up")
	proctest.RequireStep(t, serve, "record")
	serve = session.Report(t, instance, map[string]any{"doneEntry": implMissingID})
	proctest.RequireStep(t, serve, "record")
	if joined := strings.Join(serve.Diagnostics, "\n"); !strings.Contains(joined, "doneEntry does not resolve") {
		t.Fatalf("diagnostics = %v, want the doneEntryResolves failure", serve.Diagnostics)
	}
}

func TestImplementation_DoneEntryResolvesAgainstWorkBranch(t *testing.T) {
	featureDir := t.TempDir()
	proctest.WriteEntry(t, featureDir, implAnchorEntry())
	world := proctest.NewWorld(t,
		proctest.WithEntries(implAnchorEntry()),
		proctest.WithBranchDir("feature", featureDir),
	)
	session := world.Open(t, "impl-workbranch")

	serve := implToSetup(t, session, map[string]any{"anchor": implAnchorID}, "main")
	instance := serve.Instance
	serve = session.Answer(t, instance, "setup", "worktree",
		map[string]any{"wipDescription": "target-aware reads", "worktreeMode": "worktree"}, "use a worktree")
	proctest.RequireStep(t, serve, "workTarget")
	serve = session.Report(t, instance, map[string]any{"workBranch": "feature"})
	proctest.RequireStep(t, serve, "work")
	serve = session.Answer(t, instance, "work", "conclude", nil, "contract met")
	proctest.RequireStep(t, serve, "record")

	// The real dispatched capture inherits captureBranch from workBranch, so
	// the done signal exists only on the feature store — record's resolution
	// must read through the work branch to find it.
	doneID := captureDone(t, session, instance)
	if !entryOnDisk(t, featureDir, doneID) {
		t.Fatalf("done capture %s should land on the work branch store", doneID)
	}
	if entryOnDisk(t, world.GraphDir, doneID) {
		t.Fatalf("done capture %s leaked onto the base store", doneID)
	}
	serve = session.Report(t, instance, map[string]any{"doneEntry": doneID})
	proctest.RequireStep(t, serve, "landing")
	serve = session.Answer(t, instance, "landing", "landed", nil, "landed successfully")
	proctest.RequireStep(t, serve, "closeout")
	requireNoMarkers(t, world.GraphDir)
	assertBindingClearSelfGuard(t, serve.Instructions)
	for _, want := range []string{
		"session branch-binding capability",
		"does not delete the branch or worktree",
	} {
		if !strings.Contains(serve.Instructions, want) {
			t.Fatalf("worktree closeout instructions missing %q:\n%s", want, serve.Instructions)
		}
	}

	// Re-attaching replays the stored session through the real load path: the
	// worktree choice and the closeout clear guidance must survive.
	replayed := resumedInstanceServe(t, world, session.ID, "impl-workbranch-replay", instance)
	proctest.RequireStep(t, replayed, "closeout")
	if mode, _ := replayed.Collected["worktreeMode"].(string); mode != "worktree" {
		t.Fatalf("replayed worktreeMode = %v, want worktree", replayed.Collected["worktreeMode"])
	}
	assertBindingClearSelfGuard(t, replayed.Instructions)
}

func TestImplementation_WorktreeModeIsScopedToWorktreeChoice(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(implAnchorEntry()))
	session := world.Open(t, "impl-scoped-mode")

	serve := implToSetup(t, session, map[string]any{"anchor": implAnchorID}, "main")
	instance := serve.Instance
	if serve.PendingChooser == nil {
		t.Fatal("setup served no chooser")
	}
	for _, option := range serve.PendingChooser.Options {
		hasWorktreeMode := slices.Contains(option.Collect, "worktreeMode")
		if option.Choice == "worktree" {
			if !hasWorktreeMode {
				t.Fatalf("worktree collect = %v; worktreeMode must be required", option.Collect)
			}
		} else if hasWorktreeMode || slices.Contains(option.Collect, "worktreeMode?") {
			t.Fatalf("%s option can write worktreeMode: %v", option.Choice, option.Collect)
		}
	}

	if _, err := session.AnswerErr(t, instance, "setup", "branch", map[string]any{
		"wipDescription": "branch run", "worktreeMode": "worktree",
	}, "use a branch"); err == nil || !strings.Contains(err.Error(), `field "worktreeMode" is not collected by option "branch"`) {
		t.Fatalf("branch worktreeMode rejection = %v", err)
	}
	if _, err := session.AnswerErr(t, instance, "setup", "worktree", map[string]any{
		"wipDescription": "worktree run",
	}, "use a worktree"); err == nil || !strings.Contains(err.Error(), `option "worktree" requires field "worktreeMode"`) {
		t.Fatalf("missing worktreeMode rejection = %v", err)
	}
	if _, err := session.AnswerErr(t, instance, "setup", "worktree", map[string]any{
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
			featureDir := t.TempDir()
			proctest.WriteEntry(t, featureDir, implAnchorEntry())
			proctest.WriteEntry(t, featureDir, implDoneEntry())
			world := proctest.NewWorld(t,
				proctest.WithEntries(implAnchorEntry(), implDoneEntry()),
				proctest.WithBranchDir("feature", featureDir),
			)
			session := world.Open(t, "impl-preseeded-"+test.mode)

			serve := implToSetup(t, session, map[string]any{
				"anchor":       implAnchorID,
				"worktreeMode": "worktree",
			}, "main")
			instance := serve.Instance
			serve = session.Answer(t, instance, "setup", test.mode, test.setup, test.mode)
			proctest.RequireStep(t, serve, "workTarget")
			if !slices.Contains(serve.Missing, "workBranch") {
				t.Fatalf("preseeded %s run must still require an explicit workBranch report: missing=%v", test.mode, serve.Missing)
			}
			for _, want := range []string{
				"Only if this run actually enters a worktree",
				"If this run does not enter a worktree, make no session branch-binding change",
				"report `workBranch` explicitly",
				"engine never copies or adopts the binding",
			} {
				if !strings.Contains(serve.Instructions, want) {
					t.Fatalf("preseeded %s workTarget instructions missing %q:\n%s", test.mode, want, serve.Instructions)
				}
			}

			serve = session.Report(t, instance, map[string]any{"workBranch": test.workBranch})
			proctest.RequireStep(t, serve, "work")
			serve = session.Answer(t, instance, "work", "conclude", nil, "mode routing verified")
			proctest.RequireStep(t, serve, "record")
			serve = session.Report(t, instance, map[string]any{"doneEntry": implDoneID})
			if test.mode != "quick" {
				proctest.RequireStep(t, serve, "landing")
				serve = session.Answer(t, instance, "landing", "landed", nil, "landed successfully")
			}
			proctest.RequireStep(t, serve, "closeout")
			assertBindingClearSelfGuard(t, serve.Instructions)
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
			featureDir := t.TempDir()
			proctest.WriteEntry(t, featureDir, implAnchorEntry())
			proctest.WriteEntry(t, featureDir, implDoneEntry())
			world := proctest.NewWorld(t,
				proctest.WithEntries(implAnchorEntry(), implDoneEntry()),
				proctest.WithBranchDir("feature", featureDir),
			)
			session := world.Open(t, "impl-suppressed-"+test.name)

			serve := implToSetup(t, session, map[string]any{"anchor": implAnchorID}, "main")
			instance := serve.Instance
			serve = session.Answer(t, instance, "setup", "worktree", map[string]any{
				"wipDescription": "marker suppression regression",
				"worktreeMode":   "worktree",
			}, "use a worktree")
			proctest.RequireStep(t, serve, "workTarget")
			serve = session.Report(t, instance, map[string]any{
				"workBranch":   "feature",
				"worktreeMode": test.marker,
			})
			proctest.RequireStep(t, serve, "work")

			serve = session.Answer(t, instance, "work", "conclude", nil, "contract met")
			proctest.RequireStep(t, serve, "record")
			serve = session.Report(t, instance, map[string]any{"doneEntry": implDoneID})
			proctest.RequireStep(t, serve, "landing")
			serve = session.Answer(t, instance, "landing", "landed", nil, "landed successfully")
			proctest.RequireStep(t, serve, "closeout")
			assertBindingClearSelfGuard(t, serve.Instructions)

			// A re-attach replays the stored session: the suppressed marker
			// must not cost the closeout its clear guidance.
			replayed := resumedInstanceServe(t, world, session.ID, "impl-suppressed-replay-"+test.name, instance)
			proctest.RequireStep(t, replayed, "closeout")
			assertBindingClearSelfGuard(t, replayed.Instructions)
		})
	}
}

// TestImplementation_DispatchSeedsChildren is the behavioral port of the old
// spec-level dispatch-declaration test: conclude's capture child inherits the
// grounding (asserted inside captureDone), and closeout's evaluate child is
// anchored on the fresh done signal.
func TestImplementation_DispatchSeedsChildren(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(implAnchorEntry()))
	session := world.Open(t, "impl-dispatch")
	serve := startImplementationAtWork(t, session)
	instance := serve.Instance

	serve = session.Answer(t, instance, "work", "conclude", nil, "contract met")
	proctest.RequireStep(t, serve, "record")
	doneID := captureDone(t, session, instance)
	serve = session.Report(t, instance, map[string]any{"doneEntry": doneID})
	proctest.RequireStep(t, serve, "landing")
	serve = session.Answer(t, instance, "landing", "landed", nil, "merged successfully")
	proctest.RequireStep(t, serve, "closeout")
	serve = session.Answer(t, instance, "closeout", "evaluate", nil, "evaluate it")
	proctest.RequireStatus(t, serve, "completed")

	child := startImplChild(t, session, "evaluate", instance)
	proctest.RequireStep(t, child, "scope")
	if slices.Contains(child.Missing, "widenReport") {
		t.Fatalf("evaluate dispatch should seed widenReport, missing = %v", child.Missing)
	}
	if !strings.Contains(child.Instructions, doneID) {
		t.Fatalf("evaluate scope should serve the seeded done anchor's chains:\n%s", child.Instructions)
	}
}
