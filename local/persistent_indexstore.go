package local

import (
	"context"
	"fmt"
	"time"

	app "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/index"
)

// PersistentSearchIndexStore is the machine-global chromem-backed vector store
// behind sdd serve — the same content-addressed store the CLI builds and reads
// (d-cpt-6cq). One store per (repo-key, embedding fingerprint) under the cache
// root: the request fingerprint selects the final directory at call time, so a
// lazy embedder that only reports its dimensionality on first use still routes
// correctly.
//
// The adapter holds no long-lived chromem snapshot. Every operation reopens the
// store under the appropriate lock (index.ReadStore / index.WriteStore) so a
// long-running server always reflects writes committed by the CLI or another
// process. Reconciliation is monotonic: it accumulates chunks for immutable
// entries and never deletes based on request filters — the sanctioned delete
// paths are the CLI's explicit rebuild and (later) version GC, not the search
// path.
type PersistentSearchIndexStore struct {
	project   app.ProjectID
	cacheRoot string
	repoKey   string
	now       func() time.Time
}

// NewPersistentSearchIndexStore builds the adapter for one project. cacheRoot
// is the machine-global cache root; repoKey is index.RepoKey for the base repo
// or the connected repo's ID (the existing connected-repository storage
// contract).
func NewPersistentSearchIndexStore(project app.ProjectID, cacheRoot, repoKey string) *PersistentSearchIndexStore {
	return &PersistentSearchIndexStore{project: project, cacheRoot: cacheRoot, repoKey: repoKey, now: time.Now}
}

var (
	_ app.SearchIndexStore         = (*PersistentSearchIndexStore)(nil)
	_ app.SearchIndexEntryManifest = (*PersistentSearchIndexStore)(nil)
)

// storeDir resolves the store directory for a namespace, validating that the
// namespace's project matches this adapter and that a fingerprint is present.
func (s *PersistentSearchIndexStore) storeDir(namespace app.IndexNamespace) (string, error) {
	if namespace.Project != s.project {
		return "", fmt.Errorf("sdd: namespace project %q does not match store project %q", namespace.Project, s.project)
	}
	if namespace.Fingerprint == "" {
		return "", fmt.Errorf("sdd: namespace fingerprint is required")
	}
	return index.StoreDir(s.cacheRoot, s.repoKey, namespace.Fingerprint), nil
}

// IndexedEntries reads the manifest and returns one ref per stored (entry,
// version) pair — no migration, no rebuild, no document embedding. A legacy v1
// manifest loads as one version per entry, so an existing store answers its
// presence immediately. This is the presence source the application reconciles
// against: a graph entry whose current hash is absent here is embedded as an
// added version.
func (s *PersistentSearchIndexStore) IndexedEntries(_ context.Context, namespace app.IndexNamespace) ([]app.StoredEntryRef, error) {
	dir, err := s.storeDir(namespace)
	if err != nil {
		return nil, err
	}
	manifest, err := index.LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	var refs []app.StoredEntryRef
	for _, id := range manifest.EntryIDsSorted() {
		for _, v := range manifest.Entries[id].Versions {
			refs = append(refs, app.StoredEntryRef{EntryID: id, EntryHash: v.Hash})
		}
	}
	return refs, nil
}

// Manifest reports stored chunk identities across all versions. Kept for the
// SearchIndexStore contract; the application prefers IndexedEntries for
// reconciliation.
func (s *PersistentSearchIndexStore) Manifest(_ context.Context, namespace app.IndexNamespace) ([]app.StoredChunkRef, error) {
	dir, err := s.storeDir(namespace)
	if err != nil {
		return nil, err
	}
	manifest, err := index.LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	var refs []app.StoredChunkRef
	for _, id := range manifest.EntryIDsSorted() {
		for _, chunkID := range manifest.Entries[id].AllChunkIDs() {
			refs = append(refs, app.StoredChunkRef{ID: chunkID})
		}
	}
	return refs, nil
}

// Reconcile accumulates the given chunks under the exclusive lock. It groups
// upserts by entry, reopens the latest store inside the lock, adds each entry's
// rows, and writes the v1 manifest. Graph revision is ignored. Request-driven
// deletes are refused — deletion is not the search path's job.
func (s *PersistentSearchIndexStore) Reconcile(ctx context.Context, namespace app.IndexNamespace, _ string, upserts []app.IndexedChunk, deletes []string) error {
	if len(deletes) > 0 {
		return fmt.Errorf("sdd: persistent search index does not accept request-driven deletes (%d requested)", len(deletes))
	}
	if len(upserts) == 0 {
		return nil
	}
	dir, err := s.storeDir(namespace)
	if err != nil {
		return err
	}

	type entryGroup struct {
		id   string
		rows []index.Row
		hash string
	}
	var groups []*entryGroup
	byEntry := map[string]*entryGroup{}
	dims := 0
	for _, upsert := range upserts {
		if len(upsert.Vector) == 0 {
			return fmt.Errorf("sdd: vector for %s is empty", upsert.Chunk.ID)
		}
		if dims == 0 {
			dims = len(upsert.Vector)
		}
		if len(upsert.Vector) != dims {
			return fmt.Errorf("sdd: vector for %s has %d dimensions, want %d", upsert.Chunk.ID, len(upsert.Vector), dims)
		}
		group := byEntry[upsert.Chunk.EntryID]
		if group == nil {
			group = &entryGroup{id: upsert.Chunk.EntryID, hash: upsert.Chunk.EntryHash}
			byEntry[upsert.Chunk.EntryID] = group
			groups = append(groups, group)
		}
		group.rows = append(group.rows, rowFromChunk(upsert, namespace.Fingerprint))
	}

	return index.WriteStore(ctx, dir, func(idx *index.Index) error {
		manifest, err := index.LoadManifest(dir)
		if err != nil {
			return err
		}
		for _, group := range groups {
			// The application only reconciles (entry, version) pairs absent
			// from the manifest, so this is a pure add (no old chunk IDs to
			// remove) — monotonic accumulation of a NEW version, never a
			// request-shaped delete or a delete-on-change of another version.
			if err := idx.UpsertEntry(ctx, group.id, nil, group.rows); err != nil {
				return fmt.Errorf("upsert %s: %w", group.id, err)
			}
			chunkIDs := make([]string, 0, len(group.rows))
			for _, row := range group.rows {
				chunkIDs = append(chunkIDs, row.ChunkID)
			}
			manifest.AddVersion(group.id, index.EntryVersion{
				Hash:        group.hash,
				Fingerprint: namespace.Fingerprint,
				ChunkIDs:    chunkIDs,
				IndexedAt:   s.now(),
			})
		}
		return manifest.Save(dir)
	})
}

// Nearest opens a fresh read snapshot per operation and maps index hits back to
// scored chunk hits with complete citation data.
func (s *PersistentSearchIndexStore) Nearest(ctx context.Context, namespaces []app.IndexNamespace, vector []float32, limit int) ([]app.ScoredChunkHit, error) {
	if len(vector) == 0 {
		return nil, fmt.Errorf("sdd: query vector is empty")
	}
	var result []app.ScoredChunkHit
	for _, namespace := range namespaces {
		dir, err := s.storeDir(namespace)
		if err != nil {
			return nil, err
		}
		var hits []index.Hit
		err = index.ReadStore(ctx, dir, func(idx *index.Index) error {
			hits, err = idx.Query(ctx, vector, limit)
			return err
		})
		if err != nil {
			return nil, err
		}
		// Resolve each hit's version. A new (versioned) row carries entry_hash
		// metadata directly; a legacy v1 row does not, so its version is
		// recovered from the manifest. The manifest is loaded once, and only
		// when some hit needs it, so an all-versioned store never reads it.
		var manifest *index.Manifest
		for _, hit := range hits {
			entryHash := hit.EntryHash
			if entryHash == "" {
				if manifest == nil {
					manifest, err = index.LoadManifest(dir)
					if err != nil {
						return nil, err
					}
				}
				entryHash = manifest.VersionHashForChunk(hit.EntryID, hit.ChunkID)
			}
			result = append(result, app.ScoredChunkHit{
				Namespace:            namespace,
				ChunkID:              hit.ChunkID,
				EntryID:              hit.EntryID,
				EntryHash:            entryHash,
				ContentHash:          hit.ContentHash,
				Score:                float64(hit.Score),
				Body:                 hit.Body,
				Breadcrumb:           hit.Breadcrumb,
				Depth:                hit.Depth,
				IsSummary:            hit.IsSummary,
				IsAttachment:         hit.IsAttachment,
				SourceAttachmentPath: hit.SourceAttachmentPath,
			})
		}
	}
	return result, nil
}

// rowFromChunk maps an application IndexedChunk to an index.Row, carrying the
// citation metadata the finder and MCP citation renderer read back.
func rowFromChunk(chunk app.IndexedChunk, fingerprint string) index.Row {
	return index.Row{
		EntryID:              chunk.Chunk.EntryID,
		EntryHash:            chunk.Chunk.EntryHash,
		ChunkID:              chunk.Chunk.ID,
		Text:                 chunk.Chunk.Text,
		Body:                 chunk.Chunk.Body,
		Breadcrumb:           chunk.Chunk.Breadcrumb,
		Depth:                chunk.Chunk.Depth,
		IsSummary:            chunk.Chunk.IsSummary,
		IsAttachment:         chunk.Chunk.IsAttachment,
		SourceAttachmentPath: chunk.Chunk.SourceAttachmentPath,
		ContentHash:          chunk.Chunk.ContentHash,
		ModelFingerprint:     fingerprint,
		Embedding:            chunk.Vector,
	}
}
