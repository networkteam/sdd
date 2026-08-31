package application_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	sdd "github.com/networkteam/sdd/pkg/application"
	localadapter "github.com/networkteam/sdd/pkg/local"
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

func (s *conflictInjectingStore) injectConflicts(n int) {
	s.mu.Lock()
	s.remaining = n
	s.mu.Unlock()
}

func TestApplyPreparedRetriesRevisionConflictThenMerges(t *testing.T) {
	var injector *conflictInjectingStore
	// Two lost races followed by success proves the engine-internal retry lands
	// within its three-attempt bound, invisible to the caller.
	application, sessions, blobs, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		injector = &conflictInjectingStore{GraphStore: store, remaining: 2}
		return injector
	}, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "retry-merge")
	prepared := preparedEntry(t, graph.GraphStore, binding, "retry-merge", "2026/07/13-054000-s-tac-rty.md")
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
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
	// Exactly three injected conflicts exhaust the cap: paired with the two-loss
	// merge case (which lands), this pins the retry bound at exactly three.
	application, sessions, blobs, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		return &conflictInjectingStore{GraphStore: store, remaining: 3}
	}, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "exhausted")
	prepared := preparedEntry(t, graph.GraphStore, binding, "exhausted", "2026/07/13-054100-s-tac-exh.md")
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
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
	if err != nil || len(history.Items) != 1 || history.Items[0].State != sdd.RecoveryAbandoned || history.Items[0].Reason != sdd.RecoveryReasonDiscarded || history.Items[0].Actionable() {
		t.Fatalf("exhausted conflict terminal history = %+v, %v; want one non-actionable discarded item", history, err)
	}
}

func TestApplyPreparedGenuineWIPMarkerPathCollisionFailsTyped(t *testing.T) {
	application, sessions, _, graph := newDurableApplication(t, time.Now, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	anchorPath := "2026/07/13-055400-s-tac-anc.md"
	anchorID, err := model.RelPathToID(anchorPath)
	if err != nil {
		t.Fatal(err)
	}
	writerA := openBinding(t, sessions, identity.Subject, "writer-a")
	anchor := preparedEntry(t, graph.GraphStore, writerA, "anchor", anchorPath)
	created, err := application.ApplyPrepared(t.Context(), identity, "example", writerA, anchor)
	if err != nil || created.Apply.State != sdd.MutationApplied {
		t.Fatalf("anchor apply = %+v, %v", created, err)
	}
	writerB := openBinding(t, sessions, identity.Subject, "writer-b")

	// Two writers start exclusive WIP on the same entry in the same second, so
	// they share the deterministic marker ID — hence the same marker path and
	// the same batch ID — while describing the work differently. The adapter's
	// batch ledger rejects the second write typed; merge retries never clobber.
	const markerID = "20260713-055500-christopher"
	first := preparedWIP(t, graph.GraphStore, created.Binding, markerID, anchorID, "writer A takes the entry")
	firstResult, err := application.ApplyPrepared(t.Context(), identity, "example", created.Binding, first)
	if err != nil || firstResult.Apply.State != sdd.MutationApplied {
		t.Fatalf("first WIP apply = %+v, %v", firstResult, err)
	}
	second := preparedWIP(t, graph.GraphStore, writerB, markerID, anchorID, "writer B takes the entry")
	result, err := application.ApplyPrepared(t.Context(), identity, "example", writerB, second)
	if errorCode(err) != sdd.ErrorRecoveryRequired || result.Apply.State != sdd.MutationNotApplied {
		t.Fatalf("same WIP marker path collision = %+v, %v; want typed not-applied", result, err)
	}
	if !strings.Contains(err.Error(), "reused") {
		t.Fatalf("collision error message = %q", err.Error())
	}
}

func TestReplaceSummaryMergesUnderRetry(t *testing.T) {
	var injector *conflictInjectingStore
	application, sessions, _, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		injector = &conflictInjectingStore{GraphStore: store}
		return injector
	}, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "replace-summary")
	entryPath := "2026/07/13-055600-s-tac-sum.md"
	entryID, err := model.RelPathToID(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedEntry(t, graph.GraphStore, binding, "summary-target", entryPath)
	created, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if err != nil || created.Apply.State != sdd.MutationApplied {
		t.Fatalf("summary target apply = %+v, %v", created, err)
	}
	// Force two lost races on the summary-replacement apply: it must retry and
	// land through the same merge path as capture, not a bespoke apply.
	injector.injectConflicts(2)
	if _, err := application.ReplaceSummary(t.Context(), identity, "example", created.Binding, sdd.MutationTarget{Project: "example", Branch: "main"}, entryID, "Replacement summary text."); err != nil {
		t.Fatalf("ReplaceSummary under retry = %v", err)
	}
	injector.mu.Lock()
	remaining := injector.remaining
	injector.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("ReplaceSummary retry left %d injected conflicts unconsumed, want 0", remaining)
	}
	pending, err := application.ListRecoveries(t.Context(), identity, "example", false)
	if err != nil || len(pending.Items) != 0 {
		t.Fatalf("ReplaceSummary recovery projection = %+v, %v; want none", pending, err)
	}
}

// preparedWIP builds a durable intent mirroring what StartWIP produces: a WIP
// marker change at the deterministic marker path with the "wip-start-{id}"
// batch ID, so two writers sharing a marker ID share the batch ID.
func preparedWIP(t *testing.T, graph sdd.GraphStore, binding sdd.SessionBinding, markerID, entryID, description string) sdd.PreparedTransition {
	t.Helper()
	snapshot, err := graph.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	marker := &model.WIPMarker{
		ID: markerID, Entry: entryID, Participant: "Christopher", Exclusive: true, Content: description,
		Time: time.Date(2026, 7, 13, 5, 55, 0, 0, time.UTC),
	}
	batch := sdd.MutationBatch{
		ID: "wip-start-" + markerID, Message: "sdd: wip start " + entryID,
		Changes: []sdd.DocumentChange{{LogicalPath: filepath.ToSlash(model.WIPMarkerPath(markerID)), CanonicalBytes: []byte(model.FormatWIPMarker(marker))}},
	}
	digest, err := sdd.MutationBatchDigest(batch)
	if err != nil {
		t.Fatal(err)
	}
	batch.Digest = digest
	return sdd.PreparedTransition{
		Version: sdd.PreparedTransitionVersion, Target: sdd.MutationTarget{Project: "example", Branch: "main"},
		ExpectedGraphRevision: snapshot.Revision(), Batch: batch,
		Staged: sdd.SessionRef{Subject: binding.Subject, Session: binding.SessionID},
	}
}

func TestInterleavedCapturesBothLandWithoutRecovery(t *testing.T) {
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

		LLMTimeout: time.Minute,
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
		bindings[i] = openBinding(t, sessions, identity.Subject, sdd.SessionID(fmt.Sprintf("capture-%d", i)))
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
