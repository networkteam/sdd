package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/repos"
)

// newRepoTestHandler seeds a handler whose global config already holds a
// connection — the declaration-only path of RepoAdd (no clone involved).
func newRepoTestHandler(t *testing.T, repoConfig string) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	loc := repos.Locations{
		ConfigPath: filepath.Join(dir, "xdg", "sdd", "config.yaml"),
		CacheRoot:  filepath.Join(dir, "cache"),
	}
	gcfg := &repos.GlobalConfig{}
	if err := gcfg.AddRepo(repos.ConnectedRepo{RepoID: "github.com/networkteam/other", CloneURL: "git@github.com:networkteam/other.git"}); err != nil {
		t.Fatal(err)
	}
	if err := repos.SaveConfigTo(loc.ConfigPath, gcfg); err != nil {
		t.Fatal(err)
	}
	sddDir := filepath.Join(dir, "repo", ".sdd")
	if err := os.MkdirAll(sddDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if repoConfig != "" {
		if err := os.WriteFile(filepath.Join(sddDir, "config.yaml"), []byte(repoConfig), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reg := repos.NewRegistry(loc)
	return New(Options{SDDDir: sddDir, Repos: repos.NewManager(reg, git.CLI{})}), sddDir
}

// Re-running repo add against an already-connected URL is the upgrade path:
// it declares the dependency in the committed config and touches nothing
// global; a second run is a loud no-op.
func TestRepoAdd_DeclaresDependencyForExistingConnection(t *testing.T) {
	h, sddDir := newRepoTestHandler(t, "graph_dir: .sdd/graph\n# keep me\n")

	var declared []string
	cmd := &command.RepoAddCmd{
		CloneURL: "git@github.com:networkteam/other.git",
		OnDeclared: func(repoID string, already bool) {
			if already {
				t.Errorf("first declare must report alreadyDeclared=false")
			}
			declared = append(declared, repoID)
		},
	}
	if err := h.RepoAdd(context.Background(), cmd); err != nil {
		t.Fatalf("declare-only add: %v", err)
	}
	if len(declared) != 1 || declared[0] != "github.com/networkteam/other" {
		t.Errorf("OnDeclared calls = %v", declared)
	}
	data, err := os.ReadFile(filepath.Join(sddDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "dependencies: [github.com/networkteam/other]") {
		t.Errorf("dependency not declared: %s", data)
	}
	if !strings.Contains(string(data), "# keep me") {
		t.Errorf("comment lost on declare: %s", data)
	}

	// Second run: both records exist — loud no-op.
	err = h.RepoAdd(context.Background(), &command.RepoAddCmd{CloneURL: "git@github.com:networkteam/other.git"})
	if err == nil || !strings.Contains(err.Error(), "already connected and declared") {
		t.Errorf("second add should report nothing to do, got %v", err)
	}
}

func TestRepoAdd_RejectsSelfDependency(t *testing.T) {
	h, _ := newRepoTestHandler(t, "repo_id: github.com/networkteam/other\n")
	err := h.RepoAdd(context.Background(), &command.RepoAddCmd{CloneURL: "git@github.com:networkteam/other.git"})
	if err == nil || !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Errorf("self-dependency must be rejected, got %v", err)
	}
}
