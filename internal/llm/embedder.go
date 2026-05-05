package llm

import "context"

// Embedder turns a batch of texts into dense vectors. Implementations are
// thin transport adapters around an embedding service (OpenAI-compatible
// `/v1/embeddings`, Ollama `/api/embeddings`); construction is via
// factory.NewEmbedder.
//
// Dimensions returns the vector length the implementation produces — must
// be stable across calls so the index can validate row shape.
//
// Fingerprint is an opaque identifier for the (provider + model + truncation)
// triple. Used by the search index to detect rows whose embedding is stale
// after a configuration change. The format is implementation-defined, but
// implementations must guarantee that two embedders that produce
// distribution-comparable vectors share a fingerprint, and two that don't,
// don't.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
	Fingerprint() string
}
