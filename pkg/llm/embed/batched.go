package embed

import (
	"context"
	"fmt"
)

// Batched decorates an Embedder so one request is served as calls of at most
// size texts each, in order, with the vectors concatenated and the usage
// summed. Batch size is a composition value like a deadline or a rate, so an
// adapter can stay a transport that sends exactly one request per Embed:
// decorators inside Batched then apply per round-trip, decorators outside
// apply per call. Identity is taken from the first call; the first failure
// aborts the rest and is returned as is.
func Batched(e Embedder, size int) Embedder {
	if size < 1 {
		panic(fmt.Sprintf("embed: batch size must be positive, got %d", size))
	}
	return decorated{inner: e, embed: func(ctx context.Context, req Request) (Result, error) {
		if len(req.Texts) <= size {
			return e.Embed(ctx, req)
		}
		result := Result{Vectors: make([][]float32, 0, len(req.Texts))}
		for start := 0; start < len(req.Texts); start += size {
			end := min(start+size, len(req.Texts))
			part, err := e.Embed(ctx, Request{Purpose: req.Purpose, Texts: req.Texts[start:end]})
			if err != nil {
				return Result{}, err
			}
			if start == 0 {
				result.Identity = part.Identity
			}
			result.Vectors = append(result.Vectors, part.Vectors...)
			result.Usage.InputTokens += part.Usage.InputTokens
			result.Usage.OutputTokens += part.Usage.OutputTokens
			result.Usage.CacheReadTokens += part.Usage.CacheReadTokens
			result.Usage.CacheCreateTokens += part.Usage.CacheCreateTokens
			result.Usage.CostUSD += part.Usage.CostUSD
		}
		return result, nil
	}}
}
