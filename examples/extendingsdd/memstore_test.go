package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/sddtest"
)

// The conformance suites are the parity mechanism between local and external
// adapters (d-tac-wjq): the same behaviour is required of both, including the
// enumeration and deletion collection reaches through.

func TestExternalSessionStoreConformance(t *testing.T) {
	sddtest.RunSessionStoreTests(t, func(*testing.T) sddtest.SessionStoreFixture {
		return sddtest.SessionStoreFixture{
			Store: newMemorySessionStore(),
			Metadata: sdd.SessionMetadata{
				ID: "session-1", Subject: "example-user", Project: "example", Participant: "Example",
				Attachment: &sdd.Attachment{
					Subject: "example-user", ClientName: "test-client",
					LastActivity: time.Now().UTC().Round(0),
				},
			},
			Append: sdd.SessionAppend{Events: []sdd.StoredEvent{
				{CodecVersion: 1, Code: "started", Payload: json.RawMessage(`{"instance":"i_1"}`)},
			}},
		}
	})
}

func TestExternalStagedBlobStoreConformance(t *testing.T) {
	sddtest.RunStagedBlobStoreTests(t, func(*testing.T) sddtest.StagedBlobStoreFixture {
		return sddtest.StagedBlobStoreFixture{
			Store:    newMemoryStagedBlobStore(nil),
			Session:  sdd.SessionRef{Subject: "example-user", Session: "session-1"},
			Filename: "evidence.md",
			Content:  []byte("evidence"),
		}
	})
}

const collectSubject = "example-user"

// collectFixture is a composition that owns its storage entirely: memory stores
// plus a fixed clock, so retention is exercised without sleeping.
type collectFixture struct {
	app      *sdd.Application
	identity sdd.RequestIdentity
	project  sdd.ProjectID
	sessions *memorySessionStore
	blobs    *memoryStagedBlobStore
	now      time.Time
}

func newCollectFixture(t *testing.T) collectFixture {
	t.Helper()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	sessions := newMemorySessionStore()
	blobs := newMemoryStagedBlobStore(func() time.Time { return now })
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project:     sdd.ProjectRef{ID: "example", DisplayName: "Example"},
		Graph:       graphStore{},
		Sessions:    sessions,
		StagedBlobs: blobs,
		Now:         func() time.Time { return now },
		LLM:         llmRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	app, err := sdd.NewApplication(accessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return collectFixture{
		app: app, identity: sdd.RequestIdentity{Subject: collectSubject}, project: "example",
		sessions: sessions, blobs: blobs, now: now,
	}
}

// writeSession creates a session and, when ended is non-zero, closes it out with
// a terminal record of that act at that time.
func (f collectFixture) writeSession(t *testing.T, id sdd.SessionID, ended time.Time, act sdd.SessionEndAct) {
	t.Helper()
	metadata := sdd.SessionMetadata{
		ID: id, Subject: collectSubject, Project: f.project, Participant: "Example",
		Attachment: &sdd.Attachment{
			Subject: collectSubject, ClientName: "c-" + string(id),
			LastActivity: f.now.Add(-72 * time.Hour),
		},
		UpdatedAt: f.now.Add(-72 * time.Hour),
	}
	created, err := f.sessions.Create(t.Context(), metadata)
	if err != nil {
		t.Fatalf("Create(%s): %v", id, err)
	}
	if ended.IsZero() {
		return
	}
	ending := metadata
	ending.Attachment = nil
	ending.Ended = &sdd.SessionEnd{Act: act, EndedAt: ended}
	if _, err := f.sessions.Append(t.Context(), id, created.Version, sdd.SessionAppend{Metadata: &ending}); err != nil {
		t.Fatalf("ending %s: %v", id, err)
	}
}

func (f collectFixture) stage(t *testing.T, id sdd.SessionID) {
	t.Helper()
	ref := sdd.SessionRef{Subject: collectSubject, Session: id}
	if _, err := f.blobs.Stage(t.Context(), ref, "evidence.md", strings.NewReader("evidence")); err != nil {
		t.Fatalf("Stage(%s): %v", id, err)
	}
}

func (f collectFixture) collect(t *testing.T, retention time.Duration) sdd.CollectSessionsResult {
	t.Helper()
	result, err := f.app.CollectSessions(t.Context(), f.identity, f.project, sdd.CollectSessionsCmd{
		Retention: retention,
	})
	if err != nil {
		t.Fatalf("CollectSessions: %v", err)
	}
	return result
}

func (f collectFixture) listedIDs(t *testing.T) []string {
	t.Helper()
	listed, err := f.sessions.List(t.Context(), sdd.SessionFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := make([]string, 0, len(listed))
	for _, item := range listed {
		ids = append(ids, string(item.Metadata.ID))
	}
	slices.Sort(ids)
	return ids
}

func (f collectFixture) stagedIDs(t *testing.T) []string {
	t.Helper()
	refs, err := f.blobs.StagedSessions(t.Context())
	if err != nil {
		t.Fatalf("StagedSessions: %v", err)
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, string(ref.Session))
	}
	slices.Sort(ids)
	return ids
}

// TestExternalCollectionThroughExportedAPI drives the collection pass a local
// start runs, from outside the module and over composition-owned storage: the
// removal rule is the one criterion an external composition cannot take on
// trust, since it is what reaches through the ports rather than sitting behind
// them.
func TestExternalCollectionThroughExportedAPI(t *testing.T) {
	f := newCollectFixture(t)
	f.writeSession(t, "ended-old", f.now.Add(-48*time.Hour), sdd.SessionConcluded)
	f.writeSession(t, "ended-recent", f.now.Add(-1*time.Hour), sdd.SessionAbandoned)
	f.writeSession(t, "still-open", time.Time{}, "")
	for _, id := range []sdd.SessionID{"ended-old", "ended-recent", "still-open", "orphan"} {
		f.stage(t, id)
	}

	result := f.collect(t, 24*time.Hour)

	if got, want := result.RemovedSessions, []sdd.SessionID{"ended-old"}; !slices.Equal(got, want) {
		t.Fatalf("RemovedSessions = %v, want %v", got, want)
	}
	if got, want := f.listedIDs(t), []string{"ended-recent", "still-open"}; !slices.Equal(got, want) {
		t.Fatalf("listed = %v, want %v", got, want)
	}
	// The removed session's blobs go with it; the staging area of a session
	// that never existed is an orphan and goes too.
	if got, want := f.stagedIDs(t), []string{"ended-recent", "still-open"}; !slices.Equal(got, want) {
		t.Fatalf("staged = %v, want %v", got, want)
	}
	if !slices.Contains(result.RemovedStaged, (sdd.SessionRef{Subject: collectSubject, Session: "orphan"})) {
		t.Fatalf("RemovedStaged = %v, want it to contain the orphan", result.RemovedStaged)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("Skipped = %v, want nothing reported as actionable", result.Skipped)
	}

	// A second pass over the same store finds nothing left to do, which is what
	// makes a repeated or concurrent run safe.
	again := f.collect(t, 24*time.Hour)
	if len(again.RemovedSessions) != 0 || len(again.RemovedStaged) != 0 {
		t.Fatalf("second pass removed %v / %v, want nothing", again.RemovedSessions, again.RemovedStaged)
	}
	if got, want := f.listedIDs(t), []string{"ended-recent", "still-open"}; !slices.Equal(got, want) {
		t.Fatalf("listed after second pass = %v, want %v", got, want)
	}
}
