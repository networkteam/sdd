package local_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// releasedFormat is one on-disk session-log shape sdd has released. Every one
// of them must stay readable in every location sdd has written to — that is the
// whole of session compatibility, and this table is where the claim is checked.
type releasedFormat struct {
	name string
	// log is the complete file content for session id "s_compat".
	log string
	// wantParticipant is the participant a reader must recover, whether the
	// format states it in metadata or only implies it through events.
	wantParticipant string
	// wantEvents is how many events the log carries.
	wantEvents int
}

func releasedFormats() []releasedFormat {
	return []releasedFormat{
		{
			// Pre-0.16: one engine event per line, no envelope and no
			// metadata line at all.
			name: "pre-0.16 event-only",
			log: `{"v":1,"ts":"2026-07-01T12:00:00Z","session":"s_compat","seq":1,"event":"session_meta","data":{"participant":"Christopher"}}` + "\n" +
				`{"v":1,"ts":"2026-07-01T12:00:01Z","session":"s_compat","seq":2,"event":"labeled","data":{"label":"parked"}}` + "\n",
			wantParticipant: "Christopher",
			wantEvents:      2,
		},
		{
			// 0.16 through 2026-07-18: the current envelope, whose metadata
			// still carried the holder lease that attachment stamps replaced.
			// Reading this shape is what the strict-decoding outage was about
			// (s-tac-jit); lenient decoding makes it ordinary.
			name: "0.16 envelope with holder lease",
			log: `{"version":1,"metadata":{"CodecVersion":1,"ID":"s_compat","Subject":"local","Project":"PROJECT","Participant":"Christopher","Label":"","Holder":{"Subject":"local","MCPSessionID":"c-1","LeaseExpiry":"2026-07-15T10:00:00Z"},"HolderHistory":[],"UpdatedAt":"2026-07-15T09:00:00Z"}}` + "\n" +
				`{"version":2,"events":[{"CodecVersion":1,"Code":"workflow_event","Payload":{"v":1,"seq":1,"event":"started"}}]}` + "\n",
			wantParticipant: "Christopher",
			wantEvents:      1,
		},
		{
			name: "current envelope",
			log: `{"version":1,"metadata":{"CodecVersion":1,"ID":"s_compat","Subject":"local","Project":"PROJECT","Participant":"Christopher","Label":"","Attachment":null,"AttachmentHistory":null,"UpdatedAt":"2026-07-30T09:00:00Z"}}` + "\n" +
				`{"version":2,"events":[{"CodecVersion":1,"Code":"workflow_event","Payload":{"v":1,"seq":1,"event":"started"}}]}` + "\n",
			wantParticipant: "Christopher",
			wantEvents:      1,
		},
	}
}

const (
	compatSessionID = sdd.SessionID("s_compat")
	compatRepoID    = "github.com/example/compat"
)

// compatLocations builds the three locations sdd has ever written sessions to,
// over throwaway directories.
func compatLocations(t *testing.T) []localadapter.StoreLocation {
	t.Helper()
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	sddDir := filepath.Join(root, "repo", ".sdd")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sddDir, 0o700); err != nil {
		t.Fatal(err)
	}
	locations, err := localadapter.SessionLocations(stateRoot, sddDir, compatRepoID, filepath.Join(root, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 3 {
		t.Fatalf("locations = %d, want the repository-identity, identity-less and in-tree stores", len(locations))
	}
	return locations
}

// TestEveryReleasedFormatIsLiveInEveryLocation is the compatibility and
// acceptance check for the session store: for each released log shape, placed
// in each location sdd has written to, the session must be listed, loadable by
// handle, and appendable — with the append landing in the very file it was
// resolved from and nothing written to any other location.
func TestEveryReleasedFormatIsLiveInEveryLocation(t *testing.T) {
	for _, format := range releasedFormats() {
		for index := range 3 {
			locations := compatLocations(t)
			target := locations[index]
			t.Run(fmt.Sprintf("%s in %s", format.name, target.Name), func(t *testing.T) {
				logPath := filepath.Join(target.Sessions, string(compatSessionID)+".jsonl")
				if err := os.MkdirAll(target.Sessions, 0o700); err != nil {
					t.Fatal(err)
				}
				content := strings.ReplaceAll(format.log, "PROJECT", string(target.Project))
				if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}

				store, err := localadapter.NewFilesystemSessionStore(locations...)
				if err != nil {
					t.Fatal(err)
				}

				listed, err := store.List(t.Context(), sdd.SessionFilter{})
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(listed) != 1 || listed[0].Metadata.ID != compatSessionID {
					t.Fatalf("List = %+v, want the session found in %s", listed, target.Name)
				}
				if got := listed[0].Metadata.Participant; got != format.wantParticipant {
					t.Fatalf("participant = %q, want %q", got, format.wantParticipant)
				}

				stored, err := store.Load(t.Context(), compatSessionID)
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if len(stored.Events) != format.wantEvents {
					t.Fatalf("events = %d, want %d", len(stored.Events), format.wantEvents)
				}

				next, err := store.Append(t.Context(), compatSessionID, stored.Version, sdd.SessionAppend{
					Events: []sdd.StoredEvent{{CodecVersion: 1, Code: "workflow_event", Payload: json.RawMessage(`{"appended":true}`)}},
				})
				if err != nil {
					t.Fatalf("Append: %v", err)
				}

				// The append must land where the session was found: writing
				// where you read is what makes relocation unnecessary.
				after, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(after), `"appended":true`) {
					t.Fatalf("appended event is not in %s", logPath)
				}
				for other, location := range locations {
					if other == index {
						continue
					}
					stray := filepath.Join(location.Sessions, string(compatSessionID)+".jsonl")
					if _, err := os.Stat(stray); err == nil {
						t.Fatalf("append created a copy in %s; nothing may be relocated", location.Name)
					}
				}

				reloaded, err := store.Load(t.Context(), compatSessionID)
				if err != nil {
					t.Fatalf("Load after append: %v", err)
				}
				if reloaded.Version != next {
					t.Fatalf("reloaded version = %d, want %d", reloaded.Version, next)
				}
				if len(reloaded.Events) != format.wantEvents+1 {
					t.Fatalf("reloaded events = %d, want %d", len(reloaded.Events), format.wantEvents+1)
				}
			})
		}
	}
}

// TestUnreadableSessionIsSkippedNotFatal covers the forward half of
// coexistence: a log written by a newer binary than this one must not take the
// listing down with it, and must stay on disk untouched.
func TestUnreadableSessionIsSkippedNotFatal(t *testing.T) {
	locations := compatLocations(t)
	primary := locations[0]
	if err := os.MkdirAll(primary.Sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	readable := `{"version":1,"metadata":{"CodecVersion":1,"ID":"s_readable","Subject":"local","Project":"` +
		string(primary.Project) + `","Participant":"Christopher","UpdatedAt":"2026-07-30T09:00:00Z"}}` + "\n"
	if err := os.WriteFile(filepath.Join(primary.Sessions, "s_readable.jsonl"), []byte(readable), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt := "{this is not json\n"
	corruptPath := filepath.Join(primary.Sessions, "s_corrupt.jsonl")
	if err := os.WriteFile(corruptPath, []byte(corrupt), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := localadapter.NewFilesystemSessionStore(locations...)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(t.Context(), sdd.SessionFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := make([]string, 0, len(listed))
	for _, item := range listed {
		ids = append(ids, string(item.Metadata.ID))
	}
	if !slices.Equal(ids, []string{"s_readable"}) {
		t.Fatalf("List ids = %v, want the readable session with the unreadable one skipped", ids)
	}
	after, err := os.ReadFile(corruptPath)
	if err != nil || string(after) != corrupt {
		t.Fatalf("unreadable log was modified or removed: %q, %v", after, err)
	}
}

// TestLegacyEndedIsDerivedFromAttachmentHistory pins how a log written before
// the transport causes were dropped (d-cpt-rw7) still reads as ended: the
// terminal act is recovered from its attachment history, while the connection
// events those logs also recorded end nothing — so a previously concluded
// session stays collectable and a merely disconnected one stays live.
func TestLegacyEndedIsDerivedFromAttachmentHistory(t *testing.T) {
	stamp := `{"Subject":"local","ClientName":"claude","MCPSessionID":"c-1","LastActivity":"2026-07-20T09:00:00Z"}`
	record := func(cause, at string) string {
		return `{"Attachment":` + stamp + `,"EndedAt":"` + at + `","Cause":"` + cause + `"}`
	}
	for _, tc := range []struct {
		name    string
		history []string
		wantAct sdd.SessionEndAct
		wantAt  string
	}{
		{name: "trailing disconnect ends nothing", history: []string{record("disconnect", "2026-07-20T10:00:00Z")}},
		{name: "trailing claim ends nothing", history: []string{record("claim", "2026-07-20T10:00:00Z")}},
		{
			name:    "conclude ends the session",
			history: []string{record("disconnect", "2026-07-20T09:30:00Z"), record("conclude", "2026-07-20T10:00:00Z")},
			wantAct: sdd.SessionConcluded, wantAt: "2026-07-20T10:00:00Z",
		},
		{
			name:    "conclude followed by a dropped socket still ends the session",
			history: []string{record("conclude", "2026-07-20T10:00:00Z"), record("shutdown", "2026-07-20T11:00:00Z")},
			wantAct: sdd.SessionConcluded, wantAt: "2026-07-20T10:00:00Z",
		},
		{
			name:    "abandon ends the session",
			history: []string{record("abandon", "2026-07-20T10:00:00Z")},
			wantAct: sdd.SessionAbandoned, wantAt: "2026-07-20T10:00:00Z",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			locations := compatLocations(t)
			primary := locations[0]
			if err := os.MkdirAll(primary.Sessions, 0o700); err != nil {
				t.Fatal(err)
			}
			log := `{"version":1,"metadata":{"CodecVersion":1,"ID":"s_legacy","Subject":"local","Project":"` +
				string(primary.Project) + `","Participant":"Christopher","Attachment":null,"AttachmentHistory":[` +
				strings.Join(tc.history, ",") + `],"UpdatedAt":"2026-07-20T11:00:00Z"}}` + "\n"
			if err := os.WriteFile(filepath.Join(primary.Sessions, "s_legacy.jsonl"), []byte(log), 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := localadapter.NewFilesystemSessionStore(locations...)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := store.Load(t.Context(), "s_legacy")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if tc.wantAct == "" {
				if stored.Metadata.Ended != nil {
					t.Fatalf("Ended = %+v, want the session still live", stored.Metadata.Ended)
				}
				return
			}
			if stored.Metadata.Ended == nil {
				t.Fatal("Ended = nil, want the terminal act recovered from the legacy history")
			}
			if got := stored.Metadata.Ended.Act; got != tc.wantAct {
				t.Fatalf("Ended.Act = %q, want %q", got, tc.wantAct)
			}
			if got := stored.Metadata.Ended.EndedAt.Format(time.RFC3339); got != tc.wantAt {
				t.Fatalf("Ended.EndedAt = %s, want %s", got, tc.wantAt)
			}
		})
	}
}

// TestNewerMetadataFieldsAreToleratedNotRejected is the property that replaces
// the retired-field registry: a metadata field this binary does not define is
// ignored, in either direction, so no table has to be kept in step with the
// model for a real log to keep loading.
func TestNewerMetadataFieldsAreToleratedNotRejected(t *testing.T) {
	locations := compatLocations(t)
	primary := locations[0]
	if err := os.MkdirAll(primary.Sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	withUnknown := `{"version":1,"metadata":{"CodecVersion":1,"ID":"s_future","Subject":"local","Project":"` +
		string(primary.Project) + `","Participant":"Christopher","SomeFieldFromLater":{"nested":true},"UpdatedAt":"2026-07-30T09:00:00Z"}}` + "\n"
	if err := os.WriteFile(filepath.Join(primary.Sessions, "s_future.jsonl"), []byte(withUnknown), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := localadapter.NewFilesystemSessionStore(locations...)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Load(t.Context(), "s_future")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if stored.Metadata.Participant != "Christopher" {
		t.Fatalf("participant = %q, want the known fields decoded alongside the unknown one", stored.Metadata.Participant)
	}
}
