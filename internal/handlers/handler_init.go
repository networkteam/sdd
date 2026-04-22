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

	// Tracks paths we touch so the final git commit covers exactly what
	// changed. A repeat init that changes nothing yields no commit.
	var touched []string

	if !sddExisted {
		if err := os.MkdirAll(sddDir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", sddDir, err)
		}
		if err := os.WriteFile(configPath, []byte(model.FormatConfig(model.Config{GraphDir: graphDir})), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", configPath, err)
		}
		touched = append(touched, configPath)

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
	}

	// Housekeeping applied on every init (idempotent — ensureGitignoreEntries
	// skips entries already present). Covers fresh checkouts and upgrades
	// where a new entry has been added to the required set.
	gitignoreEntries := []string{".sdd/tmp/", ".sdd/config.local.yaml"}
	gitignoreAdded, err := ensureGitignoreEntries(gitignorePath, gitignoreEntries)
	if err != nil {
		log.Warn("could not update .gitignore", "path", gitignorePath, "err", err)
	} else if gitignoreAdded {
		touched = append(touched, gitignorePath)
		if cmd.OnGitignoreUpdated != nil {
			cmd.OnGitignoreUpdated(gitignorePath)
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

	// Install (or refresh) the embedded skill bundle. Track every written
	// file for the eventual commit.
	err = h.InstallSkills(ctx, &command.InstallSkillsCmd{
		Target:          cmd.Target,
		Scope:           cmd.Scope,
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
		return fmt.Errorf("installing skills: %w", err)
	}

	// Commit anything touched. Skill files installed under the user-global
	// scope (outside the repo) are filtered out — they aren't part of the
	// repo's git tree.
	if h.committer != nil {
		commitPaths := filterRepoPaths(cmd.RepoRoot, touched)
		if len(commitPaths) > 0 {
			msg := initCommitMessage(sddExisted)
			if err := h.committer.Commit(msg, commitPaths...); err != nil {
				log.Warn("git commit failed", "err", err)
			}
		}
	}

	return nil
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
