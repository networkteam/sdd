package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	localadapter "github.com/networkteam/sdd/local"
)

func TestRelocatedSessionIsListedResumedAndRecoveredAcrossWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := canonicalTempDir(t)
	runStoreGit(t, base, "init", "--quiet", "--initial-branch=main")
	runStoreGit(t, base, "config", "user.name", "Test")
	runStoreGit(t, base, "config", "user.email", "test@example.invalid")
	runStoreGit(t, base, "config", "commit.gpgsign", "false")
	runStoreGit(t, base, "remote", "add", "origin", "git@github.com:org/repo.git")
	if err := os.WriteFile(filepath.Join(base, "seed"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	runStoreGit(t, base, "add", "seed")
	runStoreGit(t, base, "commit", "--quiet", "-m", "seed")
	worktree := filepath.Join(canonicalTempDir(t), "linked")
	runStoreGit(t, base, "worktree", "add", "--quiet", "-b", "linked", worktree)

	// This is the init-time case where committed config does not yet carry
	// repo_id. Relocation must already use the identity init will derive.
	storeCfg, project := sessionStoreIdentity(&model.PerRepoConfig{}, "git@github.com:org/repo.git")
	if project != "github.com/org/repo" || storeCfg.RepoID != string(project) {
		t.Fatalf("derived relocation identity = %q, %+v", project, storeCfg)
	}
	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	basePaths, err := resolveLocalStorePaths(filepath.Join(base, model.SDDDirName), storeCfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	worktreePaths, err := resolveLocalStorePaths(filepath.Join(worktree, model.SDDDirName), storeCfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if basePaths.Sessions != worktreePaths.Sessions ||
		basePaths.StagedBlobs != worktreePaths.StagedBlobs ||
		basePaths.RepoKey != worktreePaths.RepoKey {
		t.Fatalf("global store differs across worktrees:\nbase: %+v\nwork: %+v", basePaths, worktreePaths)
	}

	sourceSessionsDir := filepath.Join(base, model.SDDDirName, "sessions")
	sourceBlobsDir := filepath.Join(base, model.SDDDirName, "staged-blobs")
	sourceSessions, err := localadapter.NewFilesystemSessionStore(sourceSessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	sourceBlobs, err := localadapter.NewFilesystemStagedBlobStore(sourceBlobsDir)
	if err != nil {
		t.Fatal(err)
	}
	sourceApp := newStoreLocalityTestApplication(
		t, "local", filepath.Join(base, model.DefaultGraphDir), sourceSessions, sourceBlobs,
	)
	identity := sdd.RequestIdentity{Subject: "local"}
	workflow, _, err := sourceApp.OpenWorkflow(t.Context(), identity, "local", sdd.WorkflowOpenRequest{
		MCPSessionID: "base-before-relocation", Label: "relocation continuity",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := sourceSessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(map[string]any{"prepared": sdd.PreparedTransition{
		Version: sdd.PreparedTransitionVersion,
		Target:  sdd.MutationTarget{Project: "local", Branch: "main"},
		Batch:   sdd.MutationBatch{ID: "pending-relocation", Digest: "pending-digest"},
		BlobOwner: sdd.BlobOwner{
			Subject: "local", Session: workflow.ID(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceSessions.Append(t.Context(), workflow.ID(), stored.Version, sdd.SessionAppend{
		Events: []sdd.StoredEvent{{
			CodecVersion: sdd.SessionCodecVersion, Code: "mutation_intent", Payload: intent,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	relocator, err := localadapter.NewFilesystemSessionStoreRelocator(
		localadapter.FilesystemSessionStoreRelocatorOptions{
			Sources: []localadapter.SessionStoreRelocationSource{{
				Kind:     localadapter.SessionStoreRelocationSourceInTree,
				Sessions: sourceSessionsDir, StagedBlobs: sourceBlobsDir, WriteTombstone: true,
			}},
			TrustedStateRoot:    locations.StateRoot,
			StableRepoAuthority: basePaths.StableRepoAuthority,
			AuthorizeInTreeSource: func(ctx context.Context, authority, sessions, blobs string) error {
				return git.AuthorizeInTreeSessionSource(ctx, base, authority, sessions, blobs)
			},
			TargetSessions: basePaths.DesiredSessions, TargetBlobs: basePaths.DesiredBlobs,
			TargetProject: project, Transformer: sdd.CurrentSessionIdentityTransformer{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}

	globalSessions, err := localadapter.NewFilesystemSessionStore(worktreePaths.Sessions)
	if err != nil {
		t.Fatal(err)
	}
	globalBlobs, err := localadapter.NewFilesystemStagedBlobStore(worktreePaths.StagedBlobs)
	if err != nil {
		t.Fatal(err)
	}
	worktreeApp := newStoreLocalityTestApplication(
		t, project, filepath.Join(worktree, model.DefaultGraphDir), globalSessions, globalBlobs,
	)
	listed, err := worktreeApp.ListWorkflowSessions(t.Context(), identity, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Session != workflow.ID() || listed[0].Label != "relocation continuity" {
		t.Fatalf("worktree session listing = %+v", listed)
	}
	if _, resumed, err := worktreeApp.ResumeWorkflow(t.Context(), identity, project, sdd.WorkflowResumeRequest{
		SessionID: workflow.ID(), MCPSessionID: "worktree-after-relocation",
		UserWords: "resume the relocated session from the linked worktree", Takeover: true,
	}); err != nil {
		t.Fatal(err)
	} else if resumed.Session != workflow.ID() || len(resumed.Open) == 0 {
		t.Fatalf("worktree resume = %+v", resumed)
	}

	baseApp := newStoreLocalityTestApplication(
		t, project, filepath.Join(base, model.DefaultGraphDir), globalSessions, globalBlobs,
	)
	recoveries, err := baseApp.ListRecoveries(t.Context(), identity, project, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveries.Items) != 1 ||
		recoveries.Items[0].MutationID != "pending-relocation" ||
		recoveries.Items[0].Target.Project != project {
		t.Fatalf("base recovery projection = %+v", recoveries.Items)
	}
}

func TestGlobalIdentityTransitionDeclineHoldsRoutingThenAcknowledgementRekeys(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := canonicalTempDir(t)
	runStoreGit(t, base, "init", "--quiet", "--initial-branch=main")
	runStoreGit(t, base, "config", "user.name", "Test")
	runStoreGit(t, base, "config", "user.email", "test@example.invalid")
	runStoreGit(t, base, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(base, "seed"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	runStoreGit(t, base, "add", "seed")
	runStoreGit(t, base, "commit", "--quiet", "-m", "seed")
	worktree := filepath.Join(canonicalTempDir(t), "linked")
	runStoreGit(t, base, "worktree", "add", "--quiet", "-b", "linked", worktree)

	locations := repos.Locations{StateRoot: filepath.Join(canonicalTempDir(t), "state")}
	identityless, err := resolveLocalStorePaths(
		filepath.Join(base, model.SDDDirName), &model.PerRepoConfig{}, locations,
	)
	if err != nil {
		t.Fatal(err)
	}
	oldSessions, err := localadapter.NewFilesystemSessionStore(identityless.Sessions)
	if err != nil {
		t.Fatal(err)
	}
	oldBlobs, err := localadapter.NewFilesystemStagedBlobStore(identityless.StagedBlobs)
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "local"}
	oldApp := newStoreLocalityTestApplication(
		t, "local", filepath.Join(base, model.DefaultGraphDir), oldSessions, oldBlobs,
	)
	original, _, err := oldApp.OpenWorkflow(t.Context(), identity, "local", sdd.WorkflowOpenRequest{
		MCPSessionID: "identityless-base", Label: "before repo identity",
	})
	if err != nil {
		t.Fatal(err)
	}
	appendPendingRelocationIntent(t, oldSessions, original.ID())

	cfg := &model.PerRepoConfig{RepoID: "github.com/org/repo"}
	pending, err := resolveLocalStorePaths(filepath.Join(base, model.SDDDirName), cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if !pending.PendingIdentity || pending.RepoKey != pending.OldKey || pending.Sessions != identityless.Sessions {
		t.Fatalf("base pending routing = %+v", pending)
	}
	if got := routedSessionProject(cfg, pending); got != "local" {
		t.Fatalf("pending routed project = %q", got)
	}
	if got := persistentIndexRepoKey(pending); got != pending.DesiredKey || got == pending.RepoKey {
		t.Fatalf("pending persistent index key = %q; routed=%q desired=%q", got, pending.RepoKey, pending.DesiredKey)
	}
	if pending.Transition == nil || pending.Transition.State != localadapter.SessionIdentityTransitionPending {
		t.Fatalf("pending transition marker = %+v", pending.Transition)
	}
	authorizedOld := localadapter.SessionStoreRelocationSource{
		Kind:     localadapter.SessionStoreRelocationSourceOldGlobal,
		Sessions: pending.OldSessions, StagedBlobs: pending.OldBlobs, WriteTombstone: true,
	}
	relocator, err := localadapter.NewFilesystemSessionStoreRelocator(
		localadapter.FilesystemSessionStoreRelocatorOptions{
			Sources:                   []localadapter.SessionStoreRelocationSource{authorizedOld},
			AuthorizedOldGlobalSource: &authorizedOld,
			TrustedStateRoot:          locations.StateRoot,
			StableRepoAuthority:       pending.StableRepoAuthority,
			TargetSessions:            pending.DesiredSessions, TargetBlobs: pending.DesiredBlobs,
			TargetProject: "github.com/org/repo", Transformer: sdd.CurrentSessionIdentityTransformer{},
			Transition: pending.Transition,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := relocator.EnsurePending(t.Context()); err != nil {
		t.Fatal(err)
	}
	worktreePending, err := resolveLocalStorePaths(filepath.Join(worktree, model.SDDDirName), cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if !worktreePending.PendingIdentity || worktreePending.Sessions != pending.OldSessions {
		t.Fatalf("worktree did not share routing hold: %+v", worktreePending)
	}

	// Declining migration leaves the marker pending. A newly opened session
	// therefore remains project-local and lands in the same old global store.
	heldApp := newStoreLocalityTestApplication(
		t, "local", filepath.Join(worktree, model.DefaultGraphDir), oldSessions, oldBlobs,
	)
	duringHold, _, err := heldApp.OpenWorkflow(t.Context(), identity, "local", sdd.WorkflowOpenRequest{
		MCPSessionID: "identityless-worktree", Label: "created during routing hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	declined, err := resolveLocalStorePaths(filepath.Join(worktree, model.SDDDirName), cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if !declined.PendingIdentity || declined.Sessions != pending.OldSessions {
		t.Fatalf("declined transition lost routing hold: %+v", declined)
	}

	if err := relocator.Relocate(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
	current, err := resolveLocalStorePaths(filepath.Join(base, model.SDDDirName), cfg, locations)
	if err != nil {
		t.Fatal(err)
	}
	if current.PendingIdentity || current.RepoKey != current.DesiredKey || current.Sessions != pending.DesiredSessions {
		t.Fatalf("completed routing = %+v", current)
	}
	if got := routedSessionProject(cfg, current); got != "github.com/org/repo" {
		t.Fatalf("completed routed project = %q", got)
	}
	if current.Transition == nil || current.Transition.State != localadapter.SessionIdentityTransitionCompleted {
		t.Fatalf("completed transition marker = %+v", current.Transition)
	}

	newSessions, err := localadapter.NewFilesystemSessionStore(current.Sessions)
	if err != nil {
		t.Fatal(err)
	}
	newBlobs, err := localadapter.NewFilesystemStagedBlobStore(current.StagedBlobs)
	if err != nil {
		t.Fatal(err)
	}
	currentApp := newStoreLocalityTestApplication(
		t, "github.com/org/repo", filepath.Join(worktree, model.DefaultGraphDir), newSessions, newBlobs,
	)
	listed, err := currentApp.ListWorkflowSessions(t.Context(), identity, "github.com/org/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("post-rekey sessions = %+v", listed)
	}
	if _, resumed, err := currentApp.ResumeWorkflow(t.Context(), identity, "github.com/org/repo", sdd.WorkflowResumeRequest{
		SessionID: original.ID(), MCPSessionID: "repo-id-resume",
		UserWords: "resume after acknowledged identity rekey", Takeover: true,
	}); err != nil {
		t.Fatal(err)
	} else if resumed.Session != original.ID() {
		t.Fatalf("resumed original = %+v", resumed)
	}
	recoveries, err := currentApp.ListRecoveries(t.Context(), identity, "github.com/org/repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveries.Items) != 1 || recoveries.Items[0].Target.Project != "github.com/org/repo" {
		t.Fatalf("post-rekey recovery = %+v", recoveries.Items)
	}
	if stored, err := newSessions.Load(t.Context(), duringHold.ID()); err != nil ||
		stored.Metadata.Project != "github.com/org/repo" {
		t.Fatalf("during-hold session identity = %+v, %v", stored.Metadata, err)
	}
}

func appendPendingRelocationIntent(t *testing.T, sessions sdd.SessionStore, session sdd.SessionID) {
	t.Helper()
	stored, err := sessions.Load(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(map[string]any{"prepared": sdd.PreparedTransition{
		Version: sdd.PreparedTransitionVersion,
		Target:  sdd.MutationTarget{Project: "local", Branch: "main"},
		Batch:   sdd.MutationBatch{ID: "pending-relocation", Digest: "pending-digest"},
		BlobOwner: sdd.BlobOwner{
			Subject: "local", Session: session,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Append(t.Context(), session, stored.Version, sdd.SessionAppend{
		Events: []sdd.StoredEvent{{
			CodecVersion: sdd.SessionCodecVersion, Code: "mutation_intent", Payload: intent,
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func newStoreLocalityTestApplication(
	t *testing.T,
	project sdd.ProjectID,
	graphDir string,
	sessions sdd.SessionStore,
	blobs sdd.StagedBlobStore,
) *sdd.Application {
	t.Helper()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{
		Project: project, GraphDir: graphDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	llm := sdd.LLMExecutorFuncs{
		CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
		ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
			return sdd.LLMResult{Output: []byte(`{"findings":[]}`), ExecutorFingerprint: "test"}, nil
		},
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: project}, DefaultBranch: "main", Graph: graph,
		Sessions: sessions, StagedBlobs: blobs, LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&localRuntimeAccess{
		project: project, participant: "Christopher", runtime: runtime,
		dependencies: map[string]*sdd.ProjectRuntime{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return application
}
