package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/repos"
)

// fakeGit is a no-op git for cross-repo cache tests: the cache is seeded
// pre-cloned on disk, so Clone is never reached and PullFFOnly just counts
// invocations (the cooldown pull EnsureReposFresh performs on a fresh read).
type fakeGit struct{ pulls int }

func (f *fakeGit) Clone(context.Context, string, string) error { return nil }
func (f *fakeGit) PullFFOnly(context.Context, string) error    { f.pulls++; return nil }

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

// BuildConnectedIndexes is the shared spine behind `sdd index --repo` and a
// cross-repo search's fill step: it freshens the connected caches and fills
// each member's index under the shared embedder, forwarding progress. This
// exercises a pre-cloned member end to end — the cooldown pull runs, the
// member index is written under (repo-id, fingerprint), and the fill
// callbacks report the repo and its chunks.
func TestBuildConnectedIndexes_FreshensAndFills(t *testing.T) {
	dir := t.TempDir()
	loc := repos.Locations{
		ConfigPath: filepath.Join(dir, "xdg", "sdd", "config.yaml"),
		CacheRoot:  filepath.Join(dir, "cache"),
	}
	const repoID = "github.com/networkteam/other"

	gcfg := &repos.GlobalConfig{}
	if err := gcfg.AddRepo(repos.ConnectedRepo{RepoID: repoID, CloneURL: "git@github.com:networkteam/other.git"}); err != nil {
		t.Fatal(err)
	}
	if err := repos.SaveConfigTo(loc.ConfigPath, gcfg); err != nil {
		t.Fatal(err)
	}

	reg := repos.NewRegistry(loc)
	cacheDir, err := reg.CacheDir(repoID)
	if err != nil {
		t.Fatal(err)
	}
	// Seed a pre-cloned cache: a .git marker (so IsCloned is true), the
	// committed config declaring the identity + graph dir, and one entry.
	if err := os.MkdirAll(filepath.Join(cacheDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	memberSDD := filepath.Join(cacheDir, ".sdd")
	if err := os.MkdirAll(memberSDD, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memberSDD, "config.yaml"),
		[]byte("repo_id: "+repoID+"\ngraph_dir: .sdd/graph\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeEntry(t, filepath.Join(cacheDir, ".sdd", "graph"),
		"20260101-100000-s-tac-aaa", "## A\nMember entry body.", "Member summary.")

	fg := &fakeGit{}
	h := New(Options{Reader: readFinderFor(t), Repos: repos.NewManager(reg, fg)})

	emb := &fakeEmbedder{}
	var startedRepos []string
	planned, indexed := 0, 0
	fill := &command.BuildConnectedIndexesCmd{
		OnRepoStart:    func(id string) { startedRepos = append(startedRepos, id) },
		OnPlanned:      func(n int) { planned += n },
		OnEntryIndexed: func(_ string, n int) { indexed += n },
	}
	if err := h.BuildConnectedIndexes(context.Background(), []string{repoID}, emb, fill); err != nil {
		t.Fatalf("BuildConnectedIndexes: %v", err)
	}

	if fg.pulls != 1 {
		t.Errorf("expected 1 cooldown pull on a fresh read, got %d", fg.pulls)
	}
	if len(startedRepos) != 1 || startedRepos[0] != repoID {
		t.Errorf("OnRepoStart calls = %v, want [%s]", startedRepos, repoID)
	}
	if planned == 0 {
		t.Error("expected planned chunks > 0")
	}
	if indexed != planned {
		t.Errorf("indexed %d chunks, planned %d — every planned chunk should land", indexed, planned)
	}

	// The member index lives under the machine-global (repo-id, fingerprint)
	// store, not inside the cache clone.
	idxDir := index.StoreDir(loc.CacheRoot, repoID, emb.Fingerprint())
	manifest, err := index.LoadManifest(idxDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest.Entries["20260101-100000-s-tac-aaa"]; !ok {
		t.Errorf("member entry missing from index manifest: %+v", manifest.Entries)
	}
}

func TestRepoAdd_RejectsSelfDependency(t *testing.T) {
	h, _ := newRepoTestHandler(t, "repo_id: github.com/networkteam/other\n")
	err := h.RepoAdd(context.Background(), &command.RepoAddCmd{CloneURL: "git@github.com:networkteam/other.git"})
	if err == nil || !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Errorf("self-dependency must be rejected, got %v", err)
	}
}
