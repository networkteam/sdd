package index

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	chromem "github.com/philippgille/chromem-go"
)

// Stage provider-owned serialization outside the live tree: chromem writes
// files in place, so a killed writer must not leave a truncated live gob.
func (i *Index) addDocuments(ctx context.Context, docs []chromem.Document) (err error) {
	if i.indexDir == "" {
		return i.coll.AddDocuments(ctx, docs, 1)
	}
	stage, err := os.MkdirTemp(i.indexDir, ".entry-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	staged, err := loadStore(stage)
	if err != nil {
		return err
	}
	if err := staged.coll.AddDocuments(ctx, docs, 1); err != nil {
		return err
	}
	err = filepath.WalkDir(staged.chromemDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(staged.chromemDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(i.chromemDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		syncErr := file.Sync()
		if err := errors.Join(syncErr, file.Close()); err != nil {
			return err
		}
		return os.Rename(path, target)
	})
	if err != nil {
		return err
	}
	loaded, err := loadStore(i.indexDir)
	if err != nil {
		return err
	}
	i.db, i.coll = loaded.db, loaded.coll
	return nil
}
