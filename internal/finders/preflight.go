package finders

import (
	"context"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/query"
)

// Preflight runs the pre-flight validator against the given query.
// Delegates to the llm package for prompt rendering, LLM invocation, and
// result parsing. Returns the parsed result regardless of finding count or
// severity; the caller decides what to block on via HasBlocking. Returns
// an error only for infrastructure failures.
//
// The participant-drift check is single-path through the LLM layer: the
// finder supplies the canonical local participant from config (see
// Finder.localParticipant) as authoritative input alongside the graph's
// established-participant set. Per d-tac-6z1 the earlier deterministic
// Go-side check was removed — same-person variant detection is a
// semantic match the LLM handles uniformly with narrative-introduced
// voices.
//
// The language-drift check receives the configured graph language from
// config (see Finder.language). Empty means no language check (English
// default); a locale code activates the check against description prose.
func (f *Finder) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	result, err := llm.Preflight(ctx, f.preflightRunner, q.Entry, q.Graph, f.localParticipant(), f.language())
	if err != nil {
		return nil, err
	}
	findings := make([]query.Finding, 0, len(result.Findings))
	for _, fd := range result.Findings {
		findings = append(findings, query.Finding{
			Severity:    query.Severity(fd.Severity),
			Category:    fd.Category,
			Observation: fd.Observation,
		})
	}
	return &query.PreflightResult{Findings: findings}, nil
}
