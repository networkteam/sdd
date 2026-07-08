// Package repos holds the connected-repos machinery for cross-repo
// references: the user-global configuration naming which repositories this
// user is connected to, and the managed read-only clone caches those
// connections resolve against. The package reads no ambient state — the
// config path and cache root arrive as an explicit Locations value, resolved
// once at the composition root (repos.DefaultLocations for the XDG
// convention). Pure reads live on Registry; the side-effectful cache
// lifecycle (clone, pull, config save) lives on Manager, orchestrated by
// handlers that invalidate the read side's GraphSource on completion.
package repos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/model"
)

// Locations names the user-global places the connected-repos machinery works
// against: the config file and the cache root. Resolved once at the
// composition root and passed down explicitly — tests point both at temp
// dirs, no environment involved.
type Locations struct {
	// ConfigPath is the user-global config file
	// (default $XDG_CONFIG_HOME/sdd/config.yaml).
	ConfigPath string
	// CacheRoot is the base directory for connected-repo clone caches
	// (default $XDG_CACHE_HOME/sdd).
	CacheRoot string
}

// DefaultLocations resolves the conventional locations: the XDG base
// directories, defaulting to ~/.config and ~/.cache. Implemented against the
// XDG convention directly (not os.UserConfigDir) so the paths are uniform
// across platforms and match the documented locations. This is the only
// place the package touches the environment — called by composition roots,
// never by library code.
func DefaultLocations() (Locations, error) {
	cfgBase := os.Getenv("XDG_CONFIG_HOME")
	cacheBase := os.Getenv("XDG_CACHE_HOME")
	if cfgBase == "" || cacheBase == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Locations{}, fmt.Errorf("resolving home dir: %w", err)
		}
		if cfgBase == "" {
			cfgBase = filepath.Join(home, ".config")
		}
		if cacheBase == "" {
			cacheBase = filepath.Join(home, ".cache")
		}
	}
	return Locations{
		ConfigPath: filepath.Join(cfgBase, "sdd", "config.yaml"),
		CacheRoot:  filepath.Join(cacheBase, "sdd"),
	}, nil
}

// GlobalConfig is the user-global SDD configuration. It embeds the shared
// user/machine settings (model.BaseConfig — participant, llm, embedding,
// sync) that seed the config overlay for every repo on this machine; its
// embedding config doubles as the global embedder every connected repo's
// index is built with — one vector space so cosine scores are comparable
// across repos (a repo indexed under a different fingerprint is excluded
// from cross-graph search and flagged by lint). Hand-editable YAML
// underneath; per-repo-only fields (repo_id above all) do not exist on this
// schema, so a misplaced one is a parse error.
type GlobalConfig struct {
	model.BaseConfig `yaml:",inline"`

	// Repos is the per-user resolution of connected repositories: for each
	// repo_id, how this machine reaches it (clone URL, homedir cache). The
	// committed per-repo `dependencies` list declares what a graph needs;
	// this list resolves how — which is why it is global-only.
	Repos []ConnectedRepo `yaml:"repos,omitempty"`
}

// ConnectedRepo names one connection: the canonical repo identity and the
// URL its cache clones from. CloneURL is per-user (ssh vs https), RepoID is
// identical for everyone (declared in the target repo's committed config).
type ConnectedRepo struct {
	RepoID   string `yaml:"repo_id"`
	CloneURL string `yaml:"clone_url"`
}

// LoadConfigFrom reads a user-global config from an explicit path. A missing
// file is not an error — it yields an empty config (no connections). Unknown
// keys are an error: a setting placed in the wrong file must surface at load
// time, never be silently dropped (d-cpt-6cq's fail-loud rule).
func LoadConfigFrom(path string) (*GlobalConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &GlobalConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading global config %s: %w", path, err)
	}
	var cfg GlobalConfig
	if err := model.StrictUnmarshalYAML(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing global config %s: %w", path, err)
	}
	return &cfg, nil
}

// SaveConfigTo writes a user-global config to an explicit path, creating
// parent directories as needed. 0600 — the embedding block may carry API keys.
func SaveConfigTo(path string, cfg *GlobalConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling global config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing global config %s: %w", path, err)
	}
	return nil
}

// Connected returns the connection for repoID, if present.
func (c *GlobalConfig) Connected(repoID string) (ConnectedRepo, bool) {
	for _, r := range c.Repos {
		if r.RepoID == repoID {
			return r, true
		}
	}
	return ConnectedRepo{}, false
}

// AddRepo registers a connection. A connection with the same repo_id must
// not already exist — re-registering is a remove + add, never a silent
// overwrite.
func (c *GlobalConfig) AddRepo(repo ConnectedRepo) error {
	if err := model.ValidateRepoID(repo.RepoID); err != nil {
		return err
	}
	if repo.CloneURL == "" {
		return fmt.Errorf("connected repo %q needs a clone_url", repo.RepoID)
	}
	if _, exists := c.Connected(repo.RepoID); exists {
		return fmt.Errorf("repo %q is already connected", repo.RepoID)
	}
	c.Repos = append(c.Repos, repo)
	return nil
}

// RemoveRepo drops the connection for repoID, reporting whether it existed.
func (c *GlobalConfig) RemoveRepo(repoID string) bool {
	for i, r := range c.Repos {
		if r.RepoID == repoID {
			c.Repos = append(c.Repos[:i], c.Repos[i+1:]...)
			return true
		}
	}
	return false
}

// UnconnectedDependencies returns the declared dependencies (committed
// repo_ids) that have no connection in this user-global config — the gap
// `sdd init` reports after a fresh clone, since the clone_url half of a
// connection is per-user and cannot ride in the committed declaration.
func (c *GlobalConfig) UnconnectedDependencies(deps []string) []string {
	var missing []string
	for _, dep := range deps {
		if _, ok := c.Connected(dep); !ok {
			missing = append(missing, dep)
		}
	}
	return missing
}

// SelectRepoIDs resolves a caller's repo selection against the connected
// set: all=true means every connected repo; an explicitly named repo that
// is not connected is an error — silent narrowing would misreport
// coverage. An empty selection resolves to nil.
func (c *GlobalConfig) SelectRepoIDs(named []string, all bool) ([]string, error) {
	if !all && len(named) == 0 {
		return nil, nil
	}
	if all {
		ids := make([]string, 0, len(c.Repos))
		for _, r := range c.Repos {
			ids = append(ids, r.RepoID)
		}
		return ids, nil
	}
	for _, id := range named {
		if _, ok := c.Connected(id); !ok {
			return nil, fmt.Errorf("repo %q is not connected — add it with `sdd repo add <clone-url>`", id)
		}
	}
	return named, nil
}
