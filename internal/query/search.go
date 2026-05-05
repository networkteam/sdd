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

// SearchQuery carries the parsed input for the sdd search command. The
// finder derives the mode from which fields are populated.
type SearchQuery struct {
	// Graph is the loaded graph used for filtering and entry resolution.
	// Required.
	Graph *model.Graph

	// Terms are the --term values: regex strings combined with AND. Each
	// must match somewhere in the entry's searchable text. Empty disables
	// text mode.
	Terms []string

	// Phrase is the --query value: a free-form phrase used for vector
	// search. Empty disables vector mode.
	Phrase string

	// Filter is the type/layer/kind filter — same shape as ListQuery.
	Filter model.GraphFilter

	// IncludeSuperseded includes entries whose derived status is
	// superseded-by-something. Default false — superseded entries are
	// excluded so search seeds reflect the current shape of the graph.
	IncludeSuperseded bool

	// Limit caps the number of returned entries. Zero means
	// DefaultSearchLimit.
	Limit int

	// MaxCitationsPerEntry caps the number of citations a single entry
	// may contribute. Zero means DefaultMaxCitationsPerEntry. Set to 1
	// for one-citation-per-entry rendering; higher to surface more of
	// an entry's matching surface (e.g. for explore-mode briefings
	// where the breadth of why-it-matched is informative).
	MaxCitationsPerEntry int
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

// EffectiveMaxCitations returns MaxCitationsPerEntry or
// DefaultMaxCitationsPerEntry when zero. Negative values fall back to
// the default — the CLI surface validates earlier, but we degrade
// gracefully for programmatic callers.
func (q SearchQuery) EffectiveMaxCitations() int {
	if q.MaxCitationsPerEntry > 0 {
		return q.MaxCitationsPerEntry
	}
	return DefaultMaxCitationsPerEntry
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
