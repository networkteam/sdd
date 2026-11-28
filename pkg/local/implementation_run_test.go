package local_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/local"
)

const runAnchorID = "20260601-120000-d-tac-ref"

// servedRepository is a Git repository with committed SDD configuration and an
// anchor entry on main, composed the way `sdd serve` composes its checkout.
func servedRepository(t *testing.T) (*gitRepository, *sdd.WorkflowSession, sdd.RequestIdentity) {
	t.Helper()
	repo := newGitRepository(t)
	repo.write(".sdd/config.yaml", "repo_id: example\ngraph_dir: .sdd/graph\ndefault_branch: main\n")
	repo.write(".sdd/graph/2026/06/01-120000-d-tac-ref.md", "---\ntype: decision\nkind: directive\nlayer: tactical\nintent: pending\nsummary: A directive the run implements.\n---\n\nA directive the run implements.\n")
	repo.git("add", ".sdd")
	repo.git("commit", "-m", "test: add the anchor")
	repo.write("global-config.yaml", "")

	graph, err := local.NewFilesystemGraphStore(local.FilesystemGraphStoreOptions{Project: "example", GraphDir: filepath.Join(repo.root, ".sdd", "graph")})
	if err != nil {
		t.Fatal(err)
	}
	targets := repo.targets()
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph:   local.BranchReadStore{GraphStore: graph, Branches: targets, DefaultBranch: "main"},
		Targets: targets, Branches: targets, Base: targets,
		LLM: pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
			switch request.Purpose {
			case pkgllm.PurposePreflight, pkgllm.PurposeWritingGuide:
				return pkgllm.Result{Text: `{"findings":[]}`}, nil
			case pkgllm.PurposeSummarize, pkgllm.PurposeTitle:
				return pkgllm.Result{Text: `{"title": "Test title", "summary": "The run delivered the anchor."}`}, nil
			}
			return pkgllm.Result{}, fmt.Errorf("unexpected LLM purpose %q", request.Purpose)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := local.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := local.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: runtimeAccess{runtime: runtime}, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{ClientName: "implementation-run"})
	if err != nil {
		t.Fatal(err)
	}
	return repo, workflow, identity
}

func advance(t *testing.T, workflow *sdd.WorkflowSession, identity sdd.RequestIdentity, instance string, report map[string]any) *sdd.WorkflowServe {
	t.Helper()
	serve, err := workflow.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: instance, Report: report})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

func requireFramingBase(t *testing.T, workflow *sdd.WorkflowSession, identity sdd.RequestIdentity, want string) {
	t.Helper()
	framing, err := workflow.Framing(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(framing[0], want) {
		t.Fatalf("framing info block = %q, want %q", framing[0], want)
	}
}

// markersOn lists the WIP marker files committed on a branch.
func (r *gitRepository) markersOn(branch string) []string {
	r.t.Helper()
	listing := r.git("ls-tree", "--name-only", branch, ".sdd/graph/wip/")
	if listing == "" {
		return nil
	}
	return strings.Split(listing, "\n")
}

func (r *gitRepository) hasPath(branch, path string) bool {
	r.t.Helper()
	return r.git("ls-tree", "--name-only", branch, path) != ""
}

// TestImplementationRunMarkerIsGoneFromMainAfterTheMerge is the fulfilment
// test of 20260923-230855-d-cpt-34w: a tracked run on a two-branch repository
// writes its marker on main at setup, the host branches off, the closing done
// and the marker's removal land on the work branch, and the merge carries both
// to main. In branch mode the serving checkout switches branch, so the derived
// base follows it and nothing is declared (20260923-233057-d-cpt-ekd); in
// worktree mode the agent declares the worktree's branch.
func TestImplementationRunMarkerIsGoneFromMainAfterTheMerge(t *testing.T) {
	for _, mode := range []string{"branch", "worktree"} {
		t.Run(mode, func(t *testing.T) {
			repo, workflow, identity := servedRepository(t)
			requireFramingBase(t, workflow, identity, "Base branch: main (the serving checkout's branch)")

			run, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "implementation", Params: map[string]any{"anchor": runAnchorID}})
			if err != nil {
				t.Fatal(err)
			}
			instance := run.Instance
			if err := workflow.LogRead(t.Context(), identity, "show", []string{runAnchorID}, nil); err != nil {
				t.Fatal(err)
			}
			if run = advance(t, workflow, identity, instance, map[string]any{"contract": "deliver the anchor", "widenReport": "anchor inspected"}); run.Step != "setup" {
				t.Fatalf("step after the contract = %q, want setup", run.Step)
			}
			run = advance(t, workflow, identity, instance, map[string]any{
				"chooser": "setup", "choice": mode, "userWords": "go",
				"fields": map[string]any{"wipDescription": "deliver the anchor"},
			})
			if run.Step != "work" {
				t.Fatalf("step after setup = %q, want work", run.Step)
			}
			if markers := repo.markersOn("main"); len(markers) != 1 {
				t.Fatalf("markers on main after setup = %v, want one", markers)
			}

			// Host work: branch off after the marker commit.
			switch mode {
			case "branch":
				repo.git("switch", "-c", "feature")
				requireFramingBase(t, workflow, identity, "Base branch: feature (the serving checkout's branch)")
			case "worktree":
				repo.git("worktree", "add", "-b", "feature", filepath.Join(canonicalTempDir(t), "feature"))
				if err := workflow.BindBranch(t.Context(), identity, "feature", false); err != nil {
					t.Fatal(err)
				}
			}

			if run = advance(t, workflow, identity, instance, map[string]any{"chooser": "work", "choice": "conclude", "userWords": "contract met"}); run.Step != "record" {
				t.Fatalf("step after conclude = %q, want record", run.Step)
			}
			capture, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "capture", Parent: instance})
			if err != nil {
				t.Fatal(err)
			}
			for _, report := range []map[string]any{
				{
					"body": "Delivered the anchor directive in commit abc1234.", "entryKind": "done", "layer": "tactical",
					"closes": []any{runAnchorID}, "topics": []any{"implementation/engine"}, "confidence": "high",
				},
				{"chooser": "playback", "choice": "confirm", "userWords": "confirm"},
			} {
				if serve := advance(t, workflow, identity, capture.Instance, report); serve.PendingChooser == nil {
					t.Fatalf("capture at %q serves no chooser: %+v", serve.Step, serve)
				}
			}
			serve := advance(t, workflow, identity, capture.Instance, map[string]any{"chooser": "verifySummary", "choice": "faithful", "fields": map[string]any{"fidelityNote": "faithful"}})
			doneID, _ := serve.Produced["entryId"].(string)
			if doneID == "" {
				t.Fatalf("capture produced = %+v", serve.Produced)
			}
			run = advance(t, workflow, identity, instance, map[string]any{"doneEntry": doneID})
			if run.Step != "closeout" {
				t.Fatalf("step after the done = %q, want closeout", run.Step)
			}
			donePath := ".sdd/graph/" + strings.Join([]string{doneID[:4], doneID[4:6], doneID[6:8] + doneID[8:]}, "/") + ".md"
			if !repo.hasPath("feature", donePath) || repo.hasPath("main", donePath) {
				t.Fatalf("the done %s must be committed on feature only", doneID)
			}
			if markers := repo.markersOn("feature"); len(markers) != 0 {
				t.Fatalf("markers on feature after the done = %v, want none", markers)
			}
			if markers := repo.markersOn("main"); len(markers) != 1 {
				t.Fatalf("main must show the work as taken until the merge, markers = %v", markers)
			}

			// Host work: back on main, merge.
			if mode == "branch" {
				repo.git("switch", "main")
			} else if err := workflow.BindBranch(t.Context(), identity, "", true); err != nil {
				t.Fatal(err)
			}
			repo.git("merge", "--no-ff", "-m", "merge feature", "feature")
			if markers := repo.markersOn("main"); len(markers) != 0 {
				t.Fatalf("markers on main after the merge = %v, want none", markers)
			}
			if !repo.hasPath("main", donePath) {
				t.Fatalf("the merge did not carry the done %s to main", doneID)
			}
			run = advance(t, workflow, identity, instance, map[string]any{"chooser": "closeout", "choice": "finish", "userWords": "merged"})
			if run.Status != "completed" {
				t.Fatalf("run status = %q, want completed", run.Status)
			}
		})
	}
}

// A detached serving checkout has no branch to derive, so the base is the
// configured default (20260923-233057-d-cpt-ekd).
func TestDetachedServingCheckoutFallsBackToTheDefaultBranch(t *testing.T) {
	repo, workflow, identity := servedRepository(t)
	targets := repo.targets()
	if base, err := targets.BaseBranch(t.Context()); err != nil || base != "main" {
		t.Fatalf("base = %q, %v; want main", base, err)
	}
	repo.git("switch", "--detach")
	if base, err := targets.BaseBranch(t.Context()); err != nil || base != "" {
		t.Fatalf("detached base = %q, %v; want none", base, err)
	}
	// No checkout has main any more: the framing says so and names the remedy.
	requireFramingBase(t, workflow, identity, "Base branch: main (configured default; the serving checkout has no branch)")
	requireFramingBase(t, workflow, identity, "No checkout has the base branch main: declare the branch you work on before writing.")

	// An unbound write fails and names both remedies.
	run, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "implementation", Params: map[string]any{"anchor": runAnchorID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.LogRead(t.Context(), identity, "show", []string{runAnchorID}, nil); err != nil {
		t.Fatal(err)
	}
	advance(t, workflow, identity, run.Instance, map[string]any{"contract": "deliver the anchor", "widenReport": "anchor inspected"})
	failed := advance(t, workflow, identity, run.Instance, map[string]any{
		"chooser": "setup", "choice": "inPlace", "userWords": "go",
		"fields": map[string]any{"wipDescription": "deliver the anchor"},
	})
	if failed.PendingOperation == nil || !strings.Contains(failed.PendingOperation.Error, `the session's base branch "main" has no usable checkout; check that branch out and retry, or declare the branch you work on`) {
		t.Fatalf("unbound write on a base without a checkout = %+v, want the remedies named", failed.PendingOperation)
	}
}
