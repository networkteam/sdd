package command

import "fmt"

// InitCmd captures intent to initialize the .sdd/ metadata directory
// at a repository root. The handler creates the directory structure,
// writes config.yaml, updates .gitignore, and optionally migrates
// legacy .sdd-tmp/ contents.
type InitCmd struct {
	// RepoRoot is the absolute path to the repository root where .sdd/
	// will be created.
	RepoRoot string

	// GraphDir is the graph directory path relative to RepoRoot.
	// Empty defaults to model.DefaultGraphDir (".sdd/graph").
	GraphDir string

	// OnCreated fires after the .sdd/ directory, config.yaml, and graph
	// dir are successfully created. Receives the absolute path to .sdd/
	// and the absolute graph directory path.
	OnCreated func(sddDir, absGraphDir string)

	// OnMigrated fires after .sdd-tmp/ contents are migrated to .sdd/tmp/.
	// Receives the count of migrated files.
	OnMigrated func(count int)

	// OnGitignoreUpdated fires after .gitignore is updated with .sdd/tmp/.
	OnGitignoreUpdated func(gitignorePath string)
}

// Validate checks required fields.
func (c *InitCmd) Validate() error {
	if c.RepoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	return nil
}
