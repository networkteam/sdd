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

	// Language is the graph authoring language (locale code) to record in
	// .sdd/config.yaml on fresh init. Empty means "use the default behavior"
	// — FormatConfig keeps the commented hint, so the graph stays English.
	// Only applied on fresh init; existing config.yaml is never rewritten.
	Language string

	// BinaryVersion is the running sdd binary's version. Stamped into each
	// installed skill file's frontmatter and, on initial init only, used
	// to derive the graph's minimum_version (unless it's a dev build).
	BinaryVersion string

	// Targets lists the agent profiles to render and install. Empty defaults
	// to the value resolved from supported_agents in .sdd/config.yaml (or
	// model.DefaultSupportedAgents on a fresh tree). The handler renders each
	// listed target into its own scope directory.
	Targets []model.AgentTarget

	// Scope selects user-global vs project-local skill installation. Empty
	// defaults to model.DefaultScope (User). When ScopeExplicit is false the
	// handler treats Scope as a fallback and prefers any value already
	// recorded under skill_scope in .sdd/config.yaml; when true the handler
	// rejects a value that contradicts the recorded scope (AC 2).
	Scope model.Scope

	// ScopeExplicit reports whether the caller passed --scope on the
	// command line versus letting it default. Used at the handler boundary
	// to distinguish "user opted in to this scope" from "no choice has
	// been made yet" — the contradiction check (AC 2) only applies when
	// the operator typed a value.
	ScopeExplicit bool

	// UserHome is the absolute path to the user's home directory. Required
	// when Scope = User.
	UserHome string

	// Force unconditionally overwrites user-modified skill files,
	// bypassing PromptOverwrite entirely.
	Force bool

	// Bump asks the handler to raise .sdd/meta.json's minimum_version to
	// the running binary's version after the regular init flow runs. Dev
	// builds are rejected with the fixed message defined on
	// BumpMinimumVersionCmd; equal versions are a no-op.
	Bump bool

	// OnMinimumVersionBumped fires when minimum_version was raised by a
	// `sdd init --bump` invocation, carrying the previous value (empty
	// when no floor was recorded) and the new value.
	OnMinimumVersionBumped func(previous, current string)

	// OnMinimumVersionUnchanged fires when --bump was passed but the
	// binary version already matched the recorded minimum (no-op).
	OnMinimumVersionUnchanged func(current string)

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

	// OnBridgeScaffolded fires when the AGENTS.md / CLAUDE.md instruction
	// bridge was created from scratch, carrying the absolute paths written.
	// Only fires for files that were absent — existing files are never
	// overwritten, so a repeat init or a project with its own files
	// produces no callback.
	OnBridgeScaffolded func(paths []string)

	// OnAgentSkillsPruned fires once per agent that an explicit --agents
	// selection dropped from the recorded set, reporting the rendered skill
	// files removed and any user-modified files preserved.
	OnAgentSkillsPruned func(result AgentPruneResult)
}

// AgentPruneResult reports the outcome of pruning a dropped agent's rendered
// skills: which files were removed and which user-modified files were kept
// (removable only under --force).
type AgentPruneResult struct {
	Target       model.AgentTarget
	InstallDir   string
	Removed      []string
	KeptModified []string
}

// Validate checks required fields.
func (c *InitCmd) Validate() error {
	if c.RepoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	return nil
}
