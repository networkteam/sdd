package application

import (
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Default show expansion depths favor grounding context while keeping the
// typically wider consumer side shallow.
const (
	DefaultShowUpDepth   = query.DefaultUpDepth
	DefaultShowDownDepth = query.DefaultDownDepth
)

type InfoRequest struct{}

type InfoResult struct {
	Project     ProjectRef
	Participant string
	Language    string
	Search      string
	Recovery    string
}

type ViewRequest struct {
	Layout            string
	Branch            string
	BranchFromSession bool
	Repos             []string
	AllRepos          bool
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
	// Budget bounds each direction's chain expansion on the serve path; the
	// zero value is unbounded — explicit pulls arrive complete (d-tac-rzi).
	Budget model.ShowTreeBudget
}

type ShowResult struct {
	Project    ProjectRef
	Entries    string
	FullIDs    []string
	SummaryIDs []string
}

type SearchRequest struct {
	Terms             []string
	Phrase            string
	Branch            string
	BranchFromSession bool
	Type              string
	Layer             string
	Kind              string
	IncludeSuperseded bool
	Limit             int
	MaxCitations      int
	Repos             []string
	AllRepos          bool
}

type SearchResult struct {
	Project  ProjectRef
	Results  string
	EntryIDs []string
}

type ReadAttachmentRequest struct {
	EntryID  string
	Filename string
	Offset   int64
	MaxBytes int
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
