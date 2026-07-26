package local

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gofrs/flock"
	app "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/finders"
	gitadapter "github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
)

func TestRelocateSessionStoreCollisionLeavesWholeSourceUnchanged(t *testing.T) {
	root := canonicalTempDir(t)
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "move.jsonl"), []byte("source-move"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "keep.jsonl"), []byte("source-keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep.jsonl"), []byte("target-wins"), 0o600); err != nil {
		t.Fatal(err)
	}

	moved, skipped, err := RelocateSessionStore(source, target)
	if err == nil {
		t.Fatal("collision must fail the all-or-nothing pass")
	}
	if len(moved) != 0 {
		t.Fatalf("moved = %+v", moved)
	}
	if len(skipped) != 1 || filepath.Base(skipped[0]) != "keep.jsonl" {
		t.Fatalf("skipped = %+v", skipped)
	}
	if _, err := os.Stat(filepath.Join(target, "move.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("unrelated payload moved despite collision: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(target, "keep.jsonl")); err != nil || string(got) != "target-wins" {
		t.Fatalf("collision clobbered target: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(source, "keep.jsonl")); err != nil || string(got) != "source-keep" {
		t.Fatalf("collision source missing: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(source, "move.jsonl")); err != nil || string(got) != "source-move" {
		t.Fatalf("unrelated source missing: %q, %v", got, err)
	}
}

func TestRelocatedLegacySessionCanBeMigratedInGlobalStore(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "repo", ".sdd", "sessions")
	sourceBlobs := filepath.Join(root, "repo", ".sdd", "staged-blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "repo")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "repo")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	id := app.SessionID("legacy")
	events := []engine.Event{
		{V: 1, TS: time.Now().UTC(), Session: string(id), Seq: 1, Event: engine.EventSessionMeta, Data: map[string]any{"participant": "Christopher"}},
		{V: 1, TS: time.Now().UTC(), Session: string(id), Seq: 2, Instance: "i_1", Event: engine.EventStarted, Data: map[string]any{"procedure": "capture", "step": "work"}},
	}
	var log bytes.Buffer
	for _, event := range events {
		if err := json.NewEncoder(&log).Encode(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sourceSessions, string(id)+".jsonl"), log.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(sourceSessions, string(id)+"-staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "evidence.md"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}

	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	if err := relocator.Relocate(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	migrator, err := NewFilesystemLegacySessionMigrator(targetSessions, targetBlobs, "local", "example")
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := migrator.ListLegacySessions(context.Background())
	if err != nil || len(candidates) != 1 {
		t.Fatalf("legacy candidates = %v, %v", candidates, err)
	}
	if err := migrator.MigrateLegacySession(context.Background(), candidates[0]); err != nil {
		t.Fatal(err)
	}
	store, err := NewFilesystemSessionStore(targetSessions)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.Participant != "Christopher" {
		t.Fatalf("relocated metadata = %+v", stored.Metadata)
	}
}

func TestFilesystemSessionStoreRelocatorMovesLegacyStagingAndBlobs(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "repo", ".sdd", "sessions")
	sourceBlobs := filepath.Join(root, "repo", ".sdd", "staged-blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "repo")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "repo")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(sourceSessions, "legacy.jsonl"), `{"event":"started"}`)
	write(filepath.Join(sourceSessions, "attachment.lock"), "top-level attachment")
	write(filepath.Join(sourceSessions, "legacy-staging", "notes.md"), "notes")
	write(filepath.Join(sourceSessions, "legacy-staging", "evidence.lock"), "attachment lock")
	write(filepath.Join(sourceBlobs, "local", "session", "blob.blob"), "blob")

	write(filepath.Join(sourceSessions, "legacy.jsonl.lock"), "stale lock")
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	var moved []string
	if err := relocator.Relocate(context.Background(), func(_, destination string) {
		moved = append(moved, destination)
	}, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		filepath.Join(targetSessions, "legacy.jsonl"),
		filepath.Join(targetSessions, "attachment.lock"),
		filepath.Join(targetSessions, "legacy-staging", "notes.md"),
		filepath.Join(targetSessions, "legacy-staging", "evidence.lock"),
		filepath.Join(targetBlobs, "local", "session", "blob.blob"),
	} {
		if !slices.Contains(moved, want) {
			t.Errorf("moved destinations %v do not contain %s", moved, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	if _, err := os.Stat(sourceBlobs); !os.IsNotExist(err) {
		t.Errorf("emptied blob source %s still exists: %v", sourceBlobs, err)
	}
	material, err := SessionStoreMaterial(sourceSessions, sourceBlobs)
	if err != nil || len(material) != 0 {
		t.Fatalf("post-relocation material = %v, %v", material, err)
	}
	if _, err := os.Stat(filepath.Join(sourceSessions, SessionRelocationTombstone)); err != nil {
		t.Fatalf("relocation tombstone missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, "legacy.jsonl.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock file migrated: %v", err)
	}
}

func TestSessionStoreMaterialRejectsSpecialFiles(t *testing.T) {
	sessions := filepath.Join(canonicalTempDir(t), "sessions")
	if err := os.MkdirAll(filepath.Join(sessions, "session-staging"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(sessions, "session-staging", "attachment")
	if err := os.Symlink("missing", link); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionStoreMaterial(sessions, filepath.Join(canonicalTempDir(t), "blobs")); err == nil ||
		!strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("special-file scan error = %v", err)
	}
}

func TestSessionStoreMaterialRejectsCorruptRelocationTombstones(t *testing.T) {
	for name, tombstone := range map[string]string{
		"invalid JSON":     `{`,
		"unsupported":      `{"version":99,"target_project":"example","target_sessions":"/sessions","target_staged_blobs":"/blobs","relocated_at":"2026-01-01T00:00:00Z"}`,
		"relative targets": `{"version":2,"target_project":"example","target_sessions":"sessions","target_staged_blobs":"blobs","relocated_at":"2026-01-01T00:00:00Z"}`,
		"unknown field":    `{"version":2,"target_project":"example","target_sessions":"/sessions","target_staged_blobs":"/blobs","relocated_at":"2026-01-01T00:00:00Z","future":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			sessions := filepath.Join(canonicalTempDir(t), "sessions")
			if err := os.MkdirAll(sessions, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sessions, SessionRelocationTombstone), []byte(tombstone), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := SessionStoreMaterial(sessions, filepath.Join(canonicalTempDir(t), "blobs")); err == nil ||
				!strings.Contains(err.Error(), "tombstone") {
				t.Fatalf("tombstone scan error = %v", err)
			}
		})
	}
}

func TestPublishNoClobberFailsWithoutAtomicLinkAndPreservesCollision(t *testing.T) {
	root := canonicalTempDir(t)
	temp := filepath.Join(root, "temp")
	destination := filepath.Join(root, "destination")
	if err := os.WriteFile(temp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := publishNoClobberWithLink(temp, destination, func(string, string) error {
		return syscall.EPERM
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported publication error = %v", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("unsupported publication left destination: %v", err)
	}
	if err := os.WriteFile(destination, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishNoClobber(temp, destination); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("collision error = %v", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "existing" {
		t.Fatalf("collision changed destination: %q, %v", got, err)
	}
}

func TestFilesystemSessionStoreRelocatorResumesManifestAfterPublicationCrash(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	relocator.afterPublish = func() { panic("simulated crash after publication") }
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("expected simulated crash")
			}
		}()
		_ = relocator.Relocate(t.Context(), nil, nil)
	}()
	if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); err != nil {
		t.Fatalf("durable manifest missing after crash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceSessions, "legacy.jsonl")); err != nil {
		t.Fatalf("source missing before manifest resume: %v", err)
	}

	resumed := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	if err := resumed.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, "legacy.jsonl")); err != nil {
		t.Fatalf("resumed target missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceSessions, "legacy.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("resumed cleanup left source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); !os.IsNotExist(err) {
		t.Fatalf("completed manifest remains: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorResumesManifestAfterSourceDeletionCrash(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "state", "sessions", "local", "0123456789ab")
	sourceBlobs := filepath.Join(root, "state", "staged-blobs", "local", "0123456789ab")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	transition := SessionIdentityTransition{
		Version: SessionIdentityTransitionVersion, State: SessionIdentityTransitionPending,
		OldKey: "local/hash", NewKey: "github.com/org/repo",
		OldSessions: sourceSessions, OldBlobs: sourceBlobs,
		CurrentSessions: sourceSessions, CurrentBlobs: sourceBlobs,
		TargetProject: "github.com/org/repo",
	}
	if err := WriteSessionIdentityTransition(sourceSessions, transition); err != nil {
		t.Fatal(err)
	}
	authorizedOld := SessionStoreRelocationSource{
		Kind:     SessionStoreRelocationSourceOldGlobal,
		Sessions: sourceSessions, StagedBlobs: sourceBlobs, WriteTombstone: true,
	}
	options := FilesystemSessionStoreRelocatorOptions{
		Sources:                   []SessionStoreRelocationSource{authorizedOld},
		AuthorizedOldGlobalSource: &authorizedOld,
		TrustedStateRoot:          filepath.Join(root, "state"),
		StableRepoAuthority:       filepath.Join(root, "repo.git"),
		TargetSessions:            targetSessions, TargetBlobs: targetBlobs, TargetProject: "github.com/org/repo",
		Transformer: app.CurrentSessionIdentityTransformer{}, Transition: &transition,
	}
	relocator, err := NewFilesystemSessionStoreRelocator(options)
	if err != nil {
		t.Fatal(err)
	}
	relocator.afterSourceDelete = func() { panic("simulated crash after source deletion") }
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("expected simulated crash")
			}
		}()
		_ = relocator.Relocate(t.Context(), nil, nil)
	}()
	cutover, err := ReadSessionIdentityTransition(sourceSessions)
	if err != nil {
		t.Fatal(err)
	}
	if cutover == nil || cutover.State != SessionIdentityTransitionCutover ||
		cutover.CurrentSessions != targetSessions {
		t.Fatalf("crash cutover marker = %+v", cutover)
	}

	resumed, err := NewFilesystemSessionStoreRelocator(options)
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	completed, err := ReadSessionIdentityTransition(sourceSessions)
	if err != nil {
		t.Fatal(err)
	}
	if completed == nil || completed.State != SessionIdentityTransitionCompleted {
		t.Fatalf("completed transition marker = %+v", completed)
	}
}

func TestFilesystemSessionStoreRelocatorRejectsConcurrentProcess(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(targetSessions, SessionRelocationLock)
	locker := exec.Command(os.Args[0], "-test.run=TestSessionRelocationLockHelperProcess")
	locker.Env = append(os.Environ(), "SDD_TEST_RELOCATION_LOCK="+lockPath)
	lockerInput, err := locker.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	lockerOutput, err := locker.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := locker.Start(); err != nil {
		t.Fatal(err)
	}
	if line, readErr := bufio.NewReader(lockerOutput).ReadString('\n'); readErr != nil || line != "locked\n" {
		t.Fatalf("helper lock readiness = %q, %v", line, readErr)
	}
	defer func() {
		_ = lockerInput.Close()
		_ = locker.Wait()
	}()

	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	err = relocator.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("contention error = %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("contention changed source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); !os.IsNotExist(err) {
		t.Fatalf("contention created manifest: %v", err)
	}
}

func TestSessionRelocationLockHelperProcess(t *testing.T) {
	lockPath := os.Getenv("SDD_TEST_RELOCATION_LOCK")
	if lockPath == "" {
		return
	}
	held := flock.New(lockPath)
	locked, err := held.TryLock()
	if err != nil || !locked {
		t.Fatalf("holding relocation lock: locked=%t err=%v", locked, err)
	}
	defer func() { _ = held.Unlock() }()
	if _, err := os.Stdout.WriteString("locked\n"); err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(os.Stdin)
}

func TestEnsurePendingIsLockedAndNeverDowngradesCompletedTransition(t *testing.T) {
	root := canonicalTempDir(t)
	oldSessions := filepath.Join(root, "state", "sessions", "local", "0123456789ab")
	oldBlobs := filepath.Join(root, "state", "staged-blobs", "local", "0123456789ab")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	transition := SessionIdentityTransition{
		Version: SessionIdentityTransitionVersion, State: SessionIdentityTransitionPending,
		OldKey: "local/old", NewKey: "example/repo",
		OldSessions: oldSessions, OldBlobs: oldBlobs,
		CurrentSessions: oldSessions, CurrentBlobs: oldBlobs, TargetProject: "example/repo",
	}
	authorizedOld := SessionStoreRelocationSource{
		Kind:     SessionStoreRelocationSourceOldGlobal,
		Sessions: oldSessions, StagedBlobs: oldBlobs, WriteTombstone: true,
	}
	options := FilesystemSessionStoreRelocatorOptions{
		Sources:                   []SessionStoreRelocationSource{authorizedOld},
		AuthorizedOldGlobalSource: &authorizedOld,
		TrustedStateRoot:          filepath.Join(root, "state"),
		StableRepoAuthority:       filepath.Join(root, "repo.git"),
		TargetSessions:            targetSessions, TargetBlobs: targetBlobs, TargetProject: "example/repo",
		Transformer: app.CurrentSessionIdentityTransformer{}, Transition: &transition,
	}
	stale, err := NewFilesystemSessionStoreRelocator(options)
	if err != nil {
		t.Fatal(err)
	}

	// Lock contention is resolved before the marker can be created.
	if err := os.MkdirAll(targetSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	held := flock.New(filepath.Join(targetSessions, SessionRelocationLock))
	locked, err := held.TryLock()
	if err != nil || !locked {
		t.Fatalf("holding lock: %t, %v", locked, err)
	}
	if err := stale.EnsurePending(t.Context()); err == nil {
		t.Fatal("EnsurePending ignored relocation lock contention")
	}
	if _, err := ReadSessionIdentityTransition(oldSessions); err != nil {
		t.Fatal(err)
	} else if marker, _ := ReadSessionIdentityTransition(oldSessions); marker != nil {
		t.Fatalf("contended EnsurePending created marker: %+v", marker)
	}
	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}

	if err := stale.EnsurePending(t.Context()); err != nil {
		t.Fatal(err)
	}
	completed := transition
	completed.State = SessionIdentityTransitionCompleted
	completed.CurrentSessions = targetSessions
	completed.CurrentBlobs = targetBlobs
	if err := WriteSessionIdentityTransition(oldSessions, completed); err != nil {
		t.Fatal(err)
	}
	if err := stale.EnsurePending(t.Context()); err != nil {
		t.Fatal(err)
	}
	current, err := ReadSessionIdentityTransition(oldSessions)
	if err != nil {
		t.Fatal(err)
	}
	if current == nil || current.State != SessionIdentityTransitionCompleted {
		t.Fatalf("stale init downgraded transition: %+v", current)
	}
}

func TestEnsurePendingRejectsSymlinkedTransitionComponents(t *testing.T) {
	for _, component := range []string{"local", "hash"} {
		t.Run(component, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsCategory := filepath.Join(stateRoot, "sessions")
			outside := filepath.Join(root, "outside")
			if err := os.MkdirAll(sessionsCategory, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			oldSessions := filepath.Join(sessionsCategory, "local", "0123456789ab")
			if component == "local" {
				if err := os.Symlink(outside, filepath.Join(sessionsCategory, "local")); err != nil {
					t.Skipf("symbolic links unavailable: %v", err)
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(oldSessions), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, oldSessions); err != nil {
					t.Skipf("symbolic links unavailable: %v", err)
				}
			}
			oldBlobs := filepath.Join(stateRoot, "staged-blobs", "local", "0123456789ab")
			if err := os.MkdirAll(oldBlobs, 0o755); err != nil {
				t.Fatal(err)
			}
			transition := SessionIdentityTransition{
				Version: SessionIdentityTransitionVersion, State: SessionIdentityTransitionPending,
				OldKey: "local/old", NewKey: "example/repo",
				OldSessions: oldSessions, OldBlobs: oldBlobs,
				CurrentSessions: oldSessions, CurrentBlobs: oldBlobs, TargetProject: "example/repo",
			}
			relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
				TrustedStateRoot: stateRoot, StableRepoAuthority: filepath.Join(root, "repo.git"),
				TargetSessions: filepath.Join(stateRoot, "sessions", "example"),
				TargetBlobs:    filepath.Join(stateRoot, "staged-blobs", "example"),
				TargetProject:  "example/repo",
				Transformer:    app.CurrentSessionIdentityTransformer{},
				Transition:     &transition,
			})
			if err != nil {
				t.Fatal(err)
			}
			err = relocator.EnsurePending(t.Context())
			if err == nil || !strings.Contains(err.Error(), "symbolic link") {
				t.Fatalf("symlinked transition %s error = %v", component, err)
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("symlinked transition mutated outside = %v, %v", entries, err)
			}
		})
	}
}

func TestEnsurePendingRejectsTransitionComponentRebind(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	oldSessions := filepath.Join(stateRoot, "sessions", "local", "0123456789ab")
	oldBlobs := filepath.Join(stateRoot, "staged-blobs", "local", "0123456789ab")
	if err := os.MkdirAll(oldSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	transition := SessionIdentityTransition{
		Version: SessionIdentityTransitionVersion, State: SessionIdentityTransitionPending,
		OldKey: "local/old", NewKey: "example/repo",
		OldSessions: oldSessions, OldBlobs: oldBlobs,
		CurrentSessions: oldSessions, CurrentBlobs: oldBlobs, TargetProject: "example/repo",
	}
	relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		TrustedStateRoot: stateRoot, StableRepoAuthority: filepath.Join(root, "repo.git"),
		TargetSessions: filepath.Join(stateRoot, "sessions", "example"),
		TargetBlobs:    filepath.Join(stateRoot, "staged-blobs", "example"),
		TargetProject:  "example/repo",
		Transformer:    app.CurrentSessionIdentityTransformer{},
		Transition:     &transition,
	})
	if err != nil {
		t.Fatal(err)
	}
	pinned := filepath.Join(stateRoot, "sessions", "local-pinned")
	relocator.beforeTransitionWrite = func() {
		relocator.beforeTransitionWrite = nil
		if err := os.Rename(filepath.Dir(oldSessions), pinned); err != nil {
			t.Skipf("platform cannot rename opened transition component: %v", err)
		}
		if err := os.MkdirAll(oldSessions, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(oldSessions, "late.jsonl"), []byte("late"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err = relocator.EnsurePending(t.Context())
	if err == nil || !strings.Contains(err.Error(), "transition root was rebound") {
		t.Fatalf("transition component rebind error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(oldSessions, "late.jsonl")); err != nil || string(got) != "late" {
		t.Fatalf("transition rebind changed recreated payload = %q, %v", got, err)
	}
	if marker, err := ReadSessionIdentityTransition(oldSessions); err != nil || marker != nil {
		t.Fatalf("transition rebind published marker to recreated path = %+v, %v", marker, err)
	}
}

func TestFilesystemSessionStoreRelocatorConfinesSymlinkedTargetAncestor(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	outside := filepath.Join(root, "outside")
	source := filepath.Join(sourceSessions, "s-staging", "evidence.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(targetSessions, "s-staging")); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	if err := relocator.Relocate(t.Context(), nil, nil); err == nil {
		t.Fatal("symlinked target ancestor was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "evidence.md")); !os.IsNotExist(err) {
		t.Fatalf("relocation wrote outside target root: %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "evidence" {
		t.Fatalf("failed confinement deleted source: %q, %v", got, err)
	}
}

func TestFilesystemSessionStoreRelocatorRejectsSymlinkedCategoryRoot(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceSessions, "session.jsonl")
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(targetSessions), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, targetSessions); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	if err := relocator.Relocate(t.Context(), nil, nil); err == nil ||
		!strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlinked target root error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "session.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("symlinked category root received payload: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("symlinked category rejection removed source: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorAuthorizesFreshPinnedInTreeSource(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "repo", ".sdd", "sessions")
	sourceBlobs := filepath.Join(root, "repo", ".sdd", "staged-blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		Sources: []SessionStoreRelocationSource{{
			Kind:     SessionStoreRelocationSourceInTree,
			Sessions: sourceSessions, StagedBlobs: sourceBlobs, WriteTombstone: true,
		}},
		TrustedStateRoot:    filepath.Join(root, "state"),
		StableRepoAuthority: filepath.Join(root, "repo.git"),
		AuthorizeInTreeSource: func(context.Context, string, string, string) error {
			return errors.New("rejected fresh source")
		},
		TargetSessions: targetSessions, TargetBlobs: targetBlobs, TargetProject: "example",
		Transformer: app.CurrentSessionIdentityTransformer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err == nil ||
		!strings.Contains(err.Error(), "rejected fresh source") {
		t.Fatalf("fresh source authorization error = %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("rejected fresh authorization changed source: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorBindsAuthorizationToPinnedSourceIdentity(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "repo", ".sdd", "sessions")
	sourceBlobs := filepath.Join(root, "repo", ".sdd", "staged-blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceSDD := filepath.Dir(sourceSessions)
	pinnedName := sourceSDD + "-pinned"
	relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		Sources: []SessionStoreRelocationSource{{
			Kind:     SessionStoreRelocationSourceInTree,
			Sessions: sourceSessions, StagedBlobs: sourceBlobs, WriteTombstone: true,
		}},
		TrustedStateRoot:    filepath.Join(root, "state"),
		StableRepoAuthority: filepath.Join(root, "repo.git"),
		AuthorizeInTreeSource: func(context.Context, string, string, string) error {
			if err := os.Rename(sourceSDD, pinnedName); err != nil {
				t.Fatal(err)
			}
			return os.MkdirAll(sourceSessions, 0o755)
		},
		TargetSessions: targetSessions, TargetBlobs: targetBlobs, TargetProject: "example",
		Transformer: app.CurrentSessionIdentityTransformer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err == nil ||
		!strings.Contains(err.Error(), "rebound") {
		t.Fatalf("authorization source-swap error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(pinnedName, "sessions", "legacy.jsonl")); err != nil {
		t.Fatalf("pinned source was changed after authorization swap: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorRejectsRecreatedInTreeAncestor(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSDD := filepath.Join(root, "repo", ".sdd")
	sourceSessions := filepath.Join(sourceSDD, "sessions")
	sourceBlobs := filepath.Join(sourceSDD, "staged-blobs")
	stateRoot := filepath.Join(root, "state")
	targetSessions := filepath.Join(stateRoot, "sessions", "example")
	targetBlobs := filepath.Join(stateRoot, "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	pinnedSDD := sourceSDD + "-pinned"
	relocator.afterTempSync = func() {
		relocator.afterTempSync = nil
		if err := os.Rename(sourceSDD, pinnedSDD); err != nil {
			t.Skipf("platform cannot rename opened in-tree ancestor: %v", err)
		}
		if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceSessions, "late.jsonl"), []byte("late"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := relocator.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "source component was rebound") {
		t.Fatalf("recreated in-tree ancestor error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(sourceSessions, "late.jsonl")); err != nil || string(got) != "late" {
		t.Fatalf("recreated in-tree ancestor changed late payload = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(pinnedSDD, "sessions", "legacy.jsonl")); err != nil ||
		string(got) != `{"event":"started"}` {
		t.Fatalf("recreated in-tree ancestor changed pinned source = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, "legacy.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("recreated in-tree ancestor published target payload: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorPruneRetainsReboundLeaf(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	stateRoot := filepath.Join(root, "state")
	targetSessions := filepath.Join(stateRoot, "sessions", "example")
	targetBlobs := filepath.Join(stateRoot, "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sourceBlobs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBlobs, "legacy.blob"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	pinnedBlobs := sourceBlobs + "-pinned"
	relocator.beforeSourcePrune = func(path string) {
		relocator.beforeSourcePrune = nil
		if path != sourceBlobs {
			t.Fatalf("unexpected prune path %s", path)
		}
		if err := os.Rename(sourceBlobs, pinnedBlobs); err != nil {
			t.Skipf("platform cannot rename opened source leaf: %v", err)
		}
		if err := os.MkdirAll(sourceBlobs, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceBlobs, "late.blob"), []byte("late"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := relocator.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "rebound") {
		t.Fatalf("prune leaf rebind error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(sourceBlobs, "late.blob")); err != nil || string(got) != "late" {
		t.Fatalf("prune leaf rebind removed recreated payload = %q, %v", got, err)
	}
	if _, err := os.Stat(pinnedBlobs); err != nil {
		t.Fatalf("prune leaf rebind removed pinned source leaf: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorRejectsTrustedCategorySymlink(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(stateRoot, "sessions")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	sourceSessions := filepath.Join(root, "source", "sessions")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		Sources: []SessionStoreRelocationSource{{
			Kind:     SessionStoreRelocationSourceInTree,
			Sessions: sourceSessions, StagedBlobs: filepath.Join(root, "source", "blobs"), WriteTombstone: true,
		}},
		TrustedStateRoot: stateRoot, StableRepoAuthority: filepath.Join(root, "repo.git"),
		AuthorizeInTreeSource: func(context.Context, string, string, string) error { return nil },
		TargetSessions:        filepath.Join(stateRoot, "sessions", "example"),
		TargetBlobs:           filepath.Join(stateRoot, "staged-blobs", "example"),
		TargetProject:         "example", Transformer: app.CurrentSessionIdentityTransformer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err == nil {
		t.Fatal("trusted category symlink was accepted")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("trusted category symlink mutated outside: %v, %v", entries, err)
	}
}

func TestFilesystemSessionStoreRelocatorRejectsCategoryRenameToSymlinkRace(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(stateRoot, "sessions", "example")
	targetBlobs := filepath.Join(stateRoot, "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	pinnedCategory := filepath.Join(stateRoot, "sessions-pinned")
	relocator.beforeIrreversible = func(stage string) {
		if stage != "source_tombstones" {
			return
		}
		relocator.beforeIrreversible = nil
		if err := os.Rename(filepath.Join(stateRoot, "sessions"), pinnedCategory); err != nil {
			t.Skipf("platform cannot rename opened category: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(stateRoot, "sessions")); err != nil {
			t.Fatal(err)
		}
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err == nil ||
		!strings.Contains(err.Error(), "category") {
		t.Fatalf("category rename-to-symlink error = %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("category race changed source: %v", err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("category race mutated outside: %v, %v", entries, err)
	}
}

func TestRelocationTempControlsNeverBecomePayload(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(filepath.Join(sourceSessions, SessionRelocationTempDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sourceSessions, SessionRelocationTempDir, "crash-leftover"),
		[]byte("partial"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(sourceSessions, "real.jsonl")
	if err := os.WriteFile(session, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := SessionStoreMaterial(sourceSessions, sourceBlobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(material) != 1 || material[0] != session {
		t.Fatalf("temp control leaked into material: %v", material)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, "crash-leftover")); !os.IsNotExist(err) {
		t.Fatalf("crash temp became a session payload: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationTempDir)); !os.IsNotExist(err) {
		t.Fatalf("target temp controls were not cleaned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceSessions, SessionRelocationTempDir)); !os.IsNotExist(err) {
		t.Fatalf("source temp controls were not cleaned: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorRejectsManifestFromAnotherCheckout(t *testing.T) {
	root := canonicalTempDir(t)
	sourceASessions := filepath.Join(root, "checkout-a", "sessions")
	sourceABlobs := filepath.Join(root, "checkout-a", "blobs")
	sourceBSessions := filepath.Join(root, "state", "sessions", "local", "abcdef012345")
	sourceBBlobs := filepath.Join(root, "state", "staged-blobs", "local", "abcdef012345")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceASessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceASessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	crashing := newRelocatorForTest(t, sourceASessions, sourceABlobs, targetSessions, targetBlobs, "example")
	crashing.afterPublish = func() { panic("simulated crash") }
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated crash")
			}
		}()
		_ = crashing.Relocate(t.Context(), nil, nil)
	}()

	transition := SessionIdentityTransition{
		Version: SessionIdentityTransitionVersion, State: SessionIdentityTransitionPending,
		OldKey: "local/b", NewKey: "example", OldSessions: sourceBSessions, OldBlobs: sourceBBlobs,
		CurrentSessions: sourceBSessions, CurrentBlobs: sourceBBlobs, TargetProject: "example",
	}
	if err := WriteSessionIdentityTransition(sourceBSessions, transition); err != nil {
		t.Fatal(err)
	}
	authorizedOld := SessionStoreRelocationSource{
		Kind:     SessionStoreRelocationSourceOldGlobal,
		Sessions: sourceBSessions, StagedBlobs: sourceBBlobs, WriteTombstone: true,
	}
	resuming, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		Sources:                   []SessionStoreRelocationSource{authorizedOld},
		AuthorizedOldGlobalSource: &authorizedOld,
		TrustedStateRoot:          filepath.Join(root, "state"),
		StableRepoAuthority:       filepath.Join(root, "repo.git"),
		TargetSessions:            targetSessions, TargetBlobs: targetBlobs, TargetProject: "example",
		Transformer: app.CurrentSessionIdentityTransformer{}, Transition: &transition,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = resuming.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "different stable repository authority") {
		t.Fatalf("cross-checkout manifest error = %v", err)
	}
	current, err := ReadSessionIdentityTransition(sourceBSessions)
	if err != nil {
		t.Fatal(err)
	}
	if current == nil || current.State != SessionIdentityTransitionPending {
		t.Fatalf("cross-checkout resume changed current marker: %+v", current)
	}
}

func TestFilesystemSessionStoreRelocatorResumesFromSiblingWorktree(t *testing.T) {
	for _, removeOriginal := range []bool{false, true} {
		name := "source present"
		if removeOriginal {
			name = "source checkout removed"
		}
		t.Run(name, func(t *testing.T) {
			if _, err := exec.LookPath("git"); err != nil {
				t.Skip("git not available")
			}
			base := canonicalTempDir(t)
			runRelocationGit(t, base, "init", "--quiet", "--initial-branch=main")
			runRelocationGit(t, base, "config", "user.name", "Test")
			runRelocationGit(t, base, "config", "user.email", "test@example.invalid")
			runRelocationGit(t, base, "config", "commit.gpgsign", "false")
			if err := os.WriteFile(filepath.Join(base, "seed"), []byte("seed"), 0o600); err != nil {
				t.Fatal(err)
			}
			runRelocationGit(t, base, "add", "seed")
			runRelocationGit(t, base, "commit", "--quiet", "-m", "seed")
			original := filepath.Join(canonicalTempDir(t), "original")
			runRelocationGit(t, base, "worktree", "add", "--quiet", "-b", "original", original)

			sourceSessions := filepath.Join(original, ".sdd", "sessions")
			sourceBlobs := filepath.Join(original, ".sdd", "staged-blobs")
			targetState := canonicalTempDir(t)
			targetSessions := filepath.Join(targetState, "state", "sessions", "example")
			targetBlobs := filepath.Join(targetState, "state", "staged-blobs", "example")
			if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			authority, err := gitadapter.StableRepoRoot(base)
			if err != nil {
				t.Fatal(err)
			}
			authorizeFrom := func(repoRoot string) InTreeSessionSourceAuthorizer {
				return func(ctx context.Context, authority, sessions, blobs string) error {
					return gitadapter.AuthorizeInTreeSessionSource(ctx, repoRoot, authority, sessions, blobs)
				}
			}
			crashing, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
				Sources: []SessionStoreRelocationSource{{
					Kind:     SessionStoreRelocationSourceInTree,
					Sessions: sourceSessions, StagedBlobs: sourceBlobs, WriteTombstone: true,
				}},
				TrustedStateRoot:    filepath.Join(targetState, "state"),
				StableRepoAuthority: authority, AuthorizeInTreeSource: authorizeFrom(original),
				TargetSessions: targetSessions, TargetBlobs: targetBlobs, TargetProject: "example",
				Transformer: app.CurrentSessionIdentityTransformer{},
			})
			if err != nil {
				t.Fatal(err)
			}
			crashing.afterPublish = func() { panic("simulated crash") }
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("expected simulated crash")
					}
				}()
				_ = crashing.Relocate(t.Context(), nil, nil)
			}()
			if removeOriginal {
				runRelocationGit(t, base, "worktree", "remove", "--force", original)
			}

			resuming, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
				TrustedStateRoot:    filepath.Join(targetState, "state"),
				StableRepoAuthority: authority, AuthorizeInTreeSource: authorizeFrom(base),
				TargetSessions: targetSessions, TargetBlobs: targetBlobs, TargetProject: "example",
				Transformer: app.CurrentSessionIdentityTransformer{},
			})
			if err != nil {
				t.Fatal(err)
			}
			resumeErr := resuming.Relocate(t.Context(), nil, nil)
			if removeOriginal {
				if resumeErr == nil || !strings.Contains(resumeErr.Error(), "no recoverable source artifact") {
					t.Fatalf("removed-source recovery error = %v", resumeErr)
				}
				return
			}
			if resumeErr != nil {
				t.Fatal(resumeErr)
			}
			if got, err := os.ReadFile(filepath.Join(targetSessions, "legacy.jsonl")); err != nil ||
				string(got) != `{"event":"started"}` {
				t.Fatalf("resumed sibling target = %q, %v", got, err)
			}
		})
	}
}

func TestFilesystemSessionStoreRelocatorRejectsForeignCloneWithSameProjectID(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := canonicalTempDir(t)
	foreign := canonicalTempDir(t)
	for _, root := range []string{base, foreign} {
		runRelocationGit(t, root, "init", "--quiet", "--initial-branch=main")
	}
	sourceSessions := filepath.Join(base, ".sdd", "sessions")
	sourceBlobs := filepath.Join(base, ".sdd", "staged-blobs")
	targetState := canonicalTempDir(t)
	targetSessions := filepath.Join(targetState, "state", "sessions", "same-project")
	targetBlobs := filepath.Join(targetState, "state", "staged-blobs", "same-project")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	baseAuthority, err := gitadapter.StableRepoRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	crashing, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		Sources: []SessionStoreRelocationSource{{
			Kind:     SessionStoreRelocationSourceInTree,
			Sessions: sourceSessions, StagedBlobs: sourceBlobs, WriteTombstone: true,
		}},
		TrustedStateRoot:    filepath.Join(targetState, "state"),
		StableRepoAuthority: baseAuthority,
		AuthorizeInTreeSource: func(ctx context.Context, authority, sessions, blobs string) error {
			return gitadapter.AuthorizeInTreeSessionSource(ctx, base, authority, sessions, blobs)
		},
		TargetSessions: targetSessions, TargetBlobs: targetBlobs, TargetProject: "same/project",
		Transformer: app.CurrentSessionIdentityTransformer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	crashing.afterPublish = func() { panic("simulated crash") }
	func() {
		defer func() { _ = recover() }()
		_ = crashing.Relocate(t.Context(), nil, nil)
	}()
	foreignAuthority, err := gitadapter.StableRepoRoot(foreign)
	if err != nil {
		t.Fatal(err)
	}
	resuming, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		TrustedStateRoot:    filepath.Join(targetState, "state"),
		StableRepoAuthority: foreignAuthority,
		AuthorizeInTreeSource: func(ctx context.Context, authority, sessions, blobs string) error {
			return gitadapter.AuthorizeInTreeSessionSource(ctx, foreign, authority, sessions, blobs)
		},
		TargetSessions: targetSessions, TargetBlobs: targetBlobs, TargetProject: "same/project",
		Transformer: app.CurrentSessionIdentityTransformer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := resuming.Relocate(t.Context(), nil, nil); err == nil ||
		!strings.Contains(err.Error(), "different stable repository authority") {
		t.Fatalf("foreign clone resume error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceSessions, "legacy.jsonl")); err != nil {
		t.Fatalf("foreign resume changed source: %v", err)
	}
}

func runRelocationGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestFilesystemSessionStoreRelocatorRejectsCraftedOrCorruptManifest(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, manifestPath string)
	}{
		{
			name: "corrupt JSON",
			mutate: func(t *testing.T, manifestPath string) {
				t.Helper()
				if err := os.WriteFile(manifestPath, []byte(`{`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "crafted destination",
			mutate: func(t *testing.T, manifestPath string) {
				t.Helper()
				encoded, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				var manifest relocationManifest
				if err := json.Unmarshal(encoded, &manifest); err != nil {
					t.Fatal(err)
				}
				manifest.Payloads[0].Destination = filepath.Join(filepath.Dir(manifest.TargetSessions), "elsewhere.jsonl")
				if err := writeJSONAtomic(manifestPath, manifest); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "crafted mode",
			mutate: func(t *testing.T, manifestPath string) {
				t.Helper()
				encoded, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				var manifest relocationManifest
				if err := json.Unmarshal(encoded, &manifest); err != nil {
					t.Fatal(err)
				}
				manifest.Payloads[0].Mode = 0o777
				if err := writeJSONAtomic(manifestPath, manifest); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := canonicalTempDir(t)
			sourceSessions := filepath.Join(root, "source", "sessions")
			sourceBlobs := filepath.Join(root, "source", "blobs")
			targetSessions := filepath.Join(root, "state", "sessions", "example")
			targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
			if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(sourceSessions, "legacy.jsonl")
			if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			crashing := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
			crashing.afterPublish = func() { panic("simulated crash") }
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("expected simulated crash")
					}
				}()
				_ = crashing.Relocate(t.Context(), nil, nil)
			}()
			manifestPath := filepath.Join(targetSessions, SessionRelocationManifest)
			test.mutate(t, manifestPath)

			resuming := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
			if err := resuming.Relocate(t.Context(), nil, nil); err == nil {
				t.Fatal("crafted manifest was accepted")
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("crafted manifest changed source: %v", err)
			}
		})
	}
}

func TestFilesystemSessionStoreRelocatorRejectsUnsupportedOrUnknownCurrentCodec(t *testing.T) {
	for name, line := range map[string]string{
		"metadata codec": `{"version":1,"metadata":{"CodecVersion":2,"ID":"session","Subject":"local","Project":"local"}}`,
		"event codec":    `{"version":1,"events":[{"CodecVersion":2,"Code":"future","Payload":{}}]}`,
		"top-level":      `{"version":1,"future":true}`,
		"nested":         `{"version":1,"metadata":{"CodecVersion":1,"ID":"session","Subject":"local","Project":"local","Future":true}}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := canonicalTempDir(t)
			sourceSessions := filepath.Join(root, "source", "sessions")
			sourceBlobs := filepath.Join(root, "source", "blobs")
			targetSessions := filepath.Join(root, "state", "sessions", "example")
			targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
			if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(sourceSessions, "session.jsonl")
			if err := os.WriteFile(source, []byte(line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
			if err := relocator.Relocate(t.Context(), nil, nil); err == nil {
				t.Fatal("unsupported current codec was accepted")
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("codec rejection changed source: %v", err)
			}
			if _, err := os.Stat(filepath.Join(targetSessions, "session.jsonl")); !os.IsNotExist(err) {
				t.Fatalf("codec rejection published target: %v", err)
			}
		})
	}
}

func TestFilesystemSessionStoreRelocatorStopsWhenPinnedTargetIsRebound(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	originalTarget := targetSessions + "-pinned"
	relocator.beforeIrreversible = func(stage string) {
		if stage != "source_tombstones" {
			return
		}
		relocator.beforeIrreversible = nil
		if err := os.Rename(targetSessions, originalTarget); err != nil {
			t.Skipf("platform cannot rename an opened target directory: %v", err)
		}
		if err := os.MkdirAll(targetSessions, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	err := relocator.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "rebound") {
		t.Fatalf("target rebind error = %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("target rebind removed source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, "legacy.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("target replacement received payload: %v", err)
	}
	if _, err := os.Stat(filepath.Join(originalTarget, "legacy.jsonl")); err != nil {
		t.Fatalf("pinned target did not retain published payload: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorDoesNotDeleteReplacedSourceName(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	relocator.beforeIrreversible = func(stage string) {
		if stage != "source_delete" {
			return
		}
		relocator.beforeIrreversible = nil
		if err := os.Rename(source, source+".original"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := relocator.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("source replacement error = %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "replacement" {
		t.Fatalf("replacement source was deleted or changed: %q, %v", got, err)
	}
}

func TestFilesystemSessionStoreRelocatorQuarantinePreservesSameNameReplacement(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	relocator.afterSourceQuarantine = func() {
		relocator.afterSourceQuarantine = nil
		if err := os.WriteFile(source, []byte("same-name replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "same-name replacement" {
		t.Fatalf("same-name replacement was overwritten or removed: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(targetSessions, "legacy.jsonl")); err != nil ||
		string(got) != `{"event":"started"}` {
		t.Fatalf("quarantined target = %q, %v", got, err)
	}
}

func TestFilesystemSessionStoreRelocatorRestoresSourceWhenTargetRebindsAfterRemoval(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	pinnedTarget := targetSessions + "-pinned"
	relocator.afterQuarantineRemove = func() {
		relocator.afterQuarantineRemove = nil
		if err := os.Rename(targetSessions, pinnedTarget); err != nil {
			t.Skipf("platform cannot rename opened target: %v", err)
		}
		if err := os.MkdirAll(targetSessions, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	err := relocator.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "rebound") {
		t.Fatalf("post-removal target rebind error = %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != `{"event":"started"}` {
		t.Fatalf("post-removal rebind did not restore source: %q, %v", got, err)
	}
}

func TestRestoreRelocationSourceNameConvergesWithoutDisplacement(t *testing.T) {
	t.Run("crash after link", func(t *testing.T) {
		dir := canonicalTempDir(t)
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = root.Close() }()
		if err := os.WriteFile(filepath.Join(dir, "restore"), []byte("owned"), 0o600); err != nil {
			t.Fatal(err)
		}
		relocator := &FilesystemSessionStoreRelocator{}
		relocator.afterSourceRestoreLink = func() {
			relocator.afterSourceRestoreLink = nil
			panic("crash after source restore link")
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("expected source restore-link crash")
				}
			}()
			_ = relocator.restoreRelocationSourceName(root, "source", "restore")
		}()
		if err := relocator.restoreRelocationSourceName(root, "source", "restore"); err != nil {
			t.Fatalf("retrying source restoration: %v", err)
		}
		sourceInfo, err := os.Lstat(filepath.Join(dir, "source"))
		if err != nil {
			t.Fatal(err)
		}
		restoreInfo, err := os.Lstat(filepath.Join(dir, "restore"))
		if err != nil || !os.SameFile(sourceInfo, restoreInfo) {
			t.Fatalf("restored source identity = %v", err)
		}
		if displaced, err := filepath.Glob(filepath.Join(
			dir, SessionRelocationQuarantineDir, "*.displaced",
		)); err != nil || len(displaced) != 0 {
			t.Fatalf("source restoration created displaced orphans = %v, %v", displaced, err)
		}
	})
	t.Run("link EEXIST preserves foreign", func(t *testing.T) {
		dir := canonicalTempDir(t)
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = root.Close() }()
		if err := os.WriteFile(filepath.Join(dir, "restore"), []byte("owned"), 0o600); err != nil {
			t.Fatal(err)
		}
		relocator := &FilesystemSessionStoreRelocator{}
		relocator.beforeSourceRestoreLink = func() {
			relocator.beforeSourceRestoreLink = nil
			if err := os.WriteFile(filepath.Join(dir, "source"), []byte("foreign"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		err = relocator.restoreRelocationSourceName(root, "source", "restore")
		if err == nil || !strings.Contains(err.Error(), "restoration collision") {
			t.Fatalf("EEXIST source restoration error = %v", err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "source")); err != nil ||
			string(got) != "foreign" {
			t.Fatalf("EEXIST source restoration changed foreign = %q, %v", got, err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "restore")); err != nil ||
			string(got) != "owned" {
			t.Fatalf("EEXIST source restoration changed control = %q, %v", got, err)
		}
	})
}

func TestRelocationSourceRestorePreservesStaleForeignAtBothCallSites(t *testing.T) {
	for _, callsite := range []string{"quarantine identity race", "post-removal rollback"} {
		t.Run(callsite, func(t *testing.T) {
			root := canonicalTempDir(t)
			sourceSessions := filepath.Join(root, "source", "sessions")
			sourceBlobs := filepath.Join(root, "source", "blobs")
			targetSessions := filepath.Join(root, "state", "sessions", "example")
			targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
			source := filepath.Join(sourceSessions, "legacy.jsonl")
			target := filepath.Join(targetSessions, "legacy.jsonl")
			if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			const original = `{"event":"started"}`
			if err := os.WriteFile(source, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			relocator := newRelocatorForTest(
				t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
			)
			quarantine, restore := relocationRecoveryNames(source, target)
			const foreign = "stale foreign source"
			if callsite == "quarantine identity race" {
				relocator.afterSourceQuarantineRename = func() {
					relocator.afterSourceQuarantineRename = nil
					if err := os.Rename(
						filepath.Join(sourceSessions, quarantine), source,
					); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(
						filepath.Join(sourceSessions, quarantine), []byte("foreign quarantine"), 0o600,
					); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				relocator.afterQuarantineRemove = func() {
					relocator.afterQuarantineRemove = nil
					if err := os.WriteFile(source, []byte(foreign), 0o600); err != nil {
						t.Fatal(err)
					}
					file, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := file.WriteString(" changed"); err != nil {
						t.Fatal(err)
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			err := relocator.Relocate(t.Context(), nil, nil)
			if err == nil {
				t.Fatal("stale source restoration race succeeded")
			}
			want := foreign
			control := restore
			if callsite == "quarantine identity race" {
				want = original
				control = quarantine
			}
			if got, err := os.ReadFile(source); err != nil || string(got) != want {
				t.Fatalf("stale source race changed visible source = %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(sourceSessions, control)); err != nil {
				t.Fatalf("stale source race lost deterministic control %s: %v", control, err)
			}
			if displaced, err := filepath.Glob(filepath.Join(
				sourceSessions, SessionRelocationQuarantineDir, "*.displaced",
			)); err != nil || len(displaced) != 0 {
				t.Fatalf("stale source race created displaced orphans = %v, %v", displaced, err)
			}
			if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); err != nil {
				t.Fatalf("stale source race lost pending manifest: %v", err)
			}
		})
	}
}

func TestFilesystemSessionStoreRelocatorResumesQuarantineCrashWindows(t *testing.T) {
	for _, stage := range []string{
		"after quarantine",
		"during restore publication",
		"after restore link",
		"after quarantine removal",
	} {
		t.Run(stage, func(t *testing.T) {
			root := canonicalTempDir(t)
			sourceSessions := filepath.Join(root, "source", "sessions")
			sourceBlobs := filepath.Join(root, "source", "blobs")
			targetSessions := filepath.Join(root, "state", "sessions", "example")
			targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
			source := filepath.Join(sourceSessions, "legacy.jsonl")
			if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			crashing := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
			switch stage {
			case "after quarantine":
				crashing.afterSourceQuarantine = func() { panic("crash after quarantine") }
			case "during restore publication":
				crashing.afterRestoreTempSync = func() { panic("crash after restore temp sync") }
			case "after restore link":
				crashing.afterRestoreLink = func() { panic("crash after restore link") }
			case "after quarantine removal":
				crashing.afterQuarantineRemove = func() { panic("crash after quarantine removal") }
			}
			func() {
				defer func() { _ = recover() }()
				_ = crashing.Relocate(t.Context(), nil, nil)
			}()
			resuming := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
			if err := resuming.Relocate(t.Context(), nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(targetSessions, "legacy.jsonl")); err != nil {
				t.Fatalf("resumed quarantine target missing: %v", err)
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Fatalf("resumed quarantine left source: %v", err)
			}
			material, err := SessionStoreMaterial(sourceSessions, sourceBlobs)
			if err != nil || len(material) != 0 {
				t.Fatalf("resumed quarantine left material notice: %v, %v", material, err)
			}
			controls := filepath.Join(sourceSessions, SessionRelocationQuarantineDir)
			if entries, err := os.ReadDir(controls); err == nil && len(entries) != 0 {
				t.Fatalf("resumed quarantine left restore controls: %v", entries)
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
		})
	}
}

func TestFilesystemSessionStoreRelocatorRejectsForeignRestoreTemp(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	target := filepath.Join(targetSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	crashing := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	crashing.afterRestoreLink = func() { panic("crash after restore link") }
	func() {
		defer func() { _ = recover() }()
		_ = crashing.Relocate(t.Context(), nil, nil)
	}()
	_, restore := relocationRecoveryNames(source, target)
	temp := filepath.Join(sourceSessions, restore+".tmp")
	if err := os.Remove(temp); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temp, []byte("foreign restore temp"), 0o600); err != nil {
		t.Fatal(err)
	}
	resuming := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	err := resuming.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "restore temp differs") {
		t.Fatalf("foreign restore temp error = %v", err)
	}
	if got, err := os.ReadFile(temp); err != nil || string(got) != "foreign restore temp" {
		t.Fatalf("foreign restore temp was removed or changed: %q, %v", got, err)
	}
}

func TestFilesystemSessionStoreRelocatorKeepsPersistedAbsentSourceImmutable(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	crashing := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	crashing.afterPublish = func() { panic("simulated crash") }
	func() {
		defer func() { _ = recover() }()
		_ = crashing.Relocate(t.Context(), nil, nil)
	}()
	if err := os.RemoveAll(filepath.Dir(sourceSessions)); err != nil {
		t.Fatal(err)
	}
	resuming := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	resuming.beforeIrreversible = func(stage string) {
		if stage != "source_tombstones" {
			return
		}
		resuming.beforeIrreversible = nil
		if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	err := resuming.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no recoverable source artifact") {
		t.Fatalf("absent-source recovery error = %v", err)
	}
}

func TestFilesystemLegacySessionMigratorPinsDesiredRootsBeforeMigration(t *testing.T) {
	root := canonicalTempDir(t)
	sessionsCategory := filepath.Join(root, "state", "sessions")
	blobsCategory := filepath.Join(root, "state", "staged-blobs")
	sessionsDir := filepath.Join(sessionsCategory, "repo")
	blobsDir := filepath.Join(blobsCategory, "repo")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	event := engine.Event{
		V: 1, TS: time.Now().UTC(), Session: "legacy", Seq: 1,
		Event: engine.EventSessionMeta, Data: map[string]any{"participant": "Test"},
	}
	file, err := os.Create(filepath.Join(sessionsDir, "legacy.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(event); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		filepath.Join(root, "state"), sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	pinnedSessions := sessionsDir + "-pinned"
	migrator.beforeMutation = func() {
		if err := os.Rename(sessionsDir, pinnedSessions); err != nil {
			t.Skipf("platform cannot rename an opened migration root: %v", err)
		}
		if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	err = migrator.MigrateLegacySession(t.Context(), filepath.Join(sessionsDir, "legacy.jsonl"))
	if err == nil || !strings.Contains(err.Error(), "rebound") {
		t.Fatalf("legacy migration target rebind error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sessionsDir, "legacy.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("replacement migration root received session: %v", err)
	}
	if format, err := classifySessionFile(filepath.Join(pinnedSessions, "legacy.jsonl")); err != nil ||
		format != sessionFormatLegacy {
		t.Fatalf("pinned legacy session was mutated: %v, %v", format, err)
	}
}

func TestFilesystemLegacySessionMigratorRejectsSymlinkedDesiredRoot(t *testing.T) {
	root := canonicalTempDir(t)
	sessionsCategory := filepath.Join(root, "state", "sessions")
	blobsCategory := filepath.Join(root, "state", "staged-blobs")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(sessionsCategory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionsDir := filepath.Join(sessionsCategory, "repo")
	if err := os.Symlink(outside, sessionsDir); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		filepath.Join(root, "state"), sessionsDir, filepath.Join(blobsCategory, "repo"),
		"local", "example",
	)
	if err == nil {
		_, err = migrator.ListLegacySessions(t.Context())
	}
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlinked legacy migration root error = %v", err)
	}
}

func TestFilesystemLegacySessionMigratorConstructorAndListAreReadOnly(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	if _, err := os.Lstat(stateRoot); !os.IsNotExist(err) {
		t.Fatalf("legacy constructor created target state: %v", err)
	}
	candidates, err := migrator.ListLegacySessions(t.Context())
	if err != nil || len(candidates) != 0 {
		t.Fatalf("absent legacy listing = %v, %v", candidates, err)
	}
	if _, err := os.Lstat(stateRoot); !os.IsNotExist(err) {
		t.Fatalf("legacy listing created target state: %v", err)
	}
}

func TestFilesystemLegacySessionMigratorNeverOverwritesRacedSessionName(t *testing.T) {
	for _, stage := range []string{"after verified", "after quarantine"} {
		t.Run(stage, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			if stage == "after verified" {
				migrator.afterSourceVerified = func() {
					migrator.afterSourceVerified = nil
					if err := os.Rename(legacyPath, legacyPath+".verified"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(legacyPath, []byte("replacement"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				migrator.afterSourceQuarantine = func() {
					migrator.afterSourceQuarantine = nil
					if err := os.WriteFile(legacyPath, []byte("replacement"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err == nil {
				t.Fatal("raced legacy name was overwritten")
			}
			if stage == "after verified" {
				if got, err := os.ReadFile(legacyPath); err != nil || string(got) != "replacement" {
					t.Fatalf("verified-race replacement = %q, %v", got, err)
				}
			} else if format, err := classifySessionFile(legacyPath); err != nil ||
				format != sessionFormatLegacy {
				t.Fatalf("quarantine-race source was not restored: %v, %v", format, err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorRecoversCrashAfterQuarantine(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	migrator.afterSourceQuarantine = func() {
		migrator.afterSourceQuarantine = nil
		panic("simulated legacy crash")
	}
	func() {
		defer func() { _ = recover() }()
		_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
	}()
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatal(err)
	}
	if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatCurrent {
		t.Fatalf("recovered legacy migration = %v, %v", format, err)
	}
	for _, control := range []string{"legacy.json", "legacy.original", "legacy.restore", "legacy.prepared"} {
		if _, err := os.Stat(filepath.Join(sessionsDir, legacyMigrationControlDir, control)); !os.IsNotExist(err) {
			t.Fatalf("successful recovery retained control %s: %v", control, err)
		}
	}
	if err := migrator.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFilesystemLegacySessionMigratorBlobWriteAheadCrashWindows(t *testing.T) {
	for _, stage := range []string{
		"before blob proof",
		"inside blob proof",
		"after blob proof",
		"before session proof",
		"inside session proof",
		"after session proof",
		"after plan",
		"after data publication",
	} {
		t.Run(stage, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			stagingDir := filepath.Join(sessionsDir, "legacy-staging")
			if err := os.MkdirAll(stagingDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stagingDir, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
				t.Fatal(err)
			}
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			switch stage {
			case "before blob proof":
				migrator.beforeBlobProofWrite = func() {
					migrator.beforeBlobProofWrite = nil
					panic("crash before blob proof")
				}
			case "inside blob proof":
				migrator.duringBlobStage = func() {
					migrator.duringBlobStage = nil
					panic("crash inside blob proof")
				}
			case "after blob proof":
				migrator.afterBlobProofPrepared = func() {
					migrator.afterBlobProofPrepared = nil
					panic("crash after blob proof")
				}
			case "before session proof":
				migrator.beforeSessionProofWrite = func() {
					migrator.beforeSessionProofWrite = nil
					panic("crash before session proof")
				}
			case "inside session proof":
				migrator.duringSessionProofWrite = func() {
					migrator.duringSessionProofWrite = nil
					panic("crash inside session proof")
				}
			case "after session proof":
				migrator.afterSessionProofPrepared = func() {
					migrator.afterSessionProofPrepared = nil
					panic("crash after session proof")
				}
			case "after plan":
				migrator.afterBlobPlanPersisted = func() {
					migrator.afterBlobPlanPersisted = nil
					panic("crash after blob plan")
				}
			case "after data publication":
				migrator.afterBlobDataPublished = func() {
					migrator.afterBlobDataPublished = nil
					panic("crash after blob data publication")
				}
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("expected simulated blob migration crash")
					}
				}()
				_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
			}()
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
				t.Fatal(err)
			}
			if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatCurrent {
				t.Fatalf("recovered staged-attachment migration = %v, %v", format, err)
			}
			if err := filepath.WalkDir(blobsDir, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if strings.HasSuffix(entry.Name(), ".legacy-tmp") {
					t.Errorf("legacy blob temp survived recovery: %s", path)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(sessionsDir, legacyMigrationControlDir, "legacy.json")); !os.IsNotExist(err) {
				t.Fatalf("legacy journal survived successful retry: %v", err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorRestoresAggregateOnQuarantineRemovalError(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	stagingDir := filepath.Join(sessionsDir, "legacy-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	migrator.removeQuarantine = func(*os.Root, string) error {
		migrator.removeQuarantine = nil
		return errors.New("injected quarantine removal failure")
	}
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err == nil ||
		!strings.Contains(err.Error(), "injected quarantine removal failure") {
		t.Fatalf("quarantine removal error = %v", err)
	}
	if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatLegacy {
		t.Fatalf("failed aggregate did not restore legacy session = %v, %v", format, err)
	}
	var blobArtifacts []string
	if err := filepath.WalkDir(blobsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if !entry.IsDir() {
			blobArtifacts = append(blobArtifacts, path)
		}
		return nil
	}); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if len(blobArtifacts) != 0 {
		t.Fatalf("failed aggregate left staged blob artifacts: %v", blobArtifacts)
	}
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatal(err)
	}
	if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatCurrent {
		t.Fatalf("retry did not publish aggregate = %v, %v", format, err)
	}
}

func TestFilesystemLegacySessionMigratorPreservesRacedForeignBlobFinals(t *testing.T) {
	for _, stage := range []string{
		"after plan data collision",
		"after data link metadata collision",
		"foreign data replacement",
		"foreign metadata replacement",
	} {
		t.Run(stage, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			stagingDir := filepath.Join(sessionsDir, "legacy-staging")
			if err := os.MkdirAll(stagingDir, 0o755); err != nil {
				t.Fatal(err)
			}
			const attachment = "evidence"
			const filename = "evidence.txt"
			if err := os.WriteFile(filepath.Join(stagingDir, filename), []byte(attachment), 0o600); err != nil {
				t.Fatal(err)
			}
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			owner := app.BlobOwner{Subject: "local", Session: "legacy"}
			id := deterministicLegacyBlobID(owner, filename, []byte(attachment))
			ownerDir, err := migrator.blobs.ownerDir(owner)
			if err != nil {
				t.Fatal(err)
			}
			dataFinal := filepath.Join(ownerDir, id+".blob")
			metadataFinal := filepath.Join(ownerDir, id+".json")
			writeForeign := func(path, content string) {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			switch stage {
			case "after plan data collision":
				migrator.afterBlobPlanPersisted = func() {
					migrator.afterBlobPlanPersisted = nil
					writeForeign(dataFinal, "foreign data collision")
				}
			case "after data link metadata collision":
				migrator.afterBlobDataPublished = func() {
					migrator.afterBlobDataPublished = nil
					writeForeign(metadataFinal, "foreign metadata collision")
				}
			case "foreign data replacement":
				migrator.afterBlobDataPublished = func() {
					migrator.afterBlobDataPublished = nil
					if err := os.Remove(dataFinal); err != nil {
						t.Fatal(err)
					}
					writeForeign(dataFinal, "foreign data replacement")
				}
			case "foreign metadata replacement":
				migrator.afterBlobMetadataPublished = func() {
					migrator.afterBlobMetadataPublished = nil
					if err := os.Remove(metadataFinal); err != nil {
						t.Fatal(err)
					}
					writeForeign(metadataFinal, "foreign metadata replacement")
				}
			}
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err == nil {
				t.Fatal("raced foreign staged-blob final did not fail migration")
			}
			if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatLegacy {
				t.Fatalf("raced blob final changed legacy session = %v, %v", format, err)
			}
			switch stage {
			case "after plan data collision":
				if got, err := os.ReadFile(dataFinal); err != nil || string(got) != "foreign data collision" {
					t.Fatalf("data collision final = %q, %v", got, err)
				}
			case "after data link metadata collision":
				if got, err := os.ReadFile(metadataFinal); err != nil || string(got) != "foreign metadata collision" {
					t.Fatalf("metadata collision final = %q, %v", got, err)
				}
			case "foreign data replacement":
				if got, err := os.ReadFile(dataFinal); err != nil || string(got) != "foreign data replacement" {
					t.Fatalf("foreign data replacement = %q, %v", got, err)
				}
			case "foreign metadata replacement":
				if got, err := os.ReadFile(metadataFinal); err != nil || string(got) != "foreign metadata replacement" {
					t.Fatalf("foreign metadata replacement = %q, %v", got, err)
				}
			}
			for _, suffix := range []string{".blob.legacy-tmp", ".json.legacy-tmp"} {
				if _, err := os.Stat(filepath.Join(ownerDir, id+suffix)); err != nil {
					t.Fatalf("ownership proof was lost after conflicting final: %s: %v", suffix, err)
				}
			}
			if _, err := os.Stat(filepath.Join(
				sessionsDir, legacyMigrationControlDir, "legacy.json",
			)); err != nil {
				t.Fatalf("journal was lost after conflicting final: %v", err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorRejectsForeignCurrentRecoveryCollision(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	migrator.afterSourceQuarantine = func() {
		migrator.afterSourceQuarantine = nil
		panic("simulated crash after quarantine")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated migration crash")
			}
		}()
		_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
	}()
	const foreign = `{"version":999,"foreign":true}`
	if err := os.WriteFile(legacyPath, []byte(foreign), 0o600); err != nil {
		t.Fatal(err)
	}
	err = migrator.MigrateLegacySession(t.Context(), legacyPath)
	if err == nil || !strings.Contains(err.Error(), "collides with journaled migration controls") {
		t.Fatalf("foreign current recovery collision error = %v", err)
	}
	if got, err := os.ReadFile(legacyPath); err != nil || string(got) != foreign {
		t.Fatalf("foreign current recovery collision changed live name = %q, %v", got, err)
	}
	for _, control := range []string{
		"legacy.json", "legacy.original", "legacy.restore", "legacy.prepared",
	} {
		if _, err := os.Stat(filepath.Join(sessionsDir, legacyMigrationControlDir, control)); err != nil {
			t.Fatalf("foreign collision lost recovery control %s: %v", control, err)
		}
	}
}

func TestFilesystemLegacySessionMigratorRejectsForeignReplacementAfterSessionPublish(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	const foreign = `{"version":999,"replacement":true}`
	migrator.afterSessionPublished = func() {
		migrator.afterSessionPublished = nil
		if err := os.Remove(legacyPath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacyPath, []byte(foreign), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err = migrator.MigrateLegacySession(t.Context(), legacyPath)
	if err == nil || !strings.Contains(err.Error(), "not migration-owned") {
		t.Fatalf("foreign post-publish replacement error = %v", err)
	}
	if got, err := os.ReadFile(legacyPath); err != nil || string(got) != foreign {
		t.Fatalf("foreign post-publish replacement changed live name = %q, %v", got, err)
	}
	for _, control := range []string{
		"legacy.json", "legacy.original", "legacy.restore", "legacy.prepared",
	} {
		if _, err := os.Stat(filepath.Join(sessionsDir, legacyMigrationControlDir, control)); err != nil {
			t.Fatalf("foreign replacement lost recovery control %s: %v", control, err)
		}
	}
}

func TestFilesystemLegacySessionMigratorRetryRestoresForeignPublishedRollbackName(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	migrator.removeQuarantine = func(*os.Root, string) error {
		migrator.removeQuarantine = nil
		return errors.New("injected rollback trigger")
	}
	const foreign = `{"version":999,"foreign_after_verify":true}`
	migrator.beforeLiveSessionQuarantine = func() {
		migrator.beforeLiveSessionQuarantine = nil
		if err := os.Remove(legacyPath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacyPath, []byte(foreign), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	migrator.afterLiveSessionQuarantine = func() {
		migrator.afterLiveSessionQuarantine = nil
		panic("crash after published rollback rename")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected crash after published rollback rename")
			}
		}()
		_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
	}()
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("crash unexpectedly retained live name: %v", err)
	}
	err = migrator.MigrateLegacySession(t.Context(), legacyPath)
	if err == nil || !strings.Contains(err.Error(), "restored foreign published-session rollback") {
		t.Fatalf("retry foreign published rollback error = %v", err)
	}
	if got, err := os.ReadFile(legacyPath); err != nil || string(got) != foreign {
		t.Fatalf("retry did not restore foreign live name = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(
		sessionsDir, legacyMigrationControlDir, "legacy.published-rollback",
	)); !os.IsNotExist(err) {
		t.Fatalf("retry retained hidden published rollback control: %v", err)
	}
	if _, err := os.Stat(filepath.Join(
		sessionsDir, legacyMigrationControlDir, "legacy.json",
	)); err != nil {
		t.Fatalf("retry lost migration journal: %v", err)
	}
}

func TestRestoreQuarantinedVisibleNameRetriesAfterLinkCrash(t *testing.T) {
	for _, contextName := range []string{
		"session rollback",
		"committed session proof cleanup",
		"blob commit",
		"blob rollback",
	} {
		t.Run(contextName, func(t *testing.T) {
			dir := canonicalTempDir(t)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			if err := os.WriteFile(filepath.Join(dir, "quarantine"), []byte(contextName), 0o600); err != nil {
				t.Fatal(err)
			}
			migrator := &FilesystemLegacySessionMigrator{}
			migrator.afterQuarantineRestoreLink = func() {
				migrator.afterQuarantineRestoreLink = nil
				panic("crash after restore link")
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("expected restore-link crash")
					}
				}()
				_ = migrator.restoreQuarantinedVisibleName(root, "quarantine", "visible")
			}()
			if err := migrator.restoreQuarantinedVisibleName(root, "quarantine", "visible"); err != nil {
				t.Fatalf("retrying restore link: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(dir, "visible")); err != nil ||
				string(got) != contextName {
				t.Fatalf("restored visible = %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "quarantine")); !os.IsNotExist(err) {
				t.Fatalf("retry retained quarantine: %v", err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorRejectsForgedPairedBlobProof(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	stagingDir := filepath.Join(sessionsDir, "legacy-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const filename = "evidence.txt"
	const attachment = "evidence"
	if err := os.WriteFile(filepath.Join(stagingDir, filename), []byte(attachment), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	migrator.afterBlobPlanPersisted = func() {
		migrator.afterBlobPlanPersisted = nil
		panic("simulated crash after proof journal")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated migration crash")
			}
		}()
		_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
	}()
	owner := app.BlobOwner{Subject: "local", Session: "legacy"}
	id := deterministicLegacyBlobID(owner, filename, []byte(attachment))
	ownerDir, err := migrator.blobs.ownerDir(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][3]string{
		{id + ".blob.legacy-tmp", id + ".blob", "forged data"},
		{id + ".json.legacy-tmp", id + ".json", `{"forged":true}`},
	} {
		proof := filepath.Join(ownerDir, pair[0])
		final := filepath.Join(ownerDir, pair[1])
		if err := os.Remove(proof); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(proof, []byte(pair[2]), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(proof, final); err != nil {
			t.Fatal(err)
		}
	}
	err = migrator.MigrateLegacySession(t.Context(), legacyPath)
	if err == nil {
		t.Fatalf("forged paired proof recovery error = %v", err)
	}
	for _, suffix := range []string{
		".blob.legacy-tmp", ".blob", ".json.legacy-tmp", ".json",
	} {
		if _, err := os.Stat(filepath.Join(ownerDir, id+suffix)); err != nil {
			t.Fatalf("forged paired proof was removed %s: %v", suffix, err)
		}
	}
	if _, err := os.Stat(filepath.Join(sessionsDir, legacyMigrationControlDir, "legacy.json")); err != nil {
		t.Fatalf("forged paired proof lost journal: %v", err)
	}
}

func TestFilesystemLegacySessionMigratorRejectsInPlaceBlobCorruption(t *testing.T) {
	for _, artifact := range []string{"data", "metadata"} {
		t.Run(artifact, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			stagingDir := filepath.Join(sessionsDir, "legacy-staging")
			if err := os.MkdirAll(stagingDir, 0o755); err != nil {
				t.Fatal(err)
			}
			const filename = "evidence.txt"
			const attachment = "evidence"
			if err := os.WriteFile(filepath.Join(stagingDir, filename), []byte(attachment), 0o600); err != nil {
				t.Fatal(err)
			}
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			owner := app.BlobOwner{Subject: "local", Session: "legacy"}
			id := deterministicLegacyBlobID(owner, filename, []byte(attachment))
			ownerDir, err := migrator.blobs.ownerDir(owner)
			if err != nil {
				t.Fatal(err)
			}
			migrator.afterBlobMetadataPublished = func() {
				migrator.afterBlobMetadataPublished = nil
				path := filepath.Join(ownerDir, id+".blob")
				if artifact == "metadata" {
					path = filepath.Join(ownerDir, id+".json")
				}
				if err := os.WriteFile(path, []byte("corrupt in place"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err = migrator.MigrateLegacySession(t.Context(), legacyPath)
			if err == nil {
				t.Fatalf("in-place %s corruption error = %v", artifact, err)
			}
			suffix := ".blob"
			if artifact == "metadata" {
				suffix = ".json"
			}
			for _, tail := range []string{suffix, suffix + ".legacy-tmp"} {
				got, readErr := os.ReadFile(filepath.Join(ownerDir, id+tail))
				if readErr != nil || string(got) != "corrupt in place" {
					t.Fatalf("in-place %s corruption was cleaned %s = %q, %v", artifact, tail, got, readErr)
				}
			}
			if _, err := os.Stat(filepath.Join(sessionsDir, legacyMigrationControlDir, "legacy.json")); err != nil {
				t.Fatalf("in-place corruption lost journal: %v", err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorRecoversPartialCommittedProofCleanup(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	stagingDir := filepath.Join(sessionsDir, "legacy-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	migrator.afterBlobProofRemoved = func() {
		migrator.afterBlobProofRemoved = nil
		panic("simulated crash during committed proof cleanup")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated migration crash")
			}
		}()
		_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
	}()
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatal(err)
	}
	if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatCurrent {
		t.Fatalf("partial committed cleanup recovery = %v, %v", format, err)
	}
	if err := filepath.WalkDir(blobsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasSuffix(entry.Name(), ".legacy-tmp") {
			t.Errorf("partial committed cleanup retained proof: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, control := range []string{"legacy.json", "legacy.prepared"} {
		if _, err := os.Stat(filepath.Join(sessionsDir, legacyMigrationControlDir, control)); !os.IsNotExist(err) {
			t.Fatalf("partial committed cleanup retained control %s: %v", control, err)
		}
	}
}

func TestFilesystemLegacySessionMigratorPreservesConflictingPreJournalProof(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	stagingDir := filepath.Join(sessionsDir, "legacy-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const filename = "evidence.txt"
	const attachment = "evidence"
	if err := os.WriteFile(filepath.Join(stagingDir, filename), []byte(attachment), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	migrator.afterBlobProofPrepared = func() {
		migrator.afterBlobProofPrepared = nil
		panic("simulated pre-journal crash")
	}
	func() {
		defer func() { _ = recover() }()
		_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
	}()
	owner := app.BlobOwner{Subject: "local", Session: "legacy"}
	id := deterministicLegacyBlobID(owner, filename, []byte(attachment))
	ownerDir, err := migrator.blobs.ownerDir(owner)
	if err != nil {
		t.Fatal(err)
	}
	proof := filepath.Join(ownerDir, id+".blob.legacy-tmp")
	if err := os.Remove(proof); err != nil {
		t.Fatal(err)
	}
	const foreign = "foreign conflicting proof"
	if err := os.WriteFile(proof, []byte(foreign), 0o600); err != nil {
		t.Fatal(err)
	}
	err = migrator.MigrateLegacySession(t.Context(), legacyPath)
	if err == nil || !strings.Contains(err.Error(), "conflicting nonmatching migration proof") {
		t.Fatalf("conflicting pre-journal proof error = %v", err)
	}
	if got, err := os.ReadFile(proof); err != nil || string(got) != foreign {
		t.Fatalf("conflicting pre-journal proof changed = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(
		sessionsDir, legacyMigrationControlDir, "legacy.json",
	)); !os.IsNotExist(err) {
		t.Fatalf("conflicting pre-journal proof gained journal: %v", err)
	}
}

func TestFilesystemLegacySessionMigratorCommittedCleanupRecovery(t *testing.T) {
	for _, stage := range []string{
		"after session proof removal",
		"proof rename error",
		"missing committed live",
	} {
		t.Run(stage, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			switch stage {
			case "after session proof removal":
				migrator.afterSessionProofRemoved = func() {
					migrator.afterSessionProofRemoved = nil
					panic("crash after session proof removal")
				}
			case "proof rename error":
				migrator.renameSessionProof = func(*os.Root, string, string) error {
					migrator.renameSessionProof = nil
					return errors.New("injected session proof rename error")
				}
			case "missing committed live":
				migrator.afterCommittedJournal = func() {
					migrator.afterCommittedJournal = nil
					if err := os.Remove(legacyPath); err != nil {
						t.Fatal(err)
					}
					panic("crash after committed live disappeared")
				}
			}
			func() {
				defer func() { _ = recover() }()
				_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
			}()
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
				t.Fatalf("recovering %s: %v", stage, err)
			}
			if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatCurrent {
				t.Fatalf("committed cleanup recovery %s = %v, %v", stage, format, err)
			}
			for _, control := range []string{"legacy.json", "legacy.prepared", "legacy.prepared-cleanup"} {
				if _, err := os.Stat(filepath.Join(
					sessionsDir, legacyMigrationControlDir, control,
				)); !os.IsNotExist(err) {
					t.Fatalf("committed cleanup %s retained %s: %v", stage, control, err)
				}
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorPreservesReplacementDuringCommittedBlobCleanup(t *testing.T) {
	for _, replaced := range []string{"live", "proof"} {
		t.Run(replaced, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			stagingDir := filepath.Join(sessionsDir, "legacy-staging")
			if err := os.MkdirAll(stagingDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stagingDir, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
				t.Fatal(err)
			}
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			const foreign = `{"version":999,"foreign_cleanup_replacement":true}`
			proof := filepath.Join(sessionsDir, legacyMigrationControlDir, "legacy.prepared")
			migrator.afterBlobProofRemoved = func() {
				migrator.afterBlobProofRemoved = nil
				path := legacyPath
				if replaced == "proof" {
					path = proof
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(foreign), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err == nil {
				t.Fatalf("%s replacement during blob proof cleanup succeeded", replaced)
			}
			path := legacyPath
			if replaced == "proof" {
				path = proof
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != foreign {
				t.Fatalf("%s replacement during blob cleanup = %q, %v", replaced, got, err)
			}
			if _, err := os.Stat(filepath.Join(
				sessionsDir, legacyMigrationControlDir, "legacy.json",
			)); err != nil {
				t.Fatalf("%s replacement lost committed journal: %v", replaced, err)
			}
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err == nil {
				t.Fatalf("%s replacement recovery did not preserve conflict", replaced)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != foreign {
				t.Fatalf("%s replacement changed on retry = %q, %v", replaced, got, err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorPreparedRollbackCrashRecovery(t *testing.T) {
	for _, stage := range []string{"after blob pair", "after session proof"} {
		t.Run(stage, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			if stage == "after blob pair" {
				stagingDir := filepath.Join(sessionsDir, "legacy-staging")
				if err := os.MkdirAll(stagingDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(stagingDir, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			migrator.removeQuarantine = func(*os.Root, string) error {
				migrator.removeQuarantine = nil
				return errors.New("injected rollback trigger")
			}
			if stage == "after blob pair" {
				migrator.afterBlobRollbackPair = func() {
					migrator.afterBlobRollbackPair = nil
					panic("crash after first blob rollback pair")
				}
			} else {
				migrator.afterSessionRollbackProof = func() {
					migrator.afterSessionRollbackProof = nil
					panic("crash after rollback session proof")
				}
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("expected rollback cleanup crash")
					}
				}()
				_ = migrator.MigrateLegacySession(t.Context(), legacyPath)
			}()
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
				t.Fatalf("recovering rollback crash %s: %v", stage, err)
			}
			if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatCurrent {
				t.Fatalf("rollback crash recovery %s = %v, %v", stage, format, err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorRestoresRacedVisibleNamesFromRollbackQuarantine(t *testing.T) {
	for _, replaced := range []string{"live session", "blob final"} {
		t.Run(replaced, func(t *testing.T) {
			root := canonicalTempDir(t)
			stateRoot := filepath.Join(root, "state")
			sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
			blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
			legacyPath := writeLegacyMigrationSession(t, sessionsDir)
			if replaced == "blob final" {
				stagingDir := filepath.Join(sessionsDir, "legacy-staging")
				if err := os.MkdirAll(stagingDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(stagingDir, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
				stateRoot, sessionsDir, blobsDir, "local", "example",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = migrator.Close() }()
			migrator.removeQuarantine = func(*os.Root, string) error {
				migrator.removeQuarantine = nil
				return errors.New("injected rollback trigger")
			}
			const foreign = "foreign raced visible name"
			foreignPath := legacyPath
			if replaced == "live session" {
				migrator.beforeLiveSessionQuarantine = func() {
					migrator.beforeLiveSessionQuarantine = nil
					if err := os.Remove(legacyPath); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(legacyPath, []byte(foreign), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				owner := app.BlobOwner{Subject: "local", Session: "legacy"}
				id := deterministicLegacyBlobID(owner, "evidence.txt", []byte("evidence"))
				ownerDir, err := migrator.blobs.ownerDir(owner)
				if err != nil {
					t.Fatal(err)
				}
				foreignPath = filepath.Join(ownerDir, id+".blob")
				migrator.beforeBlobFinalQuarantine = func() {
					migrator.beforeBlobFinalQuarantine = nil
					if err := os.Remove(foreignPath); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(foreignPath, []byte(foreign), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err == nil {
				t.Fatalf("%s rollback race succeeded", replaced)
			}
			if got, err := os.ReadFile(foreignPath); err != nil || string(got) != foreign {
				t.Fatalf("%s rollback race did not restore foreign name = %q, %v", replaced, got, err)
			}
			if _, err := os.Stat(filepath.Join(
				sessionsDir, legacyMigrationControlDir, "legacy.json",
			)); err != nil {
				t.Fatalf("%s rollback race lost journal: %v", replaced, err)
			}
		})
	}
}

func TestFilesystemLegacySessionMigratorRetriesCommittedStagingCleanup(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
	blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	stagingDir := filepath.Join(sessionsDir, "legacy-staging")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	migrator.removeLegacyStaging = func(*os.Root, string) error {
		migrator.removeLegacyStaging = nil
		return errors.New("injected staging cleanup failure")
	}
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err == nil ||
		!strings.Contains(err.Error(), "staging cleanup failure") {
		t.Fatalf("staging cleanup failure = %v", err)
	}
	journal := filepath.Join(sessionsDir, legacyMigrationControlDir, "legacy.json")
	if _, err := os.Stat(journal); err != nil {
		t.Fatalf("staging cleanup failure lost completion journal: %v", err)
	}
	candidates, err := migrator.ListLegacySessions(t.Context())
	if err != nil || len(candidates) != 1 || candidates[0] != legacyPath {
		t.Fatalf("staging cleanup retry candidates = %v, %v", candidates, err)
	}
	if err := migrator.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatalf("retrying committed staging cleanup: %v", err)
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatalf("retry retained completion journal: %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("retry retained legacy staging directory: %v", err)
	}
	if format, err := classifySessionFile(legacyPath); err != nil || format != sessionFormatCurrent {
		t.Fatalf("retry current session = %v, %v", format, err)
	}
}

func TestInTreeControlOnlyLegacyRecoveryPrecedesRelocation(t *testing.T) {
	root := canonicalTempDir(t)
	sddDir := filepath.Join(root, ".sdd")
	sessionsDir := filepath.Join(sddDir, "sessions")
	blobsDir := filepath.Join(sddDir, "staged-blobs")
	legacyPath := writeLegacyMigrationSession(t, sessionsDir)
	crashing, err := NewFilesystemLegacySessionMigrator(
		sessionsDir, blobsDir, "local", "local",
	)
	if err != nil {
		t.Fatal(err)
	}
	crashing.afterSourceQuarantine = func() { panic("control-only crash") }
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected control-only migration crash")
			}
		}()
		_ = crashing.MigrateLegacySession(t.Context(), legacyPath)
	}()
	if err := crashing.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("control-only crash retained live source: %v", err)
	}
	recovering, err := NewFilesystemLegacySessionMigrator(
		sessionsDir, blobsDir, "local", "local",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = recovering.Close() }()
	candidates, err := recovering.ListPendingLegacySessions(t.Context())
	if err != nil || len(candidates) != 1 || candidates[0] != legacyPath {
		t.Fatalf("in-tree control-only candidates = %v, %v", candidates, err)
	}
	if err := recovering.MigrateLegacySession(t.Context(), legacyPath); err != nil {
		t.Fatalf("recovering in-tree controls: %v", err)
	}

	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	relocator := newRelocatorForTest(
		t, sessionsDir, blobsDir, targetSessions, targetBlobs, "example",
	)
	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatalf("relocating recovered in-tree state: %v", err)
	}
	target := filepath.Join(targetSessions, "legacy.jsonl")
	if format, err := classifySessionFile(target); err != nil || format != sessionFormatCurrent {
		t.Fatalf("relocated recovered session = %v, %v", format, err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, legacyMigrationControlDir)); !os.IsNotExist(err) {
		t.Fatalf("relocation copied application controls: %v", err)
	}
}

func TestRelocationRollbackPreservesChangedPublishedTarget(t *testing.T) {
	for _, mutation := range []string{"append", "replace"} {
		t.Run(mutation, func(t *testing.T) {
			root := canonicalTempDir(t)
			sourceSessions := filepath.Join(root, "source", "sessions")
			sourceBlobs := filepath.Join(root, "source", "blobs")
			targetSessions := filepath.Join(root, "state", "sessions", "example")
			targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
			if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"a.jsonl", "b.jsonl"} {
				if err := os.WriteFile(filepath.Join(sourceSessions, name), []byte(name), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			relocator := newRelocatorForTest(
				t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
			)
			const foreign = "foreign desired writer"
			changed := ""
			relocator.afterPublish = func() {
				relocator.afterPublish = nil
				for _, name := range []string{"a.jsonl", "b.jsonl"} {
					path := filepath.Join(targetSessions, name)
					if _, err := os.Stat(path); err == nil {
						changed = path
						if mutation == "append" {
							file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
							if err != nil {
								t.Fatal(err)
							}
							if _, err := file.WriteString(foreign); err != nil {
								t.Fatal(err)
							}
							if err := file.Close(); err != nil {
								t.Fatal(err)
							}
						} else {
							if err := os.Remove(path); err != nil {
								t.Fatal(err)
							}
							if err := os.WriteFile(path, []byte(foreign), 0o600); err != nil {
								t.Fatal(err)
							}
						}
						break
					}
				}
				cancel()
			}
			if err := relocator.Relocate(ctx, nil, nil); err == nil {
				t.Fatal("changed-target rollback succeeded")
			}
			got, err := os.ReadFile(changed)
			if err != nil || !strings.Contains(string(got), foreign) {
				t.Fatalf("rollback removed changed desired target = %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); err != nil {
				t.Fatalf("changed-target rollback lost recovery manifest: %v", err)
			}
		})
	}
}

func TestRelocationRollbackRestoreLinkCrashConverges(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(sourceSessions, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	relocator := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	relocator.afterPublish = func() {
		relocator.afterPublish = nil
		cancel()
	}
	const changed = "changed after rollback quarantine"
	relocator.afterTargetRollbackQuarantine = func(relative string) {
		relocator.afterTargetRollbackQuarantine = nil
		if err := os.WriteFile(filepath.Join(targetSessions, relative), []byte(changed), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	relocator.afterTargetRollbackRestoreLink = func() {
		relocator.afterTargetRollbackRestoreLink = nil
		panic("crash after target rollback restore link")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected target rollback restore-link crash")
			}
		}()
		_ = relocator.Relocate(ctx, nil, nil)
	}()
	resuming := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	if err := resuming.Relocate(t.Context(), nil, nil); err == nil {
		t.Fatal("changed restored target unexpectedly resumed")
	}
	var visible string
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		path := filepath.Join(targetSessions, name)
		if got, err := os.ReadFile(path); err == nil && string(got) == changed {
			visible = path
		}
	}
	if visible == "" {
		t.Fatal("retry did not preserve changed restored target")
	}
	controls, err := filepath.Glob(filepath.Join(
		targetSessions, SessionRelocationQuarantineDir, "*.target-rollback",
	))
	if err != nil || len(controls) != 0 {
		t.Fatalf("retry retained target rollback quarantine = %v, %v", controls, err)
	}
}

func TestSessionTargetRollbackHonorsWriterLockThroughFinalUnlink(t *testing.T) {
	t.Run("active writer preserves publication", func(t *testing.T) {
		root := canonicalTempDir(t)
		sourceSessions := filepath.Join(root, "source", "sessions")
		sourceBlobs := filepath.Join(root, "source", "blobs")
		targetSessions := filepath.Join(root, "state", "sessions", "example")
		targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
		if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"a.jsonl", "b.jsonl"} {
			if err := os.WriteFile(filepath.Join(sourceSessions, name), []byte(name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		relocator := newRelocatorForTest(
			t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
		)
		var writerLock *pinnedFileLock
		relocator.afterPublish = func() {
			relocator.afterPublish = nil
			targetRoot, err := os.OpenRoot(targetSessions)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = targetRoot.Close() }()
			for _, name := range []string{"a.jsonl", "b.jsonl"} {
				if _, err := targetRoot.Lstat(name); err == nil {
					writerLock, err = lockRootedSession(targetRoot, name)
					if err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			cancel()
		}
		err := relocator.Relocate(ctx, nil, nil)
		if writerLock == nil {
			t.Fatal("writer lock was not acquired")
		}
		if err == nil || !strings.Contains(err.Error(), "active writer") {
			t.Fatalf("active-writer rollback error = %v", err)
		}
		if err := writerLock.Unlock(); err != nil {
			t.Fatal(err)
		}
		published := 0
		for _, name := range []string{"a.jsonl", "b.jsonl"} {
			if _, err := os.Stat(filepath.Join(targetSessions, name)); err == nil {
				published++
			}
		}
		if published != 1 {
			t.Fatalf("active writer publication count = %d", published)
		}
		if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); err != nil {
			t.Fatalf("active writer rollback lost manifest: %v", err)
		}
	})

	t.Run("lock held through final unlink", func(t *testing.T) {
		root := canonicalTempDir(t)
		sourceSessions := filepath.Join(root, "source", "sessions")
		sourceBlobs := filepath.Join(root, "source", "blobs")
		targetSessions := filepath.Join(root, "state", "sessions", "example")
		targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
		if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"a.jsonl", "b.jsonl"} {
			if err := os.WriteFile(filepath.Join(sourceSessions, name), []byte(name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		relocator := newRelocatorForTest(
			t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
		)
		relocator.afterPublish = func() {
			relocator.afterPublish = nil
			cancel()
		}
		checked := false
		relocator.beforeTargetRollbackFinalUnlink = func() {
			relocator.beforeTargetRollbackFinalUnlink = nil
			for _, name := range []string{"a.jsonl", "b.jsonl"} {
				file, err := relocator.targets.sessions.OpenFile(name+".lock", os.O_CREATE|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatal(err)
				}
				lock, err := tryPinnedFileLock(file)
				if err != nil {
					t.Fatal(err)
				}
				if lock.locked {
					_ = lock.Unlock()
					continue
				}
				checked = true
				if err := lock.Unlock(); err != nil {
					t.Fatal(err)
				}
				break
			}
			if !checked {
				t.Fatal("rollback did not hold a session writer lock at final unlink")
			}
		}
		if err := relocator.Relocate(ctx, nil, nil); err == nil {
			t.Fatal("cancelled relocation succeeded")
		}
		if !checked {
			t.Fatal("final unlink lock hook did not run")
		}
	})
}

func TestPublishSyncFailurePreservesRacedReplacementByIdentity(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	source := filepath.Join(sourceSessions, "legacy.jsonl")
	target := filepath.Join(targetSessions, "legacy.jsonl")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	const foreign = `{"event":"started"}`
	var replacementInfo fs.FileInfo
	relocator.syncPublishedTargetDirectory = func(*os.Root, string) error {
		relocator.syncPublishedTargetDirectory = nil
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(foreign), 0o600); err != nil {
			t.Fatal(err)
		}
		info, statErr := os.Lstat(target)
		if statErr != nil {
			t.Fatal(statErr)
		}
		replacementInfo = info
		return errors.New("injected publication directory sync failure")
	}
	err := relocator.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "directory sync failure") {
		t.Fatalf("publication sync failure = %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != foreign {
		t.Fatalf("publication cleanup removed raced replacement = %q, %v", got, err)
	}
	if current, err := os.Lstat(target); err != nil || !os.SameFile(replacementInfo, current) {
		t.Fatalf("publication cleanup changed same-content foreign identity: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); err != nil {
		t.Fatalf("publication cleanup lost pending manifest: %v", err)
	}
}

func TestTargetRollbackFinalUnlinkPreservesSameContentForeignInode(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(sourceSessions, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	relocator := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	relocator.afterPublish = func() {
		relocator.afterPublish = nil
		cancel()
	}
	var replacementInfo fs.FileInfo
	relocator.beforeTargetRollbackFinalUnlink = func() {
		relocator.beforeTargetRollbackFinalUnlink = nil
		controls, err := filepath.Glob(filepath.Join(
			targetSessions, SessionRelocationQuarantineDir, "*.target-rollback",
		))
		if err != nil || len(controls) != 1 {
			t.Fatalf("rollback quarantine controls = %v, %v", controls, err)
		}
		content, err := os.ReadFile(controls[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(controls[0]); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(controls[0], content, 0o600); err != nil {
			t.Fatal(err)
		}
		replacementInfo, err = os.Lstat(controls[0])
		if err != nil {
			t.Fatal(err)
		}
	}
	err := relocator.Relocate(ctx, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("same-content final-unlink race error = %v", err)
	}
	var preserved string
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		path := filepath.Join(targetSessions, name)
		info, statErr := os.Lstat(path)
		if statErr == nil && os.SameFile(replacementInfo, info) {
			preserved = path
		}
	}
	if preserved == "" {
		t.Fatal("same-content foreign rollback inode was not preserved visibly")
	}
	if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); err != nil {
		t.Fatalf("same-content final-unlink race lost manifest: %v", err)
	}
}

func TestTargetRollbackDualNameReconcilePreservesSameContentForeignInode(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(sourceSessions, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	crashing := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	crashing.afterPublish = func() {
		crashing.afterPublish = nil
		cancel()
	}
	var visible string
	crashing.afterTargetRollbackQuarantine = func(quarantine string) {
		crashing.afterTargetRollbackQuarantine = nil
		quarantinePath := filepath.Join(targetSessions, quarantine)
		content, err := os.ReadFile(quarantinePath)
		if err != nil {
			t.Fatal(err)
		}
		visible = filepath.Join(targetSessions, string(content))
		if err := os.Link(quarantinePath, visible); err != nil {
			t.Fatal(err)
		}
		panic("crash after creating dual rollback names")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected dual-name rollback crash")
			}
		}()
		_ = crashing.Relocate(ctx, nil, nil)
	}()

	resuming := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	var replacementInfo fs.FileInfo
	resuming.beforeTargetRollbackFinalUnlink = func() {
		resuming.beforeTargetRollbackFinalUnlink = nil
		content, err := os.ReadFile(visible)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(visible); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(visible, content, 0o600); err != nil {
			t.Fatal(err)
		}
		replacementInfo, err = os.Lstat(visible)
		if err != nil {
			t.Fatal(err)
		}
	}
	err := resuming.Relocate(t.Context(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("same-content dual-name reconcile error = %v", err)
	}
	if current, err := os.Lstat(visible); err != nil || !os.SameFile(replacementInfo, current) {
		t.Fatalf("dual-name reconcile removed same-content foreign target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, SessionRelocationManifest)); err != nil {
		t.Fatalf("same-content dual-name reconcile lost manifest: %v", err)
	}
}

func TestInitResumesCutoverManifestBeforeDesiredLegacyMigration(t *testing.T) {
	root := canonicalTempDir(t)
	stateRoot := filepath.Join(root, "state")
	oldSessions := filepath.Join(stateRoot, "sessions", "local", "0123456789ab")
	oldBlobs := filepath.Join(stateRoot, "staged-blobs", "local", "0123456789ab")
	desiredSessions := filepath.Join(stateRoot, "sessions", "github.com", "org", "repo")
	desiredBlobs := filepath.Join(stateRoot, "staged-blobs", "github.com", "org", "repo")
	legacyPath := writeLegacyMigrationSession(t, oldSessions)
	source := SessionStoreRelocationSource{
		Kind:     SessionStoreRelocationSourceOldGlobal,
		Sessions: oldSessions, StagedBlobs: oldBlobs, WriteTombstone: true,
	}
	transition := &SessionIdentityTransition{
		Version: SessionIdentityTransitionVersion, State: SessionIdentityTransitionPending,
		OldKey: "local/0123456789ab", NewKey: "github.com/org/repo",
		OldSessions: oldSessions, OldBlobs: oldBlobs,
		CurrentSessions: oldSessions, CurrentBlobs: oldBlobs,
		TargetProject: "github.com/org/repo",
	}
	options := FilesystemSessionStoreRelocatorOptions{
		Sources:                   []SessionStoreRelocationSource{source},
		AuthorizedOldGlobalSource: &source,
		TrustedStateRoot:          stateRoot, StableRepoAuthority: filepath.Join(root, "repo.git"),
		TargetSessions: desiredSessions, TargetBlobs: desiredBlobs,
		TargetProject: "github.com/org/repo",
		Transformer:   app.CurrentSessionIdentityTransformer{}, Transition: transition,
	}
	crashing, err := NewFilesystemSessionStoreRelocator(options)
	if err != nil {
		t.Fatal(err)
	}
	crashing.beforeIrreversible = func(stage string) {
		if stage == "source_delete" {
			panic("crash after cutover")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected cutover crash")
			}
		}()
		_ = crashing.Relocate(t.Context(), nil, nil)
	}()
	desiredPath := filepath.Join(desiredSessions, "legacy.jsonl")
	if format, err := classifySessionFile(desiredPath); err != nil || format != sessionFormatLegacy {
		t.Fatalf("cutover crash desired payload = %v, %v", format, err)
	}
	if _, err := os.Stat(filepath.Join(desiredSessions, SessionRelocationManifest)); err != nil {
		t.Fatalf("cutover crash lacks manifest: %v", err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("cutover crash deleted source before retry: %v", err)
	}

	resuming, err := NewFilesystemSessionStoreRelocator(options)
	if err != nil {
		t.Fatal(err)
	}
	desiredMigrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, desiredSessions, desiredBlobs, "local", "github.com/org/repo",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = desiredMigrator.Close() }()
	repoRoot := filepath.Join(root, "repo")
	handler := handlers.New(handlers.Options{
		Reader:                          finders.New(finders.Options{}),
		PreRelocationRecoveryConfigured: true,
		RelocatedSessions:               desiredMigrator,
		Relocator:                       resuming,
	})
	if err := handler.Init(t.Context(), &command.InitCmd{
		RepoRoot: repoRoot, BinaryVersion: "v0.2.0",
		Targets: []model.AgentTarget{model.AgentClaude}, Scope: model.ScopeProject,
		MigrateLegacySessions: true, RelocateSessionStore: true,
	}); err != nil {
		t.Fatalf("handler cutover retry: %v", err)
	}
	if format, err := classifySessionFile(desiredPath); err != nil || format != sessionFormatCurrent {
		t.Fatalf("post-relocation desired migration = %v, %v", format, err)
	}
	if _, err := os.Stat(filepath.Join(desiredSessions, SessionRelocationManifest)); !os.IsNotExist(err) {
		t.Fatalf("cutover retry retained manifest: %v", err)
	}
}

func writeLegacyMigrationSession(t *testing.T, sessionsDir string) string {
	t.Helper()
	const id = "legacy"
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	event := engine.Event{
		V: 1, TS: time.Now().UTC(), Session: id, Seq: 1,
		Event: engine.EventSessionMeta, Data: map[string]any{"participant": "Test"},
	}
	path := filepath.Join(sessionsDir, id+".jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(event); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRootedRuntimeStoresRejectStaticAndRacedCategorySymlinks(t *testing.T) {
	t.Run("static session category", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		outside := filepath.Join(root, "outside")
		if err := os.MkdirAll(stateRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(stateRoot, "sessions")); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		store, err := NewFilesystemSessionStoreAtStateRoot(
			stateRoot, filepath.Join(stateRoot, "sessions", "repo"),
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Create(t.Context(), app.SessionMetadata{
			CodecVersion: app.SessionCodecVersion, ID: "session", Subject: "local", Project: "example",
		})
		if err == nil {
			t.Fatal("rooted runtime accepted static session category symlink")
		}
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("static runtime symlink mutated outside: %v, %v", entries, err)
		}
	})
	t.Run("raced blob category", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
		store, err := NewFilesystemStagedBlobStoreAtStateRoot(stateRoot, blobsDir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Stage(
			t.Context(), app.BlobOwner{Subject: "local", Session: "seed"},
			"seed.txt", strings.NewReader("seed"),
		); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(root, "outside")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		pinned := filepath.Join(stateRoot, "staged-blobs-pinned")
		store.beforeRootedMutation = func() {
			if err := os.Rename(filepath.Join(stateRoot, "staged-blobs"), pinned); err != nil {
				t.Skipf("platform cannot rename opened runtime category: %v", err)
			}
			if err := os.Symlink(outside, filepath.Join(stateRoot, "staged-blobs")); err != nil {
				t.Fatal(err)
			}
		}
		_, err = store.Stage(
			t.Context(), app.BlobOwner{Subject: "local", Session: "raced"},
			"raced.txt", strings.NewReader("raced"),
		)
		if err == nil || !strings.Contains(err.Error(), "rebound") {
			t.Fatalf("raced runtime category error = %v", err)
		}
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("raced runtime symlink mutated outside: %v, %v", entries, err)
		}
	})
}

func TestTrustedStateRootNameRejectsSymlinkAndRebind(t *testing.T) {
	t.Run("static runtime symlink", func(t *testing.T) {
		root := canonicalTempDir(t)
		outside := filepath.Join(root, "outside")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		stateRoot := filepath.Join(root, "state")
		if err := os.Symlink(outside, stateRoot); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		store, err := NewFilesystemSessionStoreAtStateRoot(
			stateRoot, filepath.Join(stateRoot, "sessions", "repo"),
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Create(t.Context(), app.SessionMetadata{
			CodecVersion: app.SessionCodecVersion, ID: "session", Subject: "local", Project: "example",
		})
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("static state-root symlink error = %v", err)
		}
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("static state-root symlink mutated outside: %v, %v", entries, err)
		}
	})
	t.Run("runtime rename recreate", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		store, err := NewFilesystemStagedBlobStoreAtStateRoot(
			stateRoot, filepath.Join(stateRoot, "staged-blobs", "repo"),
		)
		if err != nil {
			t.Fatal(err)
		}
		pinned := stateRoot + "-pinned"
		store.beforeRootedMutation = func() {
			if err := os.Rename(stateRoot, pinned); err != nil {
				t.Skipf("platform cannot rename opened state root: %v", err)
			}
			if err := os.MkdirAll(stateRoot, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		_, err = store.Stage(
			t.Context(), app.BlobOwner{Subject: "local", Session: "raced"},
			"raced.txt", strings.NewReader("raced"),
		)
		if err == nil || !strings.Contains(err.Error(), "state root was rebound") {
			t.Fatalf("runtime state-root rebind error = %v", err)
		}
	})
	t.Run("legacy rename recreate", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		sessionsDir := filepath.Join(stateRoot, "sessions", "repo")
		blobsDir := filepath.Join(stateRoot, "staged-blobs", "repo")
		legacyPath := writeLegacyMigrationSession(t, sessionsDir)
		migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
			stateRoot, sessionsDir, blobsDir, "local", "example",
		)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = migrator.Close() }()
		pinned := stateRoot + "-pinned"
		migrator.beforeMutation = func() {
			if err := os.Rename(stateRoot, pinned); err != nil {
				t.Skipf("platform cannot rename opened state root: %v", err)
			}
			if err := os.MkdirAll(stateRoot, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		err = migrator.MigrateLegacySession(t.Context(), legacyPath)
		if err == nil || !strings.Contains(err.Error(), "state root was rebound") {
			t.Fatalf("legacy state-root rebind error = %v", err)
		}
		if format, err := classifySessionFile(filepath.Join(pinned, "sessions", "repo", "legacy.jsonl")); err != nil ||
			format != sessionFormatLegacy {
			t.Fatalf("legacy state-root rebind mutated source = %v, %v", format, err)
		}
	})
	t.Run("relocation rename recreate", func(t *testing.T) {
		root := canonicalTempDir(t)
		sourceSessions := filepath.Join(root, "source", "sessions")
		sourceBlobs := filepath.Join(root, "source", "blobs")
		stateRoot := filepath.Join(root, "state")
		targetSessions := filepath.Join(stateRoot, "sessions", "example")
		targetBlobs := filepath.Join(stateRoot, "staged-blobs", "example")
		source := filepath.Join(sourceSessions, "legacy.jsonl")
		if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
		pinned := stateRoot + "-pinned"
		relocator.afterTempSync = func() {
			relocator.afterTempSync = nil
			if err := os.Rename(stateRoot, pinned); err != nil {
				t.Skipf("platform cannot rename opened state root: %v", err)
			}
			if err := os.MkdirAll(stateRoot, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		err := relocator.Relocate(t.Context(), nil, nil)
		if err == nil || !strings.Contains(err.Error(), "state root was rebound") {
			t.Fatalf("relocation state-root rebind error = %v", err)
		}
		if got, err := os.ReadFile(source); err != nil || string(got) != `{"event":"started"}` {
			t.Fatalf("relocation state-root rebind changed source = %q, %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(pinned, "sessions", "example", "legacy.jsonl")); !os.IsNotExist(err) {
			t.Fatalf("relocation state-root rebind published orphan payload: %v", err)
		}
	})
}

func TestTrustedDirectoryChainsRejectAncestorAndStoreKeyRebinds(t *testing.T) {
	t.Run("static state ancestor symlink", func(t *testing.T) {
		root := canonicalTempDir(t)
		outside := filepath.Join(root, "outside")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(root, "state-parent")
		if err := os.Symlink(outside, alias); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		stateRoot := filepath.Join(alias, "nested", "state")
		store, err := NewFilesystemSessionStoreAtStateRoot(
			stateRoot, filepath.Join(stateRoot, "sessions", "github.com", "org", "repo"),
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Create(t.Context(), app.SessionMetadata{
			CodecVersion: app.SessionCodecVersion, ID: "session", Subject: "local", Project: "example",
		})
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("static state ancestor symlink error = %v", err)
		}
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("static state ancestor symlink mutated outside = %v, %v", entries, err)
		}
	})
	t.Run("state parent rename recreate", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateParent := filepath.Join(root, "authority-parent")
		stateRoot := filepath.Join(stateParent, "state")
		store, err := NewFilesystemStagedBlobStoreAtStateRoot(
			stateRoot, filepath.Join(stateRoot, "staged-blobs", "github.com", "org", "repo"),
		)
		if err != nil {
			t.Fatal(err)
		}
		pinned := stateParent + "-pinned"
		store.beforeRootedMutation = func() {
			if err := os.Rename(stateParent, pinned); err != nil {
				t.Skipf("platform cannot rename opened state parent: %v", err)
			}
			if err := os.MkdirAll(stateRoot, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		_, err = store.Stage(
			t.Context(), app.BlobOwner{Subject: "local", Session: "raced"},
			"raced.txt", strings.NewReader("raced"),
		)
		if err == nil || !strings.Contains(err.Error(), "state root was rebound") {
			t.Fatalf("state parent rebind error = %v", err)
		}
	})
	t.Run("static store key symlink", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		category := filepath.Join(stateRoot, "sessions")
		outside := filepath.Join(root, "outside")
		if err := os.MkdirAll(category, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(category, "github.com")); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		store, err := NewFilesystemSessionStoreAtStateRoot(
			stateRoot, filepath.Join(category, "github.com", "org", "repo"),
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Create(t.Context(), app.SessionMetadata{
			CodecVersion: app.SessionCodecVersion, ID: "session", Subject: "local", Project: "example",
		})
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("static store-key symlink error = %v", err)
		}
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("static store-key symlink mutated outside = %v, %v", entries, err)
		}
	})
	t.Run("runtime store key rename recreate", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		storeDir := filepath.Join(stateRoot, "staged-blobs", "github.com", "org", "repo")
		store, err := NewFilesystemStagedBlobStoreAtStateRoot(stateRoot, storeDir)
		if err != nil {
			t.Fatal(err)
		}
		keyParent := filepath.Join(stateRoot, "staged-blobs", "github.com")
		pinned := keyParent + "-pinned"
		store.beforeRootedMutation = func() {
			if err := os.Rename(keyParent, pinned); err != nil {
				t.Skipf("platform cannot rename opened store-key component: %v", err)
			}
			if err := os.MkdirAll(storeDir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		_, err = store.Stage(
			t.Context(), app.BlobOwner{Subject: "local", Session: "raced"},
			"raced.txt", strings.NewReader("raced"),
		)
		if err == nil || !strings.Contains(err.Error(), "component was rebound") {
			t.Fatalf("runtime store-key rebind error = %v", err)
		}
	})
	t.Run("relocation store key rename recreate", func(t *testing.T) {
		root := canonicalTempDir(t)
		sourceSessions := filepath.Join(root, "source", "sessions")
		sourceBlobs := filepath.Join(root, "source", "blobs")
		stateRoot := filepath.Join(root, "state")
		targetSessions := filepath.Join(stateRoot, "sessions", "github.com", "org", "repo")
		targetBlobs := filepath.Join(stateRoot, "staged-blobs", "github.com", "org", "repo")
		source := filepath.Join(sourceSessions, "legacy.jsonl")
		if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
		keyParent := filepath.Join(stateRoot, "sessions", "github.com")
		pinned := keyParent + "-pinned"
		relocator.afterTempSync = func() {
			relocator.afterTempSync = nil
			if err := os.Rename(keyParent, pinned); err != nil {
				t.Skipf("platform cannot rename opened target-key component: %v", err)
			}
			if err := os.MkdirAll(targetSessions, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		err := relocator.Relocate(t.Context(), nil, nil)
		if err == nil || !strings.Contains(err.Error(), "relocation target was rebound") {
			t.Fatalf("relocation store-key rebind error = %v", err)
		}
		if got, err := os.ReadFile(source); err != nil || string(got) != `{"event":"started"}` {
			t.Fatalf("relocation store-key rebind changed source = %q, %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(pinned, "org", "repo", "legacy.jsonl")); !os.IsNotExist(err) {
			t.Fatalf("relocation store-key rebind published orphan payload: %v", err)
		}
	})
}

func TestOldGlobalSourcesStayRootedUnderTrustedStateAuthority(t *testing.T) {
	t.Run("intermediate symlink", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		outsideSessions := filepath.Join(root, "outside-sessions")
		outsideBlobs := filepath.Join(root, "outside-blobs")
		for _, path := range []string{
			filepath.Join(stateRoot, "sessions"),
			filepath.Join(stateRoot, "staged-blobs"),
			filepath.Join(outsideSessions, "0123456789ab"),
			filepath.Join(outsideBlobs, "0123456789ab"),
		} {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		outsidePayload := filepath.Join(outsideSessions, "0123456789ab", "legacy.jsonl")
		if err := os.WriteFile(outsidePayload, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideSessions, filepath.Join(stateRoot, "sessions", "local")); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if err := os.Symlink(outsideBlobs, filepath.Join(stateRoot, "staged-blobs", "local")); err != nil {
			t.Fatal(err)
		}
		authorized := SessionStoreRelocationSource{
			Kind:           SessionStoreRelocationSourceOldGlobal,
			Sessions:       filepath.Join(stateRoot, "sessions", "local", "0123456789ab"),
			StagedBlobs:    filepath.Join(stateRoot, "staged-blobs", "local", "0123456789ab"),
			WriteTombstone: true,
		}
		relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
			Sources:                   []SessionStoreRelocationSource{authorized},
			AuthorizedOldGlobalSource: &authorized,
			TrustedStateRoot:          stateRoot, StableRepoAuthority: filepath.Join(root, "repo.git"),
			TargetSessions: filepath.Join(stateRoot, "sessions", "example"),
			TargetBlobs:    filepath.Join(stateRoot, "staged-blobs", "example"),
			TargetProject:  "example", Transformer: app.CurrentSessionIdentityTransformer{},
		})
		if err != nil {
			t.Fatal(err)
		}
		err = relocator.Relocate(t.Context(), nil, nil)
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("old-global intermediate symlink error = %v", err)
		}
		if got, err := os.ReadFile(outsidePayload); err != nil || string(got) != "outside" {
			t.Fatalf("old-global symlink mutated outside = %q, %v", got, err)
		}
	})
	t.Run("intermediate rebind", func(t *testing.T) {
		root := canonicalTempDir(t)
		stateRoot := filepath.Join(root, "state")
		localSessions := filepath.Join(stateRoot, "sessions", "local")
		localBlobs := filepath.Join(stateRoot, "staged-blobs", "local")
		sourceSessions := filepath.Join(localSessions, "0123456789ab")
		sourceBlobs := filepath.Join(localBlobs, "0123456789ab")
		targetSessions := filepath.Join(stateRoot, "sessions", "example")
		targetBlobs := filepath.Join(stateRoot, "staged-blobs", "example")
		if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(sourceBlobs, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceSessions, "legacy.jsonl"), []byte("source"), 0o600); err != nil {
			t.Fatal(err)
		}
		authorized := SessionStoreRelocationSource{
			Kind:     SessionStoreRelocationSourceOldGlobal,
			Sessions: sourceSessions, StagedBlobs: sourceBlobs, WriteTombstone: true,
		}
		relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
			Sources:                   []SessionStoreRelocationSource{authorized},
			AuthorizedOldGlobalSource: &authorized,
			TrustedStateRoot:          stateRoot, StableRepoAuthority: filepath.Join(root, "repo.git"),
			TargetSessions: targetSessions, TargetBlobs: targetBlobs,
			TargetProject: "example", Transformer: app.CurrentSessionIdentityTransformer{},
		})
		if err != nil {
			t.Fatal(err)
		}
		pinnedLocal := localSessions + "-pinned"
		outside := filepath.Join(root, "outside")
		relocator.afterTempSync = func() {
			relocator.afterTempSync = nil
			if err := os.Rename(localSessions, pinnedLocal); err != nil {
				t.Skipf("platform cannot rename old-global component: %v", err)
			}
			if err := os.MkdirAll(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, localSessions); err != nil {
				t.Fatal(err)
			}
		}
		err = relocator.Relocate(t.Context(), nil, nil)
		if err == nil || !strings.Contains(err.Error(), "source component was rebound") {
			t.Fatalf("old-global component rebind error = %v", err)
		}
		if got, err := os.ReadFile(filepath.Join(pinnedLocal, "0123456789ab", "legacy.jsonl")); err != nil ||
			string(got) != "source" {
			t.Fatalf("old-global component rebind changed source = %q, %v", got, err)
		}
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("old-global component rebind mutated outside = %v, %v", entries, err)
		}
		if _, err := os.Stat(filepath.Join(targetSessions, "legacy.jsonl")); !os.IsNotExist(err) {
			t.Fatalf("old-global component rebind published target: %v", err)
		}
	})
}

func TestRootedRegularOpenRejectsFIFOWithoutBlocking(t *testing.T) {
	root := canonicalTempDir(t)
	sessionsDir := filepath.Join(root, "state", "sessions", "repo")
	blobsDir := filepath.Join(root, "state", "staged-blobs", "repo")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(sessionsDir, "fifo.jsonl")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	migrator, err := NewFilesystemLegacySessionMigratorAtStateRoot(
		filepath.Join(root, "state"), sessionsDir, blobsDir, "local", "example",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	if _, err := migrator.ListLegacySessions(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "regular") {
		t.Fatalf("FIFO legacy listing error = %v", err)
	}
}

func TestTransitionAndTombstoneReadersRejectRegularReplacementRace(t *testing.T) {
	for _, control := range []string{"transition", "tombstone"} {
		for _, replacement := range []string{"fifo", "symlink"} {
			for _, rooted := range []bool{false, true} {
				name := control + " " + replacement
				if rooted {
					name += " rooted"
				} else {
					name += " composition"
				}
				t.Run(name, func(t *testing.T) {
					root := canonicalTempDir(t)
					sessionsDir := filepath.Join(root, "sessions")
					if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
						t.Fatal(err)
					}
					controlName := SessionIdentityTransitionMarker
					if control == "transition" {
						transition := SessionIdentityTransition{
							Version: SessionIdentityTransitionVersion,
							State:   SessionIdentityTransitionPending,
							OldKey:  "old", NewKey: "new",
							OldSessions: sessionsDir, OldBlobs: filepath.Join(root, "old-blobs"),
							CurrentSessions: sessionsDir, CurrentBlobs: filepath.Join(root, "old-blobs"),
							TargetProject: "example", UpdatedAt: time.Now().UTC(),
						}
						if err := WriteSessionIdentityTransition(sessionsDir, transition); err != nil {
							t.Fatal(err)
						}
					} else {
						controlName = SessionRelocationTombstone
						encoded, err := json.Marshal(SessionRelocationTombstoneRecord{
							Version: 2, TargetProject: "example",
							TargetSessions:    filepath.Join(root, "target-sessions"),
							TargetStagedBlobs: filepath.Join(root, "target-blobs"),
							RelocatedAt:       time.Now().UTC(),
						})
						if err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(sessionsDir, controlName), encoded, 0o600); err != nil {
							t.Fatal(err)
						}
					}
					controlPath := filepath.Join(sessionsDir, controlName)
					outside := filepath.Join(root, "outside.json")
					if err := os.WriteFile(outside, []byte(`{"version":999}`), 0o600); err != nil {
						t.Fatal(err)
					}
					triggered := false
					restoreHook := setRootedRegularOpenHookForTest(func(name string) {
						if triggered || name != controlName {
							return
						}
						triggered = true
						if err := os.Remove(controlPath); err != nil {
							t.Fatal(err)
						}
						if replacement == "fifo" {
							if err := syscall.Mkfifo(controlPath, 0o600); err != nil {
								t.Skipf("FIFO unavailable: %v", err)
							}
						} else if err := os.Symlink(outside, controlPath); err != nil {
							t.Skipf("symbolic links unavailable: %v", err)
						}
					})
					defer restoreHook()
					var readErr error
					if rooted {
						sessionRoot, err := os.OpenRoot(sessionsDir)
						if err != nil {
							t.Fatal(err)
						}
						defer func() { _ = sessionRoot.Close() }()
						if control == "transition" {
							_, readErr = readSessionIdentityTransitionRoot(sessionRoot, sessionsDir)
						} else {
							_, readErr = readSessionRelocationTombstoneRoot(sessionRoot, sessionsDir)
						}
					} else if control == "transition" {
						_, readErr = ReadSessionIdentityTransition(sessionsDir)
					} else {
						_, readErr = ReadSessionRelocationTombstone(sessionsDir)
					}
					if readErr == nil {
						t.Fatalf("%s reader accepted regular-to-%s replacement", control, replacement)
					}
					if !triggered {
						t.Fatalf("%s reader did not exercise replacement hook", control)
					}
					if got, err := os.ReadFile(outside); err != nil || string(got) != `{"version":999}` {
						t.Fatalf("%s reader changed symlink target = %q, %v", control, got, err)
					}
				})
			}
		}
	}
}

func TestRelocationManifestReaderRejectsRegularReplacementRace(t *testing.T) {
	for _, replacement := range []string{"fifo", "symlink"} {
		t.Run(replacement, func(t *testing.T) {
			root := canonicalTempDir(t)
			sourceSessions := filepath.Join(root, "source", "sessions")
			sourceBlobs := filepath.Join(root, "source", "blobs")
			targetSessions := filepath.Join(root, "state", "sessions", "example")
			targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
			if err := os.MkdirAll(sourceSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(sourceSessions, "legacy.jsonl")
			if err := os.WriteFile(source, []byte(`{"event":"started"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			crashing := newRelocatorForTest(
				t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
			)
			crashing.afterPublish = func() { panic("crash with manifest") }
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("expected relocation crash")
					}
				}()
				_ = crashing.Relocate(t.Context(), nil, nil)
			}()
			manifestPath := filepath.Join(targetSessions, SessionRelocationManifest)
			outside := filepath.Join(root, "outside.json")
			if err := os.WriteFile(outside, []byte(`{"version":999}`), 0o600); err != nil {
				t.Fatal(err)
			}
			triggered := false
			restoreHook := setRootedRegularOpenHookForTest(func(name string) {
				if triggered || name != SessionRelocationManifest {
					return
				}
				triggered = true
				if err := os.Remove(manifestPath); err != nil {
					t.Fatal(err)
				}
				if replacement == "fifo" {
					if err := syscall.Mkfifo(manifestPath, 0o600); err != nil {
						t.Skipf("FIFO unavailable: %v", err)
					}
				} else if err := os.Symlink(outside, manifestPath); err != nil {
					t.Skipf("symbolic links unavailable: %v", err)
				}
			})
			defer restoreHook()
			resuming := newRelocatorForTest(
				t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
			)
			if err := resuming.Relocate(t.Context(), nil, nil); err == nil {
				t.Fatalf("manifest reader accepted regular-to-%s replacement", replacement)
			}
			if !triggered {
				t.Fatal("manifest reader did not exercise replacement hook")
			}
			if got, err := os.ReadFile(outside); err != nil || string(got) != `{"version":999}` {
				t.Fatalf("manifest race changed symlink target = %q, %v", got, err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("manifest race changed source: %v", err)
			}
		})
	}
}

func TestFilesystemSessionStoreRelocatorExcludesLegacyMigrationControls(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	if err := os.MkdirAll(
		filepath.Join(sourceSessions, legacyMigrationControlDir), 0o700,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sourceSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	const proof = "orphan prepared proof"
	proofPath := filepath.Join(sourceSessions, legacyMigrationControlDir, "legacy.prepared")
	if err := os.WriteFile(proofPath, []byte(proof), 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := SessionStoreMaterial(sourceSessions, sourceBlobs)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(material, proofPath) {
		t.Fatalf("legacy migration proof was classified as material: %v", material)
	}
	relocator := newRelocatorForTest(
		t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example",
	)
	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(proofPath); err != nil || string(got) != proof {
		t.Fatalf("excluded legacy migration proof changed = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(
		targetSessions, legacyMigrationControlDir, "legacy.prepared",
	)); !os.IsNotExist(err) {
		t.Fatalf("legacy migration proof was relocated: %v", err)
	}
}

func TestFilesystemSessionStoreRelocatorRewritesLocalProjectIdentity(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "repo", ".sdd", "sessions")
	sourceBlobs := filepath.Join(root, "repo", ".sdd", "staged-blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "github.com", "org", "repo")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "github.com", "org", "repo")
	store, err := NewFilesystemSessionStore(sourceSessions)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(context.Background(), app.SessionMetadata{
		CodecVersion: app.SessionCodecVersion, ID: "session", Subject: "local", Project: "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := created.Metadata
	metadata.Label = "persisted"
	intent, err := json.Marshal(map[string]any{"prepared": app.PreparedTransition{
		Version: app.PreparedTransitionVersion,
		Target:  app.MutationTarget{Project: "local", Branch: "main"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	untyped, err := json.Marshal(map[string]any{"project": "local"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), "session", created.Version, app.SessionAppend{
		Metadata: &metadata,
		Events: []app.StoredEvent{
			{CodecVersion: app.SessionCodecVersion, Code: "mutation_intent", Payload: intent},
			{CodecVersion: app.SessionCodecVersion, Code: "untyped_user_payload", Payload: untyped},
		},
	}); err != nil {
		t.Fatal(err)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "github.com/org/repo")
	if err := relocator.Relocate(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	targetStore, err := NewFilesystemSessionStore(targetSessions)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := targetStore.Load(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.Project != "github.com/org/repo" || stored.Metadata.Label != "persisted" {
		t.Fatalf("rewritten metadata = %+v", stored.Metadata)
	}
	if !bytes.Contains(stored.Events[0].Payload, []byte(`"project":"github.com/org/repo"`)) {
		t.Fatalf("mutation target project was not rewritten: %s", stored.Events[0].Payload)
	}
	if !bytes.Equal(stored.Events[1].Payload, untyped) {
		t.Fatalf("untyped event payload was rewritten: %s", stored.Events[1].Payload)
	}
}

func TestFilesystemSessionStoreRelocatorCollisionLeavesSessionsStagingAndBlobs(t *testing.T) {
	root := canonicalTempDir(t)
	sourceSessions := filepath.Join(root, "source", "sessions")
	sourceBlobs := filepath.Join(root, "source", "blobs")
	targetSessions := filepath.Join(root, "state", "sessions", "example")
	targetBlobs := filepath.Join(root, "state", "staged-blobs", "example")
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(sourceSessions, "s.jsonl"),
		filepath.Join(sourceSessions, "s-staging", "evidence.md"),
		filepath.Join(sourceBlobs, "owner", "blob.blob"),
		filepath.Join(targetSessions, "s.jsonl"),
		filepath.Join(targetBlobs, "owner", "blob.blob"),
	} {
		write(path)
	}
	relocator := newRelocatorForTest(t, sourceSessions, sourceBlobs, targetSessions, targetBlobs, "example")
	var collisions []string
	err := relocator.Relocate(context.Background(), nil, func(source, _ string) {
		collisions = append(collisions, source)
	})
	if err == nil || len(collisions) != 2 {
		t.Fatalf("collision result = %v, %v", collisions, err)
	}
	for _, path := range []string{
		filepath.Join(sourceSessions, "s.jsonl"),
		filepath.Join(sourceSessions, "s-staging", "evidence.md"),
		filepath.Join(sourceBlobs, "owner", "blob.blob"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("source aggregate was partially moved: %s: %v", path, err)
		}
	}
	targetSession := filepath.Join(targetSessions, "s.jsonl")
	if got, err := os.ReadFile(targetSession); err != nil || string(got) != targetSession {
		t.Errorf("target session collision was overwritten: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(targetSessions, "s-staging", "evidence.md")); !os.IsNotExist(err) {
		t.Errorf("uncolliding staging payload published despite aggregate collision: %v", err)
	}
}

func newRelocatorForTest(t *testing.T, sourceSessions, sourceBlobs, targetSessions, targetBlobs string, project app.ProjectID) *FilesystemSessionStoreRelocator {
	t.Helper()
	trustedStateRoot, err := inferTrustedStateRoot(targetSessions, targetBlobs)
	if err != nil {
		t.Fatal(err)
	}
	relocator, err := NewFilesystemSessionStoreRelocator(FilesystemSessionStoreRelocatorOptions{
		Sources: []SessionStoreRelocationSource{{
			Kind:     SessionStoreRelocationSourceInTree,
			Sessions: sourceSessions, StagedBlobs: sourceBlobs, WriteTombstone: true,
		}},
		TrustedStateRoot:      trustedStateRoot,
		StableRepoAuthority:   filepath.Join(filepath.Dir(sourceSessions), "repo.git"),
		AuthorizeInTreeSource: func(context.Context, string, string, string) error { return nil },
		TargetSessions:        targetSessions, TargetBlobs: targetBlobs, TargetProject: project,
		Transformer: app.CurrentSessionIdentityTransformer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return relocator
}
