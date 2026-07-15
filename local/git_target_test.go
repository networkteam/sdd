package local

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	app "github.com/networkteam/sdd/application"
)

func TestMatchingWorktreesExactBranchAndDetached(t *testing.T) {
	output := []byte("worktree /repo\x00HEAD abc\x00branch refs/heads/main\x00\x00" +
		"worktree /repo-feature\x00HEAD def\x00branch refs/heads/main-feature\x00\x00" +
		"worktree /detached\x00HEAD fed\x00detached\x00\x00")
	if got := matchingWorktrees(output, "refs/heads/main"); len(got) != 1 || got[0] != "/repo" {
		t.Fatalf("main matches = %v", got)
	}
	if got := matchingWorktrees(output, "refs/heads/missing"); len(got) != 0 {
		t.Fatalf("missing matches = %v", got)
	}
}

func TestGitWorktreeAcquirerResolvesRegisteredCheckoutAndReleases(t *testing.T) {
	repo := t.TempDir()
	runGitTargetTest(t, repo, "init", "-b", "main")
	runGitTargetTest(t, repo, "config", "user.name", "Test")
	runGitTargetTest(t, repo, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTargetTest(t, repo, "add", "README.md")
	runGitTargetTest(t, repo, "commit", "-m", "fixture")
	worktree := filepath.Join(t.TempDir(), "feature")
	runGitTargetTest(t, repo, "worktree", "add", "-b", "feature", worktree)
	graph, err := NewFilesystemGraphStore(FilesystemGraphStoreOptions{Project: "example", GraphDir: filepath.Join(worktree, ".sdd", "graph")})
	if err != nil {
		t.Fatal(err)
	}
	releases := 0
	acquirer, err := NewGitWorktreeAcquirer(GitWorktreeAcquirerOptions{
		Project: "example", ServerCheckout: repo,
		Factory: func(_ context.Context, checkout string, target app.MutationTarget) (app.GraphStore, []app.MutationFinalizer, func() error, error) {
			resolvedCheckout, _ := filepath.EvalSymlinks(checkout)
			resolvedWorktree, _ := filepath.EvalSymlinks(worktree)
			if resolvedCheckout != resolvedWorktree || target.Branch != "feature" {
				t.Fatalf("factory checkout=%q target=%+v", checkout, target)
			}
			return graph, nil, func() error { releases++; return nil }, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := acquirer.Acquire(t.Context(), app.MutationTarget{Project: "example", Branch: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if err := acquired.Release(); err != nil || releases != 1 {
		t.Fatalf("release count=%d err=%v", releases, err)
	}
	if _, err := acquirer.Acquire(t.Context(), app.MutationTarget{Project: "example", Branch: "main..bad"}); err == nil {
		t.Fatal("invalid branch acquired")
	}
	if _, err := acquirer.Acquire(t.Context(), app.MutationTarget{Project: "example", Branch: "not-checked-out"}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("missing checkout error = %v", err)
	}
}

func runGitTargetTest(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s (%v)", args, output, err)
	}
}
