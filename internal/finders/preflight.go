package finders

import (
	"context"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/query"
)

// Preflight runs the pre-flight validator against the given query.
// Runs Go-side mechanical checks first (see mechanicalPreflight), then
// delegates to the llm package for rubric-based checks, merging the
// findings so callers see a unified view. Returns an error only for
// infrastructure failures.
//
// Mechanical checks cover participant coverage (AC 6), actor canonical
// write-once (AC 5), and role canonical-match + refs-head (AC 7) per
// plan d-cpt-d34. The former LLM-judged participant-drift check is
// retired in favor of the mechanical canonical check.
//
// The language-drift check receives the configured graph language from
// config (see Finder.language). Empty means no language check (English
// default); a locale code activates the check against description prose.
func (f *Finder) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	findings := mechanicalPreflight(q.Entry, q.Graph)

	llmResult, err := llm.Preflight(ctx, f.preflightRunner, q.Entry, q.Graph, f.language())
	if err != nil {
		return nil, err
	}
	for _, fd := range llmResult.Findings {
		findings = append(findings, query.Finding{
			Severity:    query.Severity(fd.Severity),
			Category:    fd.Category,
			Observation: fd.Observation,
		})
	}
	return &query.PreflightResult{Findings: findings}, nil
}
