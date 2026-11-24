package types

import "time"

// StatusNoneLabel names the derived status of an entry without lifecycle
// state (a done signal) on the structured surface, where an empty value
// would be indistinguishable from an absent field.
const StatusNoneLabel = "none"

// GraphEntry is one graph entry with its graph-derived attributes, the
// structured form the application's reads return (d-cpt-kv4).
type GraphEntry struct {
	// ID is the entry's bare ID within its own graph.
	ID string
	// DisplayID is repo-prefixed (<repo-id>:<entry-id>) for an entry of a
	// connected repo, otherwise ID.
	DisplayID string
	// ShortID is the ID without its timestamp prefix ({type}-{layer}-{suffix}),
	// repo-prefixed like DisplayID.
	ShortID string
	// RepoID is empty for local and embedded entries.
	RepoID RepoID
	Type   string
	Kind   string
	Layer  string
	// Status is the derived lifecycle status (StatusNoneLabel for entries
	// without one). StatusBy is the live closing or superseding head for the
	// compound states, as a display ID; SupersedePath the origin→head trail
	// when superseded.
	Status        string
	StatusBy      string
	SupersedePath []string
	Created       time.Time
	Participants  []string
	// Topics are the effective topic labels, including annotation membership.
	Topics     []string
	Confidence string
	Intent     string
	Summary    string
	// Body is empty when the entry was loaded summary-only (show tree nodes).
	Body        string
	Refs        []EntryRef
	Closes      []string
	Supersedes  []string
	Attachments []string
}

// EntryRef is one stored reference of an entry.
type EntryRef struct {
	ID   string
	Kind string
	Desc string
}

// ShowGroup is one shown primary with its reference neighbourhood.
type ShowGroup struct {
	Primary    GraphEntry
	Upstream   []ShowTreeItem
	Downstream []ShowTreeItem
	// UpstreamTruncated and DownstreamTruncated are the primary's direct refs
	// a budget kept out of the direction.
	UpstreamTruncated   []TruncatedRef
	DownstreamTruncated []TruncatedRef
	// UpstreamTotal and DownstreamTotal count the primary's direct refs per
	// direction: depth-1 items plus the direction's truncated refs.
	UpstreamTotal   int
	DownstreamTotal int
}

// ShowTreeItem is one node of a show tree in depth-first pre-order.
type ShowTreeItem struct {
	Depth int
	// Relations names the edges from the parent, e.g. refs, closes,
	// supersedes upstream or refd-by, closed-by, superseded-by downstream.
	Relations []string
	RefKind   string
	RefDesc   string
	// LinkKind names the link as the citing entry sets it: RefKind, else
	// closes or supersedes for those relations, else refs (see model.LinkKind).
	LinkKind string
	// TargetID is the node's display ID.
	TargetID string
	// Entry is nil for an unresolved cross-repo target; at depth ≥ 1 it
	// carries no Body.
	Entry *GraphEntry
	// ShownAbove and ShownBelow mark a node already shown earlier or a later
	// primary; its children are not repeated.
	ShownAbove bool
	ShownBelow bool
	// Truncated are this node's children hidden at a frontier;
	// TruncatedReason names it when it is not the depth limit.
	Truncated       []TruncatedRef
	TruncatedReason string
}

// TruncatedRef names a child hidden at a show tree frontier.
type TruncatedRef struct {
	ID        string
	Relations []string
	Kind      string
	RefKind   string
	RefDesc   string
	// LinkKind names the link as ShowTreeItem.LinkKind does.
	LinkKind string
}

// EntryFacets counts a filtered entry set per attribute value.
type EntryFacets struct {
	Types    map[string]int
	Kinds    map[string]int
	Layers   map[string]int
	Statuses map[string]int
}

// AddedEntry is an entry present in a later revision but not an earlier one,
// with the IDs it references (refs, closes and supersedes).
type AddedEntry struct {
	ID     string
	RefIDs []string
}

// SearchHit is one ranked search result.
type SearchHit struct {
	Entry     GraphEntry
	Score     float32
	Citations []SearchCitation
}

// SearchCitation is the matched chunk that motivated a hit.
type SearchCitation struct {
	Breadcrumb           []string
	Snippet              string
	SourceAttachmentPath string
	IsSummary            bool
	IsAttachment         bool
	Score                float32
}
