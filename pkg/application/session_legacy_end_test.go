package application_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// legacyShellData is a shell instance's started-event data; a move's omits the
// class, exactly as the engine logs it.
const (
	legacyShellData = `{"class":"shell","entry":"20260704-100000-d-prc-dlg","procedure":"user-dialogue","step":"junction"}`
	legacyMoveData  = `{"entry":"20260704-100000-d-prc-cap","procedure":"capture","step":"open"}`
)

func legacyEvent(id sdd.SessionID, seq int, instance, event, ts, data string) string {
	if data == "" {
		data = "{}"
	}
	return fmt.Sprintf(`{"v":1,"ts":%q,"session":%q,"seq":%d,"instance":%q,"event":%q,"data":%s}`,
		ts, id, seq, instance, event, data)
}

// writeLegacyLog writes a log in the shape sdd wrote before an ending was
// recorded in metadata: a metadata line stating no terminal record (unless
// endedField supplies one), then one engine event per append.
func (f collectFixture) writeLegacyLog(t *testing.T, id sdd.SessionID, endedField string, events ...string) {
	t.Helper()
	if err := os.MkdirAll(f.sessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lines := []string{fmt.Sprintf(
		`{"version":1,"metadata":{"CodecVersion":1,"ID":%q,"Subject":%q,"Project":%q,"Participant":"Christopher","Attachment":null,"AttachmentHistory":null%s,"UpdatedAt":"2026-07-30T09:00:00Z"}}`,
		id, collectSubject, f.project, endedField)}
	for i, event := range events {
		lines = append(lines, fmt.Sprintf(
			`{"version":%d,"events":[{"CodecVersion":1,"Code":"workflow_event","Payload":%s}]}`, i+2, event))
	}
	path := filepath.Join(f.sessionsDir, string(id)+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyEndIsDerivedFromTheShellsTerminalEvent covers the second-tier
// recovery: a log whose participant concluded before any ending was recorded
// states that ending only through its shell's own engine event, and the derived
// record must be the one every consumer sees — here, the refusal that keeps a
// finished dialogue from being carried on.
func TestLegacyEndIsDerivedFromTheShellsTerminalEvent(t *testing.T) {
	const (
		shellStart = "2026-07-20T09:00:00Z"
		firstEnd   = "2026-07-20T10:00:00Z"
		secondEnd  = "2026-07-20T11:00:00Z"
	)
	for _, tc := range []struct {
		name    string
		ended   string
		events  []string
		wantAct sdd.SessionEndAct
		wantAt  string
	}{
		{
			name: "a completed shell ends the session",
			events: []string{
				legacyEvent("s_legacy", 1, "i_1", "started", shellStart, legacyShellData),
				legacyEvent("s_legacy", 2, "i_1", "completed", firstEnd, ""),
			},
			wantAct: sdd.SessionConcluded, wantAt: firstEnd,
		},
		{
			name: "an abandoned shell ends the session the same way",
			events: []string{
				legacyEvent("s_legacy", 1, "i_1", "started", shellStart, legacyShellData),
				legacyEvent("s_legacy", 2, "i_1", "abandoned", firstEnd, ""),
			},
			wantAct: sdd.SessionConcluded, wantAt: firstEnd,
		},
		{
			name: "a revived shell ends the session when it too is over",
			events: []string{
				legacyEvent("s_legacy", 1, "i_1", "started", shellStart, legacyShellData),
				legacyEvent("s_legacy", 2, "i_1", "completed", firstEnd, ""),
				legacyEvent("s_legacy", 3, "i_2", "started", firstEnd, legacyShellData),
				legacyEvent("s_legacy", 4, "i_2", "abandoned", secondEnd, ""),
			},
			wantAct: sdd.SessionConcluded, wantAt: secondEnd,
		},
		{
			name: "a running shell keeps the session live however its moves ended",
			events: []string{
				legacyEvent("s_legacy", 1, "i_1", "started", shellStart, legacyShellData),
				legacyEvent("s_legacy", 2, "i_2", "started", shellStart, legacyMoveData),
				legacyEvent("s_legacy", 3, "i_2", "completed", firstEnd, ""),
			},
		},
		{
			name:  "a recorded ending is never overridden",
			ended: `,"Ended":{"Act":"abandoned","EndedAt":"` + secondEnd + `","Reason":"torn down"}`,
			events: []string{
				legacyEvent("s_legacy", 1, "i_1", "started", shellStart, legacyShellData),
				legacyEvent("s_legacy", 2, "i_1", "completed", firstEnd, ""),
			},
			wantAct: sdd.SessionAbandoned, wantAt: secondEnd,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCollectFixture(t)
			f.writeLegacyLog(t, "s_legacy", tc.ended, tc.events...)

			_, _, err := f.app.ResumeWorkflow(t.Context(), f.identity, sdd.WorkflowResumeRequest{
				SessionID: "s_legacy", ClientName: "mcp-1",
			})
			var appErr *sdd.ApplicationError
			ended := errors.As(err, &appErr) && appErr.Code == sdd.ErrorSessionEnded

			if tc.wantAct == "" {
				if ended {
					t.Fatalf("resume refused a session whose shell is still running: %v", err)
				}
				return
			}
			if !ended {
				t.Fatalf("resume of an ended session = %v, want typed ErrorSessionEnded", err)
			}
			if appErr.Ended == nil {
				t.Fatal("the refusal carries no terminal record")
			}
			if appErr.Ended.Act != tc.wantAct {
				t.Fatalf("Ended.Act = %q, want %q", appErr.Ended.Act, tc.wantAct)
			}
			if got := appErr.Ended.EndedAt.Format(time.RFC3339); got != tc.wantAt {
				t.Fatalf("Ended.EndedAt = %s, want %s", got, tc.wantAt)
			}
			if !strings.Contains(err.Error(), "start_session") {
				t.Fatalf("the refusal must name the new-session path, got %q", err.Error())
			}
		})
	}
}

// TestCollectRemovesLegacyShellEndedSessions is the collection consequence: a
// session whose ending only its shell's event states is as collectable as one
// that recorded it, and retention still decides.
func TestCollectRemovesLegacyShellEndedSessions(t *testing.T) {
	f := newCollectFixture(t)
	shellStart := f.now.Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	ended := func(id sdd.SessionID, at time.Time) []string {
		return []string{
			legacyEvent(id, 1, "i_1", "started", shellStart, legacyShellData),
			legacyEvent(id, 2, "i_1", "completed", at.Format(time.RFC3339), ""),
		}
	}
	f.writeLegacyLog(t, "s_legacy_old", "", ended("s_legacy_old", f.now.Add(-30*24*time.Hour))...)
	f.writeLegacyLog(t, "s_legacy_recent", "", ended("s_legacy_recent", f.now.Add(-time.Hour))...)
	f.writeLegacyLog(t, "s_legacy_open", "", legacyEvent("s_legacy_open", 1, "i_1", "started", shellStart, legacyShellData))

	result := f.collect(t, 14*24*time.Hour)

	removed := make([]string, 0, len(result.RemovedSessions))
	for _, id := range result.RemovedSessions {
		removed = append(removed, string(id))
	}
	if !slices.Equal(removed, []string{"s_legacy_old"}) {
		t.Fatalf("removed = %v, want the legacy session ended past retention", removed)
	}
	if got := f.listedIDs(t); !slices.Equal(got, []string{"s_legacy_open", "s_legacy_recent"}) {
		t.Fatalf("remaining = %v, want the in-window and still-running sessions", got)
	}
}
