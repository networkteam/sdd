package command

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model"
)

// InitCmd captures intent to initialize (or refresh) an SDD project. The
// handler is idempotent — running it on a fresh tree creates .sdd/, writes
// config.yaml and meta.json, and installs the embedded skill bundle; running
// it against a tree that's already set up refreshes what's drifted and
// leaves the rest alone.
type InitCmd struct {
	// RepoRoot is the absolute path to the repository root where .sdd/
	// will live.
	RepoRoot string

	// GraphDir is the graph directory path relative to RepoRoot. Empty
	// defaults to model.DefaultGraphDir (".sdd/graph").
	GraphDir string

	// Participant is the canonical author name to record in
	// .sdd/config.local.yaml. Empty means "do not change" — existing
	// values in the local config are preserved (caller resolves the value
	// interactively if a prompt is warranted). The handler writes the
	// field into the mapping without disturbing other keys (e.g. llm:).
	Participant string

	// BinaryVersion is the running sdd binary's version. Stamped into each
	// installed skill file's frontmatter and, on initial init only, used
	// to derive the graph's minimum_version (unless it's a dev build).
	BinaryVersion string

	// Target selects which agent's skills to install. Empty defaults to
	// model.DefaultAgentTarget (Claude).
	Target model.AgentTarget

	// Scope selects user-global vs project-local skill installation. Empty
	// defaults to model.DefaultScope (User).
	Scope model.Scope

	// UserHome is the absolute path to the user's home directory. Required
	// when Scope = User.
	UserHome string

	// Force unconditionally overwrites user-modified skill files,
	// bypassing PromptOverwrite entirely.
	Force bool

	// PromptOverwrite is invoked for each skill file whose on-disk copy
	// has been user-edited. Return true to overwrite, false to preserve.
	// If nil (and Force is false), modified skill files are preserved
	// without prompting.
	PromptOverwrite func(absPath string) (bool, error)

	// --- Fresh-setup callbacks (fire only on initial init) ---

	// OnCreated fires after .sdd/, config.yaml, and the graph dir are
	// freshly created on an empty tree.
	OnCreated func(sddDir, absGraphDir string)

	// OnMigrated fires after .sdd-tmp/ contents are migrated to .sdd/tmp/.
	OnMigrated func(count int)

	// OnGitignoreUpdated fires after .gitignore is updated with .sdd/tmp/.
	OnGitignoreUpdated func(gitignorePath string)

	// --- Always-fire callbacks (both initial and repeat runs) ---

	// OnMetaWritten fires when .sdd/meta.json is created. Does not fire
	// when an existing meta.json is preserved.
	OnMetaWritten func(path string)

	// OnMetaPreserved fires when .sdd/meta.json already existed and was
	// left untouched.
	OnMetaPreserved func(path string)

	// OnParticipantWritten fires when the participant field is added to or
	// updated in .sdd/config.local.yaml. Does not fire when the existing
	// value already matches (idempotent re-init produces no callback).
	OnParticipantWritten func(path, name string)

	// OnSkillsInstalled fires after the skill install pass completes,
	// carrying a per-category summary suitable for presenter output.
	OnSkillsInstalled func(result SkillInstallResult)
}

// Validate checks required fields.
func (c *InitCmd) Validate() error {
	if c.RepoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	return nil
}
