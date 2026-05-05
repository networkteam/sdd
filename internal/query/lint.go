package query

import "github.com/networkteam/sdd/internal/model"

// LintQuery captures intent to surface graph integrity issues.
type LintQuery struct {
	Graph *model.Graph
}

// LintResult is the structured output of a LintQuery: every entry that has
// at least one warning, plus the total warning count for convenience.
//
// Index-side fields (IndexConfigured, IndexEntryCount, IndexDriftCount,
// IndexFingerprint) are populated by the CLI lint action when an
// embedding provider is configured — the finder itself stays pure-graph.
// They surface the search index's health alongside graph health so a
// single `sdd lint` run reports both.
type LintResult struct {
	Entries     []*model.Entry
	TotalIssues int

	// IndexConfigured is true when an embedding provider is configured.
	// When false, the rest of the index-side fields are zero values and
	// presenters skip rendering the index section.
	IndexConfigured bool
	// IndexEntryCount is the number of entries the manifest knows about.
	IndexEntryCount int
	// IndexDriftCount is the number of manifest entries whose recorded
	// fingerprint differs from the configured embedder. Run
	// `sdd index --force` (or just `sdd search` to lazy-fill) to
	// converge.
	IndexDriftCount int
	// IndexFingerprint is the current embedder fingerprint, included in
	// the lint output so the user can see what the index is being
	// compared against.
	IndexFingerprint string
}
