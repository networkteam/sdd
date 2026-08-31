package finders

import (
	"context"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
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
func (gf *GraphFinder) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	f := gf.finder
	findings := mechanicalPreflight(q.Entry, gf.graph, f.declaredDependencies(), f.procedureRegistry)

	llmResult, err := llm.Preflight(ctx, f.preflightRunner, q.Entry, gf.graph, f.language())
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

// Preflight validates an entry against the given graph. It is the explicit-
// graph entry point handlers use (they hold a graph mid-write and consume the
// finder through the handlers.Reader interface); it binds the graph and
// forwards to the GraphFinder so the validation logic stays single.
func (f *Finder) Preflight(ctx context.Context, g *model.Graph, q query.PreflightQuery) (*query.PreflightResult, error) {
	return f.OnGraph(g).Preflight(ctx, q)
}
