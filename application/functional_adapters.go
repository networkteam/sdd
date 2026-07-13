package application

import "context"

// EmbeddingExecutorFuncs adapts mechanical embedding functions to the public
// executor port.
type EmbeddingExecutorFuncs struct {
	SpecFunc  func(context.Context) (EmbeddingSpec, error)
	EmbedFunc func(context.Context, []EmbeddingInput) ([]EmbeddingVector, error)
}

func (f EmbeddingExecutorFuncs) Spec(ctx context.Context) (EmbeddingSpec, error) {
	return f.SpecFunc(ctx)
}

func (f EmbeddingExecutorFuncs) Embed(ctx context.Context, inputs []EmbeddingInput) ([]EmbeddingVector, error) {
	return f.EmbedFunc(ctx, inputs)
}

// SearchIndexStoreFuncs adapts an index implementation while SDD retains
// chunking, embedding input, reconciliation decisions, filtering, and ranking.
type SearchIndexStoreFuncs struct {
	ManifestFunc  func(context.Context, IndexNamespace) ([]StoredChunkRef, error)
	ReconcileFunc func(context.Context, IndexNamespace, string, []IndexedChunk, []string) error
	NearestFunc   func(context.Context, []IndexNamespace, []float32, int) ([]ScoredChunkHit, error)
}

func (f SearchIndexStoreFuncs) Manifest(ctx context.Context, namespace IndexNamespace) ([]StoredChunkRef, error) {
	return f.ManifestFunc(ctx, namespace)
}

func (f SearchIndexStoreFuncs) Reconcile(ctx context.Context, namespace IndexNamespace, revision string, upserts []IndexedChunk, deletes []string) error {
	return f.ReconcileFunc(ctx, namespace, revision, upserts, deletes)
}

func (f SearchIndexStoreFuncs) Nearest(ctx context.Context, namespaces []IndexNamespace, vector []float32, limit int) ([]ScoredChunkHit, error) {
	return f.NearestFunc(ctx, namespaces, vector, limit)
}

// LLMExecutorFuncs adapts a raw model executor without moving prompt,
// parsing, validation, or gate semantics out of SDD.
type LLMExecutorFuncs struct {
	CapabilitiesFunc func(context.Context) ([]string, error)
	ExecuteFunc      func(context.Context, LLMRequest) (LLMResult, error)
}

func (f LLMExecutorFuncs) Capabilities(ctx context.Context) ([]string, error) {
	return f.CapabilitiesFunc(ctx)
}

func (f LLMExecutorFuncs) Execute(ctx context.Context, request LLMRequest) (LLMResult, error) {
	return f.ExecuteFunc(ctx, request)
}
