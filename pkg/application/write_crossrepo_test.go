package application_test

import (
	"context"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// The application write path runs pre-flight against the runtime's declared
// dependencies: a cross-repo ref into a declared, resolvable dependency is
// legitimate and draws no cross-repo finding. Regression for s-tac-uya, where
// the pre-flight finder was built without config and blocked every such ref
// as undeclared.
func TestCreateEntry_DeclaredCrossRepoRefPassesPreflight(t *testing.T) {
	foreignSnapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{
		Project: "example.org/dep", Revision: "r1",
		Entries: []sdd.EntryDocument{{
			LogicalPath: "2026/07/13-040000-s-tac-dep.md",
			Frontmatter: map[string]any{"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Foreign dependency fixture."},
			Body:        "The foreign target entry.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	llmRunner := pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
		identity := pkgllm.Identity{Provider: "test", Model: "test"}
		if request.Purpose == pkgllm.PurposePreflight {
			return pkgllm.Result{Text: `{"findings":[]}`, Identity: identity}, nil
		}
		return pkgllm.Result{Text: "A gap grounded in a foreign entry.", Identity: identity}, nil
	})
	dependency, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example.org/dep"}, Graph: staticGraphStore{snapshot: foreignSnapshot},
		LLM: llmRunner,
	})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
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
	base, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Dependencies: []string{"example.org/dep"},
		Graph: graph, LLM: llmRunner,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{base: base, dependency: dependency}, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "cross-repo-write")

	result, err := application.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Confidence: "high",
		Body: "An observation grounded in a foreign entry (example.org/dep:20260713-040000-s-tac-dep).",
		Refs: []sdd.EntryRef{{ID: "example.org/dep:20260713-040000-s-tac-dep", Kind: "grounded-in"}},
	})
	if err != nil {
		t.Fatalf("CreateEntry = %+v, err %v", result, err)
	}
	for _, f := range result.Findings {
		if f.Category == "cross-repo-dep-undeclared" || f.Category == "cross-repo-ref-unresolved" {
			t.Errorf("declared, resolvable cross-repo ref must pass pre-flight, got finding %+v", f)
		}
	}
}
