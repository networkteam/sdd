package application

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/networkteam/sdd/internal/model"
)

// ProjectConfig is the committed .sdd/config.yaml as a composition reads it:
// the identity, horizon, and layout facts every checkout shares, never the
// machine-local overlay.
type ProjectConfig struct {
	RepoID        string
	Dependencies  []string
	DefaultBranch string
	Language      string
	// GraphDir is repository-relative, .sdd/graph when the file leaves it unset.
	GraphDir string
}

// ErrNotAnSDDProject reports a tree without a committed .sdd/config.yaml.
var ErrNotAnSDDProject = errors.New("sdd: no .sdd/config.yaml in the tree")

// ReadProjectConfigFS reads the committed configuration from a repository
// root. It shares the parser with the CLI's own config resolution, so both
// read one schema; a composition that also needs the local overlay or the
// tool settings is holding a checkout of its own and uses the CLI.
func ReadProjectConfigFS(fsys fs.FS) (ProjectConfig, error) {
	raw, err := fs.ReadFile(fsys, path.Join(model.SDDDirName, "config.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return ProjectConfig{}, ErrNotAnSDDProject
	}
	if err != nil {
		return ProjectConfig{}, err
	}
	cfg, err := model.ParseConfig(raw)
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("sdd: .sdd/config.yaml: %w", err)
	}
	graphDir := cfg.GraphDir
	if graphDir == "" {
		graphDir = model.DefaultGraphDir
	}
	return ProjectConfig{
		RepoID: cfg.RepoID, Dependencies: append([]string(nil), cfg.Dependencies...),
		DefaultBranch: cfg.DefaultBranch, Language: cfg.Language, GraphDir: graphDir,
	}, nil
}
