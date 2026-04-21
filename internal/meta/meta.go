// Package meta manages the .sdd/ metadata directory: discovery, config
// reading, and path resolution. This is project-level infrastructure,
// separate from the domain model (pure) and graph finders (query-driven).
package meta

import (
	"io/fs"
	"os"
	"path/filepath"

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
// nil error if neither file exists. The local file is gitignored and carries
// per-machine overrides (API keys, Ollama endpoint, participant name).
func ReadConfig(sddDir string) (*model.Config, error) {
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

func readConfigFile(path string) (*model.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return model.ParseConfig(data)
}

// ResolveGraphDir returns the absolute graph directory path from a config
// and the repo root (parent of .sdd/). If cfg is nil or GraphDir is empty,
// falls back to model.DefaultGraphDir.
func ResolveGraphDir(repoRoot string, cfg *model.Config) string {
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

// IsSDDMetaDir returns true if the directory entry is the .sdd metadata
// directory. Used by graph scanning to skip .sdd/ contents.
func IsSDDMetaDir(d fs.DirEntry) bool {
	return d.IsDir() && d.Name() == model.SDDDirName
}
