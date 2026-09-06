package index

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gitadapter "github.com/networkteam/sdd/internal/git"
)

func TestRepoKey(t *testing.T) {
	if got := RepoKey("github.com/org/repo", "/some/root"); got != "github.com/org/repo" {
		t.Errorf("declared repo_id must win, got %q", got)
	}
	a := RepoKey("", "/home/u/project-a")
	b := RepoKey("", "/home/u/project-b")
	if !strings.HasPrefix(a, "local/") || !strings.HasPrefix(b, "local/") {
		t.Errorf("identity-less keys must be namespaced: %q, %q", a, b)
	}
	if a == b {
		t.Errorf("different roots must key differently: %q", a)
	}
	if again := RepoKey("", "/home/u/project-a"); again != a {
		t.Errorf("keying must be deterministic: %q vs %q", again, a)
	}
}

func TestRepoKeyIdentityLessWorktreeInvariant(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet", "--initial-branch=main")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "seed")
	run("commit", "--quiet", "-m", "seed")
	worktree := filepath.Join(t.TempDir(), "linked")
	run("worktree", "add", "--quiet", "-b", "linked", worktree)

	baseStableRoot, err := gitadapter.StableRepoRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	worktreeStableRoot, err := gitadapter.StableRepoRoot(worktree)
	if err != nil {
		t.Fatal(err)
	}
	baseKey := RepoKey("", baseStableRoot)
	worktreeKey := RepoKey("", worktreeStableRoot)
	if baseKey != worktreeKey {
		t.Fatalf("identity-less worktree keys differ: %q vs %q", baseKey, worktreeKey)
	}
}

func TestStoreDir(t *testing.T) {
	d1 := StoreDir("/cache", "github.com/org/repo", "ollama/model-x")
	d2 := StoreDir("/cache", "github.com/org/repo", "ollama/model-y")
	if d1 == d2 {
		t.Error("different fingerprints must resolve to different stores")
	}
	if !strings.HasPrefix(d1, filepath.Join("/cache", "index", "github.com", "org", "repo")+string(filepath.Separator)) {
		t.Errorf("store dir shape: %q", d1)
	}
}

func writeLegacyIndex(t *testing.T, dir, fingerprint string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "chromem"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chromem", "doc.gob"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Version: 1, Entries: map[string]EntryState{
		"20260101-120000-s-tac-abc": {Versions: []EntryVersion{{Hash: "h", Fingerprint: fingerprint}}},
	}}
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateDir_MoveThenSkip(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	legacy := filepath.Join(root, "repo", ".sdd", "index")
	writeLegacyIndex(t, legacy, "fp-1")

	target, moved, err := MigrateDir(legacy, cacheRoot, "github.com/org/repo")
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !moved {
		t.Fatal("first migration must move")
	}
	if want := StoreDir(cacheRoot, "github.com/org/repo", "fp-1"); target != want {
		t.Errorf("target = %q, want %q (keyed by the manifest's own fingerprint)", target, want)
	}
	if _, err := os.Stat(filepath.Join(target, "chromem", "doc.gob")); err != nil {
		t.Errorf("store content missing after move: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("legacy dir must be gone after a move")
	}

	// A second legacy copy under the same fingerprint is skipped, never
	// clobbered — the existing store wins and the legacy dir stays.
	otherLegacy := filepath.Join(root, "cacheclone", ".index")
	writeLegacyIndex(t, otherLegacy, "fp-1")
	target2, moved2, err := MigrateDir(otherLegacy, cacheRoot, "github.com/org/repo")
	if err != nil {
		t.Fatal(err)
	}
	if moved2 {
		t.Error("existing store must not be clobbered")
	}
	if target2 != target {
		t.Errorf("skip must still name the store: %q", target2)
	}
	if _, err := os.Stat(filepath.Join(otherLegacy, "manifest.json")); err != nil {
		t.Errorf("skipped legacy dir must stay in place: %v", err)
	}
}

func TestStoreGeneration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := ensureStoreDir(dir); err != nil {
		t.Fatal(err)
	}

	// Empty store (no marker, no manifest): generation 0.
	if g, err := storeGeneration(dir); err != nil || g != 0 {
		t.Fatalf("empty store generation = %d, %v; want 0", g, err)
	}

	// Legacy store: a manifest but no marker. The identity fallback yields a
	// stable non-zero token that an unchanged store keeps across reads.
	m := &Manifest{Version: 1, Entries: map[string]EntryState{
		"e": {Versions: []EntryVersion{{Hash: "h", Fingerprint: "fp", ChunkIDs: []string{"e#summary"}}}},
	}}
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	legacyGen, err := storeGeneration(dir)
	if err != nil || legacyGen == 0 {
		t.Fatalf("legacy (manifest, no marker) generation = %d, %v; want non-zero", legacyGen, err)
	}
	if again, _ := storeGeneration(dir); again != legacyGen {
		t.Errorf("legacy generation not stable across reads: %d vs %d", again, legacyGen)
	}

	if err := bumpGeneration(dir); err != nil {
		t.Fatal(err)
	}
	first, err := storeGeneration(dir)
	if err != nil || first == legacyGen {
		t.Fatalf("generation did not change: %v", err)
	}
	if err := bumpGeneration(dir); err != nil {
		t.Fatal(err)
	}
	second, err := storeGeneration(dir)
	if err != nil || second == first {
		t.Fatalf("generation did not change: %v", err)
	}
	m.AddVersion("another", EntryVersion{Hash: "h2"})
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	afterManifest, err := storeGeneration(dir)
	if err != nil || afterManifest == second {
		t.Fatalf("manifest publication without generation bump was missed: %v", err)
	}

}

func TestMigrateDir_EmptyManifestIsNoop(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".sdd", "index")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	target, moved, err := MigrateDir(legacy, filepath.Join(root, "cache"), "github.com/org/repo")
	if err != nil || moved || target != "" {
		t.Errorf("empty legacy index: (%q, %v, %v), want no-op", target, moved, err)
	}
}

// Two concurrent write stores on the same directory serialize on the
// exclusive lock, and a reader's Open waits out an in-flight write session
// — the scenario behind chromem's uncoordinated per-document files.
func TestWriteStoreSerializesWritersAndBlocksReaders(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var order []string
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}

	inSession := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		done <- WriteStore(context.Background(), dir, func(*Index) error {
			record("a-start")
			close(inSession)
			time.Sleep(300 * time.Millisecond)
			record("a-end")
			return nil
		})
	}()
	<-inSession
	go func() {
		done <- WriteStore(context.Background(), dir, func(*Index) error {
			record("b")
			return nil
		})
	}()

	// Reader while the write session is live: Open must not return before
	// the exclusive lock is released.
	if _, err := Open(dir); err != nil {
		t.Fatalf("reader open: %v", err)
	}
	mu.Lock()
	sawAEnd := false
	for _, s := range order {
		if s == "a-end" {
			sawAEnd = true
		}
	}
	mu.Unlock()
	if !sawAEnd {
		t.Error("Open returned while a write session was still in flight")
	}

	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("write session: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[0] != "a-start" || order[1] != "a-end" || order[2] != "b" {
		t.Errorf("write sessions interleaved: %v", order)
	}
}
