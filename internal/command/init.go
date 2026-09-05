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

	// StableRepoRoot is the Git common-directory identity invariant across
	// linked worktrees, used only for identity-less machine-global store keys.
	// Empty falls back to RepoRoot for non-Git callers and tests.
	StableRepoRoot string

	// GraphDir is the graph directory path relative to RepoRoot. Empty
	// defaults to model.DefaultGraphDir (".sdd/graph").
	GraphDir string

	// DefaultBranch is the concrete Git branch persisted for ordinary engine
	// captures on fresh initialization or when upgrading an older config.
	DefaultBranch string

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

	// RemoteURL is the repo's git remote URL (typically origin), resolved
	// by the caller. When it derives a canonical repo identity, init
	// records `repo_id` in .sdd/config.yaml — on fresh init and as an
	// upsert on configs that predate the field. Empty (no remote) or an
	// underivable URL leaves the repo local-only; the value is never
	// user-choosable.
	RemoteURL string

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

	// OnIndexMigrated fires per legacy index dir (in-tree .sdd/index or a
	// clone cache's .index) the init pass handled: moved=true when it was
	// moved into the machine-global store at storeDir, moved=false when a
	// store already existed there and the legacy dir was left in place for
	// manual removal (never clobbered, never merged).
	OnIndexMigrated func(legacyDir, storeDir string, moved bool)

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

	// OnRepoIDWritten fires when a derived repo_id is recorded in
	// .sdd/config.yaml — on fresh init or as an upgrade upsert. Does not
	// fire when the config already carries a value.
	OnRepoIDWritten func(repoID string)

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

	// OnSkillOrphansPruned fires once per rendered agent whose install
	// directory held files the bundle no longer carries, reporting what was
	// removed and which user-modified copies were preserved. Does not fire
	// when the sweep found nothing.
	OnSkillOrphansPruned func(result AgentPruneResult)

	// OnMCPRegistered fires for each project-scope config file written to
	// register the SDD MCP server for an agent (a fresh file or an
	// add-if-missing merge). Does not fire when an existing sdd entry is
	// left untouched.
	OnMCPRegistered func(target model.AgentTarget, path string)
}

// AgentPruneResult reports the outcome of a prune pass over one agent's
// install directory — a dropped agent's whole render, or the orphans a
// still-rendered agent's bundle no longer carries: which files were removed
// and which user-modified files were kept (removable only under --force).
type AgentPruneResult struct {
	Target       model.AgentTarget
	InstallDir   string
	Removed      []string
	KeptModified []string
}

// TouchedAnything reports whether the pass found something worth telling the
// user about — a prune that matched nothing stays silent.
func (r AgentPruneResult) TouchedAnything() bool {
	return len(r.Removed) > 0 || len(r.KeptModified) > 0
}

// Validate checks required fields.
func (c *InitCmd) Validate() error {
	if c.RepoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	return nil
}
