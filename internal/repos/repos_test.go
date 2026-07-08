package repos

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/model"
)

func TestGlobalConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("missing file must load empty: %v", err)
	}
	if len(cfg.Repos) != 0 {
		t.Fatalf("empty config has repos: %+v", cfg.Repos)
	}

	if err := cfg.AddRepo(ConnectedRepo{RepoID: "github.com/networkteam/other", CloneURL: "git@github.com:networkteam/other.git"}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.AddRepo(ConnectedRepo{RepoID: "github.com/networkteam/other", CloneURL: "x"}); err == nil {
		t.Error("duplicate repo_id must be rejected")
	}
	if err := cfg.AddRepo(ConnectedRepo{RepoID: "nohost", CloneURL: "x"}); err == nil {
		t.Error("invalid repo_id must be rejected")
	}
	cfg.Embedding = model.EmbeddingConfig{Provider: "ollama", Model: "nomic-embed-text"}

	if err := SaveConfigTo(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	repo, ok := loaded.Connected("github.com/networkteam/other")
	if !ok || repo.CloneURL != "git@github.com:networkteam/other.git" {
		t.Errorf("round-trip lost connection: %+v", loaded.Repos)
	}
	if loaded.Embedding.Provider != "ollama" {
		t.Errorf("round-trip lost embedding config: %+v", loaded.Embedding)
	}

	if !loaded.RemoveRepo("github.com/networkteam/other") {
		t.Error("RemoveRepo must report existing connection")
	}
	if loaded.RemoveRepo("github.com/networkteam/other") {
		t.Error("RemoveRepo must report missing connection")
	}
}

// TestDefaultLocationsHonorXDG covers the one place the package touches the
// environment: the default-locations constructor composition roots call.
// Everything else works against an explicit Locations value.
func TestDefaultLocationsHonorXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-cache")

	loc, err := DefaultLocations()
	if err != nil {
		t.Fatal(err)
	}
	if loc.ConfigPath != "/tmp/xdg-config/sdd/config.yaml" {
		t.Errorf("ConfigPath = %q", loc.ConfigPath)
	}
	if loc.CacheRoot != "/tmp/xdg-cache/sdd" {
		t.Errorf("CacheRoot = %q", loc.CacheRoot)
	}

	reg := NewRegistry(loc)
	dir, err := reg.CacheDir("github.com/networkteam/other")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join("/tmp/xdg-cache/sdd", "github.com", "networkteam", "other") {
		t.Errorf("CacheDir = %q", dir)
	}
	if _, err := reg.CacheDir("not a repo id"); err == nil {
		t.Error("CacheDir must validate the repo id")
	}
}

// initUpstream creates a local git repo that acts as the connected remote:
// one committed graph entry plus a committed .sdd/config.yaml declaring
// repo_id.
func initUpstream(t *testing.T, repoID string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet", "--initial-branch=main")

	if err := os.MkdirAll(filepath.Join(dir, ".sdd/graph/2026/06"), 0o755); err != nil {
		t.Fatal(err)
	}
	entry := "---\ntype: signal\nlayer: tac\nkind: gap\n---\n\nA remote gap.\n"
	if err := os.WriteFile(filepath.Join(dir, ".sdd/graph/2026/06/01-120000-s-tac-abc.md"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "graph_dir: .sdd/graph\nrepo_id: " + repoID + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".sdd/config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".sdd")
	run("commit", "--quiet", "-m", "seed")
	return dir
}

func TestCacheLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repoID := "example.com/team/other"
	upstream := initUpstream(t, repoID)

	loc := Locations{
		ConfigPath: filepath.Join(t.TempDir(), "config.yaml"),
		CacheRoot:  filepath.Join(t.TempDir(), "cache"),
	}
	mgr := NewManager(NewRegistry(loc), git.CLI{})
	cacheDir := filepath.Join(loc.CacheRoot, filepath.FromSlash(repoID))
	repo := ConnectedRepo{RepoID: repoID, CloneURL: upstream}
	ctx := context.Background()

	cloned, err := mgr.EnsureCloned(ctx, repo, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if !cloned || !IsCloned(cacheDir) {
		t.Fatal("expected a fresh clone")
	}
	if cloned, err = mgr.EnsureCloned(ctx, repo, cacheDir); err != nil || cloned {
		t.Fatalf("second EnsureCloned must be a no-op, got cloned=%v err=%v", cloned, err)
	}

	// The declared identity and graph dir resolve from the cached config.
	declared, err := DeclaredRepoID(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if declared != repoID {
		t.Errorf("DeclaredRepoID = %q, want %q", declared, repoID)
	}
	graphDir, err := GraphDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(graphDir, "2026/06/01-120000-s-tac-abc.md")); err != nil {
		t.Errorf("cached graph entry missing: %v", err)
	}

	// Within cooldown: no pull. After the marker ages out: pull runs and
	// picks up new upstream commits.
	pulled, err := mgr.CooldownPull(ctx, cacheDir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if pulled {
		t.Error("pull ran inside the cooldown window")
	}

	entry2 := "---\ntype: signal\nlayer: tac\nkind: gap\n---\n\nA second remote gap.\n"
	if err := os.WriteFile(filepath.Join(upstream, ".sdd/graph/2026/06/02-130000-s-tac-def.md"), []byte(entry2), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", upstream, "add", ".sdd")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", upstream, "-c", "user.name=test", "-c", "user.email=t@example.com", "commit", "--quiet", "-m", "second")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	pulled, err = mgr.CooldownPull(ctx, cacheDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !pulled {
		t.Error("expected a pull after cooldown expiry")
	}
	if _, err := os.Stat(filepath.Join(graphDir, "2026/06/02-130000-s-tac-def.md")); err != nil {
		t.Errorf("pull did not fetch the new entry: %v", err)
	}

	if IndexDir(cacheDir) != filepath.Join(cacheDir, ".index") {
		t.Errorf("IndexDir = %q", IndexDir(cacheDir))
	}
}
