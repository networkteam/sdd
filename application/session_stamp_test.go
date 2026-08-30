package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/application"
	localadapter "github.com/networkteam/sdd/local"
)

// waitingProcedure stays on its single step until a second field arrives, so
// a report of just the first field is a single-event (non-completing) advance.
const waitingProcedure = `---
type: decision
kind: procedure
layer: process
canonical: waiting-test
confidence: high
summary: A workflow that waits for a second field before completing.
state:
    body: {type: text, desc: first note}
    anchor: {type: text, desc: second note}
steps:
    - id: collect-both
      collect: [body, anchor]
      transitions:
          - when: hasAnchor
            to: end(completed)
---

Waits for the anchor field.
`

// countingSessionStore wraps a session store and counts Append calls, so a
// test can assert exactly how many session-store writes an engine operation
// or a serve performs.
type countingSessionStore struct {
	sdd.SessionStore
	appends atomic.Int64
}

func (s *countingSessionStore) Append(ctx context.Context, id sdd.SessionID, version uint64, append sdd.SessionAppend) (uint64, error) {
	s.appends.Add(1)
	return s.SessionStore.Append(ctx, id, version, append)
}

// newStampWorkflowApp builds an application over the given (or fresh) store
// dirs with a counting session store and the terminal-test procedure
// available. It mirrors newShellFailureApplication but exposes the append
// counter and lets the caller pin the clock and reuse dirs across runtimes.
func newStampWorkflowApp(t *testing.T, graphDir, sessionsDir string, now func() time.Time, wrap ...func(sdd.SessionStore) sdd.SessionStore) (*sdd.Application, *countingSessionStore, string, string) {
	t.Helper()
	if graphDir == "" {
		graphDir = t.TempDir()
		for name, body := range map[string]string{
			"13-060000-d-prc-trm.md": terminalProcedure,
			"13-060001-d-prc-wai.md": waitingProcedure,
		} {
			path := filepath.Join(graphDir, "2026", "07", name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if sessionsDir == "" {
		sessionsDir = t.TempDir()
	}
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	fsSessions, err := localadapter.NewFilesystemSessionStoreAt(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	sessions := &countingSessionStore{SessionStore: fsSessions}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The counting store is always returned for append assertions; an optional
	// wrap layers over it (e.g. to inject version conflicts) and is what the app
	// actually writes through.
	var store sdd.SessionStore = sessions
	for _, w := range wrap {
		store = w(store)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, Graph: graph, Sessions: store, StagedBlobs: blobs, Now: now,
		LLM:        sdd.LLMExecutorFuncs{CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil }, ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil }},
		LLMTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return application, sessions, graphDir, sessionsDir
}

// TestEngineOperationAppendsOnceAndServesDoNotWrite covers AC4: with the bind
// round-trip gone, a single engine operation performs exactly one session-store
// append (its event, carrying the stamp), and serves perform none.
func TestEngineOperationAppendsOnceAndServesDoNotWrite(t *testing.T) {
	application, sessions, _, _ := newStampWorkflowApp(t, "", "", time.Now)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
	if err != nil {
		t.Fatal(err)
	}
	serve, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"})
	if err != nil {
		t.Fatal(err)
	}

	// A non-completing advance emits exactly one engine event (the report), so
	// it performs exactly one session-store append — the operation's own event
	// with the stamp, and nothing else.
	before := sessions.appends.Load()
	if _, err := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: serve.Instance, Report: map[string]any{"body": "one"}}); err != nil {
		t.Fatal(err)
	}
	if got := sessions.appends.Load() - before; got != 1 {
		t.Fatalf("advance performed %d session-store appends, want exactly 1 (no bind round-trip)", got)
	}

	// Serves are pure reads: reopening the shell, re-serving it, and serving
	// all open work must not touch the session store.
	before = sessions.appends.Load()
	if _, err := w.Reopen(t.Context(), identity, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ServeShell(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ServeAll(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	if got := sessions.appends.Load() - before; got != 0 {
		t.Fatalf("serves performed %d session-store appends, want 0", got)
	}
}

// TestDisplacedWriterFailsTypedImmediately covers I2 and the fail-loud rule:
// once another client displaces the attachment, the displaced writer's very
// next write fails typed synchronously — not a phantom success that surfaces
// only on a later call — and nothing is persisted.
func TestDisplacedWriterFailsTypedImmediately(t *testing.T) {
	application, sessions, _, _ := newStampWorkflowApp(t, "", "", time.Now)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	serve, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"})
	if err != nil {
		t.Fatal(err)
	}
	// Another client attaches to the same session, displacing A's attachment. The
	// session was just opened, so A's attachment reads recent: the consenting
	// attach carries the user's words and takes over.
	if _, _, err := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), MCPSessionID: "mcp-b", UserWords: "pick up where the other one left off", Takeover: true}); err != nil {
		t.Fatal(err)
	}
	// A's next write — a non-completing advance — must fail typed on this call,
	// with zero durable appends, naming the displacement (not a bare ownership
	// violation).
	before := sessions.appends.Load()
	_, advErr := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: serve.Instance, Report: map[string]any{"body": "one"}})
	if advErr == nil {
		t.Fatal("displaced writer's advance should fail, got success")
	}
	var appErr *sdd.ApplicationError
	if !errors.As(advErr, &appErr) || appErr.Code != sdd.ErrorSessionDisplaced {
		t.Fatalf("displaced advance error = %v, want typed ErrorSessionDisplaced on the first call", advErr)
	}
	if got := sessions.appends.Load() - before; got != 0 {
		t.Fatalf("displaced advance persisted %d appends, want 0", got)
	}
}

// TestSameWriterVersionRaceRetriesInvisibly covers decision 12's benign race:
// when the stored version advances under a writer whose attachment is unchanged,
// the next append resyncs and retries once — no error surfaces and the write
// lands.
func TestSameWriterVersionRaceRetriesInvisibly(t *testing.T) {
	application, sessions, _, _ := newStampWorkflowApp(t, "", "", time.Now)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
	if err != nil {
		t.Fatal(err)
	}
	serve, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"})
	if err != nil {
		t.Fatal(err)
	}
	// Advance the stored version behind the binding's back, keeping the same
	// attachment: a benign same-writer version drift, not a displacement.
	stored, err := sessions.Load(t.Context(), w.ID())
	if err != nil {
		t.Fatal(err)
	}
	metadata := stored.Metadata
	if _, err := sessions.Append(t.Context(), w.ID(), stored.Version, sdd.SessionAppend{Metadata: &metadata}); err != nil {
		t.Fatal(err)
	}
	// The binding's observed version is now stale. A same-writer advance must
	// resync and retry invisibly — no error, and the write persists.
	before := sessions.appends.Load()
	if _, err := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: serve.Instance, Report: map[string]any{"body": "one"}}); err != nil {
		t.Fatalf("same-writer version race should retry invisibly, got %v", err)
	}
	if got := sessions.appends.Load() - before; got == 0 {
		t.Fatal("the retried advance did not persist its append")
	}
}

// conflictSessionStore returns a version conflict on Append while armed, so a
// test can force the retry loop to exhaust its single retry.
type conflictSessionStore struct {
	sdd.SessionStore
	fail atomic.Bool
}

func (s *conflictSessionStore) Append(ctx context.Context, id sdd.SessionID, version uint64, appendData sdd.SessionAppend) (uint64, error) {
	if s.fail.Load() {
		return version, &sdd.ApplicationError{Code: sdd.ErrorSessionConflict, Message: "session version changed"}
	}
	return s.SessionStore.Append(ctx, id, version, appendData)
}

// TestSameWriterSecondConflictSurfaces covers the tail of the benign-race retry:
// when the single reload+retry also conflicts, the conflict surfaces typed —
// with reorient guidance appended rather than a dead-end "version changed".
func TestSameWriterSecondConflictSurfaces(t *testing.T) {
	conflict := &conflictSessionStore{}
	application, _, _, _ := newStampWorkflowApp(t, "", "", time.Now, func(s sdd.SessionStore) sdd.SessionStore {
		conflict.SessionStore = s
		return conflict
	})
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
	if err != nil {
		t.Fatal(err)
	}
	serve, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"})
	if err != nil {
		t.Fatal(err)
	}
	// Arm perpetual conflicts: both the initial append and its retry lose.
	conflict.fail.Store(true)
	_, advErr := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: serve.Instance, Report: map[string]any{"body": "one"}})
	var appErr *sdd.ApplicationError
	if !errors.As(advErr, &appErr) || appErr.Code != sdd.ErrorSessionConflict {
		t.Fatalf("second conflict error = %v, want typed ErrorSessionConflict", advErr)
	}
	if !strings.Contains(advErr.Error(), "resume_session") {
		t.Fatalf("the surfaced second conflict should carry reorient guidance, got %q", advErr.Error())
	}
}

// failOnceSessionStore fails the next Append with a version conflict, then
// disarms — the shape of a single lost race.
type failOnceSessionStore struct {
	sdd.SessionStore
	armed atomic.Bool
}

func (s *failOnceSessionStore) Append(ctx context.Context, id sdd.SessionID, version uint64, appendData sdd.SessionAppend) (uint64, error) {
	if s.armed.CompareAndSwap(true, false) {
		return version, &sdd.ApplicationError{Code: sdd.ErrorSessionConflict, Message: "session version changed"}
	}
	return s.SessionStore.Append(ctx, id, version, appendData)
}

// TestConsentedAttachRetriesPastRace covers the consent-check→append race: a
// consenting attach that loses the append to a racing writer reloads, re-checks
// consent, and retries once — the user who did everything right never sees a raw
// version conflict; the attach lands.
func TestConsentedAttachRetriesPastRace(t *testing.T) {
	t0 := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	now := t0
	failOnce := &failOnceSessionStore{}
	application, sessions, _, _ := newStampWorkflowApp(t, "", "", func() time.Time { return now }, func(s sdd.SessionStore) sdd.SessionStore {
		failOnce.SessionStore = s
		return failOnce
	})
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"}); err != nil {
		t.Fatal(err)
	}
	// Idle so the foreign attach needs only the user's words; arm one lost race.
	now = t0.Add(sdd.SessionRecencyWindow + time.Minute)
	failOnce.armed.Store(true)
	_, result, err := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), MCPSessionID: "mcp-b", UserWords: "pick this up"})
	if err != nil {
		t.Fatalf("consented attach should retry past a version race, got %v", err)
	}
	if result.Session != w.ID() {
		t.Fatalf("attach after retry should land on %s, got %s", w.ID(), result.Session)
	}
	stored, err := sessions.Load(t.Context(), w.ID())
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.Attachment == nil || stored.Metadata.Attachment.MCPSessionID != "mcp-b" {
		t.Fatalf("the retried claim should leave mcp-b as the attachment, got %+v", stored.Metadata.Attachment)
	}
}

// TestAbandonMidTeardownLeavesVictimAttachmentIntact is the regression for 1a:
// teardown never becomes the attachment, so a failure at its final append
// leaves the victim's attachment untouched — no phantom hold, no abandon record.
func TestAbandonMidTeardownLeavesVictimAttachmentIntact(t *testing.T) {
	t0 := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	now := t0
	conflict := &conflictSessionStore{}
	application, sessions, _, _ := newStampWorkflowApp(t, "", "", func() time.Time { return now }, func(s sdd.SessionStore) sdd.SessionStore {
		conflict.SessionStore = s
		return conflict
	})
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"}); err != nil {
		t.Fatal(err)
	}
	// Idle so teardown is allowed; then force its final append to fail.
	now = t0.Add(sdd.SessionRecencyWindow + time.Minute)
	conflict.fail.Store(true)
	if _, err := application.AbandonWorkflowSession(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), MCPSessionID: "mcp-b"}, "boom"); err == nil {
		t.Fatal("expected the teardown's final append to fail")
	}
	conflict.fail.Store(false)
	stored, err := sessions.Load(t.Context(), w.ID())
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.Attachment == nil || stored.Metadata.Attachment.MCPSessionID != "mcp-a" {
		t.Fatalf("a failed teardown must leave the victim's attachment intact, got %+v", stored.Metadata.Attachment)
	}
	if stored.Metadata.Ended != nil {
		t.Fatalf("a failed teardown must record no abandon, got %+v", stored.Metadata.Ended)
	}
}

// TestAbandonedWhileAttachedNamesActorTimeReason covers D1: after a session is
// torn down by handle, the still-attached client's next write fails typed and
// the interpreted error names who abandoned it, when, and why.
func TestAbandonedWhileAttachedNamesActorTimeReason(t *testing.T) {
	t0 := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	now := t0
	application, sessions, _, _ := newStampWorkflowApp(t, "", "", func() time.Time { return now })
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	serve, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"})
	if err != nil {
		t.Fatal(err)
	}
	// Time passes past the recency window so the attachment reads idle: an active
	// session would refuse teardown. Another connection then tears it down.
	now = t0.Add(sdd.SessionRecencyWindow + time.Minute)
	if _, err := application.AbandonWorkflowSession(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), MCPSessionID: "mcp-b"}, "stale branch"); err != nil {
		t.Fatal(err)
	}
	stored, err := sessions.Load(t.Context(), w.ID())
	if err != nil {
		t.Fatal(err)
	}
	// Attaching to a torn-down handle is refused: an ended dialogue is kept to be
	// read, never resumed.
	_, _, reopenErr := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), MCPSessionID: "mcp-c", UserWords: "reopen the torn-down handle"})
	var reopenAppErr *sdd.ApplicationError
	if !errors.As(reopenErr, &reopenAppErr) || reopenAppErr.Code != sdd.ErrorSessionEnded {
		t.Fatalf("reopening a torn-down handle = %v, want typed ErrorSessionEnded", reopenErr)
	}
	// The terminal record wins over a newer stamp (item 5): the victim's write must
	// hear the abandon, not "taken over". No served path can stamp an ended session
	// any more, so the precondition is written directly.
	newer := stored.Metadata
	newer.Attachment = &sdd.Attachment{Subject: newer.Subject, MCPSessionID: "mcp-c", LastActivity: now}
	if _, err := sessions.Append(t.Context(), w.ID(), stored.Version, sdd.SessionAppend{Metadata: &newer}); err != nil {
		t.Fatal(err)
	}
	_, advErr := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: serve.Instance, Report: map[string]any{"body": "one"}})
	var appErr *sdd.ApplicationError
	if !errors.As(advErr, &appErr) || appErr.Code != sdd.ErrorSessionDisplaced {
		t.Fatalf("abandoned-while-attached advance error = %v, want ErrorSessionDisplaced", advErr)
	}
	msg := advErr.Error()
	for _, want := range []string{"abandoned by " + stored.Metadata.Participant, now.Format(time.RFC3339), "reason: stale branch"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("abandon message %q missing %q (actor/time/reason)", msg, want)
		}
	}
}

// TestAttachmentActiveClampsClockSkew covers item 6: a future-dated stamp from
// clock skew must not read active indefinitely — activity outside the recency
// window on either side of now is idle.
func TestAttachmentActiveClampsClockSkew(t *testing.T) {
	t0 := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	now := t0
	application, _, graphDir, sessionsDir := newStampWorkflowApp(t, "", "", func() time.Time { return now })
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "terminal-test"}); err != nil {
		t.Fatal(err)
	}
	// A fresh runtime whose clock reads far BEFORE the stamp (skew) must not
	// classify the session active — a future stamp beyond the window is idle.
	now = t0.Add(-2 * sdd.SessionRecencyWindow)
	revived, _, _, _ := newStampWorkflowApp(t, graphDir, sessionsDir, func() time.Time { return now })
	list, err := revived.ListWorkflowSessions(t.Context(), identity, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Active {
		t.Fatalf("a stamp beyond the future side of the window must read idle, got %+v", list)
	}
}

// TestKillWithoutGoodbyeClassifiesIdle covers AC9/I7: session status is derived
// from the store alone. A runtime that wrote events and open work but never
// recorded a leave leaves the attachment stamp present; a fresh runtime whose
// clock has moved past the recency window lists the session as idle with its
// open work — no liveness flag, no expiry.
func TestKillWithoutGoodbyeClassifiesIdle(t *testing.T) {
	t0 := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	now := t0
	application, _, graphDir, sessionsDir := newStampWorkflowApp(t, "", "", func() time.Time { return now })
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "terminal-test"}); err != nil {
		t.Fatal(err)
	}
	// The server dies here: no Leave/Release is ever called. A fresh runtime
	// opens over the same store with its clock moved past the recency window.
	now = t0.Add(sdd.SessionRecencyWindow + time.Minute)
	revived, _, _, _ := newStampWorkflowApp(t, graphDir, sessionsDir, func() time.Time { return now })
	sessionsList, err := revived.ListWorkflowSessions(t.Context(), identity, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionsList) != 1 {
		t.Fatalf("revived runtime listed %d sessions, want 1", len(sessionsList))
	}
	got := sessionsList[0]
	if len(got.Open) == 0 {
		t.Fatal("the killed session should still list its open work")
	}
	if got.Attachment == nil {
		t.Fatal("the attachment stamp should survive a kill without a leave event")
	}
	if got.Active {
		t.Fatal("a stale stamp past the recency window should classify idle, not active")
	}
	_ = w
}
