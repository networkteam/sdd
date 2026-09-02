package main

import (
	"context"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/sdd/pkg/mcpapp"
)

type graphStore struct{}

func (graphStore) Current(context.Context) (*sdd.Snapshot, error) { return &sdd.Snapshot{}, nil }
func (graphStore) Apply(context.Context, string, sdd.MutationBatch, sdd.StagedBlobReader) (sdd.ApplyResult, error) {
	return sdd.ApplyResult{State: sdd.MutationApplied, Revision: "r2"}, nil
}
func (graphStore) Reconcile(context.Context, string, string) (sdd.ApplyResult, error) {
	return sdd.ApplyResult{State: sdd.MutationApplied, Revision: "r2"}, nil
}
func (graphStore) ReadAttachmentPage(context.Context, string, string, int64, int) (sdd.AttachmentPage, error) {
	return sdd.AttachmentPage{}, nil
}

type embedder struct{}

func (embedder) Fingerprint() string { return "example" }
func (embedder) Embed(context.Context, embed.Request) (embed.Result, error) {
	return embed.Result{}, nil
}

type indexStore struct{}

func (indexStore) Manifest(context.Context, sdd.IndexNamespace) ([]sdd.StoredChunkRef, error) {
	return nil, nil
}
func (indexStore) Reconcile(context.Context, sdd.IndexNamespace, string, []sdd.IndexedChunk, []string) error {
	return nil
}
func (indexStore) Nearest(context.Context, []sdd.IndexNamespace, []float32, int) ([]sdd.ScoredChunkHit, error) {
	return nil, nil
}

type llmRunner struct{}

func (llmRunner) Run(context.Context, llm.Request) (llm.Result, error) {
	return llm.Result{Identity: llm.Identity{Provider: "example", Model: "stub"}}, nil
}

type accessResolver struct{ runtime *sdd.ProjectRuntime }

func (accessResolver) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject, Participant: "Example"}, nil
}
func (accessResolver) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{}, nil
}
func (r accessResolver) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return r.runtime, nil
}
func (r accessResolver) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return r.runtime, nil
}

type finalizer struct{}

func (finalizer) Name() string                                        { return "example" }
func (finalizer) Finalize(context.Context, sdd.AppliedMutation) error { return nil }

var (
	_ sdd.GraphStore        = graphStore{}
	_ sdd.SessionStore      = (*memorySessionStore)(nil)
	_ sdd.StagedBlobStore   = (*memoryStagedBlobStore)(nil)
	_ embed.Embedder        = embedder{}
	_ sdd.SearchIndexStore  = indexStore{}
	_ llm.Runner            = llmRunner{}
	_ sdd.AccessResolver    = accessResolver{}
	_ sdd.MutationFinalizer = finalizer{}
)

func TestExternalCompositionCompilesAgainstPublicPorts(t *testing.T) {
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project:     sdd.ProjectRef{ID: "example", DisplayName: "Example"},
		Graph:       graphStore{},
		Sessions:    newMemorySessionStore(),
		StagedBlobs: newMemoryStagedBlobStore(nil),
		Embedder:    embedder{},
		SearchIndex: indexStore{},
		LLM:         llmRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Project().ID != "example" {
		t.Fatalf("project = %+v", runtime.Project())
	}
	application, err := sdd.NewApplication(accessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	server, err := mcpapp.New(mcpapp.Options{Application: application, Project: "example", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	if handler == nil {
		t.Fatal("shared HTTP handler is nil")
	}
}
