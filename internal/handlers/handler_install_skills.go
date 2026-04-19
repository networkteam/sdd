package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// InstallSkills writes the embedded skill bundle to the target agent's skill
// directory. Side effects are limited to filesystem writes — the skill status
// classification itself comes from the injected reader (SkillStatus query).
func (h *Handler) InstallSkills(ctx context.Context, cmd *command.InstallSkillsCmd) error {
	log := slogutils.FromContext(ctx)

	target := cmd.Target
	if target == "" {
		target = model.DefaultAgentTarget
	}
	scope := cmd.Scope
	if scope == "" {
		scope = model.DefaultScope
	}

	status, err := h.reader.SkillStatus(ctx, query.SkillStatusQuery{
		Target:   target,
		Scope:    scope,
		RepoRoot: cmd.RepoRoot,
		UserHome: cmd.UserHome,
	})
	if err != nil {
		return fmt.Errorf("reading skill status: %w", err)
	}

	if err := os.MkdirAll(status.InstallDir, 0o755); err != nil {
		return fmt.Errorf("creating install dir %s: %w", status.InstallDir, err)
	}

	result := command.SkillInstallResult{}

	for _, entry := range status.Entries {
		switch entry.Status {
		case model.SkillStatusCurrent:
			result.Current = append(result.Current, entry.AbsPath)

		case model.SkillStatusMissing:
			if err := writeStampedEntry(entry.AbsPath, entry.Embedded, cmd.BinaryVersion); err != nil {
				return err
			}
			result.Installed = append(result.Installed, entry.AbsPath)

		case model.SkillStatusPristine:
			if err := writeStampedEntry(entry.AbsPath, entry.Embedded, cmd.BinaryVersion); err != nil {
				return err
			}
			result.Refreshed = append(result.Refreshed, entry.AbsPath)

		case model.SkillStatusModified:
			overwrite := cmd.Force
			if !overwrite && cmd.PromptOverwrite != nil {
				ok, err := cmd.PromptOverwrite(entry.AbsPath)
				if err != nil {
					return fmt.Errorf("prompting for %s: %w", entry.AbsPath, err)
				}
				overwrite = ok
			}
			if overwrite {
				if err := writeStampedEntry(entry.AbsPath, entry.Embedded, cmd.BinaryVersion); err != nil {
					return err
				}
				result.Overwritten = append(result.Overwritten, entry.AbsPath)
			} else {
				result.SkippedModified = append(result.SkippedModified, entry.AbsPath)
				log.Debug("skipped modified skill", "path", entry.AbsPath)
			}
		}
	}

	if cmd.OnInstalled != nil {
		cmd.OnInstalled(result)
	}
	return nil
}

// writeStampedEntry renders entry with injected stamps and writes the result
// atomically (via a temp file + rename) to absPath, creating intermediate
// directories as needed.
func writeStampedEntry(absPath string, entry model.SkillBundleEntry, version string) error {
	hash := model.ComputeSkillHash(entry.Content)
	rendered, err := model.RenderSkillFile(entry, version, hash)
	if err != nil {
		return fmt.Errorf("rendering %s: %w", absPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("creating parent for %s: %w", absPath, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(absPath), ".skill-*")
	if err != nil {
		return fmt.Errorf("opening temp file for %s: %w", absPath, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(rendered); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, absPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, absPath, err)
	}
	return nil
}
