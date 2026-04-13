package finders

import (
	"fmt"

	"github.com/networkteam/resonance/framework/sdd/query"
)

// Lint returns every entry in the graph that has at least one warning,
// alongside the total warning count. Pure read — graph validation runs at
// graph-construction time, this just collects the results.
func (f *Finder) Lint(q query.LintQuery) (*query.LintResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}
	entries := q.Graph.Lint()
	total := 0
	for _, e := range entries {
		total += len(e.Warnings)
	}
	return &query.LintResult{Entries: entries, TotalIssues: total}, nil
}
