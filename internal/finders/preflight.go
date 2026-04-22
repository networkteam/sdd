package finders

import (
	"context"
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Preflight runs the pre-flight validator against the given query.
// Delegates to the llm package for prompt rendering, LLM invocation, and
// result parsing. Returns the parsed result regardless of finding count or
// severity; the caller decides what to block on via HasBlocking. Returns
// an error only for infrastructure failures.
//
// In addition to the LLM-judged findings, a deterministic participant-drift
// check is appended to the result: any proposed participant name not
// matching an established name in the graph produces a medium-severity
// finding listing the established set, per d-tac-q5p.
func (f *Finder) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	result, err := llm.Preflight(ctx, f.preflightRunner, q.Entry, q.Graph)
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
	findings = append(findings, participantDriftFindings(q.Entry, q.Graph)...)
	return &query.PreflightResult{Findings: findings}, nil
}

// participantDriftFindings emits one medium-severity finding per proposed
// participant whose spelling doesn't match any name already in the graph.
// Per-participant (AC 10) so a multi-participant entry with one typo and
// one known name yields exactly one finding. Returns nil when the graph
// has no established participants (bootstrap case — nothing to drift from).
func participantDriftFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	if entry == nil || graph == nil || len(entry.Participants) == 0 {
		return nil
	}
	established := graph.AllParticipants()
	if len(established) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(established))
	for _, n := range established {
		known[n] = struct{}{}
	}
	joined := strings.Join(established, ", ")
	var out []query.Finding
	for _, p := range entry.Participants {
		if _, ok := known[p]; ok {
			continue
		}
		out = append(out, query.Finding{
			Severity: query.SeverityMedium,
			Category: "participant-drift",
			Observation: fmt.Sprintf(
				"participant %q does not match any established name in the graph (established: %s); if this is a typo, align to an existing spelling, otherwise confirm the new voice before proceeding",
				p, joined,
			),
		})
	}
	return out
}
