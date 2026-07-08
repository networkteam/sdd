package repos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/networkteam/slogutils"
)

// Git is the narrow git surface the cache lifecycle needs. The production
// implementation is internal/git's exec adapter; tests inject fakes.
type Git interface {
	// Clone clones url into dir.
	Clone(ctx context.Context, url, dir string) error
	// PullFFOnly pulls the checkout at dir, fast-forward only — the cache is
	// read-only, so a non-ff state means the remote rewrote history; surface
	// that rather than merging.
	PullFFOnly(ctx context.Context, dir string) error
}

// Manager owns the side-effectful half of the connected-repos machinery:
// cache clone and pull, and config writes. It wraps the pure Registry and is
// the dependency handlers get injected — the write-side counterpart to the
// finder-injected Registry.
type Manager struct {
	reg *Registry
	git Git
}

// NewManager builds a Manager over the registry and the git dependency.
func NewManager(reg *Registry, git Git) *Manager {
	return &Manager{reg: reg, git: git}
}

// Registry exposes the pure read surface, so a Manager-holding handler reads
// through the same paths the finders do.
func (m *Manager) Registry() *Registry {
	return m.reg
}

// Save writes the user-global config to the registry's config path.
func (m *Manager) Save(cfg *GlobalConfig) error {
	return SaveConfigTo(m.reg.loc.ConfigPath, cfg)
}

// EnsureCloned clones repo.CloneURL into dir when no clone is present yet.
// Reports whether a clone ran (false: the cache already existed).
func (m *Manager) EnsureCloned(ctx context.Context, repo ConnectedRepo, dir string) (bool, error) {
	if IsCloned(dir) {
		return false, nil
	}
	logger := slogutils.FromContext(ctx)
	logger.Info("cloning connected repo", "repo", repo.RepoID, "url", repo.CloneURL)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return false, fmt.Errorf("creating cache dir: %w", err)
	}
	if err := m.git.Clone(ctx, repo.CloneURL, dir); err != nil {
		return false, fmt.Errorf("cloning %s: %w", repo.RepoID, err)
	}
	touchLastPull(dir)
	return true, nil
}

// CooldownPull pulls the cache when the last attempt is older than cooldown.
// Reports whether a pull ran. The marker is touched on every attempt —
// cooldown bounds attempts, not successes — but a failed pull still returns
// its error (fail loud; the caller decides whether stale reads may proceed).
func (m *Manager) CooldownPull(ctx context.Context, dir string, cooldown time.Duration) (bool, error) {
	if !IsCloned(dir) {
		return false, fmt.Errorf("no clone at %s", dir)
	}
	if last, ok := lastPull(dir); ok && time.Since(last) < cooldown {
		return false, nil
	}
	return true, m.ForcePull(ctx, dir)
}

// ForcePull pulls the cache unconditionally (fast-forward only).
func (m *Manager) ForcePull(ctx context.Context, dir string) error {
	touchLastPull(dir)
	logger := slogutils.FromContext(ctx)
	logger.Debug("pulling connected repo cache", "dir", dir)
	if err := m.git.PullFFOnly(ctx, dir); err != nil {
		return fmt.Errorf("pulling cache %s: %w", dir, err)
	}
	return nil
}
