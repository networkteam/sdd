package application

import (
	"time"

	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/application/types"
)

// Default show expansion depths favor grounding context while keeping the
// typically wider consumer side shallow.
const (
	DefaultShowUpDepth   = query.DefaultUpDepth
	DefaultShowDownDepth = query.DefaultDownDepth
)

// InfoRequest selects current read authority when Branch is empty.
// A selected branch that cannot be acquired returns an error.
type InfoRequest struct {
	Branch            string
	BranchFromSession bool
}

type InfoResult struct {
	Project     ProjectRef
	Participant string
	Language    string
	Search      string
}

type ViewRequest struct {
	Layout            string
	Branch            string
	BranchFromSession bool
	Repos             []RepoID
	AllRepos          bool
	// Budget bounds the view's scaling parts on the serve path; the zero
	// value is unbounded — explicit pulls arrive complete (d-tac-rzi).
	Budget types.ViewBudget
}

type ViewResult struct {
	Project  ProjectRef
	Sections string
	// MatchedCount is the total primary units the layout produced across the
	// local graph and any queried dependency repos. Zero means the pipeline
	// matched nothing — surfaces distinguish an empty result from a failure
	// (an agent over MCP cannot tell a blank string from a broken call).
	MatchedCount int
	// KnownParticipants names the graph's canonical participants, populated
	// only when the result was empty and the layout carried a participant
	// filter — so an empty participant() view can say what names exist rather
	// than leaving an exact-match miss indistinguishable from no data.
	KnownParticipants []string
}

type ShowRequest struct {
	IDs               []string
	UpDepth           int
	DownDepth         int
	Branch            string
	BranchFromSession bool
	// Revision selects exactly that revision of the local graph; dependency
	// repos read at their current authority.
	Revision string
	// OmitMissing leaves IDs naming no entry out and lists them in
	// ShowResult.Missing; otherwise such an ID fails the read with
	// ErrorEntryNotFound.
	OmitMissing bool
	// Budget bounds each direction's chain expansion on the serve path; the
	// zero value is unbounded — explicit pulls arrive complete (d-tac-rzi).
	Budget types.ShowTreeBudget
}

// ShowResult carries the shown entries as data; surfaces render text from
// them (presenters.RenderShow).
type ShowResult struct {
	Project ProjectRef
	Entries []types.ShownEntry
	// FullIDs and SummaryIDs name the entries read in full and as summaries,
	// for read logging.
	FullIDs    []string
	SummaryIDs []string
	// Revision is the local snapshot's revision the result was read from.
	Revision string
	// Missing are the requested IDs, as given, that name no entry
	// (ShowRequest.OmitMissing).
	Missing []string
}

type SearchRequest struct {
	// IncludesRevision asks the source to observe a revision containing this write.
	IncludesRevision string
	// SyncMode is required, including for text-only requests.
	SyncMode          SearchSyncMode
	Terms             []string
	Phrase            string
	Branch            string
	BranchFromSession bool
	Type              string
	Layer             string
	Kind              string
	IncludeSuperseded bool
	// Statuses keeps only entries with one of these derived statuses before
	// ranking and limit. Naming superseded-by includes superseded entries.
	Statuses     []types.StatusKind
	Limit        int
	MaxCitations int
	Repos        []RepoID
	AllRepos     bool
}

// SearchResult carries the ranked hits as data; surfaces render text from
// them (presenters.RenderSearch). Notice is set when indexing was incomplete.
type SearchResult struct {
	Coverage []SearchCoverage
	Notice   string
	Project  ProjectRef
	Hits     []types.SearchHit
	EntryIDs []string
	// Revision is the local snapshot's revision the search ran against.
	Revision string
}

// ChangesRequest compares two revisions of the local graph on Branch. To
// empty selects the branch's current revision.
type ChangesRequest struct {
	Branch string
	From   string
	To     string
}

// ChangesResult lists entry IDs that changed from From to To, both resolved.
type ChangesResult struct {
	Project        ProjectRef
	From           string
	To             string
	Added          []types.AddedEntry
	StatusChanged  []string
	SummaryChanged []string
}

// ListEntriesRequest pages a filtered, time-ordered entry listing. Values
// within one filter field are alternatives; fields combine with AND. Types,
// Layers and Kinds accept the search filter forms, Statuses the structured
// labels, Terms the text search regex semantics over summary and body
// (attachments, which text search also reads, are not matched). Revision selects an exact
// local revision; Branch the branch's current one.
type ListEntriesRequest struct {
	// IDs, when non-nil, keeps only these entries, by full display ID
	// (<repo-id>:<entry-id> for a connected repo's entry, which also needs
	// that repo selected); unknown IDs match nothing. Naming an embedded
	// entry lists it without IncludeEmbedded.
	IDs           []string
	Branch        string
	Revision      string
	Types         []types.EntryType
	Kinds         []types.Kind
	Layers        []types.Layer
	Statuses      []types.StatusKind
	Participants  []string
	CreatedAfter  time.Time
	CreatedBefore time.Time
	Terms         []string
	// IncludeEmbedded lists the base entries compiled into sdd as well.
	IncludeEmbedded bool
	NewestFirst     bool
	Offset          int
	// Limit zero returns every entry from Offset.
	Limit    int
	Repos    []RepoID
	AllRepos bool
}

// ListEntriesResult is one page of the listing. Total and Facets cover the
// whole filtered set.
type ListEntriesResult struct {
	Project  ProjectRef
	Revision string
	Total    int
	Entries  []types.GraphEntry
	Facets   types.EntryFacets
}

type ReadAttachmentRequest struct {
	Branch            string
	BranchFromSession bool
	EntryID           string
	Filename          string
	Offset            int64
	MaxBytes          int
}

type ReadAttachmentResult struct {
	Project   ProjectRef
	Page      AttachmentPage
	Available []string
}

type ProcedureListRequest struct{}

type ProcedureListResult struct {
	Project    ProjectRef
	Procedures string
}
