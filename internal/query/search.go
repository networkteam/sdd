package query

import "github.com/networkteam/sdd/internal/model"

// SearchMode is derived from the SearchQuery's input shape: text-only,
// vector-only, or hybrid (when both --term and --query are supplied).
// Stored as a string so presenters and skill guidance can render it
// without importing the query package.
type SearchMode string

const (
	SearchModeText   SearchMode = "text"
	SearchModeVector SearchMode = "vector"
	SearchModeHybrid SearchMode = "hybrid"
)

// DefaultSearchLimit is the top-N applied when SearchQuery.Limit is zero.
const DefaultSearchLimit = 10

// DefaultMaxCitationsPerEntry caps the number of citations an entry may
// contribute to its result row. Three is enough to show "this entry
// matched on summary, body section X, and attachment section Y" without
// drowning the output in near-duplicate chunks. Override on a query
// for terser ("--max-citations 1") or wider scans.
const DefaultMaxCitationsPerEntry = 3

// SearchQuery carries the parsed input for the sdd search command. Pure
// intent: the graph it filters and resolves against is held by the
// SearchFinder that runs it, not carried here. The finder derives the mode
// from which fields are populated.
type SearchQuery struct {
	// Terms are the --term values: regex strings combined with AND. Each
	// must match somewhere in the entry's searchable text. Empty disables
	// text mode.
	Terms []string

	// Phrase is the --query value: a free-form phrase used for vector
	// search. Empty disables vector mode.
	Phrase string

	// Filter is the type/layer/kind filter (model.GraphFilter), the same
	// filter shape the view pipeline composes.
	Filter model.GraphFilter

	// IncludeSuperseded includes entries whose derived status is
	// superseded-by-something. Default false — superseded entries are
	// excluded so search seeds reflect the current shape of the graph.
	IncludeSuperseded bool

	// Limit caps the number of returned entries. Zero means
	// DefaultSearchLimit.
	Limit int

	// MaxCitationsPerEntry is the literal cap on citations a single entry
	// may contribute. The value is taken as-is: 0 suppresses citations
	// entirely (entry headers only — the mechanical "which entries match,
	// and what topics do they carry" lookup), 1 renders one line per entry,
	// higher surfaces more of the matching surface. Unlike EffectiveLimit's
	// zero-means-default rule, zero here means zero — the default is applied
	// at the CLI boundary via cmd.IsSet, so the struct field carries literal
	// intent and programmatic callers must set the cap they want.
	MaxCitationsPerEntry int

	// Repos selects connected repos to search in addition to the local
	// graph (additive, repeatable at the CLI as --repo). AllRepos selects
	// every connected repo. The orchestration layer resolves the selection
	// into per-repo search members; these fields carry the caller's intent.
	Repos    []string
	AllRepos bool
}

// Mode resolves the SearchQuery's input shape to a SearchMode.
func (q SearchQuery) Mode() SearchMode {
	hasTerms := len(q.Terms) > 0
	hasPhrase := q.Phrase != ""
	switch {
	case hasTerms && hasPhrase:
		return SearchModeHybrid
	case hasPhrase:
		return SearchModeVector
	default:
		return SearchModeText
	}
}

// EffectiveLimit returns Limit or DefaultSearchLimit when zero.
func (q SearchQuery) EffectiveLimit() int {
	if q.Limit > 0 {
		return q.Limit
	}
	return DefaultSearchLimit
}

// EffectiveMaxCitations returns the citation cap, clamping negatives to
// zero. Unlike EffectiveLimit, zero is NOT treated as "use the default":
// the field carries literal intent (0 means zero citations), and the
// default is resolved at the CLI boundary via cmd.IsSet.
func (q SearchQuery) EffectiveMaxCitations() int {
	if q.MaxCitationsPerEntry < 0 {
		return 0
	}
	return q.MaxCitationsPerEntry
}

// SearchResult carries the top-N entries with citations for rendering.
type SearchResult struct {
	Mode    SearchMode
	Entries []SearchEntry
}

// SearchEntry pairs a graph entry with its score and one-or-more
// citations. The citation slice is ordered best-first; for entries with
// multiple strongly-matching chunks (e.g. a long plan whose summary,
// approach section, and attachment all match a query) the slice carries
// each one so the agent reading the result sees the breadth of why the
// entry matched, not just its single strongest chunk.
type SearchEntry struct {
	Entry     *model.Entry
	Score     float32
	Citations []Citation
	// RepoID names the connected repo a cross-graph hit came from; empty
	// for local hits. Presenters render remote IDs with this prefix.
	RepoID string
}

// DisplayID is the identity a presenter renders: repo-prefixed for a
// cross-graph hit, bare for local ones.
func (e SearchEntry) DisplayID() string {
	if e.RepoID != "" {
		return e.RepoID + ":" + e.Entry.ID
	}
	return e.Entry.ID
}

// Citation returns the entry's primary citation — the best-scoring
// chunk. Used by callers that don't iterate the full Citations slice.
// Returns the zero value when Citations is empty (rare; only when no
// chunk hit could be associated with the entry).
func (e SearchEntry) Citation() Citation {
	if len(e.Citations) == 0 {
		return Citation{}
	}
	return e.Citations[0]
}

// Citation describes the chunk that motivated the hit — used to render
// the user-facing snippet so callers understand source. Breadcrumb is the
// heading-chain (empty for pre-heading body or summary chunks). Snippet
// is the citation body (~150 chars) with no Entry/Breadcrumb preamble.
//
// Score is the per-chunk score the finder computed (depth/summary
// adjusted, status-adjusted at entry level). Different citations from
// the same entry typically carry different scores — the presenter
// normalizes them to a relative percentage at render time so the
// reader sees both the strongest hit and how the weaker ones compare.
type Citation struct {
	Breadcrumb           []string
	Snippet              string
	SourceAttachmentPath string
	IsSummary            bool
	IsAttachment         bool
	Score                float32
}
