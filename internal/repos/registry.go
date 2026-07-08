package repos

import (
	"path/filepath"

	"github.com/networkteam/sdd/internal/model"
)

// Registry is the pure read surface over the connected-repos state: which
// repos are connected and where their caches live. It is the dependency the
// read side (finders) gets injected — it carries no way to clone, pull, or
// save, so finder purity is enforced by the type, not by discipline. The
// side-effectful lifecycle lives on Manager.
type Registry struct {
	loc Locations
}

// NewRegistry builds a Registry over explicit locations. Composition roots
// pass DefaultLocations(); tests pass temp dirs.
func NewRegistry(loc Locations) *Registry {
	return &Registry{loc: loc}
}

// Load reads the user-global config. Deliberately lazy — read per call, not
// snapshotted at construction — so a long-lived process (the MCP server)
// sees a repo connected from another terminal without a restart. A missing
// file yields an empty config.
func (r *Registry) Load() (*GlobalConfig, error) {
	return LoadConfigFrom(r.loc.ConfigPath)
}

// ConfigPath returns the user-global config file path. Exposed for the
// comment-preserving `sdd config set` upsert, which patches the file bytes
// in place rather than round-tripping through Save (which would drop
// comments).
func (r *Registry) ConfigPath() string {
	return r.loc.ConfigPath
}

// CacheRoot is the base directory for connected-repo clone caches.
func (r *Registry) CacheRoot() string {
	return r.loc.CacheRoot
}

// CacheDir resolves a connected repo's clone location under the cache root.
// The repo-id's host/path shape nests naturally under the root.
func (r *Registry) CacheDir(repoID string) (string, error) {
	if err := model.ValidateRepoID(repoID); err != nil {
		return "", err
	}
	return filepath.Join(r.loc.CacheRoot, filepath.FromSlash(repoID)), nil
}

// SelectRepoIDs resolves a caller's repo selection against the connected
// set (see GlobalConfig.SelectRepoIDs).
func (r *Registry) SelectRepoIDs(named []string, all bool) ([]string, error) {
	if !all && len(named) == 0 {
		return nil, nil
	}
	cfg, err := r.Load()
	if err != nil {
		return nil, err
	}
	return cfg.SelectRepoIDs(named, all)
}
