package finders

import (
	"context"
	"fmt"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// WritingGuide runs the writing guide against the draft in the query. The
// guide judges the draft in isolation from the dialogue and the graph around
// it — that scope is the check's design (d-cpt-20r) — but it reads with the
// framework's own kind knowledge: the graph supplies the type-system overview
// and the drafted kind's authoring fact, rendered into the prompt so kind-fit
// judgments run on meaning rather than bare tokens (s-tac-fu8). Returns an
// error only for infrastructure failures; findings, including none, are the
// result.
func (f *Finder) WritingGuide(ctx context.Context, graph *model.Graph, q query.WritingGuideQuery) (*query.WritingGuideResult, error) {
	if f.writingGuideRunner == nil {
		return nil, fmt.Errorf("writing guide: no LLM runner configured")
	}
	refFacts := llm.ReferenceFacts{
		Source:           graphFactSource{graph: graph},
		TypeSystemFactID: basefacts.OverviewFactID,
		KindFactID:       basefacts.AuthoringFactID(q.Entry.Kind),
	}
	result, err := llm.WritingGuide(ctx, f.writingGuideRunner, q.Entry, q.ClosureTargets, refFacts)
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

// graphFactSource adapts model.Graph.FactBody to the prompt-inlining
// interface, which wants the body alone.
type graphFactSource struct {
	graph *model.Graph
}

func (s graphFactSource) FactBody(id string) (string, error) {
	_, body, err := s.graph.FactBody(id)
	return body, err
}
