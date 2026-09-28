package query

import (
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// ExportQuery captures intent to export the whole graph as one structured
// document. EXPERIMENTAL: built on the internal model for UI prototyping; the
// shape is not a stable read contract.
type ExportQuery struct {
	// RepoIDs selects connected repos exported after the local graph.
	RepoIDs []string
	// Now is the reference time heat is computed against.
	Now time.Time
}

// ExportAttachmentTextLimit caps the inlined content of a text attachment.
const ExportAttachmentTextLimit = 16 * 1024

// ExportResult is the structured output of an ExportQuery: the local graph
// first, then each selected connected repo.
type ExportResult struct {
	GeneratedAt time.Time
	Repos       []ExportRepo
}

// ExportRepo is one exported graph. Unavailable marks a selected repo with no
// loadable cache. Revision is empty when the graph is not inside a git work
// tree.
type ExportRepo struct {
	RepoID      string
	Local       bool
	Unavailable bool
	Revision    string
	Entries     []ExportEntry
	WIP         []*model.WIPMarker
	LoadIssues  []model.LoadIssue
}

// ExportEntry pairs an entry with the attributes the engine derives for it in
// its owning graph. FullID is set for entries of a connected repo. LandedAt is
// zero when git history does not carry the entry file.
type ExportEntry struct {
	Entry        *model.Entry
	FullID       string
	Status       model.Status
	ClosedBy     []string
	SupersededBy []string
	Topics       []model.TopicPath
	Heat         float64
	InDegree     int
	LandedAt     time.Time
	Attachments  []ExportAttachment
}

// ExportAttachment describes one attachment file; Content is set only for
// Markdown and plain-text attachments, cut to ExportAttachmentTextLimit.
type ExportAttachment struct {
	Name        string
	Size        int64
	ContentType string
	Content     string
	Truncated   bool
}
