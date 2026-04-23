package finders

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// fakeGitSyncer lets tests script every git-surface answer the finder may
// ask for. Unset fields behave as zero values — methods never panic so
// tests can assert state-routing without wiring every field.
type fakeGitSyncer struct {
	inRepo        bool
	hasRemote     bool
	upstream      string
	upstreamErr   error
	fetchErr      error
	remoteAhead   int
	localAhead    int
	countErr      error
	mergeBase     string
	mergeBaseErr  error
	conflicts     []string
	mergeTreeErr  error
	lastRangeSpec []string
}

func (f *fakeGitSyncer) InRepo(context.Context) bool    { return f.inRepo }
func (f *fakeGitSyncer) HasRemote(context.Context) bool { return f.hasRemote }
func (f *fakeGitSyncer) UpstreamRef(context.Context) (string, error) {
	return f.upstream, f.upstreamErr
}
func (f *fakeGitSyncer) Fetch(context.Context) error { return f.fetchErr }
func (f *fakeGitSyncer) CountCommits(_ context.Context, rangeSpec, _ string) (int, error) {
	f.lastRangeSpec = append(f.lastRangeSpec, rangeSpec)
	if f.countErr != nil {
		return 0, f.countErr
	}
	if rangeSpec == "HEAD..@{u}" {
		return f.remoteAhead, nil
	}
	return f.localAhead, nil
}
func (f *fakeGitSyncer) MergeBase(context.Context, string, string) (string, error) {
	return f.mergeBase, f.mergeBaseErr
}
func (f *fakeGitSyncer) MergeTreePredict(context.Context, string, string, string) ([]string, error) {
	return f.conflicts, f.mergeTreeErr
}

func newSyncFinder(t *testing.T, gs GitSyncer, cfg *model.Config) (*Finder, string) {
	t.Helper()
	sddDir := t.TempDir()
	f := New(Options{Config: cfg, GitSyncer: gs})
	return f, sddDir
}

func TestSyncStatus_NoGitSyncerYieldsSkipped(t *testing.T) {
	f := New(Options{})
	got, err := f.SyncStatus(context.Background(), query.SyncStatusQuery{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.State != model.SyncStateSkipped {
		t.Errorf("State = %q, want skipped", got.State)
	}
}

func TestSyncStatus_NotInRepoYieldsNoRepo(t *testing.T) {
	gs := &fakeGitSyncer{inRepo: false}
	f, sddDir := newSyncFinder(t, gs, nil)
	got, err := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.State != model.SyncStateNoRepo {
		t.Errorf("State = %q, want no-repo", got.State)
	}
}

func TestSyncStatus_RespectsCooldownWithin(t *testing.T) {
	gs := &fakeGitSyncer{inRepo: true, hasRemote: true, upstream: "origin/main"}
	f, sddDir := newSyncFinder(t, gs, &model.Config{Sync: model.SyncConfig{Cooldown: "1h"}})

	// Stamp a recent fetch.
	if err := meta.TouchLastFetch(sddDir); err != nil {
		t.Fatalf("touch: %v", err)
	}

	got, err := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir, RespectCooldown: true})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.State != model.SyncStateSkipped {
		t.Errorf("State = %q, want skipped (cooldown)", got.State)
	}
	if len(gs.lastRangeSpec) != 0 {
		t.Errorf("expected no git log calls during cooldown, got %v", gs.lastRangeSpec)
	}
}

func TestSyncStatus_NoRemoteStampsAndReports(t *testing.T) {
	gs := &fakeGitSyncer{inRepo: true, hasRemote: false}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, err := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir, RespectCooldown: true})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.State != model.SyncStateNoRemote {
		t.Errorf("State = %q, want no-remote", got.State)
	}
	if _, err := os.Stat(filepath.Join(sddDir, "tmp", "last-fetch")); err != nil {
		t.Errorf("expected last-fetch marker after no-remote: %v", err)
	}
}

func TestSyncStatus_NoUpstreamStampsAndReports(t *testing.T) {
	gs := &fakeGitSyncer{inRepo: true, hasRemote: true, upstream: ""}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, err := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir, RespectCooldown: true})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.State != model.SyncStateNoUpstream {
		t.Errorf("State = %q, want no-upstream", got.State)
	}
	if _, err := os.Stat(filepath.Join(sddDir, "tmp", "last-fetch")); err != nil {
		t.Errorf("expected last-fetch marker after no-upstream: %v", err)
	}
}

func TestSyncStatus_FetchFailureReportsWithReason(t *testing.T) {
	gs := &fakeGitSyncer{
		inRepo:    true,
		hasRemote: true,
		upstream:  "origin/main",
		fetchErr:  errors.New("could not resolve host: github.com"),
	}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, err := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir, RespectCooldown: true})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.State != model.SyncStateFetchFailed {
		t.Errorf("State = %q, want fetch-failed", got.State)
	}
	if got.Reason == "" {
		t.Error("Reason should carry fetch error text")
	}
}

func TestSyncStatus_UpToDate(t *testing.T) {
	gs := &fakeGitSyncer{inRepo: true, hasRemote: true, upstream: "origin/main"}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, err := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.State != model.SyncStateUpToDate {
		t.Errorf("State = %q, want up-to-date", got.State)
	}
}

func TestSyncStatus_FastForward(t *testing.T) {
	gs := &fakeGitSyncer{
		inRepo:      true,
		hasRemote:   true,
		upstream:    "origin/main",
		remoteAhead: 3,
	}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, _ := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir})
	if got.State != model.SyncStateFastForward {
		t.Errorf("State = %q, want fast-forward", got.State)
	}
	if got.RemoteAhead != 3 {
		t.Errorf("RemoteAhead = %d, want 3", got.RemoteAhead)
	}
}

func TestSyncStatus_LocalAhead(t *testing.T) {
	gs := &fakeGitSyncer{
		inRepo:     true,
		hasRemote:  true,
		upstream:   "origin/main",
		localAhead: 2,
	}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, _ := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir})
	if got.State != model.SyncStateLocalAhead {
		t.Errorf("State = %q, want local-ahead", got.State)
	}
	if got.LocalAhead != 2 {
		t.Errorf("LocalAhead = %d, want 2", got.LocalAhead)
	}
}

func TestSyncStatus_CleanRebaseWhenDiverged(t *testing.T) {
	gs := &fakeGitSyncer{
		inRepo:      true,
		hasRemote:   true,
		upstream:    "origin/main",
		remoteAhead: 2,
		localAhead:  1,
		mergeBase:   "abc123",
		conflicts:   nil,
	}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, _ := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir})
	if got.State != model.SyncStateCleanRebase {
		t.Errorf("State = %q, want clean-rebase", got.State)
	}
	if got.RemoteAhead != 2 || got.LocalAhead != 1 {
		t.Errorf("counts = (%d, %d), want (2, 1)", got.RemoteAhead, got.LocalAhead)
	}
}

func TestSyncStatus_ConflictPredictedWhenDiverged(t *testing.T) {
	gs := &fakeGitSyncer{
		inRepo:      true,
		hasRemote:   true,
		upstream:    "origin/main",
		remoteAhead: 2,
		localAhead:  1,
		mergeBase:   "abc123",
		conflicts:   []string{".sdd/graph/2026/04/23-143213-d-tac-hsu.md"},
	}
	f, sddDir := newSyncFinder(t, gs, nil)

	got, _ := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir})
	if got.State != model.SyncStateConflictPredicted {
		t.Errorf("State = %q, want conflict-predicted", got.State)
	}
	if !reflect.DeepEqual(got.ConflictPaths, gs.conflicts) {
		t.Errorf("ConflictPaths = %v, want %v", got.ConflictPaths, gs.conflicts)
	}
}

func TestSyncStatus_CooldownBypassedWhenFlagUnset(t *testing.T) {
	gs := &fakeGitSyncer{inRepo: true, hasRemote: true, upstream: "origin/main", remoteAhead: 1}
	f, sddDir := newSyncFinder(t, gs, &model.Config{Sync: model.SyncConfig{Cooldown: "1h"}})

	// Stamp a recent fetch — but RespectCooldown=false should ignore it.
	if err := meta.TouchLastFetch(sddDir); err != nil {
		t.Fatalf("touch: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // ensure fetch runs after marker, not coincident

	got, _ := f.SyncStatus(context.Background(), query.SyncStatusQuery{SDDDir: sddDir, RespectCooldown: false})
	if got.State != model.SyncStateFastForward {
		t.Errorf("State = %q, want fast-forward (cooldown bypassed)", got.State)
	}
}
