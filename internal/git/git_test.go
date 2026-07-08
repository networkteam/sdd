package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// newTestRepo initializes a temp git repo with a committed seed file and a
// local identity, returning its path. HEAD exists so subsequent commits have a
// parent.
func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "seed.txt")
	runGit(t, dir, "commit", "-q", "-m", "seed")
	return dir
}

// commitFiles returns the file paths recorded by HEAD relative to its parent.
func commitFiles(t *testing.T, dir string) []string {
	t.Helper()
	return strings.Fields(runGit(t, dir, "show", "--name-only", "--pretty=format:", "HEAD"))
}

// TestCommit_ScopesToPathspec is the regression test for the unscoped-commit
// bug (s-tac-tdz): the auto-commit staged only the given paths but then ran
// `git commit -m` with no pathspec, recording the whole index — so any
// pre-staged unrelated work was swept into the CLI's own commit. The fix passes
// `-- <paths>` to `git commit`; this asserts a pre-staged file stays out of the
// resulting commit and remains staged.
func TestCommit_ScopesToPathspec(t *testing.T) {
	dir := newTestRepo(t)

	// Pre-stage unrelated work, as an agent might before invoking the CLI.
	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("unrelated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "unrelated.txt")

	// The file the CLI command "touched".
	if err := os.WriteFile(filepath.Join(dir, "touched.txt"), []byte("touched\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)
	if err := (CLI{}).Commit("touch", "touched.txt"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	files := commitFiles(t, dir)
	if !slices.Contains(files, "touched.txt") {
		t.Errorf("HEAD commit missing touched.txt; files = %v", files)
	}
	if slices.Contains(files, "unrelated.txt") {
		t.Errorf("HEAD commit swept in pre-staged unrelated.txt; files = %v", files)
	}

	// The unrelated work must still be staged, untouched by the commit.
	staged := strings.Fields(runGit(t, dir, "diff", "--cached", "--name-only"))
	if !slices.Contains(staged, "unrelated.txt") {
		t.Errorf("unrelated.txt no longer staged after scoped commit; staged = %v", staged)
	}
}

// TestRemovalCommit_ScopesToPathspec covers the deletion path (wip done):
// the same fix must not let a pre-staged index leak into the marker-removal
// commit, and `git commit -- <deleted-path>` must still record the deletion.
func TestRemovalCommit_ScopesToPathspec(t *testing.T) {
	dir := newTestRepo(t)

	// A tracked marker file that the command will remove.
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "marker.txt")
	runGit(t, dir, "commit", "-q", "-m", "add marker")

	// Pre-stage unrelated work.
	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("unrelated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "unrelated.txt")

	// FinishWIP removes the marker file from disk before committing.
	if err := os.Remove(filepath.Join(dir, "marker.txt")); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)
	if err := (RemovalCommitter{}).Commit("remove marker", "marker.txt"); err != nil {
		t.Fatalf("RemovalCommitter.Commit: %v", err)
	}

	files := commitFiles(t, dir)
	if !slices.Contains(files, "marker.txt") {
		t.Errorf("HEAD commit did not record marker.txt deletion; files = %v", files)
	}
	if slices.Contains(files, "unrelated.txt") {
		t.Errorf("HEAD commit swept in pre-staged unrelated.txt; files = %v", files)
	}

	// The deletion must be real (marker.txt gone from the tree) and the
	// unrelated work must still be staged.
	tracked := strings.Fields(runGit(t, dir, "ls-files"))
	if slices.Contains(tracked, "marker.txt") {
		t.Errorf("marker.txt still tracked after removal commit; tracked = %v", tracked)
	}
	if !slices.Contains(tracked, "unrelated.txt") {
		t.Errorf("unrelated.txt no longer staged after scoped commit; tracked = %v", tracked)
	}
}
