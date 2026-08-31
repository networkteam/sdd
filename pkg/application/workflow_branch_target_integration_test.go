package application_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	sdd "github.com/networkteam/sdd/pkg/application"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

type workflowBranchTargets struct{ graphs map[string]sdd.GraphStore }

func (a workflowBranchTargets) Acquire(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
	graph := a.graphs[target.Branch]
	if graph == nil {
		return nil, fmt.Errorf("branch %q is not registered", target.Branch)
	}
	return &sdd.AcquiredTarget{
		Target: target,
		Graph:  graph,
		Release: func() error {
			return nil
		},
	}, nil
}

func TestWorkflowBranchTargetCarriesCaptureReadsThroughImplementationLanding(t *testing.T) {
	const anchorID = "20260717-121000-s-tac-anc"
	baseDir := t.TempDir()
	workDir := t.TempDir()
	explicitDir := t.TempDir()
	writeWorkflowBranchEntry(t, baseDir, `---
type: signal
kind: gap
layer: tactical
summary: Branch-target read gap.
---

Branch-targeted workflow reads need to follow the written artifact.
`)
	writeWorkflowBranchEntry(t, workDir, `---
type: signal
kind: gap
layer: tactical
summary: Branch-target read gap.
---

Branch-targeted workflow reads need to follow the written artifact.
`)
	writeWorkflowBranchEntry(t, explicitDir, `---
type: signal
kind: gap
layer: tactical
summary: Branch-target read gap.
---

Branch-targeted workflow reads need to follow the written artifact.
`)
	base, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	work, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: explicitDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project:       sdd.ProjectRef{ID: "example"},
		DefaultBranch: "main",
		Graph:         base,
		Targets: workflowBranchTargets{graphs: map[string]sdd.GraphStore{
			"main":     base,
			"work":     work,
			"explicit": explicit,
		}},
		Branches: sdd.BranchValidatorFunc(func(_ context.Context, target sdd.MutationTarget) error {
			if target.Branch != "main" && target.Branch != "work" && target.Branch != "explicit" {
				return fmt.Errorf("branch %q is not registered", target.Branch)
			}
			return nil
		}),
		Sessions:    sessions,
		StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				switch request.Purpose {
				case "preflight", "writing-guide":
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				case "summary":
					return sdd.LLMResult{Output: []byte("Work branch done summary.")}, nil
				default:
					return sdd.LLMResult{}, fmt.Errorf("unexpected LLM purpose %q", request.Purpose)
				}
			},
		},

		LLMTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "branch-target"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.BindBranch(t.Context(), identity, "work", false); err != nil {
		t.Fatal(err)
	}

	implementation, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{
		Canonical: "implementation", Params: map[string]any{"anchor": anchorID},
	})
	if err != nil {
		t.Fatal(err)
	}
	implementation = advanceWorkflow(t, workflow, identity, implementation.Instance, map[string]any{
		"contract": "target-aware reads through landing", "widenReport": "anchor inspected",
	})
	implementation = advanceWorkflow(t, workflow, identity, implementation.Instance, map[string]any{"baseBranch": "main"})
	implementation = advanceWorkflow(t, workflow, identity, implementation.Instance, map[string]any{
		"chooser": "setup", "choice": "worktree", "userWords": "use a worktree",
		"fields": map[string]any{"wipDescription": "target-aware workflow reads", "worktreeMode": "worktree"},
	})
	implementation = advanceWorkflow(t, workflow, identity, implementation.Instance, map[string]any{"workBranch": "explicit"})
	implementation = advanceWorkflow(t, workflow, identity, implementation.Instance, map[string]any{
		"chooser": "work", "choice": "conclude", "userWords": "implementation complete",
	})
	if implementation.Step != "record" {
		t.Fatalf("implementation step = %q, want record", implementation.Step)
	}

	captureStart, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{
		Canonical: "capture", Parent: implementation.Instance,
	})
	if err != nil {
		t.Fatal(err)
	}
	capture := captureStart.Instance
	captureServe := advanceWorkflow(t, workflow, identity, capture, map[string]any{
		"body":        "The branch-target read regression is implemented and verified against the tracked flow.",
		"entryKind":   "done",
		"layer":       "tactical",
		"refs":        []any{map[string]any{"id": anchorID, "kind": "addresses"}},
		"topics":      []any{"implementation/engine"},
		"confidence":  "high",
		"widenReport": "anchor inspected",
	})
	if captureServe.Step != "playback" {
		t.Fatalf("capture step = %q, want playback", captureServe.Step)
	}
	if statement := playbackTargetStatement(t, captureServe); !strings.Contains(statement, "explicit") || !strings.Contains(statement, "captureBranch") {
		t.Fatalf("playback target statement = %q, want the explicitly seeded work branch", statement)
	}
	captureServe = advanceWorkflow(t, workflow, identity, capture, map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "confirm",
	})
	if captureServe.Step != "verifySummary" || !strings.Contains(captureServe.Instructions, "Work branch done summary.") {
		t.Fatalf("post-write capture serve = step %q instructions %q", captureServe.Step, captureServe.Instructions)
	}

	resumed, err := workflow.ServeAll(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	resumeFound := false
	for _, serve := range resumed.Open {
		if serve.Instance == capture && serve.Step == "verifySummary" && strings.Contains(serve.Instructions, "Work branch done summary.") {
			resumeFound = true
		}
	}
	if !resumeFound {
		t.Fatal("resume did not re-serve the work-branch summary verification")
	}

	captureServe = advanceWorkflow(t, workflow, identity, capture, map[string]any{
		"chooser": "verifySummary", "choice": "faithful",
		"fields": map[string]any{"fidelityNote": "faithful"},
	})
	doneEntry, _ := captureServe.Produced["entryId"].(string)
	if doneEntry == "" {
		t.Fatalf("capture produced = %+v", captureServe.Produced)
	}
	implementation = advanceWorkflow(t, workflow, identity, implementation.Instance, map[string]any{"doneEntry": doneEntry})
	if implementation.Step != "landing" {
		t.Fatalf("work-branch done should reach landing before merge, got %q", implementation.Step)
	}
	donePath, err := model.IDToRelPath(doneEntry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(explicitDir, donePath)); err != nil {
		t.Fatalf("explicit capture-branch done file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, donePath)); !os.IsNotExist(err) {
		t.Fatalf("base branch unexpectedly contains done file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, donePath)); !os.IsNotExist(err) {
		t.Fatalf("session-bound branch unexpectedly contains explicit capture file: %v", err)
	}
	if got := workflowBranchFileCount(t, filepath.Join(baseDir, "wip")); got != 1 {
		t.Fatalf("base WIP markers = %d, want 1", got)
	}
	if got := workflowBranchFileCount(t, filepath.Join(workDir, "wip")); got != 0 {
		t.Fatalf("work WIP markers = %d, want 0", got)
	}
}

func TestWorkflowOrdinaryCaptureUsesBindingAndUnboundUsesDefault(t *testing.T) {
	const anchorID = "20260717-121000-s-tac-anc"
	const workOnlyID = "20260717-121001-s-tac-wrk"
	currentDir := t.TempDir()
	mainDir := t.TempDir()
	workDir := t.TempDir()
	writeWorkflowBranchEntry(t, currentDir, `---
type: signal
kind: gap
layer: tactical
summary: Runtime-current-only routing anchor.
---

Runtime-current-only evidence is visible to ordinary unbound reads.
`)
	writeWorkflowBranchEntry(t, mainDir, `---
type: signal
kind: gap
layer: tactical
summary: Default-target-only routing anchor.
---

Default-target-only evidence is the ordinary unbound write authority.
`)
	writeWorkflowBranchEntry(t, workDir, `---
type: signal
kind: gap
layer: tactical
summary: Shared routing anchor.
---

The shared routing anchor exists on the work branch.
`)
	writeWorkflowBranchEntryAt(t, workDir, "2026/07/17-121001-s-tac-wrk.md", `---
type: signal
kind: gap
layer: tactical
summary: Work-exclusive reference anchor.
---

This reference exists only on the bound work branch.
`)
	currentGraph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: currentDir})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: mainDir})
	if err != nil {
		t.Fatal(err)
	}
	workGraph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	targetGraphs := map[string]sdd.GraphStore{"main": mainGraph, "work": workGraph}
	rejectNextPreflight := false
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: currentGraph,
		Targets: workflowBranchTargets{graphs: targetGraphs},
		Branches: sdd.BranchValidatorFunc(func(_ context.Context, target sdd.MutationTarget) error {
			if targetGraphs[target.Branch] == nil {
				return fmt.Errorf("branch %q is not registered", target.Branch)
			}
			return nil
		}),
		Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				switch request.Purpose {
				case "preflight":
					if rejectNextPreflight {
						rejectNextPreflight = false
						return sdd.LLMResult{Output: []byte(`{"findings":[{"severity":"high","category":"retry-route","observation":"reject this first attempt"}]}`)}, nil
					}
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				case "writing-guide":
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				case "summary":
					return sdd.LLMResult{Output: []byte("Generated routing summary.")}, nil
				default:
					return sdd.LLMResult{}, fmt.Errorf("unexpected LLM purpose %q", request.Purpose)
				}
			},
		},

		LLMTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}

	// Unbound reads deliberately stay on runtime.Graph even when the default
	// write target points at a different checkout.
	currentRead, err := application.Search(t.Context(), identity, "example", sdd.SearchRequest{Terms: []string{"runtime-current-only"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(currentRead.Results, anchorID) {
		t.Fatalf("unbound read did not use runtime graph: %q", currentRead.Results)
	}
	defaultRead, err := application.Search(t.Context(), identity, "example", sdd.SearchRequest{Terms: []string{"default-target-only"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(defaultRead.Results, anchorID) {
		t.Fatalf("unbound read leaked configured default target: %q", defaultRead.Results)
	}

	bound, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "bound-capture"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bound.BindBranch(t.Context(), identity, "work", false); err != nil {
		t.Fatal(err)
	}
	// The ref is branch-exclusive: both the engine predicate and application
	// pre-flight must validate against the bound work snapshot.
	boundID := runRoutingCapture(t, bound, identity, workOnlyID, "The bound ordinary capture is written to the work branch.", "Corrected bound summary.", true, "work")
	assertWorkflowEntryLocation(t, boundID, workDir, true)
	assertWorkflowEntryLocation(t, boundID, mainDir, false)
	boundPath, err := model.IDToRelPath(boundID)
	if err != nil {
		t.Fatal(err)
	}
	boundBytes, err := os.ReadFile(filepath.Join(workDir, boundPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(boundBytes), "Corrected bound summary.") {
		t.Fatalf("summary replacement did not use the bound target:\n%s", boundBytes)
	}

	unbound, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "unbound-capture"})
	if err != nil {
		t.Fatal(err)
	}
	unboundID := runRoutingCapture(t, unbound, identity, anchorID, "The unbound ordinary capture is written to the configured default branch.", "", false, "")
	assertWorkflowEntryLocation(t, unboundID, mainDir, true)
	assertWorkflowEntryLocation(t, unboundID, workDir, false)
	assertWorkflowEntryLocation(t, unboundID, currentDir, false)

	// A rejected default-target attempt must not pin that target into the
	// instance. Binding the session before retrying must route the eventual
	// artifact and its summary read to work.
	retry, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "rejected-then-bound"})
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.LogRead(t.Context(), identity, "show", []string{anchorID}, nil); err != nil {
		t.Fatal(err)
	}
	serve, err := retry.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	serve = advanceWorkflow(t, retry, identity, serve.Instance, map[string]any{
		"body": "The rejected unbound capture retries against the newly bound work branch.", "entryKind": "gap", "layer": "tactical",
		"refs":   []any{map[string]any{"id": anchorID, "kind": "related", "desc": "shared routing anchor"}},
		"topics": []any{"testing/routing"}, "confidence": "high",
		"widenReport": "inspected " + anchorID + " before drafting",
	})
	rejectNextPreflight = true
	serve = advanceWorkflow(t, retry, identity, serve.Instance, map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "try the write",
	})
	if serve.Step != "reviseOrOverride" {
		t.Fatalf("rejected default attempt step = %q, want reviseOrOverride", serve.Step)
	}
	if err := retry.BindBranch(t.Context(), identity, "work", false); err != nil {
		t.Fatal(err)
	}
	serve = advanceWorkflow(t, retry, identity, serve.Instance, map[string]any{
		"chooser": "reviseOrOverride", "choice": "override", "userWords": "retry on the bound work branch",
	})
	if serve.Step != "verifySummary" {
		t.Fatalf("bound retry step = %q, want verifySummary", serve.Step)
	}
	serve = advanceWorkflow(t, retry, identity, serve.Instance, map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "fields": map[string]any{"fidelityNote": "faithful"},
	})
	retryID, _ := serve.Produced["entryId"].(string)
	if retryID == "" {
		t.Fatalf("bound retry produced = %+v", serve.Produced)
	}
	assertWorkflowEntryLocation(t, retryID, workDir, true)
	assertWorkflowEntryLocation(t, retryID, mainDir, false)
	assertWorkflowEntryLocation(t, retryID, currentDir, false)
}

// playbackTargetStatement returns the one playback line stating the branch the
// write will land on — the served half of the acceptance criterion that the
// stated target and the written artifact never diverge.
func playbackTargetStatement(t *testing.T, serve *sdd.WorkflowServe) string {
	t.Helper()
	for _, line := range strings.Split(serve.Instructions, "\n") {
		if strings.HasPrefix(line, "- target branch:") {
			return line
		}
	}
	t.Fatalf("playback stated no target branch:\n%s", serve.Instructions)
	return ""
}

func runRoutingCapture(t *testing.T, workflow *sdd.WorkflowSession, identity sdd.RequestIdentity, anchorID, body, correctedSummary string, drifted bool, wantTargetBranch string) string {
	t.Helper()
	if err := workflow.LogRead(t.Context(), identity, "show", []string{anchorID}, nil); err != nil {
		t.Fatal(err)
	}
	serve, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	serve = advanceWorkflow(t, workflow, identity, serve.Instance, map[string]any{
		"body": body, "entryKind": "gap", "layer": "tactical",
		"refs":   []any{map[string]any{"id": anchorID, "kind": "related", "desc": "shared routing anchor"}},
		"topics": []any{"testing/routing"}, "confidence": "high",
		"widenReport": "inspected " + anchorID + " before drafting",
	})
	if serve.Step != "playback" {
		t.Fatalf("capture assemble step = %q, want playback", serve.Step)
	}
	statement := playbackTargetStatement(t, serve)
	if wantTargetBranch == "" && !strings.Contains(statement, "configured default branch") {
		t.Fatalf("unbound playback target statement = %q", statement)
	}
	if wantTargetBranch != "" && !strings.Contains(statement, "session branch binding") {
		t.Fatalf("bound playback target statement = %q", statement)
	}
	if serve.Branch != wantTargetBranch {
		t.Fatalf("playback served framing branch = %q, want %q", serve.Branch, wantTargetBranch)
	}
	serve = advanceWorkflow(t, workflow, identity, serve.Instance, map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "write it",
	})
	if serve.Step != "verifySummary" {
		t.Fatalf("capture write step = %q, want verifySummary", serve.Step)
	}
	report := map[string]any{"chooser": "verifySummary", "choice": "faithful", "fields": map[string]any{"fidelityNote": "faithful"}}
	if drifted {
		report = map[string]any{"chooser": "verifySummary", "choice": "drifted", "fields": map[string]any{"correctedSummary": correctedSummary}}
	}
	serve = advanceWorkflow(t, workflow, identity, serve.Instance, report)
	entryID, _ := serve.Produced["entryId"].(string)
	if serve.Status != "completed" || entryID == "" {
		t.Fatalf("capture completion = status %q produced %+v", serve.Status, serve.Produced)
	}
	return entryID
}

func assertWorkflowEntryLocation(t *testing.T, entryID, dir string, exists bool) {
	t.Helper()
	path, err := model.IDToRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(dir, path))
	if exists && err != nil {
		t.Fatalf("entry %s missing from %s: %v", entryID, dir, err)
	}
	if !exists && !os.IsNotExist(err) {
		t.Fatalf("entry %s unexpectedly present in %s: %v", entryID, dir, err)
	}
}

func writeWorkflowBranchEntry(t *testing.T, dir, content string) {
	t.Helper()
	writeWorkflowBranchEntryAt(t, dir, "2026/07/17-121000-s-tac-anc.md", content)
}

func writeWorkflowBranchEntryAt(t *testing.T, dir, relPath, content string) {
	t.Helper()
	path := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func advanceWorkflow(t *testing.T, workflow *sdd.WorkflowSession, identity sdd.RequestIdentity, instance string, report map[string]any) *sdd.WorkflowServe {
	t.Helper()
	serve, err := workflow.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: instance, Report: report})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

func workflowBranchFileCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
