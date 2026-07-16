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
	ID          string
	EntryID     string
	Ordinal     int
	Revision    string
	ContentHash string
	Text        string
}

type IndexedChunk struct {
	Chunk  CanonicalChunk
	Vector []float32
}

type StoredChunkRef struct {
	ID          string
	Revision    string
	ContentHash string
}

type ScoredChunkHit struct {
	Namespace   IndexNamespace
	ChunkID     string
	Revision    string
	ContentHash string
	Score       float64
}

type SearchIndexStore interface {
	Manifest(context.Context, IndexNamespace) ([]StoredChunkRef, error)
	Reconcile(context.Context, IndexNamespace, string, []IndexedChunk, []string) error
	Nearest(context.Context, []IndexNamespace, []float32, int) ([]ScoredChunkHit, error)
}
