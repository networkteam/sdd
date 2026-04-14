package finders

import (
	"context"

	"github.com/networkteam/resonance/framework/sdd/llm"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// Preflight runs the pre-flight validator against the given query.
// Delegates to the llm package for prompt rendering, LLM invocation, and
// result parsing. Returns the parsed result for both pass and fail cases.
// Returns an error only for infrastructure failures — a FAIL result is not an error.
func (f *Finder) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	result, err := llm.Preflight(ctx, f.preflightRunner, q.Entry, q.Graph)
	if err != nil {
		return nil, err
	}
	return &query.PreflightResult{
		Pass: result.Pass,
		Gaps: result.Gaps,
	}, nil
}
