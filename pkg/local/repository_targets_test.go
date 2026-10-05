package local_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/local"
)

// TestRepositoryTargetPublishesCaptureThroughOneCommit drives CreateEntry over
// the production local wiring: worktree files without a commit are a failure,
// the retry after a failed commit keeps the first rendered document, and a
// retry of a finished publication returns it without another commit.
func TestRepositoryTargetPublishesCaptureThroughOneCommit(t *testing.T) {
	repo := newGitRepository(t)
	repo.write(".sdd/config.yaml", "repo_id: example\ngraph_dir: .sdd/graph\ndefault_branch: main\n")
	repo.write("global-config.yaml", "")
	summaries := 0
	application, identity, binding := repo.captureApplication(t, pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
		summaries++
		return pkgllm.Result{Text: fmt.Sprintf("Rendered summary %d.", summaries)}, nil
	}))
	const entryID = "20260914-020000-s-tac-cap"
	draft := sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Confidence: "high", Body: "Local capture is committed once.",
		EntryID: entryID, Publication: sdd.PublicationKey{Session: binding.SessionID, Sequence: 1, Discriminator: "new-entry"},
	}

	allowCommits := repo.failCommits()
	if _, err := application.CreateEntry(t.Context(), identity, "example", binding, draft); err == nil {
		t.Fatal("worktree files without a commit reported success")
	}
	if count := repo.git("rev-list", "--count", "HEAD"); count != "1" {
		t.Fatalf("failed commit left %s commits", count)
	}
	allowCommits()
	created, err := application.CreateEntry(t.Context(), identity, "example", binding, draft)
	if err != nil || created.EntryID != entryID || created.Summary != "Rendered summary 1." {
		t.Fatalf("capture after failed commit = %+v, %v", created, err)
	}
	published := repo.git("rev-parse", "HEAD")
	message := repo.git("log", "-1", "--format=%B")
	if !strings.Contains(message, "SDD-Mutation: "+draft.Publication.String()) || strings.Contains(message, string(binding.SessionID)) {
		t.Fatalf("publication commit = %q; want the opaque publication ID without the session handle", message)
	}
	relPath, err := model.IDToRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	if committed := repo.git("show", published+":.sdd/graph/"+relPath); !strings.Contains(committed, "Rendered summary 1.") {
		t.Fatalf("committed entry = %q", committed)
	}

	retried, err := application.CreateEntry(t.Context(), identity, "example", binding, draft)
	if err != nil || retried.EntryID != entryID || retried.Summary != created.Summary {
		t.Fatalf("retry of a finished publication = %+v, %v", retried, err)
	}
	if repo.git("rev-parse", "HEAD") != published || repo.git("rev-list", "--count", "HEAD") != "2" {
		t.Fatal("retry did not reuse the publication commit")
	}
	if summaries != 2 {
		t.Fatalf("summaries = %d; want one per attempt without a publication, none once it exists", summaries)
	}
}

func TestRepositoryTargetsReloadConfiguration(t *testing.T) {
	repo := newGitRepository(t)
	repo.write(".sdd/config.yaml", "repo_id: committed\ndefault_branch: main\n")
	targets := repo.targets()
	for _, tt := range []struct {
		name         string
		localID      string
		globalConfig string
		wantErr      bool
	}{
		{name: "local overlay", localID: "example"},
		{name: "changed local identity", localID: "other", wantErr: true},
		{name: "changed global config", localID: "example", globalConfig: "participant: [", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo.write(".sdd/config.local.yaml", "repo_id: "+tt.localID+"\n")
			repo.write("global-config.yaml", tt.globalConfig)
			read, err := targets.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{Branch: "main"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("read acquisition error = %v, want error = %v", err, tt.wantErr)
			}
			if read != nil {
				if err := read.Release(); err != nil {
					t.Fatal(err)
				}
			}
			write, err := targets.Acquire(t.Context(), sdd.MutationTarget{Project: "example", Branch: "main"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("write acquisition error = %v, want error = %v", err, tt.wantErr)
			}
			if write != nil {
				if err := write.Release(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func (r *gitRepository) targets() *local.GitWorktreeAcquirer {
	r.t.Helper()
	targets, err := local.NewRepositoryTargets("example", r.root, filepath.Join(r.root, "global-config.yaml"))
	if err != nil {
		r.t.Fatal(err)
	}
	return targets
}

// captureApplication composes the application the way the CLI does for a
// local checkout: filesystem reads, Git-backed mutation targets, one session.
func (r *gitRepository) captureApplication(t *testing.T, runner pkgllm.Runner) (*sdd.Application, sdd.RequestIdentity, sdd.SessionBinding) {
	t.Helper()
	graph, err := local.NewFilesystemGraphStore(local.FilesystemGraphStoreOptions{Project: "example", GraphDir: filepath.Join(r.root, ".sdd", "graph")})
	if err != nil {
		t.Fatal(err)
	}
	targets := r.targets()
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph: graph, Targets: targets, Branches: targets, LLM: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := local.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := local.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: runtimeAccess{runtime: runtime}, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	const session sdd.SessionID = "s_local-handle"
	created, err := sessions.Create(t.Context(), sdd.SessionMetadata{
		ID: session, Subject: identity.Subject, Project: "example", Participant: "Christopher",
		Attachment: &sdd.Attachment{Subject: identity.Subject, ClientName: "test", LastActivity: time.Now().UTC().Round(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return application, identity, sdd.SessionBinding{SessionID: session, Subject: identity.Subject, Project: "example", Version: created.Version}
}

type runtimeAccess struct{ runtime *sdd.ProjectRuntime }

func (runtimeAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject}, nil
}
func (runtimeAccess) ResolveParticipant(context.Context, sdd.Principal, sdd.ProjectID) (string, error) {
	return "Christopher", nil
}
func (runtimeAccess) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: sdd.ProjectRef{ID: "example"}, CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}
func (a runtimeAccess) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return a.runtime, nil
}
func (runtimeAccess) ResolveDependency(_ context.Context, _ sdd.Principal, _ sdd.ProjectID, dependency sdd.RepoID) (*sdd.ProjectRuntime, error) {
	return nil, fmt.Errorf("no dependency %s", dependency)
}
func (runtimeAccess) AuthorizeSession(ctx context.Context, request sdd.SessionAccessRequest) error {
	return sdd.OwnerOnly(ctx, request)
}
