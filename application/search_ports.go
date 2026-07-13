package application

import "context"

type EmbeddingSpec struct {
	Fingerprint string
	Dimensions  int
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

type IndexNamespace struct {
	Project     ProjectID
	Fingerprint string
	Dimensions  int
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
