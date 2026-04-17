package handlers

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
)

// Init executes an InitCmd: creates .sdd/ directory structure at cmd.RepoRoot,
// writes config.yaml from the template, creates the graph directory, adds
// .sdd/tmp to .gitignore, and optionally migrates legacy .sdd-tmp/ contents.
//
// This handler does NOT use h.graphDir — all paths are derived from
// cmd.RepoRoot and cmd.GraphDir.
func (h *Handler) Init(_ context.Context, cmd *command.InitCmd) error {
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

	// Guard against double-init.
	if _, err := os.Stat(sddDir); err == nil {
		return fmt.Errorf(".sdd/ already exists at %s; remove it first to reinitialize", sddDir)
	}

	// Create .sdd/ directory.
	if err := os.MkdirAll(sddDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", sddDir, err)
	}

	// Write config.yaml.
	configContent := model.FormatConfig(model.Config{GraphDir: graphDir})
	configPath := filepath.Join(sddDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", configPath, err)
	}

	// Create graph directory.
	if err := os.MkdirAll(absGraphDir, 0755); err != nil {
		return fmt.Errorf("creating graph dir %s: %w", absGraphDir, err)
	}

	// Create .sdd/tmp/ directory.
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("creating tmp dir %s: %w", tmpDir, err)
	}

	// Update .gitignore at repo root.
	gitignorePath := filepath.Join(cmd.RepoRoot, ".gitignore")
	if err := ensureGitignoreEntries(gitignorePath, []string{".sdd/tmp/"}); err != nil {
		fmt.Fprintf(h.stderr, "warning: could not update .gitignore: %v\n", err)
	} else if cmd.OnGitignoreUpdated != nil {
		cmd.OnGitignoreUpdated(gitignorePath)
	}

	if cmd.OnCreated != nil {
		cmd.OnCreated(sddDir, absGraphDir)
	}

	// Migrate legacy .sdd-tmp/ from old graph dir location.
	oldTmpDir := filepath.Join(absGraphDir, ".sdd-tmp")
	migrated := migrateOldTmpDir(oldTmpDir, tmpDir)
	if migrated > 0 {
		if cmd.OnMigrated != nil {
			cmd.OnMigrated(migrated)
		}
	}

	// Clean old .sdd-tmp/ entry from graph dir .gitignore.
	cleanOldGraphDirGitignore(absGraphDir)

	// Commit the new structure.
	if h.committer != nil {
		paths := []string{configPath, gitignorePath}
		msg := "sdd: init .sdd/ metadata directory"
		if err := h.committer.Commit(msg, paths...); err != nil {
			fmt.Fprintf(h.stderr, "warning: git commit failed: %v\n", err)
		}
	}

	return nil
}

// ensureGitignoreEntries appends entries to .gitignore that are not already
// present. Creates the file if it does not exist.
func ensureGitignoreEntries(path string, entries []string) error {
	existing := make(map[string]bool)
	var fileData []byte

	if data, err := os.ReadFile(path); err == nil {
		fileData = data
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			existing[strings.TrimSpace(scanner.Text())] = true
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	var toAdd []string
	for _, e := range entries {
		if !existing[e] {
			toAdd = append(toAdd, e)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Ensure we start on a new line if the file doesn't end with one.
	if len(fileData) > 0 && fileData[len(fileData)-1] != '\n' {
		fmt.Fprintln(f)
	}

	for _, e := range toAdd {
		fmt.Fprintln(f, e)
	}
	return nil
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
		if err := os.WriteFile(dst, data, 0644); err != nil {
			continue
		}
		os.Remove(src)
		count++
	}

	// Remove old directory if now empty.
	remaining, _ := os.ReadDir(oldDir)
	if len(remaining) == 0 {
		os.Remove(oldDir)
	}

	return count
}

// cleanOldGraphDirGitignore removes the .sdd-tmp/ entry from a .gitignore
// in the graph directory, if present.
func cleanOldGraphDirGitignore(graphDir string) {
	path := filepath.Join(graphDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		return
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
		return
	}

	var out strings.Builder
	for i, line := range lines {
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	// Preserve trailing newline.
	if len(lines) > 0 {
		out.WriteString("\n")
	}

	os.WriteFile(path, []byte(out.String()), 0644)
}

