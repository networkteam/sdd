package handlers

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/internal/repos/repostest"
)

// countingCommitter is the internal-package committer fake for the repo
// tests (recordingCommitter lives in the external handlers_test package and
// is not visible here). It records how many commits were made.
type countingCommitter struct{ calls int }

func (c *countingCommitter) Commit(string, ...string) error { c.calls++; return nil }

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
	repostest.WriteConfig(t, loc.ConfigPath, gcfg)
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

// Freshening a cross-repo cache reports a phase-true stage — connecting then
// cloning on a first clone (mirroring RepoAdd), syncing on a due pull — and
// never indexing (that stage belongs to embedding, which text-only search does
// not run). This is the s-tac-jbm fix at the layer that knows the transitions.
func TestEnsureReposFresh_ReportsPhaseNotIndexing(t *testing.T) {
	const repoID = "github.com/networkteam/other"
	newHandler := func(t *testing.T) (*Handler, string) {
		dir := t.TempDir()
		loc := repos.Locations{
			ConfigPath: filepath.Join(dir, "xdg", "sdd", "config.yaml"),
			CacheRoot:  filepath.Join(dir, "cache"),
		}
		gcfg := &repos.GlobalConfig{}
		if err := gcfg.AddRepo(repos.ConnectedRepo{RepoID: repoID, CloneURL: "git@github.com:networkteam/other.git"}); err != nil {
			t.Fatal(err)
		}
		repostest.WriteConfig(t, loc.ConfigPath, gcfg)
		reg := repos.NewRegistry(loc)
		cacheDir, err := reg.CacheDir(repoID)
		if err != nil {
			t.Fatal(err)
		}
		return New(Options{Repos: repos.NewManager(reg, &fakeGit{})}), cacheDir
	}

	capture := func(t *testing.T, h *Handler) []model.Phase {
		var phases []model.Phase
		if _, err := h.EnsureReposFresh(context.Background(), command.EnsureReposFreshCmd{
			RepoIDs: []string{repoID},
			OnPhase: func(p model.Phase) { phases = append(phases, p) },
		}); err != nil {
			t.Fatalf("EnsureReposFresh: %v", err)
		}
		if slices.Contains(phases, model.PhaseIndexing) {
			t.Errorf("freshening must never report indexing; got %v", phases)
		}
		return phases
	}

	t.Run("cold clone reports connecting then cloning", func(t *testing.T) {
		h, _ := newHandler(t)
		if phases := capture(t, h); len(phases) < 2 || phases[0] != model.PhaseConnecting || phases[1] != model.PhaseCloning {
			t.Errorf("cold clone should report connecting then cloning; got %v", phases)
		}
	})

	t.Run("due pull reports syncing", func(t *testing.T) {
		h, cacheDir := newHandler(t)
		if err := os.MkdirAll(filepath.Join(cacheDir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if phases := capture(t, h); !slices.Contains(phases, model.PhaseSyncing) {
			t.Errorf("a due pull should report syncing; got %v", phases)
		}
	})
}

// RepoAdd's clone path reports connecting then cloning, in order, before it
// reaches the identity read — the sequence the footer shows.
func TestRepoAdd_ClonePathReportsConnectingThenCloning(t *testing.T) {
	dir := t.TempDir()
	loc := repos.Locations{
		ConfigPath: filepath.Join(dir, "xdg", "sdd", "config.yaml"),
		CacheRoot:  filepath.Join(dir, "cache"),
	}
	repostest.WriteConfig(t, loc.ConfigPath, &repos.GlobalConfig{})
	h := New(Options{Repos: repos.NewManager(repos.NewRegistry(loc), &fakeGit{})})

	// The fake clone leaves no config to read, so RepoAdd errors after cloning;
	// the phase sequence up to the clone is what we assert.
	var phases []model.Phase
	_ = h.RepoAdd(context.Background(), &command.RepoAddCmd{
		CloneURL: "git@github.com:networkteam/new.git",
		OnPhase:  func(p model.Phase) { phases = append(phases, p) },
	})
	if len(phases) != 2 || phases[0] != model.PhaseConnecting || phases[1] != model.PhaseCloning {
		t.Errorf("want [connecting cloning]; got %v", phases)
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
	repostest.WriteConfig(t, loc.ConfigPath, gcfg)

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
	if err := h.BuildConnectedIndexes(context.Background(), []string{repoID}, indexEmbedder(emb), fill); err != nil {
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

// A forced connected-index fill re-embeds every member entry despite an
// up-to-date manifest — the repair path `sdd index --repo --force` needs and
// that a plain lazy fill (which skips converged entries) would not provide.
func TestBuildConnectedIndexes_ForceRebuildsMembers(t *testing.T) {
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
	repostest.WriteConfig(t, loc.ConfigPath, gcfg)
	reg := repos.NewRegistry(loc)
	cacheDir, err := reg.CacheDir(repoID)
	if err != nil {
		t.Fatal(err)
	}
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

	h := New(Options{Reader: readFinderFor(t), Repos: repos.NewManager(reg, &fakeGit{})})
	emb := &fakeEmbedder{}

	// First fill populates the member index + manifest.
	if err := h.BuildConnectedIndexes(context.Background(), []string{repoID}, indexEmbedder(emb), &command.BuildConnectedIndexesCmd{}); err != nil {
		t.Fatalf("initial lazy fill: %v", err)
	}

	// A second lazy fill over the up-to-date index plans nothing.
	lazyPlanned := 0
	if err := h.BuildConnectedIndexes(context.Background(), []string{repoID}, indexEmbedder(emb),
		&command.BuildConnectedIndexesCmd{OnPlanned: func(n int) { lazyPlanned += n }}); err != nil {
		t.Fatalf("second lazy fill: %v", err)
	}
	if lazyPlanned != 0 {
		t.Errorf("lazy fill over an up-to-date index planned %d chunks, want 0", lazyPlanned)
	}

	// A forced fill re-embeds every member entry despite the current manifest.
	forcePlanned := 0
	if err := h.BuildConnectedIndexes(context.Background(), []string{repoID}, indexEmbedder(emb),
		&command.BuildConnectedIndexesCmd{Force: true, OnPlanned: func(n int) { forcePlanned += n }}); err != nil {
		t.Fatalf("forced fill: %v", err)
	}
	if forcePlanned == 0 {
		t.Error("forced fill planned 0 chunks — --force did not reach the member index")
	}
}

func TestRepoAdd_RejectsSelfDependency(t *testing.T) {
	h, _ := newRepoTestHandler(t, "repo_id: github.com/networkteam/other\n")
	err := h.RepoAdd(context.Background(), &command.RepoAddCmd{CloneURL: "git@github.com:networkteam/other.git"})
	if err == nil || !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Errorf("self-dependency must be rejected, got %v", err)
	}
}

const removeTargetRepoID = "github.com/networkteam/other"

// newRemoveTestHandler seeds a project-scoped handler for repo remove: a
// committed .sdd/config.yaml (repoConfig) plus a graph dir and a read finder
// backing the ref-safety scan, with an injected committer to observe commits.
func newRemoveTestHandler(t *testing.T, repoConfig string, committer Committer) (*Handler, string, string) {
	t.Helper()
	dir := t.TempDir()
	sddDir := filepath.Join(dir, ".sdd")
	graphDir := filepath.Join(sddDir, "graph")
	if err := os.MkdirAll(graphDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sddDir, "config.yaml"), []byte(repoConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(Options{SDDDir: sddDir, GraphDir: graphDir, Reader: readFinderFor(t), Committer: committer}), sddDir, graphDir
}

// writeCrossRepoRefEntry writes a local entry that references a foreign entry —
// the kind of ref that makes a dependency unsafe to remove.
func writeCrossRepoRefEntry(t *testing.T, graphDir, id, foreignRef string) {
	t.Helper()
	yyyy, mm, short := id[:4], id[4:6], id[6:]
	dir := filepath.Join(graphDir, yyyy, mm)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: signal\nlayer: tactical\nkind: gap\n" +
		"confidence: medium\nparticipants:\n  - Test\n" +
		"refs:\n  - id: " + foreignRef + "\n    kind: related\n" +
		"summary: |-\n  References a foreign entry.\n---\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, short+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A clean removal — the dependency is declared and nothing references it —
// drops it from the committed config and commits once.
func TestRepoRemove_CleanRemoval(t *testing.T) {
	committer := &countingCommitter{}
	h, sddDir, graphDir := newRemoveTestHandler(t,
		"graph_dir: .sdd/graph\ndependencies: ["+removeTargetRepoID+"]\n# keep me\n", committer)
	writeEntry(t, graphDir, "20260202-100000-s-tac-bbb", "## B\nUnrelated entry.", "Unrelated.")

	var removed []string
	err := h.RepoRemove(context.Background(), &command.RepoRemoveCmd{
		RepoID:    removeTargetRepoID,
		OnRemoved: func(id string) { removed = append(removed, id) },
	})
	if err != nil {
		t.Fatalf("clean removal: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(sddDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), removeTargetRepoID) {
		t.Errorf("dependency not removed: %s", data)
	}
	if !strings.Contains(string(data), "# keep me") {
		t.Errorf("comment lost on removal: %s", data)
	}
	if committer.calls != 1 {
		t.Errorf("Committer.Commit called %d times, want 1", committer.calls)
	}
	if len(removed) != 1 || removed[0] != removeTargetRepoID {
		t.Errorf("OnRemoved calls = %v", removed)
	}
}

// Removing a repo that is not a declared dependency is an error, and nothing
// is committed.
func TestRepoRemove_NotDeclared(t *testing.T) {
	committer := &countingCommitter{}
	h, _, _ := newRemoveTestHandler(t, "graph_dir: .sdd/graph\n", committer)
	err := h.RepoRemove(context.Background(), &command.RepoRemoveCmd{RepoID: removeTargetRepoID})
	if err == nil || !strings.Contains(err.Error(), "not a declared dependency") {
		t.Errorf("undeclared removal should error, got %v", err)
	}
	if committer.calls != 0 {
		t.Errorf("Committer.Commit called %d times, want 0", committer.calls)
	}
}

// The ref-safety guard refuses to remove a dependency that a local entry still
// references, names the stranding ref, and leaves the config untouched.
func TestRepoRemove_RefusesWhenReferenced(t *testing.T) {
	committer := &countingCommitter{}
	h, sddDir, graphDir := newRemoveTestHandler(t,
		"graph_dir: .sdd/graph\ndependencies: ["+removeTargetRepoID+"]\n", committer)
	foreignRef := removeTargetRepoID + ":20260101-100000-s-tac-aaa"
	writeCrossRepoRefEntry(t, graphDir, "20260202-100000-s-tac-bbb", foreignRef)

	err := h.RepoRemove(context.Background(), &command.RepoRemoveCmd{RepoID: removeTargetRepoID})
	if err == nil || !strings.Contains(err.Error(), "refusing to remove") {
		t.Fatalf("referenced removal should refuse, got %v", err)
	}
	if !strings.Contains(err.Error(), foreignRef) {
		t.Errorf("refusal should name the stranding ref %q, got: %v", foreignRef, err)
	}
	data, err := os.ReadFile(filepath.Join(sddDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), removeTargetRepoID) {
		t.Errorf("dependency should be unchanged after refusal: %s", data)
	}
	if committer.calls != 0 {
		t.Errorf("Committer.Commit called %d times, want 0", committer.calls)
	}
}

// --force removes a still-referenced dependency, firing OnStranded with the
// refs it orphaned and committing the change.
func TestRepoRemove_ForceStrandsAndRemoves(t *testing.T) {
	committer := &countingCommitter{}
	h, sddDir, graphDir := newRemoveTestHandler(t,
		"graph_dir: .sdd/graph\ndependencies: ["+removeTargetRepoID+"]\n", committer)
	foreignRef := removeTargetRepoID + ":20260101-100000-s-tac-aaa"
	writeCrossRepoRefEntry(t, graphDir, "20260202-100000-s-tac-bbb", foreignRef)

	var stranded []command.StrandedRef
	err := h.RepoRemove(context.Background(), &command.RepoRemoveCmd{
		RepoID:     removeTargetRepoID,
		Force:      true,
		OnStranded: func(_ string, s []command.StrandedRef) { stranded = s },
	})
	if err != nil {
		t.Fatalf("forced removal: %v", err)
	}
	if len(stranded) != 1 || stranded[0].RefID != foreignRef {
		t.Errorf("OnStranded = %+v, want one ref %q", stranded, foreignRef)
	}
	data, err := os.ReadFile(filepath.Join(sddDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), removeTargetRepoID) {
		t.Errorf("dependency not removed under --force: %s", data)
	}
	if committer.calls != 1 {
		t.Errorf("Committer.Commit called %d times, want 1", committer.calls)
	}
}
