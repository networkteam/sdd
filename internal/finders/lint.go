package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Lint returns every entry in the graph that has at least one warning,
// alongside the total warning count. Pure read — graph validation runs at
// graph-construction time, this just collects the results. Also checks
// summary hash staleness.
func (f *Finder) Lint(q query.LintQuery) (*query.LintResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	// Check summary hashes for staleness.
	validateSummaryHashes(q.Graph)

	entries := q.Graph.Lint()
	total := 0
	for _, e := range entries {
		total += len(e.Warnings)
	}
	return &query.LintResult{Entries: entries, TotalIssues: total}, nil
}

// validateSummaryHashes checks all entries for stale or missing summaries
// and adds warnings to entries where the hash doesn't match.
func validateSummaryHashes(graph *model.Graph) {
	for _, entry := range graph.Entries {
		if entry.Summary == "" && entry.SummaryHash == "" {
			entry.Warnings = append(entry.Warnings, model.Warning{
				Field:   "summary",
				Message: "missing summary (run sdd summarize to generate)",
			})
			continue
		}

		if entry.Summary != "" && entry.SummaryHash == "" {
			entry.Warnings = append(entry.Warnings, model.Warning{
				Field:   "summary_hash",
				Message: "summary exists but no hash — run sdd summarize to regenerate",
			})
			continue
		}

		// Recompute hash and compare.
		prompt, err := llm.RenderSummaryPrompt(entry, graph)
		if err != nil {
			continue // skip entries where prompt rendering fails
		}
		currentHash := llm.ComputePromptHash(prompt)
		if entry.SummaryHash != currentHash {
			entry.Warnings = append(entry.Warnings, model.Warning{
				Field:   "summary_hash",
				Value:   entry.SummaryHash,
				Message: fmt.Sprintf("stale summary hash (stored %s..., current %s...) — run sdd summarize %s", entry.SummaryHash[:8], currentHash[:8], entry.ID),
			})
		}
	}
}
