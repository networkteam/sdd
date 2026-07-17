package application_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	localadapter "github.com/networkteam/sdd/local"
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
	base, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	work, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project:       sdd.ProjectRef{ID: "example"},
		DefaultBranch: "main",
		Graph:         base,
		Targets: workflowBranchTargets{graphs: map[string]sdd.GraphStore{
			"main": base,
			"work": work,
		}},
		Sessions:    sessions,
		StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				switch request.Purpose {
				case "preflight":
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				case "summary":
					return sdd.LLMResult{Output: []byte("Work branch done summary.")}, nil
				default:
					return sdd.LLMResult{}, fmt.Errorf("unexpected LLM purpose %q", request.Purpose)
				}
			},
		},
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
		"fields": map[string]any{"wipDescription": "target-aware workflow reads"},
	})
	implementation = advanceWorkflow(t, workflow, identity, implementation.Instance, map[string]any{"workBranch": "work"})
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
	if _, err := os.Stat(filepath.Join(workDir, donePath)); err != nil {
		t.Fatalf("work-branch done file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, donePath)); !os.IsNotExist(err) {
		t.Fatalf("base branch unexpectedly contains done file: %v", err)
	}
	if got := workflowBranchFileCount(t, filepath.Join(baseDir, "wip")); got != 1 {
		t.Fatalf("base WIP markers = %d, want 1", got)
	}
	if got := workflowBranchFileCount(t, filepath.Join(workDir, "wip")); got != 0 {
		t.Fatalf("work WIP markers = %d, want 0", got)
	}
}

func writeWorkflowBranchEntry(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, "2026/07/17-121000-s-tac-anc.md")
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
