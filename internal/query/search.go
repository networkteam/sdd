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

// SearchResult carries the top-N entries with citations for rendering.
type SearchResult struct {
	Mode    SearchMode
	Entries []SearchEntry
}

// SearchEntry pairs a graph entry with its score and a one-best citation.
type SearchEntry struct {
	Entry    *model.Entry
	Score    float32
	Citation Citation
}

// Citation describes the chunk that motivated the hit — used to render
// the user-facing snippet so callers understand source. Breadcrumb is the
// heading-chain (empty for pre-heading body or summary chunks). Snippet
// is the citation body (~150 chars) with no Entry/Breadcrumb preamble.
type Citation struct {
	Breadcrumb           []string
	Snippet              string
	SourceAttachmentPath string
	IsSummary            bool
	IsAttachment         bool
}
