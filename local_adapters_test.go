package sdd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/networkteam/sdd"
	"github.com/networkteam/sdd/sddtest"
)

const localEntry = `---
type: signal
kind: gap
layer: tactical
confidence: high
participants:
  - Christopher
---

Local GraphStore conformance fixture.`

func TestFilesystemGraphStoreConformance(t *testing.T) {
	sddtest.RunGraphStoreTests(t, func(t *testing.T) sddtest.GraphStoreFixture {
		store, err := sdd.NewFilesystemGraphStore(sdd.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		initial, err := store.Current(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		batch := sdd.MutationBatch{
			ID: "mutation-1",
			Changes: []sdd.DocumentChange{{
				LogicalPath:    "2026/07/13-020000-s-tac-api.md",
				CanonicalBytes: []byte(localEntry),
			}},
		}
		batch.Digest, err = sdd.MutationBatchDigest(batch)
		if err != nil {
			t.Fatal(err)
		}
		return sddtest.GraphStoreFixture{Store: store, InitialRevision: initial.Revision(), Batch: batch}
	})
}

func TestFilesystemSessionStoreConformance(t *testing.T) {
	sddtest.RunSessionStoreTests(t, func(t *testing.T) sddtest.SessionStoreFixture {
		store, err := sdd.NewFilesystemSessionStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return sddtest.SessionStoreFixture{
			Store: store,
			Metadata: sdd.SessionMetadata{
				ID: "session-1", Subject: "christopher", Project: "example", Participant: "Christopher",
			},
			Append: sdd.SessionAppend{Events: []sdd.StoredEvent{{CodecVersion: 1, Code: "started", Payload: json.RawMessage(`{"instance":"i_1"}`)}}},
		}
	})
}

func TestFilesystemStagedBlobStoreConformance(t *testing.T) {
	sddtest.RunStagedBlobStoreTests(t, func(t *testing.T) sddtest.StagedBlobStoreFixture {
		store, err := sdd.NewFilesystemStagedBlobStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return sddtest.StagedBlobStoreFixture{
			Store: store, Owner: sdd.BlobOwner{Subject: "christopher", Session: "session-1"},
			Filename: "evidence.md", Content: []byte("evidence"),
		}
	})
}

func TestFunctionalMechanicalAdaptersConform(t *testing.T) {
	embeddings := sdd.EmbeddingExecutorFuncs{
		SpecFunc: func(context.Context) (sdd.EmbeddingSpec, error) {
			return sdd.EmbeddingSpec{Fingerprint: "fixture", Dimensions: 2}, nil
		},
		EmbedFunc: func(_ context.Context, inputs []sdd.EmbeddingInput) ([]sdd.EmbeddingVector, error) {
			vectors := make([]sdd.EmbeddingVector, len(inputs))
			for i, input := range inputs {
				vectors[i] = sdd.EmbeddingVector{ID: input.ID, Values: []float32{float32(i), 1}}
			}
			return vectors, nil
		},
	}
	sddtest.RunEmbeddingExecutorTests(t, func(*testing.T) sddtest.EmbeddingExecutorFixture {
		return sddtest.EmbeddingExecutorFixture{Executor: embeddings, Inputs: []sdd.EmbeddingInput{{ID: "one", Text: "text"}}}
	})

	index := newMemoryIndex()
	namespace := sdd.IndexNamespace{Project: "example", Fingerprint: "fixture", Dimensions: 2, Metric: "cosine"}
	chunks := []sdd.IndexedChunk{{Chunk: sdd.CanonicalChunk{ID: "chunk-1", EntryID: "entry-1", Revision: "r1", ContentHash: "h1"}, Vector: []float32{0, 1}}}
	sddtest.RunSearchIndexStoreTests(t, func(*testing.T) sddtest.SearchIndexStoreFixture {
		return sddtest.SearchIndexStoreFixture{Store: index, Namespace: namespace, Revision: "r1", Chunks: chunks, Query: []float32{0, 1}}
	})

	executor := sdd.LLMExecutorFuncs{
		CapabilitiesFunc: func(context.Context) ([]string, error) { return []string{"json-schema"}, nil },
		ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
			return sdd.LLMResult{Output: []byte(`{}`), ExecutorFingerprint: "fixture"}, nil
		},
	}
	sddtest.RunLLMExecutorTests(t, func(*testing.T) sddtest.LLMExecutorFixture {
		return sddtest.LLMExecutorFixture{Executor: executor, RequiredCapability: "json-schema"}
	})
}

type memoryIndex struct {
	manifest map[sdd.IndexNamespace][]sdd.StoredChunkRef
	chunks   map[sdd.IndexNamespace]map[string]sdd.IndexedChunk
}

func newMemoryIndex() *memoryIndex {
	return &memoryIndex{manifest: map[sdd.IndexNamespace][]sdd.StoredChunkRef{}, chunks: map[sdd.IndexNamespace]map[string]sdd.IndexedChunk{}}
}

func (m *memoryIndex) Manifest(_ context.Context, namespace sdd.IndexNamespace) ([]sdd.StoredChunkRef, error) {
	return append([]sdd.StoredChunkRef(nil), m.manifest[namespace]...), nil
}

func (m *memoryIndex) Reconcile(_ context.Context, namespace sdd.IndexNamespace, _ string, upserts []sdd.IndexedChunk, deletes []string) error {
	if m.chunks[namespace] == nil {
		m.chunks[namespace] = map[string]sdd.IndexedChunk{}
	}
	for _, id := range deletes {
		delete(m.chunks[namespace], id)
	}
	for _, chunk := range upserts {
		m.chunks[namespace][chunk.Chunk.ID] = chunk
	}
	m.manifest[namespace] = m.manifest[namespace][:0]
	for _, chunk := range m.chunks[namespace] {
		m.manifest[namespace] = append(m.manifest[namespace], sdd.StoredChunkRef{
			ID: chunk.Chunk.ID, Revision: chunk.Chunk.Revision, ContentHash: chunk.Chunk.ContentHash,
		})
	}
	return nil
}

func (m *memoryIndex) Nearest(_ context.Context, namespaces []sdd.IndexNamespace, vector []float32, limit int) ([]sdd.ScoredChunkHit, error) {
	var hits []sdd.ScoredChunkHit
	for _, namespace := range namespaces {
		for _, chunk := range m.chunks[namespace] {
			var score float64
			for i := range vector {
				score += float64(vector[i] * chunk.Vector[i])
			}
			if math.IsNaN(score) {
				return nil, fmt.Errorf("invalid score")
			}
			hits = append(hits, sdd.ScoredChunkHit{Namespace: namespace, ChunkID: chunk.Chunk.ID, Revision: chunk.Chunk.Revision, ContentHash: chunk.Chunk.ContentHash, Score: score})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

type idempotentFinalizer struct{ calls int }

func (*idempotentFinalizer) Name() string { return "fixture" }
func (f *idempotentFinalizer) Finalize(context.Context, sdd.AppliedMutation) error {
	f.calls++
	return nil
}

func TestMutationFinalizerConformance(t *testing.T) {
	finalizer := &idempotentFinalizer{}
	sddtest.RunMutationFinalizerTests(t, func(*testing.T) sddtest.MutationFinalizerFixture {
		return sddtest.MutationFinalizerFixture{Finalizer: finalizer, Applied: sdd.AppliedMutation{Project: "example", BatchID: "batch", Revision: "r2"}}
	})
	if finalizer.calls != 2 {
		t.Fatalf("finalizer calls = %d, want 2", finalizer.calls)
	}
}

type accessResolver struct{}

func (accessResolver) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject, Participant: "Christopher"}, nil
}
func (accessResolver) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: sdd.ProjectRef{ID: "example"}, CanRead: true, State: sdd.ProjectReady}}}, nil
}
func (accessResolver) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return nil, nil
}
func (accessResolver) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, nil
}

func TestAccessResolverConformance(t *testing.T) {
	sddtest.RunAccessResolverTests(t, func(*testing.T) sddtest.AccessResolverFixture {
		return sddtest.AccessResolverFixture{
			Resolver: accessResolver{}, Identity: sdd.RequestIdentity{Subject: "christopher"},
			Principal: sdd.Principal{Subject: "christopher", Participant: "Christopher"},
			Project:   "example", Dependency: "example.org/dependency", ProjectCount: 1,
		}
	})
}

func TestSessionHolderMetadataRoundTrips(t *testing.T) {
	store, err := sdd.NewFilesystemSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Round(0)
	metadata := sdd.SessionMetadata{
		ID: "held", Subject: "christopher", Project: "example",
		Holder: &sdd.SessionHolder{Subject: "christopher", MCPSessionID: "mcp-1", Generation: 1, LastActivity: now, ExpiresAt: now.Add(time.Minute)},
	}
	created, err := store.Create(t.Context(), metadata)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(t.Context(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != created.Version || loaded.Metadata.Holder == nil || loaded.Metadata.Holder.Generation != 1 {
		t.Fatalf("holder did not round trip: %+v", loaded)
	}
}

func TestFilesystemSessionStoreCASFencesIndependentInstances(t *testing.T) {
	dir := t.TempDir()
	first, err := sdd.NewFilesystemSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sdd.NewFilesystemSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := first.Create(t.Context(), sdd.SessionMetadata{ID: "shared", Subject: "christopher", Project: "example"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Append(t.Context(), "shared", created.Version, sdd.SessionAppend{Events: []sdd.StoredEvent{{CodecVersion: 1, Code: "one"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Append(t.Context(), "shared", created.Version, sdd.SessionAppend{Events: []sdd.StoredEvent{{CodecVersion: 1, Code: "two"}}}); errorCode(err) != sdd.ErrorSessionConflict {
		t.Fatalf("independent stale append error = %v", err)
	}
}
