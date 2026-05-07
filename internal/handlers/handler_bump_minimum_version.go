package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
)

// BumpMinimumVersion raises .sdd/meta.json's minimum_version field to the
// running binary's version when the binary is strictly higher (or no
// minimum was recorded). Equal versions are a no-op so repeat invocations
// stay quiet.
//
// Dev builds are rejected with a fixed message — only released binaries
// may raise the floor, otherwise a developer's local pinning could lock
// every other contributor out of the graph until that exact dev build is
// available.
func (h *Handler) BumpMinimumVersion(ctx context.Context, cmd *command.BumpMinimumVersionCmd) error {
	log := slogutils.FromContext(ctx)

	if cmd.SDDDir == "" {
		return fmt.Errorf("SDDDir is required")
	}
	if model.IsDevVersion(cmd.BinaryVersion) {
		return fmt.Errorf("cannot bump from a dev build, use a released sdd binary")
	}

	path := filepath.Join(cmd.SDDDir, model.SchemaMetaFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	meta, err := model.ParseSchemaMeta(data)
	if err != nil {
		return err
	}

	previous := ""
	if meta.MinimumVersion != nil {
		previous = *meta.MinimumVersion
	}

	if !model.ShouldBumpMinimumVersion(previous, cmd.BinaryVersion) {
		// Equal or lower binary: leave meta.json alone. Lower is normally
		// blocked by the write gate before we even reach init, but the
		// handler stays defensive.
		if cmd.OnUnchanged != nil {
			cmd.OnUnchanged(previous)
		}
		log.Debug("minimum_version unchanged", "current", previous, "binary", cmd.BinaryVersion)
		return nil
	}

	current := cmd.BinaryVersion
	meta.MinimumVersion = &current
	out, err := model.FormatSchemaMeta(*meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	if cmd.OnBumped != nil {
		cmd.OnBumped(previous, current)
	}
	log.Debug("minimum_version bumped", "previous", previous, "current", current)
	return nil
}
