package query

import "github.com/networkteam/sdd/internal/model"

// LintQuery captures intent to surface graph integrity issues. Pure intent —
// the graph is held by the GraphFinder that runs the query.
type LintQuery struct{}

// IndexLintQuery captures intent to surface search-index health: the
// resolved embedding config (flag/config merging is the shell's job) and
// the local index location. Processed by Finder.IndexLint into the
// index-side fields of a LintResult.
type IndexLintQuery struct {
	Embedding model.EmbeddingConfig
	IndexDir  string
}

// LintResult is the structured output of a LintQuery: every entry that has
// at least one warning, plus the total warning count for convenience.
//
// Index-side fields (IndexConfigured, IndexEntryCount, IndexDriftCount,
// IndexFingerprint, RepoIndexDrift) are populated by Finder.IndexLint when
// an embedding provider is configured. They surface the search index's
// health alongside graph health so a single `sdd lint` run reports both.
type LintResult struct {
	Entries     []*model.Entry
	TotalIssues int

	// LoadErrors lists entries the graph loader could not parse. They are
	// counted in TotalIssues (so the exit code reflects them) and rendered in
	// their own section — an unreadable entry is a graph-integrity problem
	// even though it carries no in-graph warnings.
	LoadErrors []model.LoadIssue

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

	// RepoIndexDrift lists connected repos whose cache index holds entries
	// embedded under a fingerprint other than the shared (global) embedder.
	// A drifted repo re-embeds on the next cross-graph search; lint
	// surfaces it so the cost isn't a surprise. Populated by the CLI lint
	// action alongside the local index fields.
	RepoIndexDrift []RepoIndexDriftInfo
}

// RepoIndexDriftInfo names one connected repo's index drift.
type RepoIndexDriftInfo struct {
	RepoID     string
	DriftCount int
	EntryCount int
}
