package application

import "context"

// EmbeddingSpec identifies the vector space an executor embeds into. The
// fingerprint must uniquely determine the embedding model and with it the
// vector dimensionality — dimensionality itself is discovered from the
// vectors on first real use, so lazy providers (ollama reports dimensions
// only with its first response) satisfy the contract without a probe call.
type EmbeddingSpec struct {
	Fingerprint string
}

type EmbeddingInput struct {
	ID      string
	Text    string
	Purpose EmbeddingPurpose
}

type EmbeddingPurpose string

const (
	EmbeddingDocument EmbeddingPurpose = "document"
	EmbeddingQuery    EmbeddingPurpose = "query"
)

type EmbeddingVector struct {
	ID     string
	Values []float32
}

type EmbeddingExecutor interface {
	Spec(context.Context) (EmbeddingSpec, error)
	Embed(context.Context, []EmbeddingInput) ([]EmbeddingVector, error)
}

// IndexNamespace keys one reconciled vector index. The fingerprint pins the
// embedding model (and thus the dimensionality), so dimensions are not part
// of the identity — stores enforce vector-length consistency per namespace
// at reconcile and query time instead.
type IndexNamespace struct {
	Project     ProjectID
	Fingerprint string
	Metric      string
}

type CanonicalChunk struct {
	ID      string
	EntryID string
	Ordinal int
	// Revision is deprecated: graph revision is a mutation-concurrency token,
	// never a vector-freshness token (d-cpt-65i). Reconciliation and hit
	// validity ignore it. Retained only for source compatibility.
	Revision    string
	ContentHash string
	Text        string
	// The following persisted citation and identity fields carry everything a
	// store needs to render a citation and answer entry-presence queries
	// without re-deriving chunks. Both the CLI indexer and the application
	// vector search populate them through the shared chunk-derivation helper.
	Body                 string
	Breadcrumb           []string
	Depth                int
	IsSummary            bool
	IsAttachment         bool
	SourceAttachmentPath string
	// EntryHash is the entry-state hash (entry content + summary + attachment
	// bytes) — the same definition as the CLI manifest state hash.
	EntryHash string
}

type IndexedChunk struct {
	Chunk  CanonicalChunk
	Vector []float32
}

type StoredChunkRef struct {
	ID string
	// Revision is deprecated and ignored by reconciliation (see CanonicalChunk).
	Revision    string
	ContentHash string
}

// StoredEntryRef identifies one graph entry represented in a persistent index.
// Slice 1 keys presence by entry ID alone.
type StoredEntryRef struct {
	EntryID string
}

type ScoredChunkHit struct {
	Namespace IndexNamespace
	ChunkID   string
	EntryID   string
	// Revision is deprecated and ignored by hit validity (see CanonicalChunk).
	Revision    string
	ContentHash string
	Score       float64
	// Persisted citation fields, rendered directly into search citations so a
	// hit needs no re-derivation of its source chunk.
	Body                 string
	Breadcrumb           []string
	Depth                int
	IsSummary            bool
	IsAttachment         bool
	SourceAttachmentPath string
}

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
