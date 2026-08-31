package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// recordingSessionStore captures each append's payload, so a test can assert
// which metadata and which events landed in the SAME atomic append rather than
// only what the store holds afterwards.
type recordingSessionStore struct {
	sdd.SessionStore
	mu      sync.Mutex
	appends []sdd.SessionAppend
}

func (s *recordingSessionStore) Append(ctx context.Context, id sdd.SessionID, version uint64, appendData sdd.SessionAppend) (uint64, error) {
	next, err := s.SessionStore.Append(ctx, id, version, appendData)
	if err == nil {
		s.mu.Lock()
		s.appends = append(s.appends, appendData)
		s.mu.Unlock()
	}
	return next, err
}

// firstEnd returns the terminal record the first append to carry one wrote, and
// whether that same append also carried a terminal engine event for the named
// instance — the atomicity a terminal record requires. Later appends carry the
// already-persisted record forward and say nothing about how it landed.
func (s *recordingSessionStore) firstEnd(t *testing.T, instance string) (*sdd.SessionEnd, bool) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, appendData := range s.appends {
		if appendData.Metadata == nil || appendData.Metadata.Ended == nil {
			continue
		}
		for _, stored := range appendData.Events {
			var event struct {
				Event    string
				Instance string
			}
			if err := json.Unmarshal(stored.Payload, &event); err != nil {
				t.Fatalf("decoding recorded event: %v", err)
			}
			if event.Instance == instance && (event.Event == "completed" || event.Event == "abandoned") {
				return appendData.Metadata.Ended, true
			}
		}
		return appendData.Metadata.Ended, false
	}
	return nil, false
}

// newConcludeApp builds an application whose session appends are recorded, with
// the clock pinned so a terminal record's timestamp is assertable.
func newConcludeApp(t *testing.T, now time.Time) (*sdd.Application, *recordingSessionStore) {
	t.Helper()
	recorder := &recordingSessionStore{}
	application, _, _, _ := newStampWorkflowApp(t, "", "", func() time.Time { return now }, func(s sdd.SessionStore) sdd.SessionStore {
		recorder.SessionStore = s
		return recorder
	})
	return application, recorder
}

func concludeReport() map[string]any {
	return map[string]any{"chooser": "junction", "choice": "conclude", "userWords": "we're done for today"}
}

// TestConcludeWritesTerminalRecordInOneAppend pins the write site: answering the
// shell's conclude junction records SessionConcluded in the same append as the
// act it records, and the serve carries the way on rather than a spent position.
// It holds whether or not the session still has a thread open — conclude is
// un-gated, so a left-behind move must not defer the terminal record.
func TestConcludeWritesTerminalRecordInOneAppend(t *testing.T) {
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	for _, openThread := range []bool{false, true} {
		name := "quiescent"
		if openThread {
			name = "with a thread left behind"
		}
		t.Run(name, func(t *testing.T) {
			application, recorder := newConcludeApp(t, now)
			identity := sdd.RequestIdentity{Subject: "christopher"}
			w, shell, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
			if err != nil {
				t.Fatal(err)
			}
			if openThread {
				if _, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"}); err != nil {
					t.Fatal(err)
				}
			}

			serve, err := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: shell.Instance, Report: concludeReport()})
			if err != nil {
				t.Fatal(err)
			}
			if serve.Status != "completed" {
				t.Fatalf("conclude should complete the shell, got %s at %q", serve.Status, serve.Step)
			}
			if !strings.Contains(serve.Instructions, "start_session") || !strings.Contains(serve.Instructions, "finished") {
				t.Fatalf("the conclude serve should state the session is finished and name start_session, got %q", serve.Instructions)
			}
			// The thread is left standing, not settled — that is what the serve reports.
			if got := len(w.OpenInstances()); got != map[bool]int{false: 0, true: 1}[openThread] {
				t.Fatalf("conclude changed the open instances: %d", got)
			}

			end, withAct := recorder.firstEnd(t, shell.Instance)
			if end == nil {
				t.Fatal("conclude wrote no terminal record")
			}
			if !withAct {
				t.Fatal("the terminal record landed in an append of its own — it must be written with the act it records")
			}
			if end.Act != sdd.SessionConcluded {
				t.Fatalf("conclude recorded act %q, want %q", end.Act, sdd.SessionConcluded)
			}
			if !end.EndedAt.Equal(now) {
				t.Fatalf("terminal record EndedAt = %s, want %s", end.EndedAt, now)
			}
		})
	}
}

// TestConcludedSessionIsNoLongerOpenWork pins the listing consequence of leaving
// threads behind: ended-ness is the authority, so a concluded session's still-
// running instances are never offered as open work, while a session nobody ended
// keeps listing its own.
func TestConcludedSessionIsNoLongerOpenWork(t *testing.T) {
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	application, _ := newConcludeApp(t, now)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, shell, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"}); err != nil {
		t.Fatal(err)
	}

	listed, err := application.ListWorkflowSessions(t.Context(), identity, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || len(listed[0].Open) != 1 {
		t.Fatalf("an unended session should list its open work, got %+v", listed)
	}

	if _, err := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: shell.Instance, Report: concludeReport()}); err != nil {
		t.Fatal(err)
	}
	listed, err = application.ListWorkflowSessions(t.Context(), identity, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("a concluded session should stay readable in the listing, got %+v", listed)
	}
	if len(listed[0].Open) != 0 {
		t.Fatalf("a concluded session must hold no open work, got %+v", listed[0].Open)
	}
}

// TestConcludedSessionRefusesRevival covers the post-conclude lifecycle: nothing
// served carries a finished dialogue on — a move start, an advance, and an attach
// by handle each refuse, naming the one path that works (s-tac-3be).
func TestConcludedSessionRefusesRevival(t *testing.T) {
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	application, _ := newConcludeApp(t, now)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, shell, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: shell.Instance, Report: concludeReport()}); err != nil {
		t.Fatal(err)
	}
	if !w.Finished() {
		t.Fatal("a concluded session should report finished")
	}

	refusals := map[string]error{}
	_, refusals["start_procedure"] = w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"})
	_, refusals["next"] = w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: shell.Instance, Report: map[string]any{"body": "one"}})
	_, _, refusals["resume_session"] = application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{
		SessionID: w.ID(), MCPSessionID: "mcp-2", UserWords: "pick the concluded dialogue back up",
	})
	for name, err := range refusals {
		var appErr *sdd.ApplicationError
		if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorSessionEnded {
			t.Fatalf("%s against a concluded session = %v, want typed ErrorSessionEnded", name, err)
		}
		if !strings.Contains(err.Error(), "start_session") {
			t.Fatalf("%s refusal must name the new-session path, got %q", name, err.Error())
		}
	}
}

// TestQuiescentLeaveEndsTheSessionAndOpenWorkParks pins the second conclude write
// site against the park rule: a shell-only session left behind is genuinely ended
// (and so collectable), while a session with an open move parks — parked is not
// ended.
func TestQuiescentLeaveEndsTheSessionAndOpenWorkParks(t *testing.T) {
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)

	t.Run("quiescent", func(t *testing.T) {
		application, recorder := newConcludeApp(t, now)
		identity := sdd.RequestIdentity{Subject: "christopher"}
		w, shell, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Leave(t.Context(), identity); err != nil {
			t.Fatal(err)
		}
		end, withAct := recorder.firstEnd(t, shell.Instance)
		if end == nil || end.Act != sdd.SessionConcluded {
			t.Fatalf("leaving a quiescent session should record a concluded end, got %+v", end)
		}
		if !withAct {
			t.Fatal("the auto-conclude's terminal record landed in an append of its own")
		}
	})

	t.Run("open move parks", func(t *testing.T) {
		application, recorder := newConcludeApp(t, now)
		identity := sdd.RequestIdentity{Subject: "christopher"}
		w, shell, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-1"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "waiting-test"}); err != nil {
			t.Fatal(err)
		}
		if err := w.Leave(t.Context(), identity); err != nil {
			t.Fatal(err)
		}
		if end, _ := recorder.firstEnd(t, shell.Instance); end != nil {
			t.Fatalf("leaving a session with open work must end nothing, got %+v", end)
		}
	})
}
