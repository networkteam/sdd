package application_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// collectFixture builds a runtime over throwaway stores plus a fixed clock, so
// retention can be exercised without sleeping.
type collectFixture struct {
	app         *sdd.Application
	identity    sdd.RequestIdentity
	project     sdd.ProjectID
	sessions    *localadapter.FilesystemSessionStore
	blobs       *localadapter.FilesystemStagedBlobStore
	sessionsDir string
	now         time.Time
}

const collectSubject = "local"

func newCollectFixture(t *testing.T) collectFixture {
	t.Helper()
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	sessions, err := localadapter.NewFilesystemSessionStoreAt(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(filepath.Join(root, "staged-blobs"))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{
		Project: "local", GraphDir: filepath.Join(root, "graph"),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "local"}, Graph: graph,
		Sessions: sessions, StagedBlobs: blobs,
		Now: func() time.Time { return now },
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	app, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return collectFixture{
		app: app, sessions: sessions, blobs: blobs, sessionsDir: sessionsDir, now: now,
		project: "local", identity: sdd.RequestIdentity{Subject: collectSubject},
	}
}

// writeSession creates a session and, when ended is non-zero, closes it out with
// a terminal record of that act at that time.
func (f collectFixture) writeSession(t *testing.T, id sdd.SessionID, ended time.Time, act sdd.SessionEndAct) {
	t.Helper()
	attachment := sdd.Attachment{
		Subject: collectSubject, ClientName: "test", MCPSessionID: "c-" + string(id),
		LastActivity: f.now.Add(-72 * time.Hour),
	}
	metadata := sdd.SessionMetadata{
		ID: id, Subject: collectSubject, Project: f.project,
		Participant: "Christopher", Attachment: &attachment, UpdatedAt: f.now.Add(-72 * time.Hour),
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
	owner := sdd.SessionRef{Subject: collectSubject, Session: id}
	if _, err := f.blobs.Stage(t.Context(), owner, "evidence.md", strings.NewReader("evidence")); err != nil {
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

// TestCollectRemovesOnlyEndedSessionsPastRetention covers the pass's whole
// removal rule in one shape: ended and past the window goes with its blobs,
// ended but inside the window stays, and never-ended stays regardless of age.
func TestCollectRemovesOnlyEndedSessionsPastRetention(t *testing.T) {
	f := newCollectFixture(t)
	retention := 14 * 24 * time.Hour

	f.writeSession(t, "s_old", f.now.Add(-30*24*time.Hour), sdd.SessionConcluded)
	f.stage(t, "s_old")
	f.writeSession(t, "s_abandoned", f.now.Add(-30*24*time.Hour), sdd.SessionAbandoned)
	f.writeSession(t, "s_recent", f.now.Add(-1*time.Hour), sdd.SessionConcluded)
	f.stage(t, "s_recent")
	f.writeSession(t, "s_open", time.Time{}, "")
	f.stage(t, "s_open")

	result := f.collect(t, retention)

	removed := make([]string, 0, len(result.RemovedSessions))
	for _, id := range result.RemovedSessions {
		removed = append(removed, string(id))
	}
	slices.Sort(removed)
	if !slices.Equal(removed, []string{"s_abandoned", "s_old"}) {
		t.Fatalf("removed = %v, want the two ended sessions past retention", removed)
	}
	if got := f.listedIDs(t); !slices.Equal(got, []string{"s_open", "s_recent"}) {
		t.Fatalf("remaining = %v, want the in-window and never-ended sessions", got)
	}

	// The removed session's blobs go with it; the survivors keep theirs.
	if _, err := f.blobs.Stat(t.Context(), sdd.SessionRef{Subject: collectSubject, Session: "s_old"}, ""); err == nil {
		t.Fatal("expected the removed session's staged blobs to be gone")
	}
	owners, err := f.blobs.StagedSessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, owner := range owners {
		kept = append(kept, string(owner.Session))
	}
	slices.Sort(kept)
	if !slices.Equal(kept, []string{"s_open", "s_recent"}) {
		t.Fatalf("staged owners = %v, want only the surviving sessions", kept)
	}
}

// TestCollectRemovesOrphanedStagedBlobs covers the self-healing half: once a
// session's log is gone nothing points at its blob directory, so enumeration is
// the only way a previously interrupted removal ever finishes.
func TestCollectRemovesOrphanedStagedBlobs(t *testing.T) {
	f := newCollectFixture(t)
	f.writeSession(t, "s_live", time.Time{}, "")
	f.stage(t, "s_live")
	f.stage(t, "s_vanished")

	result := f.collect(t, time.Hour)

	var removed []string
	for _, owner := range result.RemovedStaged {
		removed = append(removed, string(owner.Session))
	}
	if !slices.Equal(removed, []string{"s_vanished"}) {
		t.Fatalf("removed owners = %v, want only the orphan", removed)
	}
	owners, err := f.blobs.StagedSessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 1 || owners[0].Session != "s_live" {
		t.Fatalf("remaining owners = %+v, want the live session's", owners)
	}
}

// TestCollectIsIdempotentAndConcurrencySafe pins the property that replaces all
// coordination: the target set is recomputed each run, so a repeat finds nothing
// to do and never errors on work already done.
func TestCollectIsIdempotentAndConcurrencySafe(t *testing.T) {
	f := newCollectFixture(t)
	f.writeSession(t, "s_old", f.now.Add(-30*24*time.Hour), sdd.SessionConcluded)
	f.stage(t, "s_old")

	first := f.collect(t, time.Hour)
	if len(first.RemovedSessions) != 1 {
		t.Fatalf("first pass removed %d, want 1", len(first.RemovedSessions))
	}
	second := f.collect(t, time.Hour)
	if len(second.RemovedSessions) != 0 || len(second.RemovedStaged) != 0 {
		t.Fatalf("second pass removed %+v, want nothing left to do", second)
	}
}

// TestCollectLeavesUnreadableSessionsAlone is the criterion that an unreadable
// log is never treated as garbage: it may belong to a newer binary.
func TestCollectLeavesUnreadableSessionsAlone(t *testing.T) {
	f := newCollectFixture(t)
	f.writeSession(t, "s_old", f.now.Add(-30*24*time.Hour), sdd.SessionConcluded)

	const corrupt = "{not json at all\n"
	unreadable := filepath.Join(f.sessionsDir, "s_unreadable.jsonl")
	if err := os.WriteFile(unreadable, []byte(corrupt), 0o600); err != nil {
		t.Fatal(err)
	}

	f.collect(t, time.Hour)

	after, err := os.ReadFile(unreadable)
	if err != nil {
		t.Fatalf("the unreadable log was removed; it may belong to a newer binary: %v", err)
	}
	if string(after) != corrupt {
		t.Fatalf("the unreadable log was modified: %q", after)
	}
}
