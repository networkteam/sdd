package finders

import (
	"context"

	"github.com/networkteam/resonance/framework/sdd/llm"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// Preflight runs the pre-flight validator against the given query.
// Delegates to the llm package for prompt rendering, LLM invocation, and
// result parsing. Returns the parsed result regardless of finding count or
// severity; the caller decides what to block on via HasBlocking. Returns
// an error only for infrastructure failures.
func (f *Finder) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	result, err := llm.Preflight(ctx, f.preflightRunner, q.Entry, q.Graph, q.ProposedAttachments)
	if err != nil {
		return nil, err
	}
	findings := make([]query.Finding, 0, len(result.Findings))
	for _, f := range result.Findings {
		findings = append(findings, query.Finding{
			Severity:    query.Severity(f.Severity),
			Category:    f.Category,
			Observation: f.Observation,
		})
	}
	return &query.PreflightResult{Findings: findings}, nil
}
