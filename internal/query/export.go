package query

import (
	"fmt"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// ExportDependencies selects what each dependency repo contributes to an
// export.
type ExportDependencies string

const (
	// ExportDependenciesNone exports the local graph only.
	ExportDependenciesNone ExportDependencies = "none"
	// ExportDependenciesReferenced exports the dependency entries the local
	// graph cites, expanded upstream by ExportQuery.Hops (model.CitedAcross).
	ExportDependenciesReferenced ExportDependencies = "referenced"
	// ExportDependenciesAll exports whole dependency graphs.
	ExportDependenciesAll ExportDependencies = "all"
)

// ParseExportDependencies resolves a dependency selection mode by name.
func ParseExportDependencies(s string) (ExportDependencies, error) {
	switch mode := ExportDependencies(s); mode {
	case ExportDependenciesNone, ExportDependenciesReferenced, ExportDependenciesAll:
		return mode, nil
	}
	return "", fmt.Errorf("unknown dependency selection %q (known: none, referenced, all)", s)
}

// ExportQuery captures intent to export the whole graph as one structured
// document. EXPERIMENTAL: built on the internal model; the shape is not a
// stable read contract.
type ExportQuery struct {
	// Dependencies selects what each dependency repo contributes.
	Dependencies ExportDependencies
	// DependencyIDs is the local repo's declared dependency closure, in walk
	// order (model.DependencyClosure).
	DependencyIDs []string
	// Hops bounds the upstream expansion of ExportDependenciesReferenced.
	Hops int
	// Now is the reference time heat is computed against.
	Now time.Time
}

// ExportAttachmentTextLimit caps the inlined content of a text attachment.
const ExportAttachmentTextLimit = 16 * 1024

// ExportResult is the structured output of an ExportQuery: the local graph
// first, then each dependency repo. Embedded carries the base entries
// compiled into the binary once, outside every repo: they are identical in
// each graph, so any repo's ID for one resolves there. Their derived
// attributes are the local graph's.
type ExportResult struct {
	GeneratedAt time.Time
	Repos       []ExportRepo
	Embedded    []ExportEntry
}

// ExportRepo is one exported graph. Unavailable marks a dependency with no
// loadable cache. Revision is empty when the graph is not inside a git work
// tree. Selection is set for dependency repos.
type ExportRepo struct {
	RepoID      string
	Local       bool
	Unavailable bool
	Revision    string
	Selection   *ExportSelection
	Entries     []ExportEntry
	WIP         []*model.WIPMarker
	LoadIssues  []model.LoadIssue
}

// ExportSelection records how much of a dependency graph was exported:
// TotalEntries counts the repo's own entries, exported or not.
type ExportSelection struct {
	Mode         ExportDependencies
	Hops         int
	TotalEntries int
}

// ExportEntry pairs an entry with the attributes the engine derives for it in
// its owning graph. FullID is set for entries of a dependency repo. LandedAt
// is zero when git history does not carry the entry file.
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
