// Machine-global store resolution and the locking seam. The vector index
// lives outside every working tree in one content-addressed store per
// (repo-key, embedder-fingerprint) under the cache root — shared by a
// repo's checkout, its worktrees, and its connected-repo cache, so the same
// content is embedded once per machine (d-cpt-6cq / d-tac-nhx). All index
// locking is centralized here: no other package takes index locks.
package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// storeSubdir namespaces index stores under the cache root. It cannot
// collide with connected-repo clone caches: those nest under repo IDs,
// which always start with a dotted hostname.
const storeSubdir = "index"

// localKeyPrefix namespaces identity-less repos (no committed repo_id).
// Same non-collision argument as storeSubdir.
const localKeyPrefix = "local"

// RepoKey returns the identity a repository's index store is keyed by: the
// declared repo_id when the repo has one, else a hash of the absolute repo
// root under the "local" namespace. A moved identity-less repo therefore
// re-embeds — accepted: `sdd init` migration covers the common case, and
// minting a synthetic ID would invent a second identity concept.
func RepoKey(repoID, repoRoot string) string {
	if repoID != "" {
		return repoID
	}
	sum := sha256.Sum256([]byte(repoRoot))
	return localKeyPrefix + "/" + hex.EncodeToString(sum[:])[:12]
}

// StoreDir resolves the machine-global store directory for one (repo-key,
// embedder-fingerprint) pair: <cacheRoot>/index/<repo-key>/<fp-hash>. The
// fingerprint is hashed because it is a free-form embedder string, not a
// path; keying by fingerprint means a changed embedder starts a fresh
// store instead of drifting inside an existing one.
func StoreDir(cacheRoot, repoKey, fingerprint string) string {
	sum := sha256.Sum256([]byte(fingerprint))
	return filepath.Join(cacheRoot, storeSubdir, filepath.FromSlash(repoKey), hex.EncodeToString(sum[:])[:12])
}

// ManifestFingerprint returns the dominant embedder fingerprint recorded in
// a manifest — the fingerprint a legacy in-tree index was built under, used
// to pick its target store during migration. Empty when the manifest holds
// no fingerprints.
func ManifestFingerprint(m *Manifest) string {
	counts := map[string]int{}
	best, bestN := "", 0
	for _, state := range m.Entries {
		for _, v := range state.Versions {
			if v.Fingerprint == "" {
				continue
			}
			counts[v.Fingerprint]++
			if counts[v.Fingerprint] > bestN {
				best, bestN = v.Fingerprint, counts[v.Fingerprint]
			}
		}
	}
	return best
}

// MigrateDir moves a legacy index directory (an in-tree .sdd/index or a
// clone-cache .index) into the machine-global store, keyed by the
// fingerprint its own manifest records. Move-if-absent: when the target
// store already exists the legacy directory is left untouched and skipped —
// never clobbered, never merged. Returns the target dir and what happened.
func MigrateDir(legacyDir, cacheRoot, repoKey string) (target string, migrated bool, err error) {
	manifest, err := LoadManifest(legacyDir)
	if err != nil {
		return "", false, fmt.Errorf("reading legacy index manifest: %w", err)
	}
	fingerprint := ManifestFingerprint(manifest)
	if fingerprint == "" {
		// Nothing indexed — no store to create; the caller may drop the dir.
		return "", false, nil
	}
	target = StoreDir(cacheRoot, repoKey, fingerprint)
	if _, err := os.Stat(target); err == nil {
		return target, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", false, fmt.Errorf("creating store parent: %w", err)
	}
	if err := os.Rename(legacyDir, target); err != nil {
		// Cross-device rename (repo and cache on different volumes): copy
		// into a sibling temp dir first so a failed copy never leaves a
		// half-populated store at the final path, then swap atomically.
		tmp := target + ".migrating"
		if copyErr := copyTree(legacyDir, tmp); copyErr != nil {
			_ = os.RemoveAll(tmp)
			return "", false, fmt.Errorf("migrating index %s -> %s: %w", legacyDir, target, copyErr)
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.RemoveAll(tmp)
			return "", false, fmt.Errorf("finalizing index migration to %s: %w", target, err)
		}
		if err := os.RemoveAll(legacyDir); err != nil {
			return "", false, fmt.Errorf("removing migrated legacy index %s: %w", legacyDir, err)
		}
	}
	return target, true, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

// lockFile is the advisory lock guarding a store directory. chromem-go
// persists one gob file per document with no coordination of its own, so
// writers must serialize and readers must not load mid-write.
func lockFile(indexDir string) *flock.Flock {
	return flock.New(filepath.Join(indexDir, ".lock"))
}

// lockRetryInterval paces lock acquisition attempts. Sessions are seconds
// to minutes long (embedding round-trips), so a coarse interval is fine.
const lockRetryInterval = 200 * time.Millisecond

// WriteStore runs fn against a freshly loaded store while holding the store's
// exclusive lock — the one write boundary for every index mutation (build,
// lazy-fill, MCP reconcile). The lock is acquired BEFORE the snapshot is
// loaded and held through fn, so a writer never operates on a snapshot that a
// concurrent process has moved on from. Manifest reads and saves belonging to
// the write must happen inside fn so concurrent writers cannot clobber each
// other's manifest state. An empty indexDir runs fn against a fresh in-memory
// store without locking (tests). Blocks until the lock is available or ctx ends.
func WriteStore(ctx context.Context, indexDir string, fn func(*Index) error) error {
	if indexDir == "" {
		return fn(OpenInMemory())
	}
	if err := ensureStoreDir(indexDir); err != nil {
		return err
	}
	l := lockFile(indexDir)
	if _, err := l.TryLockContext(ctx, lockRetryInterval); err != nil {
		return fmt.Errorf("acquiring index write lock at %s: %w", indexDir, err)
	}
	defer func() { _ = l.Unlock() }()
	store, err := loadStore(indexDir)
	if err != nil {
		return err
	}
	if err := fn(store); err != nil {
		return err
	}
	// Bump the write generation only when fn actually mutated the store, and
	// while the exclusive lock is still held, so readers detect the change on
	// their next generation check. A no-op write session (a lazy-fill with
	// nothing to do) leaves the generation untouched, keeping readers' cached
	// snapshots valid.
	if store.dirty {
		if err := bumpGeneration(indexDir); err != nil {
			return fmt.Errorf("bumping store generation at %s: %w", indexDir, err)
		}
	}
	return nil
}

// ReadStore runs fn against a freshly loaded store while holding the store's
// shared lock — the read counterpart to WriteStore. A reader loads its
// snapshot under the lock so it never decodes half-written chromem documents,
// and it opens fresh per call so it reflects writes committed by the CLI or
// another process. An empty indexDir runs fn against a fresh in-memory store
// without locking (tests).
func ReadStore(ctx context.Context, indexDir string, fn func(*Index) error) error {
	if indexDir == "" {
		return fn(OpenInMemory())
	}
	if err := ensureStoreDir(indexDir); err != nil {
		return err
	}
	l := lockFile(indexDir)
	if _, err := l.TryRLockContext(ctx, lockRetryInterval); err != nil {
		return fmt.Errorf("acquiring index read lock at %s: %w", indexDir, err)
	}
	defer func() { _ = l.Unlock() }()
	store, err := loadStore(indexDir)
	if err != nil {
		return err
	}
	return fn(store)
}

// generationPath is the store's write-generation marker — a small counter file
// bumped under the exclusive lock by every mutating write session.
func generationPath(indexDir string) string {
	return filepath.Join(indexDir, "generation")
}

// bumpGeneration increments the store's write-generation marker (creating it on
// first write), written atomically so a torn read never yields a bogus counter.
// Called under the exclusive lock from WriteStore.
func bumpGeneration(indexDir string) error {
	current, _, err := readGenerationMarker(indexDir)
	if err != nil {
		return err
	}
	next := current + 1
	final := generationPath(indexDir)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatUint(next, 10)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// readGenerationMarker reads the explicit generation counter, reporting whether
// the marker file exists.
func readGenerationMarker(indexDir string) (value uint64, present bool, err error) {
	data, err := os.ReadFile(generationPath(indexDir))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parsing generation marker at %s: %w", indexDir, err)
	}
	return n, true, nil
}

// storeGeneration returns a token that changes whenever the store is written.
// It prefers the explicit marker (bumped by every mutating WriteStore); for a
// legacy store that has not been written since the upgrade it falls back to the
// manifest sidecar's identity (size+mtime), which the atomic manifest save
// always changes. An empty store (no marker, no manifest) is generation 0.
//
// The fallback means an unchanged legacy store still caches — any write, which
// rewrites the manifest, invalidates it — and the first local write then
// creates the marker and takes over. A store with neither marker nor manifest
// reloads until the first write, never wrongly reusing a stale snapshot.
func storeGeneration(indexDir string) (uint64, error) {
	if n, present, err := readGenerationMarker(indexDir); err != nil {
		return 0, err
	} else if present {
		return n, nil
	}
	info, err := os.Stat(manifestPath(indexDir))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return uint64(info.ModTime().UnixNano()) ^ (uint64(info.Size()) << 20), nil
}

// SnapshotCache holds a read snapshot and the store generation it was loaded
// at, so a long-running reader answers from memory when the store is unchanged.
// A caller shares one per store directory and guards it with its own mutex —
// SnapshotCache carries no locking of its own.
type SnapshotCache struct {
	index *Index
	gen   uint64
	valid bool
}

// ReadCached runs fn against a store snapshot under the shared lock, reusing
// cache when the on-disk generation is unchanged and loading a fresh snapshot
// otherwise. The generation is read AFTER acquiring the lock and the lock is
// held through fn, so a writer cannot slip between the check and the query and
// go unseen. It reports whether it loaded a fresh snapshot (for reload
// counting). cache must be non-nil; the caller guards it against concurrent use.
func ReadCached(ctx context.Context, indexDir string, cache *SnapshotCache, fn func(*Index) error) (reloaded bool, err error) {
	if indexDir == "" {
		// In-memory (tests): nothing persists, so there is no generation to
		// reuse across calls.
		return true, fn(OpenInMemory())
	}
	if err := ensureStoreDir(indexDir); err != nil {
		return false, err
	}
	l := lockFile(indexDir)
	if _, err := l.TryRLockContext(ctx, lockRetryInterval); err != nil {
		return false, fmt.Errorf("acquiring index read lock at %s: %w", indexDir, err)
	}
	defer func() { _ = l.Unlock() }()
	gen, err := storeGeneration(indexDir)
	if err != nil {
		return false, err
	}
	if !cache.valid || cache.index == nil || cache.gen != gen {
		store, err := loadStore(indexDir)
		if err != nil {
			return false, err
		}
		cache.index, cache.gen, cache.valid = store, gen, true
		reloaded = true
	}
	return reloaded, fn(cache.index)
}
