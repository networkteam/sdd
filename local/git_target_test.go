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
	duplicate := append(append([]byte(nil), output...), []byte("worktree /repo-copy\x00HEAD aaa\x00branch refs/heads/main\x00\x00")...)
	if got := matchingWorktrees(duplicate, "refs/heads/main"); len(got) != 2 {
		t.Fatalf("duplicate main matches = %v", got)
	}
}

func TestGitWorktreeAcquirerRejectsMultipleMatchesAndChangedHEAD(t *testing.T) {
	acquirer, err := NewGitWorktreeAcquirer(GitWorktreeAcquirerOptions{
		Project: "example", ServerCheckout: canonicalTempDir(t),
		Factory: func(context.Context, string, app.MutationTarget) (app.GraphStore, []app.MutationFinalizer, func() error, error) {
			t.Fatal("factory must not run for an invalid checkout")
			return nil, nil, nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := app.MutationTarget{Project: "example", Branch: "feature"}

	t.Run("multiple", func(t *testing.T) {
		acquirer.runGit = func(_ context.Context, args ...string) ([]byte, error) {
			if strings.Contains(strings.Join(args, " "), "worktree list") {
				return []byte("worktree /one\x00branch refs/heads/feature\x00\x00worktree /two\x00branch refs/heads/feature\x00\x00"), nil
			}
			return nil, nil
		}
		if _, err := acquirer.Acquire(t.Context(), target); err == nil || !strings.Contains(err.Error(), "found 2") {
			t.Fatalf("multiple checkout error = %v", err)
		}
	})

	t.Run("changed HEAD", func(t *testing.T) {
		acquirer.runGit = func(_ context.Context, args ...string) ([]byte, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "worktree list"):
				return []byte("worktree /one\x00branch refs/heads/feature\x00\x00"), nil
			case strings.Contains(joined, "symbolic-ref"):
				return []byte("refs/heads/other\n"), nil
			default:
				return nil, nil
			}
		}
		if _, err := acquirer.Acquire(t.Context(), target); err == nil || !strings.Contains(err.Error(), "HEAD changed") {
			t.Fatalf("changed HEAD error = %v", err)
		}
	})
}

func TestGitWorktreeAcquirerResolvesRegisteredCheckoutAndReleases(t *testing.T) {
	repo := canonicalTempDir(t)
	runGitTargetTest(t, repo, "init", "-b", "main")
	runGitTargetTest(t, repo, "config", "user.name", "Test")
	runGitTargetTest(t, repo, "config", "user.email", "test@example.invalid")
	runGitTargetTest(t, repo, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTargetTest(t, repo, "add", "README.md")
	runGitTargetTest(t, repo, "commit", "-m", "fixture")
	worktree := filepath.Join(canonicalTempDir(t), "feature")
	runGitTargetTest(t, repo, "worktree", "add", "-b", "feature", worktree)
	graph, err := NewFilesystemGraphStore(FilesystemGraphStoreOptions{Project: "example", GraphDir: filepath.Join(worktree, ".sdd", "graph")})
	if err != nil {
		t.Fatal(err)
	}
	releases := 0
	factories := 0
	acquirer, err := NewGitWorktreeAcquirer(GitWorktreeAcquirerOptions{
		Project: "example", ServerCheckout: repo,
		Factory: func(_ context.Context, checkout string, target app.MutationTarget) (app.GraphStore, []app.MutationFinalizer, func() error, error) {
			factories++
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
	if err := acquirer.ValidateBranch(t.Context(), app.MutationTarget{Project: "example", Branch: "feature"}); err != nil {
		t.Fatal(err)
	}
	if factories != 0 {
		t.Fatalf("resolve-only validation opened %d target runtime(s)", factories)
	}
	acquired, err := acquirer.Acquire(t.Context(), app.MutationTarget{Project: "example", Branch: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if err := acquired.Release(); err != nil || releases != 1 || factories != 1 {
		t.Fatalf("factory count=%d release count=%d err=%v", factories, releases, err)
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
