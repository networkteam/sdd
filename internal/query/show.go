package query

import "github.com/networkteam/sdd/internal/model"

// ShowQuery captures intent to render a set of entries with their reference
// chains. Upstream and downstream each expand to their own depth; a depth of 0
// skips that direction (downstream off, or upstream off, or both → primary
// only).
type ShowQuery struct {
	Graph     *model.Graph
	IDs       []string
	UpDepth   int // upstream expansion depth (grounding chain); 0 = no upstream
	DownDepth int // downstream expansion depth (consumers); 0 = no downstream
}

// Default expansion depths applied by the CLI when the flags are omitted.
// Upstream goes deeper to capture an entry's grounding; downstream stays
// shallow because consumers fan out far faster (a hub contract has dozens of
// immediate referrers).
const (
	DefaultUpDepth   = 2
	DefaultDownDepth = 1
)

// ShowGroup is one primary's full tree: the primary entry, its upstream chain,
// and its downstream chain. Primary-derived attributes (status, supersede
// trail, effective topics) are carried alongside so presenters render without
// touching the graph. Multiple groups are joined with separators.
type ShowGroup struct {
	Primary *model.Entry
	// PrimaryID is the display identity — repo-prefixed
	// (<repo-id>:<entry-id>) when the primary lives in a connected repo's
	// graph, bare otherwise.
	PrimaryID            string
	PrimaryStatus        model.Status
	PrimarySupersedePath []string
	PrimaryTopics        []model.TopicPath
	Upstream             []model.ShowTreeItem
	Downstream           []model.ShowTreeItem
}

// ShowResult is the structured output for a ShowQuery — one group per primary.
type ShowResult struct {
	Graph  *model.Graph // needed to render derived attributes like status
	Groups []ShowGroup
}
