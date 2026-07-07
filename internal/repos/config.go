// Package repos holds the connected-repos machinery for cross-repo
// references: the user-global configuration naming which repositories this
// user is connected to, and the managed read-only clone caches those
// connections resolve against. Everything here is plain functions — side
// effects (clone, pull, index builds) are orchestrated by handlers that
// call into this package and invalidate the read side's GraphSource on
// completion.
package repos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/model"
)

// GlobalConfig is the user-global SDD configuration at
// ~/.config/sdd/config.yaml (XDG): the connected repos and the single
// shared embedding config that defines the one vector space cross-graph
// search merges over. Hand-editable YAML underneath.
type GlobalConfig struct {
	Repos []ConnectedRepo `yaml:"repos,omitempty"`
	// Embedding is the global embedder every connected repo's index is
	// built with — one vector space so cosine scores are comparable across
	// repos. A repo indexed under a different fingerprint is excluded from
	// cross-graph search and flagged by lint.
	Embedding model.EmbeddingConfig `yaml:"embedding,omitempty"`
}

// ConnectedRepo names one connection: the canonical repo identity and the
// URL its cache clones from. CloneURL is per-user (ssh vs https), RepoID is
// identical for everyone (declared in the target repo's committed config).
type ConnectedRepo struct {
	RepoID   string `yaml:"repo_id"`
	CloneURL string `yaml:"clone_url"`
}

// ConfigPath resolves the user-global config location: $XDG_CONFIG_HOME/sdd/config.yaml,
// defaulting to ~/.config/sdd/config.yaml. Implemented against the XDG
// convention directly (not os.UserConfigDir) so the path is uniform across
// platforms and matches the documented location.
func ConfigPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "sdd", "config.yaml"), nil
}

// LoadConfig reads the user-global config from the default path. A missing
// file is not an error — it yields an empty config (no connections).
func LoadConfig() (*GlobalConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadConfigFrom(path)
}

// LoadConfigFrom reads a user-global config from an explicit path.
func LoadConfigFrom(path string) (*GlobalConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &GlobalConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading global config %s: %w", path, err)
	}
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing global config %s: %w", path, err)
	}
	return &cfg, nil
}

// SaveConfig writes the user-global config to the default path, creating
// parent directories as needed. 0600 — the embedding block may carry API keys.
func SaveConfig(cfg *GlobalConfig) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return SaveConfigTo(path, cfg)
}

// SaveConfigTo writes a user-global config to an explicit path.
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
