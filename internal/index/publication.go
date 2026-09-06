package index

import (
	"context"
	"errors"
	"fmt"
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
	if err := filepath.WalkDir(i.chromemDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return syncPublicationDirectory(path)
		}
		return nil
	}); err != nil {
		return err
	}
	loaded, err := loadStore(i.indexDir)
	if err != nil {
		return err
	}
	i.db, i.coll = loaded.db, loaded.coll
	return nil
}

// Build the published read view once per store snapshot, before nearest-N
// selection, so unpublished rows neither fill the hit limit nor require an
// all-candidate result allocation on each query.
func (i *Index) publishedCollection(ctx context.Context) (*chromem.Collection, error) {
	if i.indexDir == "" {
		return i.coll, nil
	}
	if i.published != nil {
		return i.published, nil
	}
	manifest := i.manifest
	db := chromem.NewDB()
	collection, err := db.GetOrCreateCollection(CollectionName, nil, embedFuncStub)
	if err != nil {
		return nil, err
	}
	for _, state := range manifest.Entries {
		for _, version := range state.Versions {
			for _, id := range version.ChunkIDs {
				doc, err := i.coll.GetByID(ctx, id)
				if err != nil {
					return nil, err
				}
				if doc.Metadata[MetaEntryHash] != "" && doc.Metadata[MetaEntryHash] != version.Hash {
					return nil, fmt.Errorf("index: published chunk version mismatch")
				}
				doc.Metadata[MetaEntryHash] = version.Hash
				if err := collection.AddDocument(ctx, doc); err != nil {
					return nil, err
				}
			}
		}
	}
	i.published = collection
	return collection, nil
}

func syncPublicationDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
