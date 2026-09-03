package application_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// newClosureFixture composes three projects: A is the home and declares B
// under the repo ID example.test/b, which differs from B's project ID as it
// does in every composition but the local one; C is readable but undeclared.
// Alice is a full member of A and B and reads C; B has no write authority of
// its own, as a local dependency has today.
func newClosureFixture(t *testing.T) (*sdd.Application, sdd.RequestIdentity) {
	t.Helper()
	root := t.TempDir()
	graphB := filepath.Join(root, "project-b")
	entryPath := filepath.Join(graphB, "2026", "07", "13-120000-s-tac-dep.md")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryPath, []byte(`---
type: signal
kind: fact
layer: tactical
confidence: high
summary: Project B dependency fixture.
---

Project B is readable as an authorized dependency.`), 0o644); err != nil {
		t.Fatal(err)
	}
	llm := pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
		return pkgllm.Result{Text: `{"findings":[]}`, Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
	})
	runtimes := map[sdd.ProjectID]*sdd.ProjectRuntime{}
	for _, project := range []struct {
		id           sdd.ProjectID
		dir          string
		dependencies []string
		branch       string
	}{
		{id: "project-a", dir: filepath.Join(root, "project-a"), dependencies: []string{"example.test/b"}, branch: "main"},
		{id: "project-b", dir: graphB},
		{id: "project-c", dir: filepath.Join(root, "project-c")},
	} {
		graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: project.id, GraphDir: project.dir})
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
			Project: sdd.ProjectRef{ID: project.id, DisplayName: string(project.id)}, Dependencies: project.dependencies,
			DefaultBranch: project.branch, Graph: graph, LLM: llm,
		})
		if err != nil {
			t.Fatal(err)
		}
		runtimes[project.id] = runtime
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	access := &compositionAccess{
		runtimes: runtimes,
		permissions: map[string]map[sdd.ProjectID]compositionPermission{
			"alice": {"project-a": {read: true, write: true}, "project-b": {read: true, write: true}, "project-c": {read: true}},
		},
		dependencies: map[string]sdd.ProjectID{"example.test/b": "project-b"},
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: access, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	return application, sdd.RequestIdentity{Subject: "alice"}
}

// TestInstanceTargetsProjectInDependencyClosure pins where an instance's
// project comes from and what gates it (d-cpt-yjc): a start call's project is
// recorded, a dispatched child derives its parent's, a plain move derives the
// home project, a project outside the home's declared closure is refused, and
// the targets survive a reload from the ledger.
func TestInstanceTargetsProjectInDependencyClosure(t *testing.T) {
	application, alice := newClosureFixture(t)
	workflow, _, err := application.OpenWorkflow(t.Context(), alice, "project-a", sdd.WorkflowOpenRequest{ClientName: "alice-mcp"})
	if err != nil {
		t.Fatal(err)
	}

	explicit, err := workflow.Start(t.Context(), alice, sdd.WorkflowStartRequest{Canonical: "capture", Project: "project-b"})
	if err != nil {
		t.Fatalf("start in dependency: %v", err)
	}
	if explicit.Project != "project-b" {
		t.Fatalf("explicit project serve = %q, want project-b", explicit.Project)
	}

	home, err := workflow.Start(t.Context(), alice, sdd.WorkflowStartRequest{Canonical: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	if home.Project != "project-a" {
		t.Fatalf("derived project serve = %q, want the home project", home.Project)
	}

	// A move dispatched from an instance in the dependency derives its project.
	child, err := workflow.Start(t.Context(), alice, sdd.WorkflowStartRequest{Canonical: "capture", Parent: explicit.Instance})
	if err != nil {
		t.Fatal(err)
	}
	if child.Project != "project-b" {
		t.Fatalf("dispatched child serve = %q, want the parent's project", child.Project)
	}

	_, err = workflow.Start(t.Context(), alice, sdd.WorkflowStartRequest{Canonical: "capture", Project: "project-c"})
	if applicationErrorCode(err) != sdd.ErrorProjectUnavailable || !strings.Contains(err.Error(), "dependency closure") {
		t.Fatalf("start outside the closure = %v, want a closure refusal", err)
	}
	// The declared repo ID is not a project: the closure holds resolved projects.
	_, err = workflow.Start(t.Context(), alice, sdd.WorkflowStartRequest{Canonical: "capture", Project: "example.test/b"})
	if applicationErrorCode(err) != sdd.ErrorProjectUnavailable || !strings.Contains(err.Error(), "dependency closure") {
		t.Fatalf("start on the declared string = %v, want a closure refusal", err)
	}

	if project, branch, fromBinding, err := workflow.ReadScope(t.Context(), alice, "project-b"); err != nil || project != "project-b" || branch != "" || fromBinding {
		t.Fatalf("read scope in dependency = %q %q %v %v, want project-b on its default", project, branch, fromBinding, err)
	}
	if _, _, _, err := workflow.ReadScope(t.Context(), alice, "project-c"); applicationErrorCode(err) != sdd.ErrorProjectUnavailable {
		t.Fatalf("read scope outside the closure = %v, want a closure refusal", err)
	}

	// A write in the dependency reaches the dependency's runtime and fails
	// there, naming it: locally a dependency has no write authority yet.
	_, err = application.CreateEntry(t.Context(), alice, "project-b", workflow.Binding(), sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Body: "A gap noticed while working in the dependency.",
	})
	if err == nil || !strings.Contains(err.Error(), "project-b") {
		t.Fatalf("write in dependency = %v, want a refusal naming project-b", err)
	}

	resumed, result, err := application.ResumeWorkflow(t.Context(), alice, sdd.WorkflowResumeRequest{SessionID: workflow.ID(), ClientName: "alice-again"})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Project() != "project-a" {
		t.Fatalf("resumed home = %q, want project-a", resumed.Project())
	}
	projects := map[string]sdd.ProjectID{}
	for _, serve := range result.Open {
		projects[serve.Instance] = serve.Project
	}
	if projects[explicit.Instance] != "project-b" || projects[home.Instance] != "project-a" || projects[child.Instance] != "project-b" {
		t.Fatalf("resumed instance projects = %v, want the recorded and derived targets restored", projects)
	}
}
