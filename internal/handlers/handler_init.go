package handlers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Init executes an InitCmd. The operation is idempotent:
//
//   - On an empty tree, it creates .sdd/, writes config.yaml and meta.json,
//     creates the graph dir, updates .gitignore, and installs the embedded
//     skill bundle.
//   - On an existing tree, it leaves config.yaml and meta.json alone,
//     ensures the expected directories are in place, and runs the skill
//     install pass to refresh whatever's drifted (user-modified files are
//     routed through cmd.PromptOverwrite).
//
// The skill install step and the meta-write step are implemented as nested
// commands (h.InstallSkills and h.WriteSchemaMeta), keeping each side
// effect in its own handler method.
func (h *Handler) Init(ctx context.Context, cmd *command.InitCmd) error {
	log := slogutils.FromContext(ctx)
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	graphDir := cmd.GraphDir
	if graphDir == "" {
		graphDir = model.DefaultGraphDir
	}

	sddDir := filepath.Join(cmd.RepoRoot, model.SDDDirName)
	absGraphDir := filepath.Join(cmd.RepoRoot, graphDir)
	tmpDir := filepath.Join(sddDir, "tmp")
	configPath := filepath.Join(sddDir, "config.yaml")
	gitignorePath := filepath.Join(cmd.RepoRoot, ".gitignore")

	sddExisted, err := pathExists(sddDir)
	if err != nil {
		return err
	}

	// Reconcile skill_scope before any filesystem mutation so a contradiction
	// fails the run before partial state lands. recordedScope is read from
	// the existing config.yaml when present; effectiveScope is what the
	// install pass uses; persistScope is true when we need to write the
	// chosen value back to config (fresh init or upgrade from a config that
	// predates the field).
	effectiveScope, persistScope, err := resolveSkillScope(sddExisted, configPath, cmd.Scope, cmd.ScopeExplicit)
	if err != nil {
		return err
	}

	// Resolve which agent profiles to render. On a fresh tree this is the
	// caller's selection (or the Claude-only default), persisted by the config
	// write below. On an existing tree an explicit --agents selection replaces
	// and persists the recorded list (d-tac-jin); recordedAgents lets us prune
	// the renders of any agent the selection drops.
	effectiveAgents, recordedAgents, persistAgents, err := resolveSupportedAgents(sddExisted, configPath, cmd.Targets)
	if err != nil {
		return err
	}

	// Tracks paths we touch so the final git commit covers exactly what
	// changed. A repeat init that changes nothing yields no commit.
	var touched []string

	// Derive the canonical repo identity from the remote (ssh and https
	// forms normalize equal). No remote or an underivable URL leaves the
	// repo local-only — a legitimate state, not an error.
	var derivedRepoID string
	if cmd.RemoteURL != "" {
		if id, err := model.DeriveRepoID(cmd.RemoteURL); err == nil {
			derivedRepoID = id
		} else {
			log.Debug("remote URL derives no repo identity; staying local-only", "remote", cmd.RemoteURL, "err", err)
		}
	}

	if !sddExisted {
		if err := os.MkdirAll(sddDir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", sddDir, err)
		}
		if err := os.WriteFile(configPath, []byte(model.FormatConfig(model.PerRepoConfig{GraphDir: graphDir, RepoID: derivedRepoID, Language: cmd.Language, SkillScope: effectiveScope, SupportedAgents: effectiveAgents})), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", configPath, err)
		}
		touched = append(touched, configPath)
		if derivedRepoID != "" && cmd.OnRepoIDWritten != nil {
			cmd.OnRepoIDWritten(derivedRepoID)
		}

		if err := os.MkdirAll(absGraphDir, 0o755); err != nil {
			return fmt.Errorf("creating graph dir %s: %w", absGraphDir, err)
		}
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			return fmt.Errorf("creating tmp dir %s: %w", tmpDir, err)
		}

		if cmd.OnCreated != nil {
			cmd.OnCreated(sddDir, absGraphDir)
		}

		// Best-effort migration from the pre-.sdd/ tmp location; harmless
		// no-op when the legacy directory doesn't exist.
		oldTmpDir := filepath.Join(absGraphDir, ".sdd-tmp")
		if migrated := migrateOldTmpDir(oldTmpDir, tmpDir); migrated > 0 {
			if cmd.OnMigrated != nil {
				cmd.OnMigrated(migrated)
			}
		}
		if err := cleanOldGraphDirGitignore(absGraphDir); err != nil {
			log.Warn("could not clean old graph dir .gitignore", "graphDir", absGraphDir, "err", err)
		}
	} else {
		// Existing .sdd/: ensure the directory tree is intact but preserve
		// user-edited config.yaml as-is.
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			return fmt.Errorf("creating tmp dir %s: %w", tmpDir, err)
		}
		if err := os.MkdirAll(absGraphDir, 0o755); err != nil {
			return fmt.Errorf("creating graph dir %s: %w", absGraphDir, err)
		}

		// Upgrade case: record the derived repo_id when the existing config
		// predates the field. An already-recorded value is never touched —
		// the committed identity is shared by every user of the repo.
		if derivedRepoID != "" {
			existing, readErr := os.ReadFile(configPath)
			if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
				return fmt.Errorf("reading %s: %w", configPath, readErr)
			}
			recorded, err := model.ParseConfig(existing)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", configPath, err)
			}
			if recorded.RepoID == "" {
				updated, err := model.SetYAMLField(existing, "repo_id", derivedRepoID)
				if err != nil {
					return fmt.Errorf("updating %s: %w", configPath, err)
				}
				if !bytes.Equal(existing, updated) {
					if err := os.WriteFile(configPath, updated, 0o644); err != nil {
						return fmt.Errorf("writing %s: %w", configPath, err)
					}
					touched = append(touched, configPath)
					if cmd.OnRepoIDWritten != nil {
						cmd.OnRepoIDWritten(derivedRepoID)
					}
				}
			}
		}

		// Upgrade case: an existing config.yaml predates the skill_scope
		// field. Upsert it now so subsequent runs read the recorded value
		// rather than guessing. Comments and other keys round-trip via
		// SetYAMLField.
		if persistScope {
			existing, readErr := os.ReadFile(configPath)
			if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
				return fmt.Errorf("reading %s: %w", configPath, readErr)
			}
			updated, err := model.SetYAMLField(existing, "skill_scope", string(effectiveScope))
			if err != nil {
				return fmt.Errorf("updating %s: %w", configPath, err)
			}
			if !bytes.Equal(existing, updated) {
				if err := os.WriteFile(configPath, updated, 0o644); err != nil {
					return fmt.Errorf("writing %s: %w", configPath, err)
				}
				touched = append(touched, configPath)
			}
		}

		// Persist an explicit --agents selection to supported_agents and prune
		// the renders of any agent it drops (d-tac-jin). A bare init (no
		// --agents) leaves the recorded value and its renders untouched. Prune
		// runs before the config write: a failed prune then leaves
		// supported_agents still recording the dropped agent, so a re-run
		// retries the drop rather than orphaning files under a config that
		// already claims they're gone.
		if persistAgents {
			// Prune only under project scope, where renders are per-project and
			// committed. Under user scope the skill dirs are shared across every
			// project on the machine, so a per-project drop must not delete a
			// render another project relies on — the recorded list still updates
			// below, leaving the shared render in place.
			if effectiveScope == model.ScopeProject {
				if dropped := agentsDiff(recordedAgents, effectiveAgents); len(dropped) > 0 {
					if err := h.pruneAgentSkills(ctx, dropped, effectiveScope, cmd, &touched); err != nil {
						return err
					}
				}
			}

			existing, readErr := os.ReadFile(configPath)
			if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
				return fmt.Errorf("reading %s: %w", configPath, readErr)
			}
			updated, err := model.SetYAMLSequence(existing, "supported_agents", agentTargetsToStrings(effectiveAgents))
			if err != nil {
				return fmt.Errorf("updating %s: %w", configPath, err)
			}
			if !bytes.Equal(existing, updated) {
				if err := os.WriteFile(configPath, updated, 0o644); err != nil {
					return fmt.Errorf("writing %s: %w", configPath, err)
				}
				touched = append(touched, configPath)
			}
		}
	}

	// Housekeeping applied on every init (idempotent — ensureGitignoreEntries
	// skips entries already present). Covers fresh checkouts and upgrades
	// where a new entry has been added to the required set.
	gitignoreEntries := []string{".sdd/tmp/", ".sdd/config.local.yaml", ".sdd/index/", ".sdd/stats/", ".sdd/sessions/"}
	gitignoreAdded, err := ensureGitignoreEntries(gitignorePath, gitignoreEntries)
	if err != nil {
		log.Warn("could not update .gitignore", "path", gitignorePath, "err", err)
	} else if gitignoreAdded {
		touched = append(touched, gitignorePath)
		if cmd.OnGitignoreUpdated != nil {
			cmd.OnGitignoreUpdated(gitignorePath)
		}
	}

	// Scaffold the AGENTS.md / CLAUDE.md instruction bridge when a non-Claude
	// agent is in play and the files are absent. AGENTS.md is the cross-tool
	// canonical instruction file; CLAUDE.md imports it via @AGENTS.md. Existing
	// files are never overwritten — a project's own instructions are sacred.
	if bridgePaths, err := scaffoldInstructionBridge(cmd.RepoRoot, effectiveAgents); err != nil {
		return err
	} else if len(bridgePaths) > 0 {
		touched = append(touched, bridgePaths...)
		if cmd.OnBridgeScaffolded != nil {
			cmd.OnBridgeScaffolded(bridgePaths)
		}
	}

	// Write the participant into .sdd/config.local.yaml when a value was
	// resolved by the caller. Preserves any other keys already present in
	// the file (notably the llm: block from d-tac-bes) by operating on a
	// yaml.Node tree rather than re-marshaling the Config struct.
	if cmd.Participant != "" {
		configLocalPath := filepath.Join(sddDir, "config.local.yaml")
		existing, readErr := os.ReadFile(configLocalPath)
		if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			return fmt.Errorf("reading %s: %w", configLocalPath, readErr)
		}
		updated, err := model.SetYAMLField(existing, "participant", cmd.Participant)
		if err != nil {
			return fmt.Errorf("updating %s: %w", configLocalPath, err)
		}
		if !bytes.Equal(existing, updated) {
			if err := os.WriteFile(configLocalPath, updated, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", configLocalPath, err)
			}
			// Deliberately NOT appended to `touched`: config.local.yaml is
			// gitignored, so staging it would fail the commit. The write
			// is still recorded via OnParticipantWritten for CLI feedback.
			if cmd.OnParticipantWritten != nil {
				cmd.OnParticipantWritten(configLocalPath, cmd.Participant)
			}
		}
	}

	// Write .sdd/meta.json when absent. MinimumVersion is populated only on
	// a released-version binary — dev builds leave the field empty so local
	// development doesn't pin a floor the graph's owner didn't choose.
	var minVersion *string
	if !model.IsDevVersion(cmd.BinaryVersion) {
		v := cmd.BinaryVersion
		minVersion = &v
	}
	err = h.WriteSchemaMeta(ctx, &command.WriteSchemaMetaCmd{
		SDDDir:         sddDir,
		SchemaVersion:  model.CurrentGraphSchemaVersion,
		MinimumVersion: minVersion,
		OnWritten: func(path string) {
			touched = append(touched, path)
			if cmd.OnMetaWritten != nil {
				cmd.OnMetaWritten(path)
			}
		},
		OnPreserved: func(path string) {
			if cmd.OnMetaPreserved != nil {
				cmd.OnMetaPreserved(path)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("writing schema meta: %w", err)
	}

	// `sdd init --bump` raises minimum_version to the running binary
	// after meta.json is in place. Runs before the skill install so a
	// rejected dev-build bump doesn't leave the install dir half-written.
	// The handler enforces the dev-build refusal; the CLI also fast-fails
	// before invoking us, but the second guard keeps the contract clean
	// when the API is consumed directly.
	if cmd.Bump {
		err := h.BumpMinimumVersion(ctx, &command.BumpMinimumVersionCmd{
			SDDDir:        sddDir,
			BinaryVersion: cmd.BinaryVersion,
			OnBumped: func(previous, current string) {
				metaPath := filepath.Join(sddDir, model.SchemaMetaFileName)
				touched = append(touched, metaPath)
				if cmd.OnMinimumVersionBumped != nil {
					cmd.OnMinimumVersionBumped(previous, current)
				}
			},
			OnUnchanged: func(current string) {
				if cmd.OnMinimumVersionUnchanged != nil {
					cmd.OnMinimumVersionUnchanged(current)
				}
			},
		})
		if err != nil {
			return fmt.Errorf("bumping minimum_version: %w", err)
		}
	}

	// Install (or refresh) the embedded skill bundle. Track every written
	// file for the eventual commit.
	for _, target := range effectiveAgents {
		err = h.InstallSkills(ctx, &command.InstallSkillsCmd{
			Target:          target,
			Scope:           effectiveScope,
			RepoRoot:        cmd.RepoRoot,
			UserHome:        cmd.UserHome,
			BinaryVersion:   cmd.BinaryVersion,
			Force:           cmd.Force,
			PromptOverwrite: cmd.PromptOverwrite,
			OnInstalled: func(r command.SkillInstallResult) {
				touched = append(touched, r.Installed...)
				touched = append(touched, r.Refreshed...)
				touched = append(touched, r.Overwritten...)
				if cmd.OnSkillsInstalled != nil {
					cmd.OnSkillsInstalled(r)
				}
			},
		})
		if err != nil {
			return fmt.Errorf("installing skills for %s: %w", target, err)
		}
	}

	// Register the SDD MCP server per agent so engine mode works out of the
	// box (d-tac-wfl). Project scope only for now: the registration files
	// live in the repo tree; user-scope registration (home-dir config) is a
	// separate path, deferred. User scope still installs skills above — it
	// just skips this step.
	if effectiveScope == model.ScopeProject {
		if err := h.registerMCPServers(effectiveAgents, cmd, &touched); err != nil {
			return err
		}
	}

	// Commit anything touched. Skill files installed under the user-global
	// scope (outside the repo) are filtered out — they aren't part of the
	// repo's git tree.
	if h.committer != nil {
		commitPaths := filterRepoPaths(cmd.RepoRoot, touched)
		if len(commitPaths) > 0 {
			msg := initCommitMessage(sddExisted)
			if err := h.committer.Commit(msg, commitPaths...); err != nil {
				return fmt.Errorf("git commit: %w", err)
			}
		}
	}

	return nil
}

// resolveSkillScope picks the scope `sdd init` should install under and
// reports whether the chosen value needs to be persisted.
//
// Precedence:
//
//   - Fresh `.sdd/`: use the explicit flag value if given, otherwise the
//     default. Always persisted (the new config gets the field on first
//     write).
//   - Existing config with skill_scope recorded: the recorded value wins.
//     If the caller passed --scope and it disagrees, error out before
//     touching the filesystem; manual edit of .sdd/config.yaml is the
//     resolution path (per AC 2). Same value passed explicitly is a no-op.
//   - Existing config missing skill_scope (upgrade path): use the
//     explicit flag value if given, otherwise the default. Persisted so
//     subsequent runs round-trip without re-deriving.
func resolveSkillScope(sddExisted bool, configPath string, flagScope model.Scope, flagExplicit bool) (effective model.Scope, persist bool, err error) {
	requested := flagScope
	if requested == "" {
		requested = model.DefaultScope
	}

	if !sddExisted {
		return requested, true, nil
	}

	data, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		return "", false, fmt.Errorf("reading %s: %w", configPath, readErr)
	}
	var recorded model.Scope
	if len(data) > 0 {
		cfg, parseErr := model.ParseConfig(data)
		if parseErr != nil {
			return "", false, fmt.Errorf("parsing %s: %w", configPath, parseErr)
		}
		if cfg != nil {
			recorded = cfg.SkillScope
		}
	}

	if recorded != "" {
		if flagExplicit && flagScope != "" && flagScope != recorded {
			return "", false, fmt.Errorf(
				"--scope=%s contradicts skill_scope=%s recorded in %s; edit the file directly to change scope",
				flagScope, recorded, configPath,
			)
		}
		return recorded, false, nil
	}

	// Upgrade: config.yaml exists but predates skill_scope. Persist whatever
	// the operator chose (or the default) so the next run is deterministic.
	return requested, true, nil
}

// resolveSupportedAgents picks the agent targets `sdd init` should render,
// reports the previously recorded list, and whether the chosen value must be
// persisted to supported_agents via a sequence upsert.
//
//   - Fresh `.sdd/`: the caller-provided targets if any, else
//     model.DefaultSupportedAgents. Written by the fresh-init config write, so
//     persist is false here (no upsert needed).
//   - Existing tree, explicit --agents: the selection fully replaces the
//     recorded list and is persisted (d-tac-jin). recorded is returned so the
//     caller can prune the renders of any dropped agent.
//   - Existing tree, no --agents, recorded value present: the recorded value
//     wins, not re-persisted.
//   - Existing tree, no --agents, no recorded value (pre-multi-agent upgrade):
//     the default, re-derived each run rather than silently persisted.
func resolveSupportedAgents(sddExisted bool, configPath string, cmdTargets []model.AgentTarget) (effective, recorded []model.AgentTarget, persist bool, err error) {
	if sddExisted {
		data, readErr := os.ReadFile(configPath)
		if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			return nil, nil, false, fmt.Errorf("reading %s: %w", configPath, readErr)
		}
		if len(data) > 0 {
			cfg, parseErr := model.ParseConfig(data)
			if parseErr != nil {
				return nil, nil, false, fmt.Errorf("parsing %s: %w", configPath, parseErr)
			}
			if cfg != nil {
				recorded = cfg.SupportedAgents
			}
		}
		// An explicit --agents selection replaces and persists the recorded
		// list; a bare init leaves it as-is.
		if len(cmdTargets) > 0 {
			return cmdTargets, recorded, true, nil
		}
		if len(recorded) > 0 {
			return recorded, recorded, false, nil
		}
		return model.DefaultSupportedAgents, recorded, false, nil
	}
	if len(cmdTargets) > 0 {
		return cmdTargets, nil, false, nil
	}
	return model.DefaultSupportedAgents, nil, false, nil
}

// agentTargetsToStrings converts agent targets to their string form for the
// supported_agents sequence upsert.
func agentTargetsToStrings(targets []model.AgentTarget) []string {
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = string(t)
	}
	return out
}

// agentsDiff returns the targets in a that are not in b (set difference) —
// used to find which previously-recorded agents an explicit selection dropped.
func agentsDiff(a, b []model.AgentTarget) []model.AgentTarget {
	keep := make(map[model.AgentTarget]bool, len(b))
	for _, t := range b {
		keep[t] = true
	}
	var out []model.AgentTarget
	for _, t := range a {
		if !keep[t] {
			out = append(out, t)
		}
	}
	return out
}

// registerMCPServers writes the project-scope MCP registration for each agent
// so engine mode works without manual setup (d-tac-wfl): a .mcp.json entry
// plus an mcp__sdd__* allow rule for Claude Code, and a .codex/config.toml
// entry forwarding SSH_AUTH_SOCK for Codex (d-tac-ay1). Writes are
// add-if-missing — an existing sdd entry is left untouched — so re-running
// init is idempotent and never clobbers a user's customization. Every written
// file is appended to touched so the init commit records it.
func (h *Handler) registerMCPServers(agents []model.AgentTarget, cmd *command.InitCmd, touched *[]string) error {
	for _, target := range agents {
		targets, err := model.MCPRegistrationTargets(target, cmd.RepoRoot)
		if err != nil {
			return fmt.Errorf("resolving MCP registration for %s: %w", target, err)
		}
		for _, rt := range targets {
			existing, readErr := os.ReadFile(rt.Path)
			if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
				return fmt.Errorf("reading %s: %w", rt.Path, readErr)
			}
			merged, changed, err := rt.Merge(existing)
			if err != nil {
				return fmt.Errorf("registering MCP server in %s: %w", rt.Path, err)
			}
			if !changed {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(rt.Path), 0o755); err != nil {
				return fmt.Errorf("creating dir for %s: %w", rt.Path, err)
			}
			if err := os.WriteFile(rt.Path, merged, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", rt.Path, err)
			}
			*touched = append(*touched, rt.Path)
			if cmd.OnMCPRegistered != nil {
				cmd.OnMCPRegistered(target, rt.Path)
			}
		}
	}
	return nil
}

// pruneAgentSkills removes the sdd-rendered skill files of dropped agents,
// symmetric with install's overwrite protection: only files sdd produced and
// the user hasn't modified (Current or Pristine) are removed, while
// user-modified files are preserved and reported, deleted only under --force.
// Non-sdd files sharing the directory aren't bundle entries, so SkillStatus
// never classifies them and they're left untouched. Skill subdirectories left
// empty by the removals, and the now-empty parent skills dir, are cleaned up.
// Removed paths are appended to touched so the commit records the deletions.
func (h *Handler) pruneAgentSkills(ctx context.Context, dropped []model.AgentTarget, scope model.Scope, cmd *command.InitCmd, touched *[]string) error {
	for _, target := range dropped {
		status, err := h.reader.SkillStatus(ctx, query.SkillStatusQuery{
			Target:   target,
			Scope:    scope,
			RepoRoot: cmd.RepoRoot,
			UserHome: cmd.UserHome,
		})
		if err != nil {
			return fmt.Errorf("classifying %s skills for prune: %w", target, err)
		}

		var removed, keptModified []string
		skillDirs := map[string]bool{}
		for _, e := range status.Entries {
			if e.Status == model.SkillStatusMissing {
				continue
			}
			if e.Status == model.SkillStatusModified && !cmd.Force {
				keptModified = append(keptModified, e.AbsPath)
				continue
			}
			// Current, Pristine, or (Modified under --force): sdd-owned.
			if err := os.Remove(e.AbsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("removing %s: %w", e.AbsPath, err)
			}
			removed = append(removed, e.AbsPath)
			skillDirs[filepath.Join(status.InstallDir, e.Skill)] = true
		}

		// Remove emptied skill subdirs (and their empty descendants), then the
		// parent skills dir if nothing remains. A dir still holding a user's
		// non-sdd skill is left in place.
		for dir := range skillDirs {
			pruneEmptyDirs(dir)
		}
		if entries, err := os.ReadDir(status.InstallDir); err == nil && len(entries) == 0 {
			_ = os.Remove(status.InstallDir)
		}

		*touched = append(*touched, removed...)
		if cmd.OnAgentSkillsPruned != nil && (len(removed) > 0 || len(keptModified) > 0) {
			cmd.OnAgentSkillsPruned(command.AgentPruneResult{
				Target:       target,
				InstallDir:   status.InstallDir,
				Removed:      removed,
				KeptModified: keptModified,
			})
		}
	}
	return nil
}

// pruneEmptyDirs removes dir and its empty subdirectories bottom-up. A dir
// that still holds files is left in place.
func pruneEmptyDirs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			pruneEmptyDirs(filepath.Join(dir, e.Name()))
		}
	}
	if remaining, err := os.ReadDir(dir); err == nil && len(remaining) == 0 {
		_ = os.Remove(dir)
	}
}

// Instruction-bridge scaffold content. AGENTS.md is the cross-tool canonical
// instruction file (read by Codex and other AGENTS.md-aware tools); CLAUDE.md
// imports it through Claude Code's documented @AGENTS.md mechanism so both
// agents share one baseline. Written only when absent — never overwriting a
// project's own files. Backticks rule out a raw string literal here.
const agentsMDScaffold = "# AGENTS.md\n" +
	"\n" +
	"This project uses SDD (Signal → Dialogue → Decision): decisions and\n" +
	"signals live in a graph under `.sdd/graph/`, managed by the `sdd` CLI.\n" +
	"\n" +
	"- Run `sdd view` for the current state.\n" +
	"- Work with the graph through dialogue using the SDD skill — `/sdd` in\n" +
	"  Claude Code, or the `sdd` skill in Codex.\n" +
	"- Skills are rendered per agent and committed under `.claude/skills/`\n" +
	"  (Claude Code) and `.agents/skills/` (Codex). Don't edit them by hand;\n" +
	"  regenerate with `sdd init`.\n" +
	"\n" +
	"<!-- Add project-specific, agent-neutral guidance below. -->\n"

const claudeMDScaffold = "@AGENTS.md\n" +
	"\n" +
	"<!-- Claude Code-specific guidance goes here, after the import above. -->\n"

// scaffoldInstructionBridge writes the AGENTS.md / CLAUDE.md bridge when a
// non-Claude agent is among the rendered targets and the files do not already
// exist. Returns the absolute paths it created (empty when nothing was
// written). Existing files are left untouched.
func scaffoldInstructionBridge(repoRoot string, agents []model.AgentTarget) ([]string, error) {
	if !includesNonClaude(agents) {
		return nil, nil
	}

	files := []struct{ name, content string }{
		{"AGENTS.md", agentsMDScaffold},
		{"CLAUDE.md", claudeMDScaffold},
	}

	var written []string
	for _, f := range files {
		p := filepath.Join(repoRoot, f.name)
		exists, err := pathExists(p)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		if err := os.WriteFile(p, []byte(f.content), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", p, err)
		}
		written = append(written, p)
	}
	return written, nil
}

// includesNonClaude reports whether the target set contains any agent other
// than Claude — the condition under which the AGENTS.md bridge has a consumer.
func includesNonClaude(agents []model.AgentTarget) bool {
	for _, a := range agents {
		if a != model.AgentClaude {
			return true
		}
	}
	return false
}

// pathExists reports whether path is accessible. A non-existence error is
// distinguished from other stat failures.
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", path, err)
}

// filterRepoPaths drops any absolute paths that don't live under repoRoot.
// Used to keep the init commit scoped to the repo's own tree — skills
// installed under ~/.claude/skills/ are real filesystem writes but not
// tracked in the repo.
func filterRepoPaths(repoRoot string, paths []string) []string {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		abs = repoRoot
	}
	prefix := abs + string(filepath.Separator)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

func initCommitMessage(sddExisted bool) string {
	if sddExisted {
		return "sdd: refresh installed skills and metadata"
	}
	return "sdd: init .sdd/ metadata directory"
}

// ensureGitignoreEntries appends entries to .gitignore that are not already
// present. Creates the file if it does not exist. Returns true when at least
// one entry was added so the caller can decide whether to record the file
// as touched.
func ensureGitignoreEntries(path string, entries []string) (bool, error) {
	existing := make(map[string]bool)
	var fileData []byte

	if data, err := os.ReadFile(path); err == nil {
		fileData = data
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			existing[strings.TrimSpace(scanner.Text())] = true
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	var toAdd []string
	for _, e := range entries {
		if !existing[e] {
			toAdd = append(toAdd, e)
		}
	}
	if len(toAdd) == 0 {
		return false, nil
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Ensure we start on a new line if the file doesn't end with one.
	if len(fileData) > 0 && fileData[len(fileData)-1] != '\n' {
		fmt.Fprintln(f)
	}

	for _, e := range toAdd {
		fmt.Fprintln(f, e)
	}
	return true, nil
}

// migrateOldTmpDir moves files from oldDir to newDir. Returns the count
// of migrated files. Uses copy-then-remove for cross-device safety.
func migrateOldTmpDir(oldDir, newDir string) int {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return 0
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(oldDir, e.Name())
		dst := filepath.Join(newDir, e.Name())

		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			continue
		}
		_ = os.Remove(src) // best-effort; stale source is not fatal
		count++
	}

	// Remove old directory if now empty.
	remaining, _ := os.ReadDir(oldDir)
	if len(remaining) == 0 {
		_ = os.Remove(oldDir) // best-effort; empty dir is harmless if it lingers
	}

	return count
}

// cleanOldGraphDirGitignore removes the .sdd-tmp/ entry from a .gitignore
// in the graph directory, if present. Returns nil when there's nothing to
// clean (no .gitignore or no matching entry).
func cleanOldGraphDirGitignore(graphDir string) error {
	path := filepath.Join(graphDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var lines []string
	changed := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == ".sdd-tmp" || strings.TrimSpace(line) == ".sdd-tmp/" {
			changed = true
			continue
		}
		lines = append(lines, line)
	}

	if !changed {
		return nil
	}

	var out strings.Builder
	for i, line := range lines {
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	if len(lines) > 0 {
		out.WriteString("\n")
	}

	return os.WriteFile(path, []byte(out.String()), 0o644)
}
