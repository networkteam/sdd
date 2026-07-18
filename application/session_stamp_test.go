package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
func newStampWorkflowApp(t *testing.T, graphDir, sessionsDir string, now func() time.Time) (*sdd.Application, *countingSessionStore, string, string) {
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
	fsSessions, err := localadapter.NewFilesystemSessionStore(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	sessions := &countingSessionStore{SessionStore: fsSessions}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, Graph: graph, Sessions: sessions, StagedBlobs: blobs, Now: now,
		LLM: sdd.LLMExecutorFuncs{CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil }, ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil }},
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
	// Another client attaches to the same session, displacing A's attachment.
	if _, _, err := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), MCPSessionID: "mcp-b"}); err != nil {
		t.Fatal(err)
	}
	// A's next write — a non-completing advance — must fail typed on this call,
	// with zero durable appends.
	before := sessions.appends.Load()
	_, advErr := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: serve.Instance, Report: map[string]any{"body": "one"}})
	if advErr == nil {
		t.Fatal("displaced writer's advance should fail, got success")
	}
	var appErr *sdd.ApplicationError
	if !errors.As(advErr, &appErr) || appErr.Code != sdd.ErrorSessionOwnership {
		t.Fatalf("displaced advance error = %v, want typed ErrorSessionOwnership on the first call", advErr)
	}
	if got := sessions.appends.Load() - before; got != 0 {
		t.Fatalf("displaced advance persisted %d appends, want 0", got)
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
