package finders

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/truncate"
)

// GitHistory is the git surface the graph export reads provenance from.
type GitHistory interface {
	// InWorkTree reports whether dir lies inside a git work tree.
	InWorkTree(ctx context.Context, dir string) bool
	// HeadRevision returns the commit hash HEAD points at, "" before the
	// first commit.
	HeadRevision(ctx context.Context, dir string) (string, error)
}

// attachmentSniffBytes is how much of a non-text attachment is read to
// detect its content type.
const attachmentSniffBytes = 512

// attachmentTypes pins the content type of the attachment formats graphs
// commonly carry, so the export does not depend on the host's MIME tables.
var attachmentTypes = map[string]string{
	".md":       "text/markdown; charset=utf-8",
	".markdown": "text/markdown; charset=utf-8",
	".txt":      "text/plain; charset=utf-8",
	".log":      "text/plain; charset=utf-8",
	".json":     "application/json",
}

// Export assembles the held graph and its dependency repos into one document
// carrying each entry with the attributes the engine derives for it in its
// owning graph (see query.ExportQuery).
func (gf *GraphFinder) Export(ctx context.Context, q query.ExportQuery) (*query.ExportResult, error) {
	if gf.graph == nil {
		return nil, fmt.Errorf("export: graph is required")
	}
	repoID, err := gf.finder.localRepoID()
	if err != nil {
		return nil, err
	}
	x := exporter{now: q.Now}
	if x.decay, err = model.DecayByName(model.DefaultDecayName); err != nil {
		return nil, err
	}

	local, err := gf.exportRepo(ctx, x, repoID, nil)
	if err != nil {
		return nil, err
	}
	local.Local = true
	result := &query.ExportResult{GeneratedAt: q.Now, Repos: []query.ExportRepo{local}}
	embedded := map[string]bool{}
	x.addEmbedded(result, embedded, gf.graph)
	if q.Dependencies == query.ExportDependenciesNone {
		return result, nil
	}

	closure, err := gf.finder.DependencyClosure(ctx, query.DependencyClosureQuery{})
	if err != nil {
		return nil, err
	}
	var cited map[model.RepoID]map[string]bool
	if q.Dependencies == query.ExportDependenciesReferenced {
		if cited, err = model.CitedAcross(gf.graph, closure, q.Hops); err != nil {
			return nil, err
		}
	}
	for _, id := range closure {
		member, err := gf.graph.MemberGraph(id)
		if err != nil {
			return nil, fmt.Errorf("loading graph for %s: %w", id, err)
		}
		if member == nil {
			result.Repos = append(result.Repos, query.ExportRepo{RepoID: id, Unavailable: true})
			continue
		}
		var selected map[string]bool
		if cited != nil {
			selected = cited[id]
			if selected == nil {
				selected = map[string]bool{}
			}
		}
		repo, err := gf.finder.OnGraph(member).exportRepo(ctx, x, id, selected)
		if err != nil {
			return nil, err
		}
		repo.Selection = &query.ExportSelection{Mode: q.Dependencies, Hops: q.Hops, TotalEntries: ownEntries(member)}
		result.Repos = append(result.Repos, repo)
		x.addEmbedded(result, embedded, member)
	}
	return result, nil
}

// exporter holds what every exported entry is derived against.
type exporter struct {
	decay model.DecayFunc
	now   time.Time
}

// addEmbedded appends the base entries g carries that are not listed yet,
// derived in g. Collecting them from every exported graph keeps a base entry
// the local graph overrides: a dependency that does not override it still
// references the original.
func (x exporter) addEmbedded(result *query.ExportResult, listed map[string]bool, g *model.Graph) {
	for _, e := range g.Entries {
		if e.Embedded && !listed[e.ID] {
			listed[e.ID] = true
			result.Embedded = append(result.Embedded, x.entry(g, e, nil))
		}
	}
}

// exportRepo exports the held graph's own entries (embedded base entries are
// exported once, outside any repo), restricted to selected when non-nil.
func (gf *GraphFinder) exportRepo(ctx context.Context, x exporter, repoID model.RepoID, selected map[string]bool) (query.ExportRepo, error) {
	g := gf.graph
	keep := func(id string) bool { return selected == nil || selected[id] }
	markers, err := gf.WIPMarkers()
	if err != nil {
		return query.ExportRepo{}, err
	}
	repo := query.ExportRepo{RepoID: repoID, LoadIssues: g.LoadIssues}
	for _, m := range markers {
		if keep(m.Entry) {
			repo.WIP = append(repo.WIP, m)
		}
	}

	if h, dir := gf.finder.gitHistory, g.GraphDir(); h != nil && dir != "" && h.InWorkTree(ctx, dir) {
		if repo.Revision, err = h.HeadRevision(ctx, dir); err != nil {
			return query.ExportRepo{}, err
		}
	}

	for _, e := range g.Entries {
		if e.Embedded || !keep(e.ID) {
			continue
		}
		attachments, err := gf.exportAttachments(e)
		if err != nil {
			return query.ExportRepo{}, err
		}
		repo.Entries = append(repo.Entries, x.entry(g, e, attachments))
	}
	return repo, nil
}

// entry derives one entry's exported attributes in its owning graph g.
func (x exporter) entry(g *model.Graph, e *model.Entry, attachments []query.ExportAttachment) query.ExportEntry {
	entry := query.ExportEntry{
		Entry:        e,
		Status:       g.DerivedStatus(e),
		ClosedBy:     g.ClosedBy[e.ID],
		SupersededBy: g.SupersededBy[e.ID],
		Topics:       g.EffectiveTopics(e),
		Heat:         model.HeatScore(g, e, x.decay, x.now),
		InDegree:     int(model.InDegreeScore(g, e)),
		Attachments:  attachments,
	}
	if key, ok := g.DisplayID(e.ID); ok && key != e.ID {
		entry.FullID = key
	}
	return entry
}

// ownEntries counts a graph's entries apart from the embedded base entries.
func ownEntries(g *model.Graph) int {
	n := 0
	for _, e := range g.Entries {
		if !e.Embedded {
			n++
		}
	}
	return n
}

// exportAttachments describes an entry's attachments through the shared
// accessor, inlining Markdown and plain-text content up to the export limit.
func (gf *GraphFinder) exportAttachments(e *model.Entry) ([]query.ExportAttachment, error) {
	var out []query.ExportAttachment
	for _, rel := range e.Attachments {
		name := filepath.Base(rel)
		head, err := gf.ReadAttachment(query.ReadAttachmentQuery{EntryID: e.ID, Name: name, MaxBytes: attachmentSniffBytes})
		if err != nil {
			return nil, err
		}
		a := query.ExportAttachment{Name: name, Size: head.TotalBytes, ContentType: attachmentContentType(name, head.Content)}
		if strings.HasPrefix(a.ContentType, "text/markdown") || strings.HasPrefix(a.ContentType, "text/plain") {
			// Read past the limit by one rune so the cut can tell a file that
			// ends exactly at the limit from a longer one.
			page, err := gf.ReadAttachment(query.ReadAttachmentQuery{EntryID: e.ID, Name: name, MaxBytes: query.ExportAttachmentTextLimit + utf8.UTFMax})
			if err != nil {
				return nil, err
			}
			text := truncate.Bytes(page.Content, query.ExportAttachmentTextLimit, "")
			a.Content, a.Truncated = text.Text, page.More || !text.Cut.Clean()
		}
		out = append(out, a)
	}
	return out, nil
}

func attachmentContentType(name, head string) string {
	if t, ok := attachmentTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return t
	}
	return http.DetectContentType([]byte(head))
}
