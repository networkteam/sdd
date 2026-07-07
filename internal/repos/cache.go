package repos

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/model"
)

// Connected-repo caches are managed read-only clones at
// $XDG_CACHE_HOME/sdd/<repo-id>/ (default ~/.cache/sdd/<repo-id>/): lazy
// clone on first need, cooldown-gated pull on reads, forced pull via
// `sdd repo sync`. The repo-id's host/path shape nests naturally under the
// cache root. The clone is never written to except the per-repo search
// index at <dir>/.index/.

// lastPullMarker sits inside the clone's .git dir (never the worktree) and
// carries the time of the last pull attempt as its mtime. Touched on every
// attempt, success or not — the cooldown bounds attempts, not successes,
// mirroring the local graph's last-fetch marker.
const lastPullMarker = "sdd-last-pull"

// DefaultPullCooldown gates how often a read triggers a cache pull.
const DefaultPullCooldown = 15 * time.Minute

// CacheRoot resolves the cache base: $XDG_CACHE_HOME/sdd, defaulting to
// ~/.cache/sdd.
func CacheRoot() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home dir: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "sdd"), nil
}

// CacheDir resolves a connected repo's clone location under the cache root.
func CacheDir(repoID string) (string, error) {
	if err := model.ValidateRepoID(repoID); err != nil {
		return "", err
	}
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(repoID)), nil
}

// IsCloned reports whether the cache dir holds a git clone.
func IsCloned(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// EnsureCloned clones repo.CloneURL into dir when no clone is present yet.
// Reports whether a clone ran (false: the cache already existed).
func EnsureCloned(ctx context.Context, repo ConnectedRepo, dir string) (bool, error) {
	if IsCloned(dir) {
		return false, nil
	}
	logger := slogutils.FromContext(ctx)
	logger.Info("cloning connected repo", "repo", repo.RepoID, "url", repo.CloneURL)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return false, fmt.Errorf("creating cache dir: %w", err)
	}
	if err := runGit(ctx, "", "clone", "--quiet", repo.CloneURL, dir); err != nil {
		return false, fmt.Errorf("cloning %s: %w", repo.RepoID, err)
	}
	touchLastPull(dir)
	return true, nil
}

// CooldownPull pulls the cache when the last attempt is older than cooldown.
// Reports whether a pull ran. The marker is touched on every attempt —
// cooldown bounds attempts, not successes — but a failed pull still returns
// its error (fail loud; the caller decides whether stale reads may proceed).
func CooldownPull(ctx context.Context, dir string, cooldown time.Duration) (bool, error) {
	if !IsCloned(dir) {
		return false, fmt.Errorf("no clone at %s", dir)
	}
	if last, ok := lastPull(dir); ok && time.Since(last) < cooldown {
		return false, nil
	}
	return true, ForcePull(ctx, dir)
}

// ForcePull pulls the cache unconditionally (fast-forward only — the cache
// is read-only, so a non-ff state means the remote rewrote history; surface
// that rather than merging).
func ForcePull(ctx context.Context, dir string) error {
	touchLastPull(dir)
	logger := slogutils.FromContext(ctx)
	logger.Debug("pulling connected repo cache", "dir", dir)
	if err := runGit(ctx, dir, "pull", "--quiet", "--ff-only"); err != nil {
		return fmt.Errorf("pulling cache %s: %w", dir, err)
	}
	return nil
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
func readCachedConfig(dir string) (*model.Config, error) {
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

// runGit executes a git command, in dir when non-empty, folding stderr into
// the returned error so failures carry git's own message.
func runGit(ctx context.Context, dir string, args ...string) error {
	verb := args[0]
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		if len(msg) > 0 {
			return fmt.Errorf("git %s: %w: %s", verb, err, msg)
		}
		return fmt.Errorf("git %s: %w", verb, err)
	}
	return nil
}
