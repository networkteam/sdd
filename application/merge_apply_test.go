package application_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/application"
	localadapter "github.com/networkteam/sdd/local"
)

// conflictInjectingStore forces the leading `remaining` Apply calls to return
// the adapter's typed revision conflict without touching the graph, modelling a
// concurrent process that moved the revision between the caller's fresh read
// and its CAS apply.
type conflictInjectingStore struct {
	sdd.GraphStore
	mu        sync.Mutex
	remaining int
}

func (s *conflictInjectingStore) Apply(ctx context.Context, revision string, batch sdd.MutationBatch, blobs sdd.StagedBlobReader) (sdd.ApplyResult, error) {
	s.mu.Lock()
	force := s.remaining > 0
	if force {
		s.remaining--
	}
	s.mu.Unlock()
	if force {
		moved := revision + "-moved"
		return sdd.ApplyResult{State: sdd.MutationNotApplied, Revision: moved}, &sdd.ApplicationError{Code: sdd.ErrorGraphConflict, Message: "graph revision changed", Revision: moved}
	}
	return s.GraphStore.Apply(ctx, revision, batch, blobs)
}

func TestApplyPreparedRetriesRevisionConflictThenMerges(t *testing.T) {
	var injector *conflictInjectingStore
	// Two lost races followed by success proves the engine-internal retry lands
	// within its three-attempt bound, invisible to the caller.
	application, _, blobs, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		injector = &conflictInjectingStore{GraphStore: store, remaining: 2}
		return injector
	}, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	bound, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "retry-merge", MCPSessionID: "mcp", Chooser: sdd.ChooserAgent})
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedEntry(t, graph.GraphStore, bound.Binding, "retry-merge", "2026/07/13-054000-s-tac-rty.md")
	result, err := application.ApplyPrepared(t.Context(), identity, "example", bound.Binding, prepared)
	if err != nil || result.Apply.State != sdd.MutationApplied {
		t.Fatalf("bounded retry apply = %+v, %v", result, err)
	}
	if blobs.released != 1 {
		t.Fatalf("applied retry released blobs %d times, want 1", blobs.released)
	}
	pending, err := application.ListRecoveries(t.Context(), identity, "example", false)
	if err != nil || len(pending.Items) != 0 {
		t.Fatalf("retry-merge recovery projection = %+v, %v", pending, err)
	}
	injector.mu.Lock()
	remaining := injector.remaining
	injector.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("retry consumed injected conflicts leaving %d, want 0", remaining)
	}
}

func TestApplyPreparedExhaustedConflictFailsTypedNeverRecovery(t *testing.T) {
	// remaining exceeds the retry bound, so every attempt loses the race.
	application, _, blobs, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		return &conflictInjectingStore{GraphStore: store, remaining: 5}
	}, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	bound, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "exhausted", MCPSessionID: "mcp", Chooser: sdd.ChooserAgent})
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedEntry(t, graph.GraphStore, bound.Binding, "exhausted", "2026/07/13-054100-s-tac-exh.md")
	result, err := application.ApplyPrepared(t.Context(), identity, "example", bound.Binding, prepared)
	if errorCode(err) != sdd.ErrorGraphConflict || result.Apply.State != sdd.MutationNotApplied {
		t.Fatalf("exhausted retries = %+v, %v; want typed graph conflict", result, err)
	}
	if !strings.Contains(err.Error(), "re-try") {
		t.Fatalf("exhausted conflict message = %q, want a re-try invitation", err.Error())
	}
	// A revision conflict never files a recovery: the contended intent is
	// auto-discarded, releasing its retained blobs.
	if blobs.released != 1 {
		t.Fatalf("contended discard released blobs %d times, want 1", blobs.released)
	}
	pending, err := application.ListRecoveries(t.Context(), identity, "example", false)
	if err != nil || len(pending.Items) != 0 {
		t.Fatalf("exhausted conflict actionable recovery = %+v, %v; want none", pending, err)
	}
	history, err := application.ListRecoveries(t.Context(), identity, "example", true)
	if err != nil || len(history.Items) != 1 || history.Items[0].State != sdd.RecoveryDiscarded || history.Items[0].Actionable {
		t.Fatalf("exhausted conflict terminal history = %+v, %v; want one non-actionable discarded item", history, err)
	}
}

func TestApplyPreparedGenuineLogicalPathCollisionFailsTyped(t *testing.T) {
	application, _, _, graph := newDurableApplication(t, time.Now, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	bound, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{SessionID: "collision", MCPSessionID: "mcp", Chooser: sdd.ChooserAgent})
	if err != nil {
		t.Fatal(err)
	}
	first := preparedEntry(t, graph.GraphStore, bound.Binding, "collide", "2026/07/13-055000-s-tac-col.md")
	applied, err := application.ApplyPrepared(t.Context(), identity, "example", bound.Binding, first)
	if err != nil || applied.Apply.State != sdd.MutationApplied {
		t.Fatalf("first apply = %+v, %v", applied, err)
	}
	// A second write reusing the mutation ID at the same logical path with
	// different content — the shape of two writers racing the same WIP marker
	// path — is a genuine collision the adapter's batch ledger rejects typed;
	// merge-clean retries do not silently clobber it.
	second := preparedEntry(t, graph.GraphStore, applied.Binding, "collide", "2026/07/13-055000-s-tac-col.md")
	second.Batch.Changes[0].CanonicalBytes = []byte("---\ntype: signal\nkind: done\nlayer: tactical\nsummary: Durable transition fixture.\n---\n\nColliding body.\n")
	second.Batch.Changes[0].Document.Body = "Colliding body."
	second.Batch.Digest, err = sdd.MutationBatchDigest(second.Batch)
	if err != nil {
		t.Fatal(err)
	}
	result, err := application.ApplyPrepared(t.Context(), identity, "example", applied.Binding, second)
	if errorCode(err) != sdd.ErrorRecoveryRequired || result.Apply.State != sdd.MutationNotApplied {
		t.Fatalf("genuine collision = %+v, %v; want typed not-applied", result, err)
	}
	if !strings.Contains(err.Error(), "reused") {
		t.Fatalf("collision error message = %q", err.Error())
	}
}

func TestInterleavedCapturesBothLandWithoutRecovery(t *testing.T) {
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const captures = 2
	var preflightCalls int64
	entered := make(chan struct{}, captures)
	release := make(chan struct{})
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph,
		Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				if request.Purpose == "preflight" {
					atomic.AddInt64(&preflightCalls, 1)
					// Artificially slow stage: block until both captures have
					// pinned their prepare-time snapshot and entered pre-flight,
					// so their applies genuinely interleave through the retry.
					entered <- struct{}{}
					<-release
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				}
				return sdd.LLMResult{Output: []byte("Interleaved capture summary.")}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}

	bindings := make([]sdd.SessionBinding, captures)
	for i := range bindings {
		bound, err := application.BindSession(t.Context(), identity, "example", sdd.BindSessionRequest{
			SessionID: sdd.SessionID(fmt.Sprintf("capture-%d", i)), MCPSessionID: fmt.Sprintf("mcp-%d", i), Chooser: sdd.ChooserAgent,
		})
		if err != nil {
			t.Fatal(err)
		}
		bindings[i] = bound.Binding
	}

	type outcome struct {
		id  string
		err error
	}
	results := make(chan outcome, captures)
	for i := range bindings {
		go func(n int) {
			created, err := application.CreateEntry(t.Context(), identity, "example", bindings[n], sdd.EntryDraft{
				Kind: "gap", Layer: "tactical", Body: fmt.Sprintf("Interleaved capture body %d.", n), Confidence: "high",
			})
			results <- outcome{id: created.EntryID, err: err}
		}(i)
	}
	for range captures {
		<-entered
	}
	close(release)

	ids := map[string]bool{}
	for range captures {
		out := <-results
		if out.err != nil || out.id == "" {
			t.Fatalf("interleaved capture = %q, %v", out.id, out.err)
		}
		ids[out.id] = true
	}
	if len(ids) != captures {
		t.Fatalf("interleaved captures produced %d distinct entries, want %d", len(ids), captures)
	}
	if atomic.LoadInt64(&preflightCalls) != captures {
		t.Fatalf("pre-flight ran %d times, want %d (once per capture)", preflightCalls, captures)
	}
	recoveries, err := application.ListRecoveries(t.Context(), identity, "example", false)
	if err != nil || len(recoveries.Items) != 0 {
		t.Fatalf("interleaved capture recovery projection = %+v, %v; want none", recoveries, err)
	}
}
