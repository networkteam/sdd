package repos

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// Connected-repo caches are managed read-only clones under the Locations
// cache root (default $XDG_CACHE_HOME/sdd/<repo-id>/): lazy clone on first
// need, cooldown-gated pull on reads, forced pull via `sdd repo sync` — all
// lifecycle on Manager. The helpers here are pure reads over a given cache
// dir; none touch ambient state. The clone is never written to except the
// per-repo search index at <dir>/.index/.

// lastPullMarker sits inside the clone's .git dir (never the worktree) and
// carries the time of the last pull attempt as its mtime. Touched on every
// attempt, success or not — the cooldown bounds attempts, not successes,
// mirroring the local graph's last-fetch marker.
const lastPullMarker = "sdd-last-pull"

// DefaultPullCooldown gates how often a read triggers a cache pull.
const DefaultPullCooldown = 15 * time.Minute

// IsCloned reports whether the cache dir holds a git clone.
func IsCloned(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// DeclaredRepoID reads the repo_id the cached repo declares in its committed
// .sdd/config.yaml. Verification for `sdd repo add`: the connection is only
// valid when the target declares the identity the caller registers it under.
func DeclaredRepoID(dir string) (string, error) {
	cfg, err := readCachedConfig(dir)
	if err != nil {
		return "", err
	}
	if cfg == nil || cfg.RepoID == "" {
		return "", fmt.Errorf("repo at %s declares no repo_id in .sdd/config.yaml — run `sdd init` there (or add repo_id) first", dir)
	}
	return cfg.RepoID, nil
}

// GraphDir resolves the cached repo's graph directory from its committed
// config, defaulting to the conventional location.
func GraphDir(dir string) (string, error) {
	cfg, err := readCachedConfig(dir)
	if err != nil {
		return "", err
	}
	graphDir := model.DefaultGraphDir
	if cfg != nil && cfg.GraphDir != "" {
		graphDir = cfg.GraphDir
	}
	return filepath.Join(dir, filepath.FromSlash(graphDir)), nil
}

// IndexDir is the per-repo search index location inside the cache. Distinct
// from the local repo's .sdd/index/ — the cache worktree is otherwise
// untouched.
func IndexDir(dir string) string {
	return filepath.Join(dir, ".index")
}

// readCachedConfig reads the cached repo's committed .sdd/config.yaml.
// A missing file returns nil without error — the cache may point at a repo
// that was never initialized; callers decide whether that matters.
func readCachedConfig(dir string) (*model.PerRepoConfig, error) {
	data, err := os.ReadFile(filepath.Join(dir, model.SDDDirName, "config.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cached repo config: %w", err)
	}
	return model.ParseConfig(data)
}

func lastPull(dir string) (time.Time, bool) {
	info, err := os.Stat(filepath.Join(dir, ".git", lastPullMarker))
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

func touchLastPull(dir string) {
	path := filepath.Join(dir, ".git", lastPullMarker)
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		// Marker missing — create it. A failure here only widens pull
		// frequency, never blocks the pull itself.
		_ = os.WriteFile(path, nil, 0o644)
	}
}
