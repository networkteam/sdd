package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/git"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestModifiedFilesAndHeadContent(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "config", "user.email", "test@example.com")
	gitIn(t, root, "config", "user.name", "Test")
	graph := filepath.Join(root, "graph")
	writeTestFile(t, filepath.Join(graph, "2026", "seed.md"), "seed\n")
	writeTestFile(t, filepath.Join(graph, "gone.md"), "gone\n")
	writeTestFile(t, filepath.Join(root, "outside.txt"), "outside\n")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "seed")

	writeTestFile(t, filepath.Join(graph, "2026", "seed.md"), "changed\n")
	writeTestFile(t, filepath.Join(graph, "untracked.md"), "new\n")
	writeTestFile(t, filepath.Join(root, "outside.txt"), "changed\n")
	if err := os.Remove(filepath.Join(graph, "gone.md")); err != nil {
		t.Fatal(err)
	}

	modified, err := git.CLI{}.ModifiedFiles(t.Context(), graph)
	if err != nil {
		t.Fatalf("ModifiedFiles: %v", err)
	}
	if want := filepath.Join(graph, "2026", "seed.md"); len(modified) != 1 || modified[0] != want {
		t.Errorf("only the modified tracked file under the directory should be listed, as an absolute path, got %v", modified)
	}

	head, err := git.CLI{}.HeadContent(t.Context(), filepath.Join(graph, "2026", "seed.md"))
	if err != nil {
		t.Fatalf("HeadContent: %v", err)
	}
	if string(head) != "seed\n" {
		t.Errorf("HEAD content should be the committed version, got %q", head)
	}
}
