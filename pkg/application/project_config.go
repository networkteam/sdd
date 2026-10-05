package application

import (
	"io/fs"

	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
)

// ProjectConfig is the committed .sdd/config.yaml as a composition reads it:
// the identity, horizon, and layout facts every checkout shares, never the
// machine-local overlay.
type ProjectConfig struct {
	RepoID        RepoID
	Dependencies  []RepoID
	DefaultBranch string
	Language      string
	// GraphDir is repository-relative, .sdd/graph when the file leaves it unset.
	GraphDir string
}

// ErrNotAnSDDProject reports a tree without a committed .sdd/config.yaml.
var ErrNotAnSDDProject = meta.ErrNotAnSDDProject

// ReadProjectConfigFS reads the committed configuration from a repository
// root. It shares the reader with the CLI, so both read one schema; a
// composition that also needs the local overlay or the tool settings is
// holding a checkout of its own and uses the CLI.
func ReadProjectConfigFS(fsys fs.FS) (ProjectConfig, error) {
	cfg, err := meta.ReadCommittedConfigFS(fsys)
	if err != nil {
		return ProjectConfig{}, err
	}
	graphDir := cfg.GraphDir
	if graphDir == "" {
		graphDir = model.DefaultGraphDir
	}
	return ProjectConfig{
		RepoID: cfg.RepoID, Dependencies: append([]RepoID(nil), cfg.Dependencies...),
		DefaultBranch: cfg.DefaultBranch, Language: cfg.Language, GraphDir: graphDir,
	}, nil
}
