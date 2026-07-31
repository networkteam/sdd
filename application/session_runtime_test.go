package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	localadapter "github.com/networkteam/sdd/local"
)

type trackingBlobStore struct {
	sdd.StagedBlobStore
	mu         sync.Mutex
	retained   int
	released   int
	releaseErr error
}

func (s *trackingBlobStore) Retain(ctx context.Context, owner sdd.SessionRef, id string, blobs []string) error {
	s.mu.Lock()
	s.retained++
	s.mu.Unlock()
	return s.StagedBlobStore.Retain(ctx, owner, id, blobs)
}

func (s *trackingBlobStore) Release(ctx context.Context, owner sdd.SessionRef, id string) error {
	s.mu.Lock()
	s.released++
	err := s.releaseErr
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.StagedBlobStore.Release(ctx, owner, id)
}

type toggleAppendSessionStore struct {
	sdd.SessionStore
	mu  sync.Mutex
	err error
}

func (s *toggleAppendSessionStore) Append(ctx context.Context, id sdd.SessionID, version uint64, append sdd.SessionAppend) (uint64, error) {
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return 0, err
	}
	return s.SessionStore.Append(ctx, id, version, append)
}

func (s *toggleAppendSessionStore) fail(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

type unknownAfterApplyStore struct{ sdd.GraphStore }

func (s unknownAfterApplyStore) Apply(ctx context.Context, revision string, batch sdd.MutationBatch, blobs sdd.StagedBlobReader) (sdd.ApplyResult, error) {
	result, err := s.GraphStore.Apply(ctx, revision, batch, blobs)
	if err != nil {
		return result, err
	}
	return sdd.ApplyResult{State: sdd.MutationUnknown, Revision: result.Revision}, errors.New("injected lost apply acknowledgement")
}

type pendingUnknownStore struct {
	sdd.GraphStore
	mu         sync.Mutex
	reconciles int
}

type mutableTargetAcquirer struct {
	mu         sync.Mutex
	graph      sdd.GraphStore
	finalizers []sdd.MutationFinalizer
	failNext   bool
	releaseErr error
}

type activityTargetAcquirer struct {
	mu           sync.Mutex
	graph        sdd.GraphStore
	active       bool
	acquisitions int
	releases     int
}

func (a *activityTargetAcquirer) Acquire(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active {
		return nil, errors.New("target acquisition overlapped another operation")
	}
	a.active = true
	a.acquisitions++
	return &sdd.AcquiredTarget{
		Target: target, Graph: a.graph,
		Release: func() error {
			a.mu.Lock()
			defer a.mu.Unlock()
			a.active = false
			a.releases++
			return nil
		},
	}, nil
}

func (a *activityTargetAcquirer) isActive() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active
}

func (a *mutableTargetAcquirer) Acquire(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failNext {
		a.failNext = false
		return nil, errors.New("injected acquisition failure")
	}
	releaseErr := a.releaseErr
	return &sdd.AcquiredTarget{
		Target: target, Graph: a.graph, Finalizers: append([]sdd.MutationFinalizer(nil), a.finalizers...),
		Release: func() error { return releaseErr },
	}, nil
}

func (a *mutableTargetAcquirer) setReleaseError(err error) {
	a.mu.Lock()
	a.releaseErr = err
	a.mu.Unlock()
}

func (s *pendingUnknownStore) Apply(context.Context, string, sdd.MutationBatch, sdd.StagedBlobReader) (sdd.ApplyResult, error) {
	return sdd.ApplyResult{State: sdd.MutationUnknown}, errors.New("injected unknown apply outcome")
}

func (s *pendingUnknownStore) Reconcile(context.Context, string, string) (sdd.ApplyResult, error) {
	s.mu.Lock()
	s.reconciles++
	s.mu.Unlock()
	return sdd.ApplyResult{State: sdd.MutationUnknown}, errors.New("injected non-definitive reconciliation")
}

type failOnceFinalizer struct {
	mu    sync.Mutex
	calls int
}

type recordingRecoveryAuthorizer struct {
	mu      sync.Mutex
	request sdd.RecoveryAccessRequest
}

func (a *recordingRecoveryAuthorizer) AuthorizeRecovery(_ context.Context, request sdd.RecoveryAccessRequest) error {
	a.mu.Lock()
	a.request = request
	a.mu.Unlock()
	return nil
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

// openBinding creates a durable session with an attachment and returns the
// matching write binding. verifyBinding matches the binding's MCP session id
// against the stored attachment, so both carry "mcp".
func openBinding(t *testing.T, sessions sdd.SessionStore, subject string, id sdd.SessionID) sdd.SessionBinding {
	t.Helper()
	created, err := sessions.Create(t.Context(), sdd.SessionMetadata{
		CodecVersion: sdd.SessionCodecVersion, ID: id, Subject: subject, Project: "example", Participant: subject,
		Attachment: &sdd.Attachment{Subject: subject, MCPSessionID: "mcp", LastActivity: time.Now().UTC().Round(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sdd.SessionBinding{SessionID: id, Subject: subject, Project: "example", MCPSessionID: "mcp", Version: created.Version}
}

// TestIncumbentContinuityHoldsWithoutExpiry proves I3 by construction: with
// arbitrary elapsed time and no competing claim, the driving client's next
// write succeeds — there is no expiry to inject, so the assertion is
// clock-free in the sense that no lease can revoke the incumbent.
func TestIncumbentContinuityHoldsWithoutExpiry(t *testing.T) {
	now := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	application, sessions, _, graph := newDurableApplication(t, func() time.Time { return now }, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "incumbent")
	first := preparedEntry(t, graph.GraphStore, binding, "incumbent-first", "2026/07/13-050000-s-tac-in1.md")
	r1, err := application.ApplyPrepared(t.Context(), identity, "example", binding, first)
	if err != nil || r1.Apply.State != sdd.MutationApplied {
		t.Fatalf("first apply = %+v, %v", r1, err)
	}
	now = now.Add(72 * time.Hour)
	second := preparedEntry(t, graph.GraphStore, r1.Binding, "incumbent-second", "2026/07/13-050100-s-tac-in2.md")
	r2, err := application.ApplyPrepared(t.Context(), identity, "example", r1.Binding, second)
	if err != nil || r2.Apply.State != sdd.MutationApplied {
		t.Fatalf("incumbent second apply after elapsed time = %+v, %v", r2, err)
	}
}

// TestReleaseClearsTheStampWithoutEndingTheSession covers d-cpt-rw7: stepping
// away clears the live stamp — so status derives from the store alone — and
// records nothing, because a connection going away is not an act on the
// dialogue.
func TestReleaseClearsTheStampWithoutEndingTheSession(t *testing.T) {
	application, sessions, _, _ := newDurableApplication(t, time.Now, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "release-stamp")
	if err := application.ReleaseSession(t.Context(), identity, "example", binding); err != nil {
		t.Fatal(err)
	}
	stored, err := sessions.Load(t.Context(), "release-stamp")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.Attachment != nil {
		t.Fatalf("attachment not cleared: %+v", stored.Metadata.Attachment)
	}
	if stored.Metadata.Ended != nil {
		t.Fatalf("release ended the session: %+v", stored.Metadata.Ended)
	}
}

func TestPreparedTransitionRecoversUnknownApplyAndFinalizer(t *testing.T) {
	finalizer := &failOnceFinalizer{}
	application, sessions, blobs, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		return unknownAfterApplyStore{GraphStore: store}
	}, []sdd.MutationFinalizer{finalizer})
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "recover")
	prepared := preparedEntry(t, graph.GraphStore, binding, "recover-unknown", "2026/07/13-051000-s-tac-rec.md")
	unknown, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if errorCode(err) != sdd.ErrorRecoveryRequired || unknown.Apply.State != sdd.MutationUnknown {
		t.Fatalf("unknown ApplyPrepared = %+v, %v", unknown, err)
	}
	if blobs.released != 0 {
		t.Fatal("unknown outcome released staged-blob retention")
	}
	if _, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{Session: unknown.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryDiscard}); errorCode(err) != sdd.ErrorRecoveryRequired {
		t.Fatalf("discard after reconciled applied error = %v", err)
	}
	recoveredResult, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{Session: unknown.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryFinalizeRetry})
	recovered := recoveredResult.Transition
	if errorCode(err) != sdd.ErrorRecoveryRequired || recovered.Apply.State != sdd.MutationApplied || finalizer.calls != 1 {
		t.Fatalf("first recovery = %+v, %v; finalizer calls=%d", recovered, err, finalizer.calls)
	}
	recoveredResult, err = application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{Session: recovered.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryFinalizeRetry})
	recovered = recoveredResult.Transition
	if err != nil || recovered.Apply.State != sdd.MutationApplied || finalizer.calls != 2 || blobs.released != 1 {
		t.Fatalf("second recovery = %+v, %v; finalizer calls=%d released=%d", recovered, err, finalizer.calls, blobs.released)
	}
	history, err := application.ListRecoveries(t.Context(), identity, "example", true)
	if err != nil || len(history.Items) != 1 || history.Items[0].State != sdd.RecoveryDelivered || history.Items[0].Actionable() {
		t.Fatalf("recovered history = %+v, %v", history, err)
	}
	restarted, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: graph.dir})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := restarted.Reconcile(t.Context(), prepared.Batch.ID, prepared.Batch.Digest)
	if err != nil || reconciled.State != sdd.MutationApplied {
		t.Fatalf("restart Reconcile = %+v, %v", reconciled, err)
	}
}

func TestPreparedTransitionMergesUnrelatedAppendAndRejectsStaleBinding(t *testing.T) {
	application, sessions, blobs, graph := newDurableApplication(t, time.Now, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "merge")
	prepared := preparedEntry(t, graph.GraphStore, binding, "merge-revision", "2026/07/13-052000-s-tac-stl.md")
	advance := preparedEntry(t, graph.GraphStore, binding, "external", "2026/07/13-052100-s-tac-ext.md")
	if _, err := graph.Apply(t.Context(), advance.ExpectedGraphRevision, advance.Batch, nil); err != nil {
		t.Fatal(err)
	}
	// The prepared write pins the pre-advance revision, but an unrelated append
	// moved the store. It merges cleanly against the revalidated fresh revision
	// instead of failing the stale pin, and never files a recovery.
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if err != nil || result.Apply.State != sdd.MutationApplied || blobs.released != 1 {
		t.Fatalf("merge-under-append ApplyPrepared = %+v, %v; released=%d", result, err, blobs.released)
	}
	pending, err := application.ListRecoveries(t.Context(), identity, "example", false)
	if err != nil || len(pending.Items) != 0 {
		t.Fatalf("merge-under-append recovery projection = %+v, %v", pending, err)
	}
	// The successful write advanced the session; a second write presenting the
	// pre-write binding version is fenced as a stale binding.
	other := preparedEntry(t, graph.GraphStore, binding, "stale-binding", "2026/07/13-052200-s-tac-bnd.md")
	if _, err := application.ApplyPrepared(t.Context(), identity, "example", binding, other); errorCode(err) != sdd.ErrorSessionConflict {
		t.Fatalf("stale binding error = %v", err)
	}
}

func TestReconcileMutationRefreshesIntentOnlyProjectionBeforeVerbSelection(t *testing.T) {
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	targets := &mutableTargetAcquirer{graph: graph, failNext: true}
	application, sessions, blobs, _ := newDurableApplicationWithTargets(t, graph, targets)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "intent-only")
	prepared := preparedEntry(t, graph, binding, "intent-only", "2026/07/13-052300-s-tac-int.md")
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if errorCode(err) != sdd.ErrorRecoveryRequired || result.Apply.State != sdd.MutationUnknown {
		t.Fatalf("intent-only ApplyPrepared = %+v, %v", result, err)
	}

	refreshed, err := application.ReconcileMutation(t.Context(), identity, "example", sdd.RecoveryReconcileRequest{
		Session: result.Binding.SessionID, MutationID: prepared.Batch.ID,
	})
	if err != nil || refreshed.Item.State != sdd.RecoveryPending || refreshed.Item.Reason != sdd.RecoveryReasonNotApplied || refreshed.Transition.Apply.State != sdd.MutationNotApplied {
		t.Fatalf("ReconcileMutation = %+v, %v", refreshed, err)
	}
	stored, err := sessions.Load(t.Context(), binding.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range stored.Events {
		if event.Code == "recovery_attempt" && strings.Contains(string(event.Payload), `"verb":"reconcile"`) {
			found = true
		}
	}
	if !found {
		t.Fatal("reconcile-only recovery attempt was not recorded")
	}
	applied, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{
		Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryApply,
	})
	if err != nil || applied.Transition.Apply.State != sdd.MutationApplied || blobs.released != 1 {
		t.Fatalf("recovery apply = %+v, %v; released=%d", applied, err, blobs.released)
	}
	history, err := application.ListRecoveries(t.Context(), identity, "example", true)
	if err != nil || len(history.Items) != 1 || history.Items[0].State != sdd.RecoveryDelivered || history.Items[0].Actionable() {
		t.Fatalf("recovery apply history = %+v, %v", history, err)
	}
}

func TestPreparedTransitionRejectsEmptyTargetAndStructuredDivergence(t *testing.T) {
	application, sessions, _, graph := newDurableApplication(t, time.Now, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "validate-prepared")

	emptyTarget := preparedEntry(t, graph.GraphStore, binding, "empty-target", "2026/07/13-052301-s-tac-emp.md")
	emptyTarget.Target.Branch = ""
	if _, err := application.ApplyPrepared(t.Context(), identity, "example", binding, emptyTarget); errorCode(err) != sdd.ErrorWriteDenied {
		t.Fatalf("empty target error = %v", err)
	}
	stored, err := sessions.Load(t.Context(), binding.SessionID)
	if err != nil || len(stored.Events) != 0 {
		t.Fatalf("empty target persisted events = %d, %v", len(stored.Events), err)
	}
	foreignTarget := preparedEntry(t, graph.GraphStore, binding, "foreign-target", "2026/07/13-052306-s-tac-for.md")
	foreignTarget.Target.Project = "connected.example/foreign"
	if _, err := application.ApplyPrepared(t.Context(), identity, "example", binding, foreignTarget); errorCode(err) != sdd.ErrorWriteDenied {
		t.Fatalf("connected target exposure error = %v", err)
	}
	stored, err = sessions.Load(t.Context(), binding.SessionID)
	if err != nil || len(stored.Events) != 0 {
		t.Fatalf("foreign target persisted events = %d, %v", len(stored.Events), err)
	}

	diverged := preparedEntry(t, graph.GraphStore, binding, "diverged", "2026/07/13-052302-s-tac-div.md")
	diverged.Batch.Changes[0].Document.Body = "Different structured body."
	diverged.Batch.Digest, err = sdd.MutationBatchDigest(diverged.Batch)
	if err != nil {
		t.Fatal(err)
	}
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, diverged)
	if errorCode(err) != sdd.ErrorRecoveryRequired || result.Apply.State != sdd.MutationNotApplied || !strings.Contains(err.Error(), "structured entry and canonical bytes diverge") {
		t.Fatalf("structured divergence = %+v, %v", result, err)
	}
}

func TestCreateEntryResolvesConcreteDefaultWithoutCWDAndReleasesAroundLLM(t *testing.T) {
	graphDir := t.TempDir()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	targets := &activityTargetAcquirer{graph: graph}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	llmCalls := 0
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph, Targets: targets,
		Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				if targets.isActive() {
					return sdd.LLMResult{}, errors.New("LLM executed while mutation target was acquired")
				}
				llmCalls++
				if request.Purpose == "preflight" {
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				}
				return sdd.LLMResult{Output: []byte("Concrete target resolution is independent of cwd.")}, nil
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
	binding := openBinding(t, sessions, identity.Subject, "create-default")
	t.Chdir(t.TempDir())
	created, err := application.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "fact", Layer: "tactical", Body: "Concrete target resolution must remain independent of the process working directory.", Confidence: "high",
		Topics: []string{"implementation/engine"}, Index: &sdd.FactIndex{Title: "Concrete target resolution", Topic: "implementation/engine"},
	})
	if err != nil || created.EntryID == "" {
		t.Fatalf("CreateEntry = %+v, %v", created, err)
	}
	targets.mu.Lock()
	acquisitions, releases, active := targets.acquisitions, targets.releases, targets.active
	targets.mu.Unlock()
	if llmCalls != 2 || acquisitions != 2 || releases != 2 || active {
		t.Fatalf("LLM calls=%d acquisitions=%d releases=%d active=%v", llmCalls, acquisitions, releases, active)
	}
	stored, err := sessions.Load(t.Context(), binding.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !sessionHasEvent(stored.Events, "mutation_intent", `"Target":{"project":"example","branch":"main"}`) {
		t.Fatalf("ordinary capture did not persist its concrete default target: %+v", stored.Events)
	}
	rel, err := model.IDToRelPath(created.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(graphDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "index:\n    title: Concrete target resolution\n    topic: implementation/engine\n") {
		t.Fatalf("written entry missing nested index:\n%s", written)
	}
}

func TestPreparedRevalidationToleratesJSONRoundTripScalarTypes(t *testing.T) {
	application, sessions, _, graph := newDurableApplication(t, time.Now, nil, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "scalar-round-trip")
	prepared := preparedEntry(t, graph.GraphStore, binding, "scalar-round-trip", "2026/07/13-052305-s-tac-sca.md")
	prepared.Batch.Changes[0].CanonicalBytes = []byte("---\ntype: signal\nkind: done\nlayer: tactical\nsummary: Durable transition fixture.\ntime: 2026-07-13T05:23:05Z\n---\n\nDurable transition fixture body.\n")
	prepared.Batch.Changes[0].Document.Frontmatter["time"] = "2026-07-13T05:23:05Z"
	digest, err := sdd.MutationBatchDigest(prepared.Batch)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Batch.Digest = digest
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if err != nil || result.Apply.State != sdd.MutationApplied {
		t.Fatalf("scalar round-trip apply = %+v, %v", result, err)
	}
}

func TestPreparedAttachmentCrossesHomeStagingIntoTargetGraph(t *testing.T) {
	home, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	target, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	targets := &mutableTargetAcquirer{graph: target}
	application, sessions, blobs, _ := newDurableApplicationWithHomeAndTargets(t, home, targets)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "cross-target-attachment")
	owner := sdd.SessionRef{Subject: binding.Subject, Session: binding.SessionID}
	want := []byte("evidence from the home session\n")
	blob, err := application.StageBlob(t.Context(), identity, "example", owner, "evidence.txt", want)
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedEntry(t, target, binding, "cross-target", "2026/07/13-052303-s-tac-att.md")
	prepared.Target.Branch = "work"
	prepared.BlobIDs = []string{blob.ID}
	prepared.Batch.Attachments = []sdd.AttachmentMaterialization{{
		BlobID: blob.ID, Digest: blob.Digest, Size: blob.Size, SourceName: blob.Filename,
		LogicalPath: "2026/07/13-052303-s-tac-att/evidence.txt",
	}}
	prepared.Batch.Digest, err = sdd.MutationBatchDigest(prepared.Batch)
	if err != nil {
		t.Fatal(err)
	}
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if err != nil || result.Apply.State != sdd.MutationApplied {
		t.Fatalf("cross-target apply = %+v, %v", result, err)
	}
	page, err := target.ReadAttachmentPage(t.Context(), "20260713-052303-s-tac-att", "evidence.txt", 0, 1024)
	if err != nil || string(page.Content) != string(want) {
		t.Fatalf("target attachment = %q, %v", page.Content, err)
	}
	if _, err := home.ReadAttachmentPage(t.Context(), "20260713-052303-s-tac-att", "evidence.txt", 0, 1024); err == nil {
		t.Fatal("cross-target attachment was written to the home graph")
	}
	if blobs.released != 1 {
		t.Fatalf("home blob release count = %d", blobs.released)
	}
}

func TestLegacyIntentRequiresAuthorizedAuditedTargetBinding(t *testing.T) {
	authorizer := &recordingRecoveryAuthorizer{}
	application, sessions, _, graph := newDurableApplication(t, time.Now, nil, nil, authorizer)
	metadata := sdd.SessionMetadata{CodecVersion: sdd.SessionCodecVersion, ID: "legacy-v1", Subject: "christopher", Project: "example"}
	stored, err := sessions.Create(t.Context(), metadata)
	if err != nil {
		t.Fatal(err)
	}
	binding := sdd.SessionBinding{SessionID: metadata.ID, Subject: metadata.Subject, Project: metadata.Project, Version: stored.Version}
	prepared := preparedEntry(t, graph.GraphStore, binding, "legacy-v1", "2026/07/13-052304-s-tac-leg.md")
	prepared.Version = sdd.LegacyPreparedTransitionVersion
	prepared.Target = sdd.MutationTarget{}
	payload, err := json.Marshal(map[string]any{"prepared": prepared})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Append(t.Context(), metadata.ID, stored.Version, sdd.SessionAppend{Events: []sdd.StoredEvent{{
		CodecVersion: sdd.SessionCodecVersion, Code: "mutation_intent", Payload: payload,
	}}}); err != nil {
		t.Fatal(err)
	}

	list, err := application.ListRecoveries(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, "example", false)
	if err != nil || len(list.Items) != 1 || !list.Items[0].LegacyUnroutable || list.Items[0].Target.Branch != "" {
		t.Fatalf("legacy projection = %+v, %v", list, err)
	}
	boundTarget, err := application.RecoverMutation(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, "example", sdd.RecoveryRequest{
		Session: metadata.ID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryBindTarget,
		Target: sdd.MutationTarget{Project: "example", Branch: "main"}, Reason: "operator selected the historical branch",
	})
	if err != nil || boundTarget.Item.LegacyUnroutable || boundTarget.Item.Target.Branch != "main" {
		t.Fatalf("bind target = %+v, %v", boundTarget, err)
	}
	authorizer.mu.Lock()
	request := authorizer.request
	authorizer.mu.Unlock()
	if request.Verb != sdd.RecoveryBindTarget || request.Target.Branch != "main" {
		t.Fatalf("bind authorization = %+v", request)
	}
	stored, err = sessions.Load(t.Context(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sessionHasEvent(stored.Events, "legacy_target_bound", `"actor":"christopher"`) || !sessionHasEvent(stored.Events, "legacy_target_bound", `"reason":"operator selected the historical branch"`) {
		t.Fatalf("legacy binding audit = %+v", stored.Events)
	}
	refreshed, err := application.ReconcileMutation(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, "example", sdd.RecoveryReconcileRequest{Session: metadata.ID, MutationID: prepared.Batch.ID})
	if err != nil || refreshed.Item.State != sdd.RecoveryPending || refreshed.Item.Reason != sdd.RecoveryReasonNotApplied {
		t.Fatalf("bound legacy reconciliation = %+v, %v", refreshed, err)
	}
}

func TestRecoveryNonApplyPathsSurfaceTargetReleaseErrors(t *testing.T) {
	t.Run("discard", func(t *testing.T) {
		graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		targets := &mutableTargetAcquirer{graph: graph}
		application, sessions, _, _ := newDurableApplicationWithTargets(t, graph, targets)
		identity := sdd.RequestIdentity{Subject: "christopher"}
		binding := openBinding(t, sessions, identity.Subject, "release-discard")
		// A structurally diverged intent files a discardable not-applied
		// recovery without ever reaching the graph store.
		prepared := preparedEntry(t, graph, binding, "release-discard", "2026/07/13-052310-s-tac-dis.md")
		prepared.Batch.Changes[0].Document.Body = "Diverged structured body."
		prepared.Batch.Digest, err = sdd.MutationBatchDigest(prepared.Batch)
		if err != nil {
			t.Fatal(err)
		}
		result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
		if errorCode(err) != sdd.ErrorRecoveryRequired {
			t.Fatalf("diverged apply = %+v, %v", result, err)
		}
		targets.setReleaseError(errors.New("injected target release failure"))
		discarded, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryDiscard})
		if discarded.Item.State != sdd.RecoveryAbandoned || discarded.Item.Reason != sdd.RecoveryReasonDiscarded || err == nil || !strings.Contains(err.Error(), "injected target release failure") {
			t.Fatalf("discard = %+v, %v", discarded, err)
		}
	})

	t.Run("abandon unknown", func(t *testing.T) {
		base, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		graph := &pendingUnknownStore{GraphStore: base}
		targets := &mutableTargetAcquirer{graph: graph}
		application, sessions, _, _ := newDurableApplicationWithTargets(t, graph, targets)
		identity := sdd.RequestIdentity{Subject: "christopher"}
		binding := openBinding(t, sessions, identity.Subject, "release-abandon")
		prepared := preparedEntry(t, base, binding, "release-abandon", "2026/07/13-052320-s-tac-abn.md")
		result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
		if errorCode(err) != sdd.ErrorRecoveryRequired {
			t.Fatalf("unknown apply = %+v, %v", result, err)
		}
		targets.setReleaseError(errors.New("injected target release failure"))
		abandoned, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryAbandonUnknown})
		if abandoned.Item.State != sdd.RecoveryAbandoned || abandoned.Item.Reason != sdd.RecoveryReasonAbandonedUnknown || err == nil || !strings.Contains(err.Error(), "injected target release failure") {
			t.Fatalf("abandon = %+v, %v", abandoned, err)
		}
	})

	t.Run("finalize retry", func(t *testing.T) {
		base, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		graph := unknownAfterApplyStore{GraphStore: base}
		finalizer := &failOnceFinalizer{}
		targets := &mutableTargetAcquirer{graph: graph, finalizers: []sdd.MutationFinalizer{finalizer}}
		application, sessions, _, _ := newDurableApplicationWithTargets(t, graph, targets)
		identity := sdd.RequestIdentity{Subject: "christopher"}
		binding := openBinding(t, sessions, identity.Subject, "release-finalize")
		prepared := preparedEntry(t, base, binding, "release-finalize", "2026/07/13-052330-s-tac-fin.md")
		result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
		if errorCode(err) != sdd.ErrorRecoveryRequired {
			t.Fatalf("unknown apply = %+v, %v", result, err)
		}
		targets.setReleaseError(errors.New("injected target release failure"))
		_, err = application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryFinalizeRetry})
		if err == nil || !strings.Contains(err.Error(), "injected target release failure") {
			t.Fatalf("finalize retry error = %v", err)
		}
	})
}

func TestReadSurfacesNeverReplayPendingMutation(t *testing.T) {
	var pendingStore *pendingUnknownStore
	application, sessions, blobs, graph := newDurableApplication(t, time.Now, func(store sdd.GraphStore) sdd.GraphStore {
		pendingStore = &pendingUnknownStore{GraphStore: store}
		return pendingStore
	}, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "no-replay")
	prepared := preparedEntry(t, graph.GraphStore, binding, "pending-unknown", "2026/07/13-052500-s-tac-unk.md")
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if errorCode(err) != sdd.ErrorRecoveryRequired || result.Apply.State != sdd.MutationUnknown {
		t.Fatalf("pending ApplyPrepared = %+v, %v", result, err)
	}
	if _, err := application.ListRecoveries(t.Context(), identity, "example", false); err != nil {
		t.Fatal(err)
	}
	info, err := application.Info(t.Context(), identity, "example", sdd.InfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info.Recovery, "pending-unknown") || !strings.Contains(info.Recovery, string(sdd.RecoveryReasonOutcomeUnknown)) {
		t.Fatalf("Info recovery notice = %q", info.Recovery)
	}
	view, err := application.View(t.Context(), identity, "example", sdd.ViewRequest{Layout: "active:as-list"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.Sections, "pending-unknown") {
		t.Fatalf("View recovery notice = %q", view.Sections)
	}
	pendingStore.mu.Lock()
	reconciles := pendingStore.reconciles
	pendingStore.mu.Unlock()
	if reconciles != 0 || blobs.released != 0 {
		t.Fatalf("read surfaces reconciled %d times; released=%d", reconciles, blobs.released)
	}
	if _, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{
		Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryApply,
	}); errorCode(err) != sdd.ErrorRecoveryRequired {
		t.Fatalf("apply after unknown reconciliation error = %v", err)
	}
	if _, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{
		Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryDiscard,
	}); errorCode(err) != sdd.ErrorRecoveryRequired {
		t.Fatalf("discard after unknown reconciliation error = %v", err)
	}
	abandoned, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{
		Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryAbandonUnknown, Reason: "operator accepts unknown history",
	})
	if err != nil || abandoned.Item.State != sdd.RecoveryAbandoned || abandoned.Item.Reason != sdd.RecoveryReasonAbandonedUnknown || blobs.released != 1 {
		t.Fatalf("abandon unknown = %+v, %v; released=%d", abandoned, err, blobs.released)
	}
}

func TestRecoveryAuthorizationReceivesActorOwnerTargetAndDistinctVerb(t *testing.T) {
	authorizer := &recordingRecoveryAuthorizer{}
	application, sessions, _, graph := newDurableApplication(t, time.Now, nil, nil, authorizer)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "authorize-recovery")
	// A structurally diverged intent yields an actionable recovery item to
	// discard, without depending on a graph revision race.
	prepared := preparedEntry(t, graph.GraphStore, binding, "authorize-discard", "2026/07/13-052600-s-tac-aut.md")
	prepared.Batch.Changes[0].Document.Body = "Diverged structured body."
	digest, err := sdd.MutationBatchDigest(prepared.Batch)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Batch.Digest = digest
	result, err := application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if errorCode(err) != sdd.ErrorRecoveryRequired {
		t.Fatalf("diverged apply = %+v, %v", result, err)
	}
	if _, err := application.RecoverMutation(t.Context(), identity, "example", sdd.RecoveryRequest{
		Session: result.Binding.SessionID, MutationID: prepared.Batch.ID, Verb: sdd.RecoveryDiscard, Reason: "operator chose discard",
	}); err != nil {
		t.Fatal(err)
	}
	authorizer.mu.Lock()
	request := authorizer.request
	authorizer.mu.Unlock()
	if request.Actor.Subject != "christopher" || request.OriginalSubject != "christopher" || request.OriginalSession != "authorize-recovery" || request.Target.Branch != "main" || request.Verb != sdd.RecoveryDiscard {
		t.Fatalf("recovery authorization request = %+v", request)
	}
}

func TestPreparedTransitionSurfacesIntentAppendAndRetentionReleaseFailures(t *testing.T) {
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	baseSessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessions := &toggleAppendSessionStore{SessionStore: baseSessions}
	baseBlobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs := &trackingBlobStore{StagedBlobStore: baseBlobs, releaseErr: errors.New("injected release failure")}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil }, ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "append-failure")
	prepared := preparedEntry(t, graph, binding, "append-failure", "2026/07/13-053000-s-tac-fai.md")
	sessions.fail(errors.New("injected intent append failure"))

	_, err = application.ApplyPrepared(t.Context(), identity, "example", binding, prepared)
	if err == nil || !strings.Contains(err.Error(), "injected intent append failure") || !strings.Contains(err.Error(), "injected release failure") {
		t.Fatalf("ApplyPrepared error = %v, want both append and release failures", err)
	}
}

func TestSessionReplayFailsClosedForUnsupportedCodec(t *testing.T) {
	application, sessions, _, _ := newDurableApplication(t, time.Now, nil, nil)
	_, err := sessions.Create(t.Context(), sdd.SessionMetadata{CodecVersion: 99, ID: "future", Subject: "christopher", Project: "example"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = application.ResumeWorkflow(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, "example", sdd.WorkflowResumeRequest{SessionID: "future", MCPSessionID: "mcp"})
	var migration *sdd.ApplicationError
	if !errors.As(err, &migration) || migration.Code != sdd.ErrorMigrationRequired || migration.Version != 99 {
		t.Fatalf("unsupported codec error = %#v", err)
	}
}

type graphFixture struct {
	sdd.GraphStore
	dir string
}

func newDurableApplication(t *testing.T, now func() time.Time, wrap func(sdd.GraphStore) sdd.GraphStore, finalizers []sdd.MutationFinalizer, authorizers ...sdd.RecoveryAuthorizer) (*sdd.Application, *localadapter.FilesystemSessionStore, *trackingBlobStore, graphFixture) {
	t.Helper()
	dir := t.TempDir()
	baseGraph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var graph sdd.GraphStore = baseGraph
	if wrap != nil {
		graph = wrap(graph)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	baseBlobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs := &trackingBlobStore{StagedBlobStore: baseBlobs}
	authorizer := sdd.RecoveryAuthorizer(sdd.RecoveryAuthorizerFunc(func(context.Context, sdd.RecoveryAccessRequest) error { return nil }))
	if len(authorizers) > 0 {
		authorizer = authorizers[0]
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph, Sessions: sessions, StagedBlobs: blobs, Now: now, Finalizers: finalizers,
		Recovery: authorizer,
		LLM:      sdd.LLMExecutorFuncs{CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil }, ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil }},
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

func newDurableApplicationWithTargets(t *testing.T, graph sdd.GraphStore, targets sdd.TargetAcquirer) (*sdd.Application, *localadapter.FilesystemSessionStore, *trackingBlobStore, graphFixture) {
	return newDurableApplicationWithHomeAndTargets(t, graph, targets)
}

func newDurableApplicationWithHomeAndTargets(t *testing.T, home sdd.GraphStore, targets sdd.TargetAcquirer) (*sdd.Application, *localadapter.FilesystemSessionStore, *trackingBlobStore, graphFixture) {
	t.Helper()
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	baseBlobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs := &trackingBlobStore{StagedBlobStore: baseBlobs}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: home, Targets: targets,
		Sessions: sessions, StagedBlobs: blobs,
		Recovery: sdd.RecoveryAuthorizerFunc(func(context.Context, sdd.RecoveryAccessRequest) error { return nil }),
		LLM:      sdd.LLMExecutorFuncs{CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil }, ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return application, sessions, blobs, graphFixture{GraphStore: home}
}

func sessionHasEvent(events []sdd.StoredEvent, code, payloadFragment string) bool {
	for _, event := range events {
		if event.Code == code && strings.Contains(string(event.Payload), payloadFragment) {
			return true
		}
	}
	return false
}

func preparedEntry(t *testing.T, graph sdd.GraphStore, binding sdd.SessionBinding, id, path string) sdd.PreparedTransition {
	t.Helper()
	snapshot, err := graph.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	canonical := []byte("---\ntype: signal\nkind: done\nlayer: tactical\nsummary: Durable transition fixture.\n---\n\nDurable transition fixture body.\n")
	document := sdd.EntryDocument{LogicalPath: path, Frontmatter: map[string]any{
		"type": "signal", "kind": "done", "layer": "tactical", "summary": "Durable transition fixture.",
	}, Body: "Durable transition fixture body."}
	batch := sdd.MutationBatch{ID: id, Changes: []sdd.DocumentChange{{LogicalPath: path, Document: &document, CanonicalBytes: canonical}}}
	digest, err := sdd.MutationBatchDigest(batch)
	if err != nil {
		t.Fatal(err)
	}
	batch.Digest = digest
	return sdd.PreparedTransition{
		Version: sdd.PreparedTransitionVersion, Target: sdd.MutationTarget{Project: "example", Branch: "main"}, ExpectedGraphRevision: snapshot.Revision(), Batch: batch,
		Staged: sdd.SessionRef{Subject: binding.Subject, Session: binding.SessionID},
	}
}

func errorCode(err error) sdd.ErrorCode {
	if applicationErr, ok := errors.AsType[*sdd.ApplicationError](err); ok {
		return applicationErr.Code
	}
	return ""
}
