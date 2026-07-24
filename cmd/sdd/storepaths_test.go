package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	app "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	localadapter "github.com/networkteam/sdd/local"
)

func TestResolveLocalStorePathsSharedAcrossWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := canonicalTempDir(t)
	runStoreGit(t, root, "init", "--quiet", "--initial-branch=main")
	runStoreGit(t, root, "config", "user.name", "Test")
	runStoreGit(t, root, "config", "user.email", "test@example.invalid")
	runStoreGit(t, root, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	runStoreGit(t, root, "add", "seed")
	runStoreGit(t, root, "commit", "--quiet", "-m", "seed")
	worktree := filepath.Join(canonicalTempDir(t), "linked")
	runStoreGit(t, root, "worktree", "add", "--quiet", "-b", "linked", worktree)

	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	base, err := resolveLocalStorePaths(filepath.Join(root, model.SDDDirName), &model.PerRepoConfig{}, locations)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := resolveLocalStorePaths(filepath.Join(worktree, model.SDDDirName), &model.PerRepoConfig{}, locations)
	if err != nil {
		t.Fatal(err)
	}
	if base.Sessions != linked.Sessions || base.StagedBlobs != linked.StagedBlobs || base.RepoKey != linked.RepoKey {
		t.Fatalf("worktree store paths differ:\nbase:   %+v\nlinked: %+v", base, linked)
	}
}

func TestPostCutoverRoutingStaysOnDesiredStoreAcrossWorktreesWhenSourcesReappear(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	for _, state := range []localadapter.SessionIdentityTransitionState{
		localadapter.SessionIdentityTransitionCutover,
		localadapter.SessionIdentityTransitionCompleted,
	} {
		t.Run(string(state), func(t *testing.T) {
			root := canonicalTempDir(t)
			runStoreGit(t, root, "init", "--quiet", "--initial-branch=main")
			runStoreGit(t, root, "config", "user.name", "Test")
			runStoreGit(t, root, "config", "user.email", "test@example.invalid")
			runStoreGit(t, root, "config", "commit.gpgsign", "false")
			if err := os.WriteFile(filepath.Join(root, "seed"), []byte("seed"), 0o644); err != nil {
				t.Fatal(err)
			}
			runStoreGit(t, root, "add", "seed")
			runStoreGit(t, root, "commit", "--quiet", "-m", "seed")
			worktree := filepath.Join(canonicalTempDir(t), "linked")
			runStoreGit(t, root, "worktree", "add", "--quiet", "-b", "linked", worktree)

			cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
			locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
			initial, err := resolveLocalStorePaths(filepath.Join(root, model.SDDDirName), cfg, locations)
			if err != nil {
				t.Fatal(err)
			}
			transition := localadapter.SessionIdentityTransition{
				Version: localadapter.SessionIdentityTransitionVersion, State: state,
				OldKey: initial.OldKey, NewKey: initial.DesiredKey,
				OldSessions: initial.OldSessions, OldBlobs: initial.OldBlobs,
				CurrentSessions: initial.DesiredSessions, CurrentBlobs: initial.DesiredBlobs,
				TargetProject: "github.com/org/repo",
			}
			if err := localadapter.WriteSessionIdentityTransition(initial.OldSessions, transition); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{
				filepath.Join(initial.OldSessions, "old-reappeared.jsonl"),
				filepath.Join(root, model.SDDDirName, "sessions", "in-tree-reappeared.jsonl"),
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"event":"reappeared"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, checkout := range []string{root, worktree} {
				paths, err := resolveLocalStorePaths(filepath.Join(checkout, model.SDDDirName), cfg, locations)
				if err != nil {
					t.Fatal(err)
				}
				if paths.PendingIdentity || paths.Sessions != paths.DesiredSessions ||
					paths.StagedBlobs != paths.DesiredBlobs ||
					routedSessionProject(cfg, paths) != "github.com/org/repo" {
					t.Fatalf("post-cutover routing for %s = %+v", checkout, paths)
				}
			}
			notice, err := sessionRelocationNotice(filepath.Join(root, model.SDDDirName), cfg, locations)
			if err != nil {
				t.Fatal(err)
			}
			if notice == "" || !strings.Contains(strings.ToLower(notice), "relocation") {
				t.Fatalf("reappeared-source recovery notice = %q", notice)
			}
		})
	}
}

func TestSessionRelocationNotice(t *testing.T) {
	sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	cfg := &model.PerRepoConfig{}
	if notice, err := sessionRelocationNotice(sddDir, cfg, locations); err != nil || notice != "" {
		t.Fatalf("absent store notice = %q, %v", notice, err)
	}
	sessionsDir := filepath.Join(sddDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "x.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	notice, err := sessionRelocationNotice(sddDir, cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if notice == "" {
		t.Fatal("leftover session must produce a standing notice")
	}

	if err := os.Remove(filepath.Join(sessionsDir, "x.jsonl")); err != nil {
		t.Fatal(err)
	}
	localPaths, err := resolveLocalStorePaths(sddDir, cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	tombstone, err := json.Marshal(map[string]any{
		"version":             2,
		"target_project":      app.ProjectID("local"),
		"target_sessions":     localPaths.DesiredSessions,
		"target_staged_blobs": localPaths.DesiredBlobs,
		"relocated_at":        time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, ".relocated"), tombstone, 0o600); err != nil {
		t.Fatal(err)
	}
	if notice, err := sessionRelocationNotice(sddDir, cfg, locations); err != nil || notice != "" {
		t.Fatalf("tombstone-only notice = %q, %v", notice, err)
	}
	blob := filepath.Join(sddDir, "staged-blobs", "owner", "x.blob")
	if err := os.MkdirAll(filepath.Dir(blob), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blob, []byte("blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	if notice, err := sessionRelocationNotice(sddDir, cfg, locations); err != nil || notice == "" {
		t.Fatalf("blob-only notice = %q, %v", notice, err)
	}
}

func TestCurrentSessionRelocationNoticeReloadsRepoID(t *testing.T) {
	configRoot := filepath.Join(canonicalTempDir(t), "config")
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	root := canonicalTempDir(t)
	sddDir := filepath.Join(root, model.SDDDirName)
	if err := os.MkdirAll(sddDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(sddDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("default_branch: main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	initial, err := resolveLocalStorePaths(sddDir, &model.PerRepoConfig{}, locations)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(initial.OldSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(initial.OldSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if notice, err := currentSessionRelocationNotice(sddDir, locations); err != nil || notice != "" {
		t.Fatalf("identity-less current notice = %q, %v", notice, err)
	}

	if err := os.WriteFile(configPath, []byte("default_branch: main\nrepo_id: github.com/org/repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	notice, err := currentSessionRelocationNotice(sddDir, locations)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notice, "Session identity transition pending") {
		t.Fatalf("reloaded notice = %q", notice)
	}
}

func TestSessionRelocationNoticeRejectsCorruptTombstone(t *testing.T) {
	sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
	sessionsDir := filepath.Join(sddDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, localadapter.SessionRelocationTombstone), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := sessionRelocationNotice(
		sddDir, &model.PerRepoConfig{}, repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")},
	)
	if err == nil || !strings.Contains(err.Error(), "tombstone") {
		t.Fatalf("corrupt tombstone notice error = %v", err)
	}
}

func TestRepoIDRoutingClassifiesInTreeTombstoneTargets(t *testing.T) {
	for _, test := range []struct {
		name        string
		target      func(localStorePaths) (app.ProjectID, string, string)
		wantHold    bool
		wantErr     bool
		needsUpdate bool
	}{
		{
			name: "old local target requires cutover",
			target: func(paths localStorePaths) (app.ProjectID, string, string) {
				return "local", paths.OldSessions, paths.OldBlobs
			},
			wantHold: true, needsUpdate: true,
		},
		{
			name: "desired target is silent",
			target: func(paths localStorePaths) (app.ProjectID, string, string) {
				return "github.com/org/repo", paths.DesiredSessions, paths.DesiredBlobs
			},
		},
		{
			name: "arbitrary prior repository identity fails",
			target: func(paths localStorePaths) (app.ProjectID, string, string) {
				return "github.com/other/repo",
					filepath.Join(filepath.Dir(paths.DesiredSessions), "other"),
					filepath.Join(filepath.Dir(paths.DesiredBlobs), "other")
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
			if err := os.MkdirAll(filepath.Join(sddDir, "sessions"), 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
			locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
			withoutTombstone, err := resolveLocalStorePaths(sddDir, cfg, locations)
			if err != nil {
				t.Fatal(err)
			}
			project, sessions, blobs := test.target(withoutTombstone)
			writeStoreTombstone(t, filepath.Join(sddDir, "sessions"), project, sessions, blobs)

			paths, err := resolveLocalStorePaths(sddDir, cfg, locations)
			if test.wantErr {
				if err == nil {
					t.Fatalf("mismatched tombstone was accepted: %+v", paths)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if paths.PendingIdentity != test.wantHold ||
				paths.InTreeTombstoneNeedsUpdate != test.needsUpdate {
				t.Fatalf("classified paths = %+v", paths)
			}
		})
	}
}

func TestAcknowledgedOldTargetTombstoneCutsOverToDesiredRouting(t *testing.T) {
	sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
	inTreeSessions := filepath.Join(sddDir, "sessions")
	inTreeBlobs := filepath.Join(sddDir, "staged-blobs")
	if err := os.MkdirAll(inTreeSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	initial, err := resolveLocalStorePaths(sddDir, cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	writeStoreTombstone(t, inTreeSessions, "local", initial.OldSessions, initial.OldBlobs)
	pending, err := resolveLocalStorePaths(sddDir, cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if !pending.PendingIdentity || !pending.InTreeTombstoneNeedsUpdate {
		t.Fatalf("old-target tombstone routing = %+v", pending)
	}
	relocator, err := localadapter.NewFilesystemSessionStoreRelocator(
		localadapter.FilesystemSessionStoreRelocatorOptions{
			Sources: []localadapter.SessionStoreRelocationSource{{
				Kind:     localadapter.SessionStoreRelocationSourceInTree,
				Sessions: inTreeSessions, StagedBlobs: inTreeBlobs, WriteTombstone: true,
			}},
			TrustedStateRoot:      locations.StateRoot,
			StableRepoAuthority:   pending.StableRepoAuthority,
			AuthorizeInTreeSource: func(context.Context, string, string, string) error { return nil },
			TargetSessions:        pending.DesiredSessions, TargetBlobs: pending.DesiredBlobs,
			TargetProject: "github.com/org/repo", Transformer: app.CurrentSessionIdentityTransformer{},
			Transition: pending.Transition,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	current, err := resolveLocalStorePaths(sddDir, cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if current.PendingIdentity || current.InTreeTombstoneNeedsUpdate ||
		current.Sessions != current.DesiredSessions {
		t.Fatalf("post-ack tombstone routing = %+v", current)
	}
}

func writeStoreTombstone(
	t *testing.T,
	sessionsDir string,
	project app.ProjectID,
	targetSessions string,
	targetBlobs string,
) {
	t.Helper()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"version": 2, "target_project": project,
		"target_sessions": targetSessions, "target_staged_blobs": targetBlobs,
		"relocated_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, localadapter.SessionRelocationTombstone), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInTreeOnlyMaterialHoldsRepoIDSessionRoutingOnOldGlobalStore(t *testing.T) {
	root := canonicalTempDir(t)
	sddDir := filepath.Join(root, model.SDDDirName)
	session := filepath.Join(sddDir, "sessions", "legacy.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
	paths, err := resolveLocalStorePaths(
		sddDir, cfg, repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !paths.PendingIdentity || paths.RepoKey != paths.OldKey || paths.Sessions != paths.OldSessions {
		t.Fatalf("in-tree-only routing = %+v", paths)
	}
	if paths.Sessions == filepath.Join(sddDir, "sessions") {
		t.Fatal("declined transition routed runtime sessions back into the in-tree source")
	}
	if got := routedSessionProject(cfg, paths); got != "local" {
		t.Fatalf("in-tree-only project = %q", got)
	}
	if got := persistentIndexRepoKey(paths); got != paths.DesiredKey {
		t.Fatalf("in-tree-only persistent index key = %q, want %q", got, paths.DesiredKey)
	}
	sessions, err := localadapter.NewFilesystemSessionStore(paths.Sessions)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Create(t.Context(), app.SessionMetadata{
		CodecVersion: app.SessionCodecVersion, ID: "during-decline",
		Subject: "local", Project: "local",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.OldSessions, "during-decline.jsonl")); err != nil {
		t.Fatalf("declined session was not written to old global store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sddDir, "sessions", "during-decline.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("declined session was written back into in-tree source: %v", err)
	}
}

func TestAcknowledgedInTreeHoldAggregatesInTreeAndOldGlobalPayloads(t *testing.T) {
	sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
	inTreeSessions := filepath.Join(sddDir, "sessions")
	if err := os.MkdirAll(inTreeSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inTreeSessions, "before.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	pending, err := resolveLocalStorePaths(sddDir, cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pending.OldSessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending.OldSessions, "during.jsonl"), []byte(`{"event":"started"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	authorizedOld := localadapter.SessionStoreRelocationSource{
		Kind:     localadapter.SessionStoreRelocationSourceOldGlobal,
		Sessions: pending.OldSessions, StagedBlobs: pending.OldBlobs, WriteTombstone: true,
	}
	relocator, err := localadapter.NewFilesystemSessionStoreRelocator(
		localadapter.FilesystemSessionStoreRelocatorOptions{
			Sources: []localadapter.SessionStoreRelocationSource{
				{
					Kind:     localadapter.SessionStoreRelocationSourceInTree,
					Sessions: inTreeSessions, StagedBlobs: filepath.Join(sddDir, "staged-blobs"), WriteTombstone: true,
				},
				authorizedOld,
			},
			AuthorizedOldGlobalSource: &authorizedOld,
			TrustedStateRoot:          locations.StateRoot,
			StableRepoAuthority:       pending.StableRepoAuthority,
			AuthorizeInTreeSource:     func(context.Context, string, string, string) error { return nil },
			TargetSessions:            pending.DesiredSessions, TargetBlobs: pending.DesiredBlobs,
			TargetProject: "github.com/org/repo", Transformer: app.CurrentSessionIdentityTransformer{},
			Transition: pending.Transition,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"before.jsonl", "during.jsonl"} {
		if _, err := os.Stat(filepath.Join(pending.DesiredSessions, name)); err != nil {
			t.Fatalf("aggregate target %s missing: %v", name, err)
		}
	}
	current, err := resolveLocalStorePaths(sddDir, cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if current.PendingIdentity || current.Sessions != current.DesiredSessions {
		t.Fatalf("aggregate cutover routing = %+v", current)
	}
}

func TestInitCompositionAcknowledgesDirectInTreeTransitionWithoutOldPayloadSource(t *testing.T) {
	for _, tombstoneOnly := range []bool{false, true} {
		name := "in-tree payload"
		if tombstoneOnly {
			name = "in-tree tombstone only"
		}
		t.Run(name, func(t *testing.T) {
			root := canonicalTempDir(t)
			sddDir := filepath.Join(root, model.SDDDirName)
			inTreeSessions := filepath.Join(sddDir, "sessions")
			if err := os.MkdirAll(inTreeSessions, 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
			locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
			initial, err := resolveLocalStorePaths(sddDir, cfg, locations)
			if err != nil {
				t.Fatal(err)
			}
			if tombstoneOnly {
				writeStoreTombstone(t, inTreeSessions, "local", initial.OldSessions, initial.OldBlobs)
			} else if err := os.WriteFile(
				filepath.Join(inTreeSessions, "legacy.jsonl"), []byte(`{"event":"started"}`), 0o600,
			); err != nil {
				t.Fatal(err)
			}
			pending, err := resolveLocalStorePaths(sddDir, cfg, locations)
			if err != nil {
				t.Fatal(err)
			}
			if pending.Transition == nil {
				t.Fatal("direct in-tree transition did not synthesize a bounded marker")
			}
			if _, err := os.Lstat(pending.OldSessions); !os.IsNotExist(err) {
				t.Fatalf("old root existed before acknowledgement: %v", err)
			}
			sources, authorizedOld := relocationSourcesForInit(pending, sddDir)
			if len(sources) != 1 || sources[0].Kind != localadapter.SessionStoreRelocationSourceInTree {
				t.Fatalf("production relocation sources = %+v", sources)
			}
			relocator, err := localadapter.NewFilesystemSessionStoreRelocator(
				localadapter.FilesystemSessionStoreRelocatorOptions{
					Sources: sources, AuthorizedOldGlobalSource: &authorizedOld,
					TrustedStateRoot:      locations.StateRoot,
					StableRepoAuthority:   pending.StableRepoAuthority,
					AuthorizeInTreeSource: func(context.Context, string, string, string) error { return nil },
					TargetSessions:        pending.DesiredSessions, TargetBlobs: pending.DesiredBlobs,
					TargetProject: "github.com/org/repo",
					Transformer:   app.CurrentSessionIdentityTransformer{}, Transition: pending.Transition,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
				t.Fatal(err)
			}
			oldMaterial, err := localadapter.SessionStoreMaterial(pending.OldSessions, pending.OldBlobs)
			if err != nil {
				t.Fatal(err)
			}
			if len(oldMaterial) != 0 {
				t.Fatalf("synthetic transition treated old marker root as payload source: %v", oldMaterial)
			}
		})
	}
}

func TestInterruptedLegacyMigrationControlKeepsOldIdentityRouting(t *testing.T) {
	sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	identityless, err := resolveLocalStorePaths(
		sddDir, &model.PerRepoConfig{}, locations,
	)
	if err != nil {
		t.Fatal(err)
	}
	controlDir := filepath.Join(identityless.OldSessions, ".legacy-migration")
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		t.Fatal(err)
	}
	event := map[string]any{
		"v": 1, "ts": time.Now().UTC(), "session": "legacy", "seq": 1,
		"event": "session_meta", "data": map[string]any{"participant": "Test"},
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(
		filepath.Join(controlDir, "legacy.original"), encoded, 0o600,
	); err != nil {
		t.Fatal(err)
	}

	pending, err := resolveLocalStorePaths(
		sddDir, &model.PerRepoConfig{RepoID: "github.com/org/repo"}, locations,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !pending.PendingIdentity || pending.Sessions != pending.OldSessions {
		t.Fatalf("interrupted legacy control did not retain old routing: %+v", pending)
	}
	if len(pending.OldLegacySessions) != 1 ||
		filepath.Base(pending.OldLegacySessions[0]) != "legacy.jsonl" {
		t.Fatalf("old legacy recovery candidates = %v", pending.OldLegacySessions)
	}
	sources, _ := relocationSourcesForInit(pending, sddDir)
	foundOld := false
	for _, source := range sources {
		foundOld = foundOld || source.Kind == localadapter.SessionStoreRelocationSourceOldGlobal
	}
	if !foundOld {
		t.Fatal("interrupted legacy control did not authorize old source after recovery")
	}
}

func TestInTreeLegacyMigrationControlRequiresAcknowledgedSourceRecovery(t *testing.T) {
	sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
	controlDir := filepath.Join(sddDir, "sessions", ".legacy-migration")
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(controlDir, "legacy.json"), []byte(`{"version":4}`), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	paths, err := resolveLocalStorePaths(
		sddDir, &model.PerRepoConfig{RepoID: "github.com/org/repo"}, locations,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths.InTreeLegacySessions) != 1 ||
		filepath.Base(paths.InTreeLegacySessions[0]) != "legacy.jsonl" {
		t.Fatalf("in-tree legacy recovery candidates = %v", paths.InTreeLegacySessions)
	}
	if !paths.PendingIdentity {
		t.Fatal("in-tree application control did not require relocation acknowledgement")
	}
	sources, _ := relocationSourcesForInit(paths, sddDir)
	foundInTree := false
	for _, source := range sources {
		foundInTree = foundInTree || source.Kind == localadapter.SessionStoreRelocationSourceInTree
	}
	if !foundInTree {
		t.Fatal("in-tree application control was not selected for pre-relocation recovery")
	}
}

func TestSessionRelocationNoticeIncludesControlOnlyRecovery(t *testing.T) {
	t.Run("identity-less in-tree", func(t *testing.T) {
		sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
		controlDir := filepath.Join(sddDir, "sessions", ".legacy-migration")
		if err := os.MkdirAll(controlDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(controlDir, "legacy.json"), []byte(`{"version":4}`), 0o600); err != nil {
			t.Fatal(err)
		}
		notice, err := sessionRelocationNotice(
			sddDir, &model.PerRepoConfig{},
			repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")},
		)
		if err != nil || !strings.Contains(notice, "pending migration transaction") {
			t.Fatalf("control-only in-tree notice = %q, %v", notice, err)
		}
	})
	t.Run("post-cutover old store", func(t *testing.T) {
		sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
		cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
		locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
		paths, err := resolveLocalStorePaths(sddDir, cfg, locations)
		if err != nil {
			t.Fatal(err)
		}
		controlDir := filepath.Join(paths.OldSessions, ".legacy-migration")
		if err := os.MkdirAll(controlDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(controlDir, "legacy.json"), []byte(`{"version":4}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := localadapter.WriteSessionIdentityTransition(
			paths.OldSessions,
			localadapter.SessionIdentityTransition{
				Version: localadapter.SessionIdentityTransitionVersion,
				State:   localadapter.SessionIdentityTransitionCutover,
				OldKey:  paths.OldKey, NewKey: paths.DesiredKey,
				OldSessions: paths.OldSessions, OldBlobs: paths.OldBlobs,
				CurrentSessions: paths.DesiredSessions, CurrentBlobs: paths.DesiredBlobs,
				TargetProject: "github.com/org/repo",
			},
		); err != nil {
			t.Fatal(err)
		}
		notice, err := sessionRelocationNotice(sddDir, cfg, locations)
		if err != nil || !strings.Contains(notice, "Abandoned session state reappeared") {
			t.Fatalf("post-cutover control-only notice = %q, %v", notice, err)
		}
	})
}

func TestResolveLocalStorePathsRejectsUnsafeInputs(t *testing.T) {
	sddDir := filepath.Join(canonicalTempDir(t), model.SDDDirName)
	if _, err := resolveLocalStorePaths(sddDir, &model.PerRepoConfig{}, repos.Locations{StateRoot: "relative"}); err == nil {
		t.Fatal("relative state root accepted")
	}
	if _, err := confinedStatePath(canonicalTempDir(t), "sessions", "../../escape"); err == nil {
		t.Fatal("escaping repo key accepted")
	}
	if _, err := confinedStatePath(canonicalTempDir(t), "sessions", "../staged-blobs/escape"); err == nil {
		t.Fatal("cross-category repo key accepted")
	}
	if _, err := confinedStatePath(canonicalTempDir(t), "sessions", "local/./escape"); err == nil {
		t.Fatal("dot-segment repo key accepted")
	}
	if _, err := confinedStatePath(canonicalTempDir(t), "../sessions", "local/key"); err == nil {
		t.Fatal("escaping category accepted")
	}
	if _, err := resolveLocalStorePaths(
		sddDir,
		&model.PerRepoConfig{RepoID: "../../escape"},
		repos.Locations{StateRoot: canonicalTempDir(t)},
	); err == nil {
		t.Fatal("invalid repo_id accepted")
	}
}

func runStoreGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
