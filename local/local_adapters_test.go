package local_test

import (
	"bytes"
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

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/engine"
	localadapter "github.com/networkteam/sdd/local"
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
		store, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
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
		store, err := localadapter.NewFilesystemSessionStore(t.TempDir())
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
		store, err := localadapter.NewFilesystemStagedBlobStore(t.TempDir())
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
			return sdd.EmbeddingSpec{Fingerprint: "fixture"}, nil
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

	memIndex := newMemoryIndex()
	namespace := sdd.IndexNamespace{Project: "example", Fingerprint: "fixture", Metric: "cosine"}
	chunks, queryVec := conformanceSearchChunks()
	sddtest.RunSearchIndexStoreTests(t, func(*testing.T) sddtest.SearchIndexStoreFixture {
		return sddtest.SearchIndexStoreFixture{Store: memIndex, Namespace: namespace, Chunks: chunks, Query: queryVec}
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
	cacheRoot := t.TempDir()
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
	store, err := localadapter.NewFilesystemSessionStore(t.TempDir())
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
	first, err := localadapter.NewFilesystemSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := localadapter.NewFilesystemSessionStore(dir)
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

func TestFilesystemSessionStoreIgnoresLegacyAndUnreadableRecords(t *testing.T) {
	dir := t.TempDir()
	store, err := localadapter.NewFilesystemSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	metadata := sdd.SessionMetadata{CodecVersion: 1, ID: "current", Subject: "local", Project: "example"}
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

	assertOnlyCurrent := func() {
		t.Helper()
		listed, err := store.List(t.Context(), sdd.SessionFilter{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(listed) != 1 || listed[0].Metadata.ID != "current" {
			t.Fatalf("List = %+v, want only current", listed)
		}
	}
	assertOnlyCurrent()

	// A pre-0.16 process may create another legacy record after the first
	// scan. Classification is repeated, so it remains non-blocking.
	if err := os.WriteFile(filepath.Join(dir, "later.jsonl"), []byte(strings.ReplaceAll(legacy, "legacy", "later")), 0o600); err != nil {
		t.Fatal(err)
	}
	assertOnlyCurrent()

	for _, id := range []sdd.SessionID{"legacy", "broken", "later"} {
		_, err := store.Load(t.Context(), id)
		var migration *sdd.ApplicationError
		if !errors.As(err, &migration) || migration.Code != sdd.ErrorMigrationRequired {
			t.Fatalf("Load(%s) error = %v, want migration required", id, err)
		}
		if strings.Contains(strings.ToLower(err.Error()), "sdd init") {
			t.Fatalf("Load(%s) leaked a CLI instruction: %v", id, err)
		}
	}
}

func TestFilesystemLegacySessionMigrationPreservesEventsAndStagedAttachments(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	blobsDir := filepath.Join(root, "staged-blobs")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := sdd.SessionID("legacy")
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	events := []engine.Event{
		{V: 1, TS: now, Session: string(id), Seq: 1, Event: engine.EventSessionMeta, Data: map[string]any{"participant": "Christopher"}},
		{V: 1, TS: now.Add(time.Second), Session: string(id), Seq: 2, Event: engine.EventLabeled, Data: map[string]any{"label": "Parked migration"}},
		{V: 1, TS: now.Add(2 * time.Second), Session: string(id), Seq: 3, Instance: "i_1", Event: engine.EventStarted, Data: map[string]any{"procedure": "capture", "step": "work"}},
		{V: 1, TS: now.Add(3 * time.Second), Session: string(id), Seq: 4, Instance: "i_1", Event: engine.EventReport, Data: map[string]any{"fields": map[string]any{"body": "parked draft"}}},
	}
	var legacy bytes.Buffer
	writtenPayloads := make([][]byte, 0, len(events))
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		writtenPayloads = append(writtenPayloads, payload)
		legacy.Write(payload)
		legacy.WriteByte('\n')
	}
	legacyPath := filepath.Join(sessionsDir, string(id)+".jsonl")
	if err := os.WriteFile(legacyPath, legacy.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	stagingDir := filepath.Join(sessionsDir, string(id)+"-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "evidence.md"), []byte("legacy evidence"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrator, err := localadapter.NewFilesystemLegacySessionMigrator(sessionsDir, blobsDir, "local", "example")
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := migrator.ListLegacySessions(t.Context())
	if err != nil || len(candidates) != 1 || candidates[0] != legacyPath {
		t.Fatalf("candidates = %v, %v", candidates, err)
	}
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatal(err)
	}

	store, err := localadapter.NewFilesystemSessionStore(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Load(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.ID != id || stored.Metadata.Subject != "local" || stored.Metadata.Project != "example" ||
		stored.Metadata.Participant != "Christopher" || stored.Metadata.Label != "Parked migration" || !stored.Metadata.UpdatedAt.Equal(now.Add(3*time.Second)) {
		t.Fatalf("metadata = %+v", stored.Metadata)
	}
	if len(stored.Events) != 5 {
		t.Fatalf("events = %d, want 5", len(stored.Events))
	}
	for index, want := range writtenPayloads {
		if stored.Events[index].Code != sdd.WorkflowEventCode || !bytes.Equal(stored.Events[index].Payload, want) {
			t.Fatalf("event %d = %+v, want payload %s", index, stored.Events[index], want)
		}
	}
	var staged struct {
		Handle string `json:"handle"`
		BlobID string `json:"blob_id"`
	}
	if err := json.Unmarshal(stored.Events[4].Payload, &staged); err != nil {
		t.Fatal(err)
	}
	if staged.Handle != "evidence.md" || staged.BlobID == "" {
		t.Fatalf("staged event = %+v", staged)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(blobsDir)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := blobs.Open(t.Context(), sdd.BlobOwner{Subject: "local", Session: id}, staged.BlobID)
	if err != nil {
		t.Fatal(err)
	}
	content := new(bytes.Buffer)
	if _, err := content.ReadFrom(reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if content.String() != "legacy evidence" {
		t.Fatalf("staged content = %q", content.String())
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("legacy staging directory remains after migration: %v", err)
	}

	var replayEvents []engine.Event
	for _, item := range stored.Events {
		if item.Code != sdd.WorkflowEventCode {
			continue
		}
		var event engine.Event
		if err := json.Unmarshal(item.Payload, &event); err != nil {
			t.Fatal(err)
		}
		replayEvents = append(replayEvents, event)
	}
	step := &engine.Step{ID: "work"}
	spec := &engine.Spec{
		Canonical: "capture",
		State:     map[string]engine.VarDecl{"body": {Type: engine.VarType{Base: engine.TypeText}}},
		Steps:     []*engine.Step{step},
		StepByID:  map[string]*engine.Step{"work": step},
	}
	engineRuntime := engine.New(engine.NewRegistry(), engine.StaticGraphs{})
	replayed, err := engineRuntime.ReplaySession(string(id), stored.Metadata.Participant, replayEvents, func(canonical string) (*engine.Spec, error) {
		if canonical != "capture" {
			return nil, fmt.Errorf("unexpected procedure %s", canonical)
		}
		return spec, nil
	}, nil)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}
	instance, ok := replayed.Instance("i_1")
	if !ok {
		t.Fatal("parked instance was not restored")
	}
	if body, _ := instance.Store.Get("body"); body != "parked draft" {
		t.Fatalf("replayed body = %v", body)
	}

	before, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("idempotent migration rewrote a current session")
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "residue.md"), []byte("already migrated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("legacy staging residue remains after idempotent migration: %v", err)
	}
	afterCleanup, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterCleanup) {
		t.Fatal("idempotent staging cleanup rewrote a current session")
	}
	candidates, err = migrator.ListLegacySessions(t.Context())
	if err != nil || len(candidates) != 0 {
		t.Fatalf("remaining candidates after migration = %v, %v", candidates, err)
	}

	// A legacy server can write another record after an earlier completed
	// migration. The next pass discovers only that new record.
	laterPath := filepath.Join(sessionsDir, "later.jsonl")
	later := bytes.ReplaceAll(writtenPayloads[0], []byte(`"legacy"`), []byte(`"later"`))
	if err := os.WriteFile(laterPath, append(later, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates, err = migrator.ListLegacySessions(t.Context())
	if err != nil || !slices.Equal(candidates, []string{laterPath}) {
		t.Fatalf("later candidates = %v, %v", candidates, err)
	}
}

func TestFilesystemLegacySessionMigrationLeavesMalformedRecordUntouched(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionsDir, "broken.jsonl")
	want := []byte("{not-json\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	stagingDir := filepath.Join(sessionsDir, "broken-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stagedPath := filepath.Join(stagingDir, "evidence.md")
	if err := os.WriteFile(stagedPath, []byte("legacy evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator, err := localadapter.NewFilesystemLegacySessionMigrator(sessionsDir, filepath.Join(root, "blobs"), "local", "example")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.MigrateLegacySession(t.Context(), path); err == nil {
		t.Fatal("malformed migration unexpectedly succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("malformed record changed: %q", got)
	}
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(staged) != "legacy evidence" {
		t.Fatalf("legacy staging changed after failed migration: %q", staged)
	}
}

func errorCode(err error) sdd.ErrorCode {
	var applicationError *sdd.ApplicationError
	if errors.As(err, &applicationError) {
		return applicationError.Code
	}
	return ""
}
