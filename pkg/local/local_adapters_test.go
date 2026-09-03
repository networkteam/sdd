package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm/embed"
	localadapter "github.com/networkteam/sdd/pkg/local"
	"github.com/networkteam/sdd/pkg/sddtest"
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
		store, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: canonicalTempDir(t)})
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
		secondBatch := sdd.MutationBatch{
			ID: "mutation-2",
			Changes: []sdd.DocumentChange{{
				LogicalPath:    "2026/07/13-030000-s-tac-two.md",
				CanonicalBytes: []byte(localEntry),
			}},
		}
		secondBatch.Digest, err = sdd.MutationBatchDigest(secondBatch)
		if err != nil {
			t.Fatal(err)
		}
		return sddtest.GraphStoreFixture{Store: store, InitialRevision: initial.Revision(), Batch: batch, SecondBatch: secondBatch}
	})
}

func TestFilesystemSessionStoreConformance(t *testing.T) {
	sddtest.RunSessionStoreTests(t, func(t *testing.T) sddtest.SessionStoreFixture {
		store, err := localadapter.NewFilesystemSessionStoreAt(canonicalTempDir(t))
		if err != nil {
			t.Fatal(err)
		}
		return sddtest.SessionStoreFixture{
			Store: store,
			Metadata: sdd.SessionMetadata{
				ID: "session-1", Subject: "christopher", Project: "example", Participant: "Christopher",
				Attachment: &sdd.Attachment{Subject: "christopher", ClientName: "test-client", LastActivity: time.Now().UTC().Round(0)},
			},
			Append: sdd.SessionAppend{Events: []sdd.StoredEvent{{CodecVersion: 1, Code: "started", Payload: json.RawMessage(`{"instance":"i_1"}`)}}},
		}
	})
}

func TestFilesystemStagedBlobStoreConformance(t *testing.T) {
	sddtest.RunStagedBlobStoreTests(t, func(t *testing.T) sddtest.StagedBlobStoreFixture {
		store, err := localadapter.NewFilesystemStagedBlobStoreAt(canonicalTempDir(t))
		if err != nil {
			t.Fatal(err)
		}
		return sddtest.StagedBlobStoreFixture{
			Store: store, Session: sdd.SessionRef{Subject: "christopher", Session: "session-1"},
			Filename: "evidence.md", Content: []byte("evidence"),
		}
	})
}

func TestFunctionalMechanicalAdaptersConform(t *testing.T) {
	embeddings := embed.EmbedderFunc{Space: "fixture", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		vectors := make([][]float32, len(req.Texts))
		for i := range req.Texts {
			vectors[i] = []float32{float32(i), 1}
		}
		return embed.Result{Vectors: vectors}, nil
	}}
	sddtest.RunEmbedderTests(t, func(*testing.T) sddtest.EmbedderFixture {
		return sddtest.EmbedderFixture{Embedder: embeddings, Texts: []string{"text"}}
	})

	memIndex := newMemoryIndex()
	namespace := sdd.IndexNamespace{Project: "example", Fingerprint: "fixture", Metric: "cosine"}
	chunks, queryVec := conformanceSearchChunks()
	sddtest.RunSearchIndexStoreTests(t, func(*testing.T) sddtest.SearchIndexStoreFixture {
		return sddtest.SearchIndexStoreFixture{Store: memIndex, Namespace: namespace, Chunks: chunks, Query: queryVec}
	})
}

func TestMemorySearchIndexStoreConforms(t *testing.T) {
	namespace := sdd.IndexNamespace{Project: "memory", Fingerprint: "fixture", Metric: "cosine"}
	chunks, queryVec := conformanceSearchChunks()
	store := localadapter.NewMemorySearchIndexStore()
	sddtest.RunSearchIndexStoreTests(t, func(*testing.T) sddtest.SearchIndexStoreFixture {
		return sddtest.SearchIndexStoreFixture{
			Store: store, Namespace: namespace, Chunks: chunks, Query: queryVec,
			Reopen: func() sdd.SearchIndexStore { return store },
		}
	})
}

// TestPersistentSearchIndexStoreConforms runs the same contract against the
// chromem-backed machine-global store that sdd serve wires in production,
// including the reopen-persistence assertion (a fresh adapter over the same
// cache directory still answers with the reconciled chunks).
func TestPersistentSearchIndexStoreConforms(t *testing.T) {
	cacheRoot := canonicalTempDir(t)
	const project = sdd.ProjectID("persistent")
	const repoKey = "example.org/repo"
	namespace := sdd.IndexNamespace{Project: project, Fingerprint: "fixture", Metric: "cosine"}
	chunks, queryVec := conformanceSearchChunks()
	sddtest.RunSearchIndexStoreTests(t, func(*testing.T) sddtest.SearchIndexStoreFixture {
		return sddtest.SearchIndexStoreFixture{
			Store:     localadapter.NewPersistentSearchIndexStore(project, cacheRoot, repoKey),
			Namespace: namespace, Chunks: chunks, Query: queryVec,
			Reopen: func() sdd.SearchIndexStore {
				return localadapter.NewPersistentSearchIndexStore(project, cacheRoot, repoKey)
			},
		}
	})
}

// conformanceSearchChunks returns a two-entry chunk set carrying citation
// metadata and equal-length vectors, with a query vector that lands on the
// first summary chunk.
func conformanceSearchChunks() ([]sdd.IndexedChunk, []float32) {
	return []sdd.IndexedChunk{
		{Chunk: sdd.CanonicalChunk{ID: "entry-1#v-v1#summary", EntryID: "entry-1", EntryHash: "v1", ContentHash: "h1", Text: "alpha summary", Body: "alpha summary body", IsSummary: true}, Vector: []float32{1, 0}},
		{Chunk: sdd.CanonicalChunk{ID: "entry-1#v-v1#body-0", EntryID: "entry-1", EntryHash: "v1", ContentHash: "h2", Text: "alpha body", Body: "alpha body text", Breadcrumb: []string{"Section"}, Depth: 2}, Vector: []float32{0.9, 0.1}},
		{Chunk: sdd.CanonicalChunk{ID: "entry-2#v-w1#summary", EntryID: "entry-2", EntryHash: "w1", ContentHash: "h3", Text: "beta summary", Body: "beta summary body", IsSummary: true}, Vector: []float32{0, 1}},
	}, []float32{1, 0}
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
			if len(vector) != len(chunk.Vector) {
				return nil, fmt.Errorf("query vector has %d dimensions, want %d", len(vector), len(chunk.Vector))
			}
			var score float64
			for i := range vector {
				score += float64(vector[i] * chunk.Vector[i])
			}
			if math.IsNaN(score) {
				return nil, fmt.Errorf("invalid score")
			}
			hits = append(hits, sdd.ScoredChunkHit{
				Namespace: namespace, ChunkID: chunk.Chunk.ID, EntryID: chunk.Chunk.EntryID,
				Revision: chunk.Chunk.Revision, ContentHash: chunk.Chunk.ContentHash, Score: score,
				Body: chunk.Chunk.Body, Breadcrumb: chunk.Chunk.Breadcrumb, Depth: chunk.Chunk.Depth,
				IsSummary: chunk.Chunk.IsSummary, IsAttachment: chunk.Chunk.IsAttachment,
				SourceAttachmentPath: chunk.Chunk.SourceAttachmentPath,
			})
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
	return sdd.Principal{Subject: identity.Subject}, nil
}

func (accessResolver) ResolveParticipant(context.Context, sdd.Principal, sdd.ProjectID) (string, error) {
	return "Christopher", nil
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
func (accessResolver) AuthorizeSession(ctx context.Context, request sdd.SessionAccessRequest) error {
	return sdd.OwnerOnly(ctx, request)
}

func TestAccessResolverConformance(t *testing.T) {
	sddtest.RunAccessResolverTests(t, func(*testing.T) sddtest.AccessResolverFixture {
		return sddtest.AccessResolverFixture{
			Resolver: accessResolver{}, Identity: sdd.RequestIdentity{Subject: "christopher"},
			Principal:   sdd.Principal{Subject: "christopher"},
			Participant: "Christopher",
			Project:     "example", Dependency: "example.org/dependency", ProjectCount: 1,
		}
	})
}

func TestSessionAttachmentMetadataRoundTrips(t *testing.T) {
	store, err := localadapter.NewFilesystemSessionStoreAt(canonicalTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Round(0)
	metadata := sdd.SessionMetadata{
		ID: "attached", Subject: "christopher", Project: "example",
		Attachment: &sdd.Attachment{Subject: "christopher", ClientName: "test-client", LastActivity: now},
	}
	created, err := store.Create(t.Context(), metadata)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(t.Context(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != created.Version || loaded.Metadata.Attachment == nil || loaded.Metadata.Attachment.ClientName != "test-client" {
		t.Fatalf("attachment did not round trip: %+v", loaded)
	}
}

// TestSessionLoadToleratesLegacyHolderMetadata proves the in-place store
// evolution (codec version unchanged): a session file written by an older
// binary carrying the removed holder JSON still loads — the unknown fields are
// ignored and the attachment reads nil.
func TestSessionLoadToleratesLegacyHolderMetadata(t *testing.T) {
	dir := canonicalTempDir(t)
	store, err := localadapter.NewFilesystemSessionStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":1,"metadata":{"CodecVersion":1,"ID":"legacy-holder","Subject":"christopher","Project":"example","Participant":"Christopher","Holder":{"Subject":"christopher","MCPSessionID":"mcp-1","Generation":1,"ExpiresAt":"2026-07-13T05:01:00Z"},"HolderHistory":[{"Reason":"released"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "legacy-holder.jsonl"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(t.Context(), "legacy-holder")
	if err != nil {
		t.Fatalf("legacy holder session failed to load: %v", err)
	}
	if loaded.Metadata.Subject != "christopher" || loaded.Metadata.Attachment != nil {
		t.Fatalf("legacy holder metadata did not load tolerantly: %+v", loaded.Metadata)
	}
}

func TestFilesystemSessionStoreCASFencesIndependentInstances(t *testing.T) {
	dir := canonicalTempDir(t)
	first, err := localadapter.NewFilesystemSessionStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := localadapter.NewFilesystemSessionStoreAt(dir)
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

func TestFilesystemSessionStoreReadsEveryReleasedFormatAndSkipsUnreadable(t *testing.T) {
	dir := canonicalTempDir(t)
	store, err := localadapter.NewFilesystemSessionStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	metadata := sdd.SessionMetadata{ID: "current", Subject: "local", Project: "local"}
	if _, err := store.Create(t.Context(), metadata); err != nil {
		t.Fatal(err)
	}
	legacy := `{"v":1,"ts":"2026-07-01T12:00:00Z","session":"legacy","seq":1,"event":"session_meta","data":{"participant":"Christopher"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "legacy.jsonl"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("{not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), sdd.SessionFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := make([]string, 0, len(page.Sessions))
	for _, item := range page.Sessions {
		ids = append(ids, string(item.Metadata.ID))
	}
	if !slices.Equal(ids, []string{"current", "legacy"}) {
		t.Fatalf("List ids = %v, want the current and legacy logs with the unreadable one skipped", ids)
	}

	// The pre-0.16 event-only log is read, not converted: its events arrive
	// verbatim and the metadata it never carried is recovered from them.
	stored, err := store.Load(t.Context(), "legacy")
	if err != nil {
		t.Fatalf("Load(legacy): %v", err)
	}
	if stored.Metadata.Participant != "Christopher" {
		t.Fatalf("legacy participant = %q, want it folded from the session_meta event", stored.Metadata.Participant)
	}
	if len(stored.Events) != 1 || string(stored.Events[0].Payload) != strings.TrimSpace(legacy) {
		t.Fatalf("legacy events = %+v, want the raw line preserved verbatim", stored.Events)
	}

	// Appending to a log found in an older format keeps it live in place.
	next, err := store.Append(t.Context(), "legacy", stored.Version, sdd.SessionAppend{
		Events: []sdd.StoredEvent{{CodecVersion: 1, Code: "one"}},
	})
	if err != nil {
		t.Fatalf("Append(legacy): %v", err)
	}
	reloaded, err := store.Load(t.Context(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Version != next || len(reloaded.Events) != 2 {
		t.Fatalf("reloaded legacy = version %d with %d events, want %d and 2", reloaded.Version, len(reloaded.Events), next)
	}

	if _, err := store.Load(t.Context(), "broken"); err == nil {
		t.Fatal("Load(broken) succeeded, want a decode error")
	} else if strings.Contains(strings.ToLower(err.Error()), "sdd init") {
		t.Fatalf("Load(broken) leaked a CLI instruction: %v", err)
	}
}

func errorCode(err error) sdd.ErrorCode {
	if applicationError, ok := errors.AsType[*sdd.ApplicationError](err); ok {
		return applicationError.Code
	}
	return ""
}
