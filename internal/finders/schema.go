package finders

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// SchemaStatus reads .sdd/meta.json (if present) and evaluates its
// compatibility fields against the calling binary's version and schema
// version. Pure read; no side effects.
//
// When meta.json is absent the result reports MetaExists = false and
// Compatible = true — an uninitialised graph is the init command's
// responsibility, not the write gate's.
func (f *Finder) SchemaStatus(ctx context.Context, q query.SchemaStatusQuery) (*query.SchemaStatusResult, error) {
	if q.SDDDir == "" {
		return nil, fmt.Errorf("SDDDir is required")
	}

	path := filepath.Join(q.SDDDir, model.SchemaMetaFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &query.SchemaStatusResult{
				MetaExists:    false,
				Compatibility: model.CompatibilityResult{Compatible: true},
			}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	meta, err := model.ParseSchemaMeta(data)
	if err != nil {
		return nil, err
	}

	return &query.SchemaStatusResult{
		MetaExists:    true,
		Meta:          meta,
		Compatibility: model.CheckCompatibility(*meta, q.BinaryVersion, q.BinarySchemaVersion),
	}, nil
}
