package types

import "time"

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
	RepoID       RepoID
	Type         EntryType
	Kind         Kind
	Layer        Layer
	Status       Status
	Created      time.Time
	Participants []string
	// Topics are the effective topics, including annotation membership.
	Topics     []TopicPath
	Confidence Confidence
	Intent     Intent
	Summary    string
	// Lead is the summary's first sentence, else the body's; set also when
	// Body is left out.
	Lead string
	// Body is empty when the entry was loaded summary-only (show tree nodes).
	Body        string
	Refs        []EntryRef
	Closes      []string
	Supersedes  []string
	Attachments []string
	// Canonical and Aliases are set on actor entries, Canonical and Class on
	// procedures, Actor on roles.
	Canonical string
	Aliases   []string
	Class     ProcedureClass
	Actor     string
	// Index is set on facts enrolled in the fact index.
	Index *FactIndex
	// Procedure is set on procedure entries.
	Procedure *ProcedureSpec
}

// FactIndex is a fact's index enrollment.
type FactIndex struct {
	Title string
	Topic TopicPath
}

// ProcedureSpec is a procedure's machine part, each field the YAML text of
// that frontmatter key as stored (empty when absent).
type ProcedureSpec struct {
	Params  string
	State   string
	Steps   string
	Framing string
}

// EntryRef is one stored reference of an entry.
type EntryRef struct {
	ID   string
	Kind RefKind
	Desc string
}

// ShownEntry is one requested entry with its reference neighbourhood.
type ShownEntry struct {
	Entry      GraphEntry
	Upstream   []ShowTreeItem
	Downstream []ShowTreeItem
	// UpstreamTruncated and DownstreamTruncated are the entry's direct refs
	// a budget kept out of the direction.
	UpstreamTruncated   []TruncatedRef
	DownstreamTruncated []TruncatedRef
	// UpstreamTotal and DownstreamTotal count the entry's direct refs per
	// direction: depth-1 items plus the direction's truncated refs.
	UpstreamTotal   int
	DownstreamTotal int
}

// ShowTreeItem is one node of a show tree in depth-first pre-order.
type ShowTreeItem struct {
	Depth int
	// Relations names the edges from the parent, e.g. refs, closes,
	// supersedes upstream or refd-by, closed-by, superseded-by downstream.
	Relations []Relation
	RefKind   RefKind
	RefDesc   string
	// LinkKind names the link as the citing entry sets it: RefKind, else
	// closes or supersedes for those relations, else refs (see model.LinkKind).
	LinkKind LinkKind
	// TargetID is the node's display ID.
	TargetID string
	// Entry is nil for an unresolved cross-repo target; at depth ≥ 1 it
	// carries no Body.
	Entry *GraphEntry
	// ShownAbove and ShownBelow mark a node already shown earlier or a later
	// requested entry; its children are not repeated.
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
	Relations []Relation
	Kind      Kind
	RefKind   RefKind
	RefDesc   string
	// LinkKind names the link as ShowTreeItem.LinkKind does.
	LinkKind LinkKind
}

// EntryFacets counts a filtered entry set per attribute value.
type EntryFacets struct {
	Types    map[EntryType]int
	Kinds    map[Kind]int
	Layers   map[Layer]int
	Statuses map[StatusKind]int
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
