package application

import (
	"context"

	"github.com/networkteam/sdd/pkg/application/types"
)

type IndexNamespace = types.IndexNamespace
type CanonicalChunk = types.CanonicalChunk
type IndexedChunk = types.IndexedChunk
type StoredChunkRef = types.StoredChunkRef
type StoredEntryRef = types.StoredEntryRef
type ScoredChunkHit = types.ScoredChunkHit

type SearchIndexStore interface {
	Manifest(context.Context, IndexNamespace) ([]StoredChunkRef, error)
	Reconcile(context.Context, IndexNamespace, string, []IndexedChunk, []string) error
	Nearest(context.Context, []IndexNamespace, []float32, int) ([]ScoredChunkHit, error)
}

// SearchIndexEntryManifest is an optional capability a persistent store
// implements so the application can reconcile on entry presence (monotonic
// accumulation of immutable-entry chunks) instead of chunk-identity
// comparison. A store that does not implement it falls back to the
// compatibility reconciliation path in vector search.
type SearchIndexEntryManifest interface {
	IndexedEntries(context.Context, IndexNamespace) ([]StoredEntryRef, error)
}
