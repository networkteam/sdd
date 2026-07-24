package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStableRepoRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	runGit(t, root, "init", "--quiet", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	runGit(t, root, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "seed")
	runGit(t, root, "commit", "--quiet", "-m", "seed")
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	commonDir := filepath.Join(canonicalRoot, ".git")

	if got, err := StableRepoRoot(root); err != nil || got != commonDir {
		t.Fatalf("main worktree identity = %q, want %q", got, commonDir)
	}

	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, root, "worktree", "add", "--quiet", "-b", "linked", worktree)
	if got, err := StableRepoRoot(worktree); err != nil || got != commonDir {
		t.Fatalf("linked worktree identity = %q, want %q", got, commonDir)
	}

	plain := t.TempDir()
	if got, err := StableRepoRoot(plain); err != nil || got != plain {
		t.Fatalf("non-git root = %q, want unchanged %q", got, plain)
	}
}

func TestStableRepoRootPreservesDistinctCommonDirectoryShapes(t *testing.T) {
	for name, commonDir := range map[string]string{
		"separate git dir": "/state/git/repository.git",
		"submodule":        "/repo/.git/modules/nested",
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			got, err := stableRepoRootWith("/repo/worktree", func(args ...string) ([]byte, error) {
				calls++
				if calls == 1 {
					return []byte("true\n"), nil
				}
				return []byte(commonDir + "\n"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != commonDir {
				t.Fatalf("common-directory identity = %q, want %q", got, commonDir)
			}
		})
	}
}

func TestStableRepoRootSurfacesCommonDirResolutionFailure(t *testing.T) {
	calls := 0
	_, err := stableRepoRootWith("/repo", func(args ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("true\n"), nil
		}
		return []byte("permission denied"), errors.New("git failed")
	})
	if err == nil || !strings.Contains(err.Error(), "resolving Git common directory") {
		t.Fatalf("error = %v, want common-directory failure", err)
	}
}
