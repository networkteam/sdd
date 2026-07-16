package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/mcpapp"
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

type sessionStore struct{}

func (sessionStore) Create(context.Context, sdd.SessionMetadata) (sdd.StoredSession, error) {
	return sdd.StoredSession{}, nil
}
func (sessionStore) Load(context.Context, sdd.SessionID) (sdd.StoredSession, error) {
	return sdd.StoredSession{}, nil
}
func (sessionStore) List(context.Context, sdd.SessionFilter) ([]sdd.StoredSession, error) {
	return nil, nil
}
func (sessionStore) Append(context.Context, sdd.SessionID, uint64, sdd.SessionAppend) (uint64, error) {
	return 1, nil
}

type blobStore struct{}

func (blobStore) Stage(context.Context, sdd.BlobOwner, string, io.Reader) (sdd.StagedBlob, error) {
	return sdd.StagedBlob{}, nil
}
func (blobStore) Stat(context.Context, sdd.BlobOwner, string) (sdd.StagedBlob, error) {
	return sdd.StagedBlob{}, nil
}
func (blobStore) Open(context.Context, sdd.BlobOwner, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (blobStore) Retain(context.Context, sdd.BlobOwner, string, []string) error { return nil }
func (blobStore) Release(context.Context, sdd.BlobOwner, string) error          { return nil }

type embeddingExecutor struct{}

func (embeddingExecutor) Spec(context.Context) (sdd.EmbeddingSpec, error) {
	return sdd.EmbeddingSpec{Fingerprint: "example"}, nil
}
func (embeddingExecutor) Embed(context.Context, []sdd.EmbeddingInput) ([]sdd.EmbeddingVector, error) {
	return nil, nil
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

type llmExecutor struct{}

func (llmExecutor) Capabilities(context.Context) ([]string, error) { return nil, nil }
func (llmExecutor) Execute(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
	return sdd.LLMResult{ExecutorFingerprint: "example"}, nil
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
	_ sdd.SessionStore      = sessionStore{}
	_ sdd.StagedBlobStore   = blobStore{}
	_ sdd.EmbeddingExecutor = embeddingExecutor{}
	_ sdd.SearchIndexStore  = indexStore{}
	_ sdd.LLMExecutor       = llmExecutor{}
	_ sdd.AccessResolver    = accessResolver{}
	_ sdd.MutationFinalizer = finalizer{}
)

func TestExternalCompositionCompilesAgainstPublicPorts(t *testing.T) {
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project:     sdd.ProjectRef{ID: "example", DisplayName: "Example"},
		Graph:       graphStore{},
		Sessions:    sessionStore{},
		StagedBlobs: blobStore{},
		Embeddings:  embeddingExecutor{},
		SearchIndex: indexStore{},
		LLM:         llmExecutor{},
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
	var handler http.Handler = server.Handler()
	if handler == nil {
		t.Fatal("shared HTTP handler is nil")
	}
}
