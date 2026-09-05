package application_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// Regression test for the 2026-07-13 root-runtime rebase: an embedding
// provider that discovers its dimensionality lazily (ollama without a
// configured dimensions value reports 0 until its first embed response)
// must still serve vector search. The spec gate used to reject such an
// executor with "embedding executor returned invalid spec" before any
// embed call could happen.
func TestVectorSearchWithLazyDimensionEmbedder(t *testing.T) {
	graphDir := t.TempDir()
	entryPath := filepath.Join(graphDir, "2026", "07", "13-030000-s-tac-api.md")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryPath, []byte(applicationEntry), 0o644); err != nil {
		t.Fatal(err)
	}
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: graphDir})
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
	// Mirrors the lazy ollama embedder: the spec never announces a
	// dimensionality up front — vectors reveal it on first real use.
	embeddings := embed.EmbedderFunc{Space: "lazy-test", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		vectors := make([][]float32, len(req.Texts))
		for i, text := range req.Texts {
			values := []float32{0, 1}
			if req.Purpose == embed.PurposeQuery || strings.Contains(text, "protocol-neutral") {
				values = []float32{1, 0}
			}
			vectors[i] = values
		}
		return embed.Result{Vectors: vectors}, nil
	}}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example", DisplayName: "Example"},
		Graph:   graph, Embedder: embeddings,
		SearchIndex: localadapter.NewMemorySearchIndexStore(),
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &runtimeAccessResolver{runtime: runtime}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: resolver, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher", Scopes: []string{"project:read"}}

	vector, err := application.Search(t.Context(), identity, "example", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "application runtime", Limit: 5, MaxCitations: 1})
	if err != nil {
		t.Fatalf("vector Search with lazy-dimension embedder: %v", err)
	}
	if !strings.Contains(vector.Results, "s-tac-api") {
		t.Fatalf("vector Search = %q, want hit for s-tac-api", vector.Results)
	}

	// A second search must land in the same namespace — the first call
	// discovering the dimensionality must not fork the index identity.
	again, err := application.Search(t.Context(), identity, "example", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "application runtime", Limit: 5, MaxCitations: 1})
	if err != nil || !strings.Contains(again.Results, "s-tac-api") {
		t.Fatalf("repeated vector Search = %q, %v", again.Results, err)
	}
}
