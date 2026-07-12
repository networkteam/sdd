package sdd_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/networkteam/sdd"
)

type trackingBlobStore struct {
	sdd.StagedBlobStore
	mu       sync.Mutex
	retained int
	released int
}

func (s *trackingBlobStore) Retain(ctx context.Context, owner sdd.BlobOwner, id string, blobs []string) error {
	s.mu.Lock()
	s.retained++
	s.mu.Unlock()
	return s.StagedBlobStore.Retain(ctx, owner, id, blobs)
}

func (s *trackingBlobStore) Release(ctx context.Context, owner sdd.BlobOwner, id string) error {
	s.mu.Lock()
	s.released++
	s.mu.Unlock()
	return s.StagedBlobStore.Release(ctx, owner, id)
}

type unknownAfterApplyStore struct{ sdd.GraphStore }

func (s unknownAfterApplyStore) Apply(ctx context.Context, revision string, batch sdd.MutationBatch, blobs sdd.StagedBlobReader) (sdd.ApplyResult, error) {
	result, err := s.GraphStore.Apply(ctx, revision, batch, blobs)
	if err != nil {
		return result, err
	}
	return sdd.ApplyResult{State: sdd.MutationUnknown, Revision: result.Revision}, errors.New("injected lost apply acknowledgement")
}

type failOnceFinalizer struct {
	mu    sync.Mutex
	calls int
}

func (*failOnceFinalizer) Name() string { return "fail-once" }
func (f *failOnceFinalizer) Finalize(context.Context, sdd.AppliedMutation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 1 {
		return errors.New("injected finalizer failure")
	}
	return nil
}

func TestSessionHoldersUseChooserTTLTakeoverAndCASFencing(t *testing.T) {
	now := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	application, sessions, _, graph := newDurableApplication(t, func() time.Time { return now }, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	first, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "held", MCPSessionID: "mcp-1", Chooser: sdd.ChooserUser})
	if err != nil || !first.Created || first.Binding.Generation != 1 {
		t.Fatalf("first BindSession = %+v, %v", first, err)
	}
	listed, err := application.ListSessions(t.Context(), identity, "example")
	if err != nil || len(listed.Sessions) != 1 || !listed.Sessions[0].HolderLive {
		t.Fatalf("live session listing = %+v, %v", listed, err)
	}
	_, err = application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "held", MCPSessionID: "mcp-2", Chooser: sdd.ChooserAgent})
	var inUse *sdd.ApplicationError
	if !errors.As(err, &inUse) || inUse.Code != sdd.ErrorSessionInUse || inUse.Holder == nil || inUse.Holder.MCPSessionID != "mcp-1" {
		t.Fatalf("live competing bind error = %#v", err)
	}
	second, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "held", MCPSessionID: "mcp-2", Chooser: sdd.ChooserAgent, Takeover: true})
	if err != nil || second.Binding.Generation != 2 {
		t.Fatalf("explicit takeover = %+v, %v", second, err)
	}
	prepared := preparedEntry(t, graph.GraphStore, first.Binding, "old-holder", "2026/07/13-050000-s-tac-old.md")
	if _, err := application.ApplyPrepared(t.Context(), identity, "example", first.Binding, prepared); errorCode(err) != sdd.ErrorSessionOwnership {
		t.Fatalf("displaced holder ApplyPrepared error = %v", err)
	}
	now = now.Add(6 * time.Minute)
	third, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "held", MCPSessionID: "mcp-3", Chooser: sdd.ChooserGate})
	if err != nil || third.Binding.Generation != 3 {
		t.Fatalf("expired takeover = %+v, %v", third, err)
	}
	stored, err := sessions.Load(t.Context(), "held")
	if err != nil || len(stored.Metadata.HolderHistory) != 2 || stored.Metadata.HolderHistory[0].Reason != "explicit_takeover" || stored.Metadata.HolderHistory[1].Reason != "expired_takeover" {
		t.Fatalf("holder history = %+v, %v", stored.Metadata.HolderHistory, err)
	}
	if err := application.ReleaseSession(t.Context(), identity, "example", third.Binding); err != nil {
		t.Fatal(err)
	}
	listed, err = application.ListSessions(t.Context(), identity, "example")
	if err != nil || listed.Sessions[0].Holder != nil || listed.Sessions[0].HolderLive {
		t.Fatalf("released session listing = %+v, %v", listed, err)
	}
}

func TestPreparedTransitionRecoversUnknownApplyAndFinalizer(t *testing.T) {
	finalizer := &failOnceFinalizer{}
	application, _, blobs, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		return unknownAfterApplyStore{GraphStore: store}
	}, []sdd.MutationFinalizer{finalizer})
	identity := sdd.RequestIdentity{Subject: "christopher"}
	bound, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "recover", MCPSessionID: "mcp-1", Chooser: sdd.ChooserAgent})
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedEntry(t, graph.GraphStore, bound.Binding, "recover-unknown", "2026/07/13-051000-s-tac-rec.md")
	unknown, err := application.ApplyPrepared(t.Context(), identity, "example", bound.Binding, prepared)
	if errorCode(err) != sdd.ErrorRecoveryRequired || unknown.Apply.State != sdd.MutationUnknown {
		t.Fatalf("unknown ApplyPrepared = %+v, %v", unknown, err)
	}
	if blobs.released != 0 {
		t.Fatal("unknown outcome released staged-blob retention")
	}
	recovered, err := application.RecoverPrepared(t.Context(), identity, "example", unknown.Binding, prepared.Batch.ID)
	if errorCode(err) != sdd.ErrorRecoveryRequired || recovered.Apply.State != sdd.MutationApplied || finalizer.calls != 1 {
		t.Fatalf("first recovery = %+v, %v; finalizer calls=%d", recovered, err, finalizer.calls)
	}
	recovered, err = application.RecoverPrepared(t.Context(), identity, "example", recovered.Binding, prepared.Batch.ID)
	if err != nil || recovered.Apply.State != sdd.MutationApplied || finalizer.calls != 2 || blobs.released != 1 {
		t.Fatalf("second recovery = %+v, %v; finalizer calls=%d released=%d", recovered, err, finalizer.calls, blobs.released)
	}
	restarted, err := sdd.NewFilesystemGraphStore(sdd.FilesystemGraphStoreOptions{Project: "example", GraphDir: graph.dir})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := restarted.Reconcile(t.Context(), prepared.Batch.ID, prepared.Batch.Digest)
	if err != nil || reconciled.State != sdd.MutationApplied {
		t.Fatalf("restart Reconcile = %+v, %v", reconciled, err)
	}
}

func TestPreparedTransitionPersistsNotAppliedAndRejectsStaleBinding(t *testing.T) {
	application, _, blobs, graph := newDurableApplication(t, time.Now, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	bound, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "stale", MCPSessionID: "mcp-1", Chooser: sdd.ChooserAgent})
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedEntry(t, graph.GraphStore, bound.Binding, "stale-revision", "2026/07/13-052000-s-tac-stl.md")
	advance := preparedEntry(t, graph.GraphStore, bound.Binding, "external", "2026/07/13-052100-s-tac-ext.md")
	if _, err := graph.Apply(t.Context(), advance.ExpectedGraphRevision, advance.Batch, nil); err != nil {
		t.Fatal(err)
	}
	result, err := application.ApplyPrepared(t.Context(), identity, "example", bound.Binding, prepared)
	if errorCode(err) != sdd.ErrorGraphConflict || result.Apply.State != sdd.MutationNotApplied || blobs.released != 1 {
		t.Fatalf("stale graph ApplyPrepared = %+v, %v; released=%d", result, err, blobs.released)
	}
	other := preparedEntry(t, graph.GraphStore, bound.Binding, "stale-binding", "2026/07/13-052200-s-tac-bnd.md")
	if _, err := application.ApplyPrepared(t.Context(), identity, "example", bound.Binding, other); errorCode(err) != sdd.ErrorSessionConflict {
		t.Fatalf("stale binding error = %v", err)
	}
}

func TestSessionReplayFailsClosedForUnsupportedCodec(t *testing.T) {
	application, sessions, _, _ := newDurableApplication(t, time.Now, nil, nil)
	_, err := sessions.Create(t.Context(), sdd.SessionMetadata{CodecVersion: 99, ID: "future", Subject: "christopher", Project: "example"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.BindSession(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, "example", sdd.BindSessionRequest{SessionID: "future", MCPSessionID: "mcp", Chooser: sdd.ChooserUser})
	var migration *sdd.ApplicationError
	if !errors.As(err, &migration) || migration.Code != sdd.ErrorMigrationRequired || migration.Version != 99 {
		t.Fatalf("unsupported codec error = %#v", err)
	}
}

type graphFixture struct {
	sdd.GraphStore
	dir string
}

func newDurableApplication(t *testing.T, now func() time.Time, wrap func(sdd.GraphStore) sdd.GraphStore, finalizers []sdd.MutationFinalizer) (*sdd.Application, *sdd.FilesystemSessionStore, *trackingBlobStore, graphFixture) {
	t.Helper()
	dir := t.TempDir()
	baseGraph, err := sdd.NewFilesystemGraphStore(sdd.FilesystemGraphStoreOptions{Project: "example", GraphDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var graph sdd.GraphStore = baseGraph
	if wrap != nil {
		graph = wrap(graph)
	}
	sessions, err := sdd.NewFilesystemSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	baseBlobs, err := sdd.NewFilesystemStagedBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs := &trackingBlobStore{StagedBlobStore: baseBlobs}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, Graph: graph, Sessions: sessions, StagedBlobs: blobs, Now: now, Finalizers: finalizers,
		LLM: sdd.LLMExecutorFuncs{CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil }, ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &runtimeAccessResolver{runtime: runtime}
	application, err := sdd.NewApplication(resolver)
	if err != nil {
		t.Fatal(err)
	}
	return application, sessions, blobs, graphFixture{GraphStore: baseGraph, dir: dir}
}

func preparedEntry(t *testing.T, graph sdd.GraphStore, binding sdd.SessionBinding, id, path string) sdd.PreparedTransition {
	t.Helper()
	snapshot, err := graph.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	canonical := []byte("---\ntype: signal\nkind: done\nlayer: tactical\nsummary: Durable transition fixture.\n---\n\nDurable transition fixture body.\n")
	batch := sdd.MutationBatch{ID: id, Changes: []sdd.DocumentChange{{LogicalPath: path, CanonicalBytes: canonical}}}
	digest, err := sdd.MutationBatchDigest(batch)
	if err != nil {
		t.Fatal(err)
	}
	batch.Digest = digest
	return sdd.PreparedTransition{
		Version: sdd.PreparedTransitionVersion, ExpectedGraphRevision: snapshot.Revision(), Batch: batch,
		BlobOwner: sdd.BlobOwner{Subject: binding.Subject, Session: binding.SessionID},
	}
}

func errorCode(err error) sdd.ErrorCode {
	var applicationErr *sdd.ApplicationError
	if errors.As(err, &applicationErr) {
		return applicationErr.Code
	}
	return ""
}
