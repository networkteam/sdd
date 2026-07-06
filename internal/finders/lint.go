package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Lint returns every entry in the graph that has at least one warning,
// alongside the total warning count. Pure read — graph validation runs at
// graph-construction time, this just collects the results. Also flags entries
// that are missing a summary.
func (f *Finder) Lint(q query.LintQuery) (*query.LintResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	validateSummaries(q.Graph)

	entries := q.Graph.Lint()
	total := 0
	for _, e := range entries {
		total += len(e.Warnings)
	}
	return &query.LintResult{Entries: entries, TotalIssues: total}, nil
}

// validateSummaries flags entries that have no summary yet. Under the
// on-demand summary model (d-cpt-4qi) summaries carry no staleness tracking —
// an entry's meaning is fixed at creation, so there is nothing to detect drift
// against; lint only surfaces the absence of a summary.
func validateSummaries(graph *model.Graph) {
	for _, entry := range graph.Entries {
		// Embedded base entries ship their summary with the binary — there
		// is no file to regenerate.
		if entry.Embedded {
			continue
		}
		if entry.Summary == "" {
			entry.Warnings = append(entry.Warnings, model.Warning{
				Field:   "summary",
				Message: "missing summary (run sdd summarize to generate)",
			})
		}
	}
}
