package local

import (
	"context"

	"github.com/networkteam/sdd/internal/index"
	app "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/application/types"
)

func (s *PersistentSearchIndexStore) EntryPublished(ctx context.Context, version app.SearchEntryVersion) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	dir, err := s.storeDir(version.Namespace)
	if err != nil {
		return false, err
	}
	manifest, err := index.LoadManifest(dir)
	if err != nil {
		return false, err
	}
	return publishedVersion(manifest, version), nil
}

func publishedVersion(manifest *index.Manifest, version app.SearchEntryVersion) bool {
	state := manifest.Entries[version.EntryID]
	for _, stored := range state.Versions {
		if stored.Hash == version.EntryHash && stored.Fingerprint == version.Namespace.Fingerprint {
			return true
		}
	}
	return false
}

func (s *PersistentSearchIndexStore) PublishEntry(ctx context.Context, version app.SearchEntryVersion, chunks []app.IndexedChunk) error {
	if err := types.ValidateEntryPublication(version, chunks); err != nil {
		return err
	}
	dir, err := s.storeDir(version.Namespace)
	if err != nil {
		return err
	}
	return index.WriteStore(ctx, dir, func(idx *index.Index) error {
		manifest, err := index.LoadManifest(dir)
		if err != nil {
			return err
		}
		if publishedVersion(manifest, version) {
			return nil
		}
		rows := make([]index.Row, len(chunks))
		ids := make([]string, len(chunks))
		for i, chunk := range chunks {
			rows[i] = rowFromChunk(chunk, version.Namespace.Fingerprint)
			ids[i] = chunk.Chunk.ID
		}
		if err := idx.UpsertEntry(ctx, version.EntryID, nil, rows); err != nil {
			return err
		}
		manifest.AddVersion(version.EntryID, index.EntryVersion{Hash: version.EntryHash, Fingerprint: version.Namespace.Fingerprint, ChunkIDs: ids, IndexedAt: s.now()})
		return manifest.Save(dir)
	})
}
