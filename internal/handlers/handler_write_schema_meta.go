package handlers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
)

// WriteSchemaMeta creates .sdd/meta.json when it does not yet exist. Existing
// files are preserved as-is — both graph_schema_version and minimum_version
// have write-once semantics in the init flow (schema bumps are migration
// concerns, minimum_version is set at graph creation time and preserved
// thereafter).
func (h *Handler) WriteSchemaMeta(ctx context.Context, cmd *command.WriteSchemaMetaCmd) error {
	log := slogutils.FromContext(ctx)

	if cmd.SDDDir == "" {
		return fmt.Errorf("SDDDir is required")
	}

	path := filepath.Join(cmd.SDDDir, model.SchemaMetaFileName)

	if _, err := os.Stat(path); err == nil {
		if cmd.OnPreserved != nil {
			cmd.OnPreserved(path)
		}
		log.Debug("schema meta preserved", "path", path)
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	meta := model.SchemaMeta{
		GraphSchemaVersion: cmd.SchemaVersion,
		MinimumVersion:     cmd.MinimumVersion,
	}
	data, err := model.FormatSchemaMeta(meta)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cmd.SDDDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", cmd.SDDDir, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	if cmd.OnWritten != nil {
		cmd.OnWritten(path)
	}
	log.Debug("schema meta written", "path", path)
	return nil
}
