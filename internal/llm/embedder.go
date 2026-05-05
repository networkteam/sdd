package llm

import "context"

// Embedder turns a batch of texts into dense vectors. Implementations are
// thin transport adapters around an embedding service (OpenAI-compatible
// `/v1/embeddings`, Ollama `/api/embed`); construction is via embed.New.
//
// The interface splits embedding into a document-side and a query-side
// because instruction-tuned encoders (Qwen3, E5, Nomic, BGE) want
// asymmetric prefixes — `passage:` on documents, `query:` on queries, or
// `Instruct: …\nQuery:…` only on queries. The split makes the call-site
// intent explicit so the wrong template can't silently slip onto the
// wrong side. Untemplated models (OpenAI text-embedding-3) treat both
// methods as equivalent.
//
// Dimensions returns the vector length the implementation produces — must
// be stable across calls so the index can validate row shape.
//
// Fingerprint is an opaque identifier for the (provider + model + truncation
// + document template) tuple. Used by the search index to detect rows
// whose embedding is stale after a configuration change. The format is
// implementation-defined, but implementations must guarantee that two
// embedders that produce distribution-comparable document vectors share
// a fingerprint, and two that don't, don't. The query template
// deliberately does NOT factor into the fingerprint — query template
// changes affect retrieval quality but never invalidate indexed
// embeddings, so they're a free-tweak knob (see EmbeddingConfig).
type Embedder interface {
	// EmbedDocuments embeds texts as index-side passages, applying the
	// configured DocumentTemplate. Used by the indexer at build /
	// lazy-fill time.
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedQueries embeds texts as retrieval-side queries, applying the
	// configured QueryTemplate. Used by the search finder at query time.
	EmbedQueries(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
	Fingerprint() string
}
