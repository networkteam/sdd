package application_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	localadapter "github.com/networkteam/sdd/local"
)

// End-to-end write-path coverage for the capture procedure's `supersedes`
// param. The engine's guards see the param through a shape-tolerant helper, so
// it clears resolve-or-block and the inspection gate and renders at playback;
// only the write path's reader decides whether it persists. A reader that
// misses the declared shape drops a confirmed supersede in silence and leaves
// both entries active (20260812-140802-s-tac-exr).

const (
	supersedeTargetID  = "20260717-130000-s-tac-old"
	supersedeSecondID  = "20260717-130200-s-tac-oldr"
	supersedeGroundID  = "20260717-130500-s-tac-gnd"
	supersedeGroundRef = "the observation every reading rests on"
)

func TestWorkflowCaptureSupersedesPersistsThroughRealNewEntry(t *testing.T) {
	e := runSupersedeCapture(t, []any{supersedeTargetID})
	if len(e.Supersedes) != 1 || e.Supersedes[0] != supersedeTargetID {
		t.Fatalf("supersedes = %v, want [%s] — the confirmed edge was dropped at the write path", e.Supersedes, supersedeTargetID)
	}
}

// A multi-target supersede must be expressible through the engine — retiring
// two heads with one replacement, without falling back to the CLI
// (20260731-083311-s-tac-4vx).
func TestWorkflowCaptureSupersedesPersistsMultipleTargets(t *testing.T) {
	e := runSupersedeCapture(t, []any{supersedeTargetID, supersedeSecondID})
	if len(e.Supersedes) != 2 || e.Supersedes[0] != supersedeTargetID || e.Supersedes[1] != supersedeSecondID {
		t.Fatalf("supersedes = %v, want [%s %s]", e.Supersedes, supersedeTargetID, supersedeSecondID)
	}
}

// runSupersedeCapture drives a fact draft through the real capture procedure —
// start with the `supersedes` param, report, playback confirm, summary verify —
// and returns the persisted entry.
func runSupersedeCapture(t *testing.T, supersedes []any) *model.Entry {
	t.Helper()
	dir := t.TempDir()
	writeSupersedeWorkflowTarget(t, dir, supersedeTargetID, "The earlier reading this capture replaces.")
	writeSupersedeWorkflowTarget(t, dir, supersedeSecondID, "The other earlier reading this capture replaces.")
	writeSupersedeWorkflowTarget(t, dir, supersedeGroundID, supersedeGroundRef)

	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: dir})
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
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				if request.Purpose == "preflight" || request.Purpose == "writing-guide" {
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				}
				return sdd.LLMResult{Output: []byte("The replacement fact carrying the current reading.")}, nil
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
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "supersede-workflow"})
	if err != nil {
		t.Fatal(err)
	}

	// Every lifecycle and ref target must be served at full depth or the
	// inspection gate holds the draft at assemble.
	if err := workflow.LogRead(t.Context(), identity, "show", []string{supersedeTargetID, supersedeSecondID, supersedeGroundID}, nil); err != nil {
		t.Fatal(err)
	}

	started, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{
		Canonical: "capture",
		Params:    map[string]any{"supersedes": supersedes},
	})
	if err != nil {
		t.Fatalf("start capture: %v", err)
	}
	instance := started.Instance

	serve := advanceWorkflow(t, workflow, identity, instance, map[string]any{
		"body":        "The replacement fact carrying the current reading, retiring the earlier one.",
		"entryKind":   "fact",
		"layer":       "tactical",
		"confidence":  "high",
		"widenReport": "read the superseded entries in full before drafting their replacement",
		"topics":      []any{"type-system/lifecycle"},
		"refs": []any{
			map[string]any{"id": supersedeGroundID, "kind": "grounded-in", "desc": supersedeGroundRef},
		},
	})
	if serve.Step != "playback" {
		t.Fatalf("supersede report should reach playback, got %q (missing %v, diagnostics %v)", serve.Step, serve.Missing, serve.Diagnostics)
	}
	// Playback is the verification contract: the user confirms what they see,
	// so every target must be rendered among the labelled fields.
	for _, id := range supersedes {
		if !strings.Contains(serve.Instructions, id.(string)) {
			t.Errorf("playback does not render supersede target %s:\n%s", id, serve.Instructions)
		}
	}

	serve = advanceWorkflow(t, workflow, identity, instance, map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "confirm",
	})
	if serve.Step != "verifySummary" {
		t.Fatalf("supersede confirm should reach verifySummary, got %q", serve.Step)
	}

	serve = advanceWorkflow(t, workflow, identity, instance, map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "fields": map[string]any{"fidelityNote": "faithful"},
	})
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}
	return loadEntryByID(t, dir, entryID)
}

func writeSupersedeWorkflowTarget(t *testing.T, dir, id, summary string) {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: signal\nkind: fact\nlayer: tactical\nsummary: " + summary + "\n---\n\n" + summary + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
