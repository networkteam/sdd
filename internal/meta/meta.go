// Package meta manages the .sdd/ metadata directory: discovery, config
// reading, and path resolution. This is project-level infrastructure,
// separate from the domain model (pure) and graph finders (query-driven).
package meta

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// DiscoverRoot walks up from startDir looking for a directory named ".sdd".
// Returns the repo root (parent of .sdd/) or empty string if not found.
func DiscoverRoot(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, model.SDDDirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root.
			return ""
		}
		dir = parent
	}
}

// SDDDir returns the absolute path to .sdd/ given the repo root.
func SDDDir(repoRoot string) string {
	return filepath.Join(repoRoot, model.SDDDirName)
}

// ReadConfig reads and parses .sdd/config.yaml from the given .sdd directory,
// then overlays any .sdd/config.local.yaml present. Returns nil config with
// nil error if neither file exists. The local file is gitignored, parses as
// the same PerRepoConfig schema, and carries genuinely machine-specific
// overrides (API keys, endpoints).
func ReadConfig(sddDir string) (*model.PerRepoConfig, error) {
	base, err := readConfigFile(filepath.Join(sddDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	overlay, err := readConfigFile(filepath.Join(sddDir, "config.local.yaml"))
	if err != nil {
		return nil, err
	}
	if base == nil && overlay == nil {
		return nil, nil
	}
	return model.MergeConfig(base, overlay), nil
}

// ResolveConfig builds the effective per-repo config from the full overlay:
// the user-global base settings under the committed .sdd/config.yaml under
// .sdd/config.local.yaml. CLI flags apply on top at the call site. Returns
// nil (no config) only when the global base is empty and neither per-repo
// file exists — with global settings present, a repo without config files
// still resolves to them.
func ResolveConfig(global model.BaseConfig, sddDir string) (*model.PerRepoConfig, error) {
	fileCfg, err := ReadConfig(sddDir)
	if err != nil {
		return nil, err
	}
	if fileCfg == nil {
		if global.IsZero() {
			return nil, nil
		}
		return &model.PerRepoConfig{BaseConfig: global}, nil
	}
	return model.MergeConfig(&model.PerRepoConfig{BaseConfig: global}, fileCfg), nil
}

func readConfigFile(path string) (*model.PerRepoConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	cfg, err := model.ParseConfig(data)
	if err != nil {
		// The path names which file carries the offense — the parse error
		// itself already names the key and line.
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// ResolveGraphDir returns the absolute graph directory path from a config
// and the repo root (parent of .sdd/). If cfg is nil or GraphDir is empty,
// falls back to model.DefaultGraphDir.
func ResolveGraphDir(repoRoot string, cfg *model.PerRepoConfig) string {
	graphDir := model.DefaultGraphDir
	if cfg != nil && cfg.GraphDir != "" {
		graphDir = cfg.GraphDir
	}
	if filepath.IsAbs(graphDir) {
		return graphDir
	}
	return filepath.Join(repoRoot, graphDir)
}

// TmpDir returns the path to .sdd/tmp/ given the .sdd/ directory path.
func TmpDir(sddDir string) string {
	return filepath.Join(sddDir, "tmp")
}

// LastFetchPath returns the path to the background-sync last-fetch marker.
// The file's mtime records the last time sdd attempted git fetch; its
// presence short-circuits further fetches within the configured cooldown.
func LastFetchPath(sddDir string) string {
	return filepath.Join(TmpDir(sddDir), "last-fetch")
}

// ReadLastFetch returns the mtime of the last-fetch marker, or the zero
// time if the marker does not exist. A zero result means "never fetched,"
// treat as stale.
func ReadLastFetch(sddDir string) (time.Time, error) {
	info, err := os.Stat(LastFetchPath(sddDir))
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// TouchLastFetch stamps the last-fetch marker with the current time,
// creating the .sdd/tmp/ directory and the file itself as needed. Called
// whether the fetch succeeded or failed — the cooldown applies to attempts,
// not successes, so transient offline periods don't re-fetch per command.
func TouchLastFetch(sddDir string) error {
	if err := os.MkdirAll(TmpDir(sddDir), 0o755); err != nil {
		return err
	}
	path := LastFetchPath(sddDir)
	now := time.Now()
	if _, err := os.Stat(path); err == nil {
		return os.Chtimes(path, now, now)
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

// IsSDDMetaDir returns true if the directory entry is the .sdd metadata
// directory. Used by graph scanning to skip .sdd/ contents.
func IsSDDMetaDir(d fs.DirEntry) bool {
	return d.IsDir() && d.Name() == model.SDDDirName
}
