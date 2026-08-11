package finders

import (
	"context"
	"fmt"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/query"
)

// WritingGuide runs the writing guide against the draft in the query. Unlike
// Preflight it takes no graph: the guide judges the draft in isolation — that
// scope is the check's design (d-cpt-20r), not a missing dependency, and the
// one thing it needs from outside the draft arrives already described in the
// query's closure targets. Returns an error only for infrastructure failures;
// findings, including none, are the result.
func (f *Finder) WritingGuide(ctx context.Context, q query.WritingGuideQuery) (*query.WritingGuideResult, error) {
	if f.writingGuideRunner == nil {
		return nil, fmt.Errorf("writing guide: no LLM runner configured")
	}
	result, err := llm.WritingGuide(ctx, f.writingGuideRunner, q.Entry, q.ClosureTargets)
	if err != nil {
		return nil, err
	}
	findings := make([]query.GuideFinding, 0, len(result.Findings))
	for _, fd := range result.Findings {
		findings = append(findings, query.GuideFinding{
			Reasoning: fd.Reasoning,
			Axis:      fd.Axis,
			Quote:     fd.Quote,
			Repair:    fd.Repair,
			Severity:  query.GuideSeverity(fd.Severity),
		})
	}
	return &query.WritingGuideResult{Findings: findings}, nil
}
