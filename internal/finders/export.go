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
	// FileArrivals maps each file under dir (slash-separated, relative to
	// dir) to the committer time of the first-parent commit that added it.
	FileArrivals(ctx context.Context, dir string) (map[string]time.Time, error)
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

// Export assembles the held graph and the selected connected repos into one
// document carrying each entry with the attributes the engine derives for it
// in its owning graph (see query.ExportQuery).
func (gf *GraphFinder) Export(ctx context.Context, q query.ExportQuery) (*query.ExportResult, error) {
	if gf.graph == nil {
		return nil, fmt.Errorf("export: graph is required")
	}
	repoID, err := gf.finder.localRepoID()
	if err != nil {
		return nil, err
	}
	decay, err := model.DecayByName(model.DefaultDecayName)
	if err != nil {
		return nil, err
	}

	local, err := gf.exportRepo(ctx, repoID, true, decay, q.Now)
	if err != nil {
		return nil, err
	}
	result := &query.ExportResult{GeneratedAt: q.Now, Repos: []query.ExportRepo{local}}
	for _, id := range q.RepoIDs {
		member, err := gf.graph.MemberGraph(id)
		if err != nil {
			return nil, fmt.Errorf("loading graph for %s: %w", id, err)
		}
		if member == nil {
			result.Repos = append(result.Repos, query.ExportRepo{RepoID: id, Unavailable: true})
			continue
		}
		repo, err := gf.finder.OnGraph(member).exportRepo(ctx, id, false, decay, q.Now)
		if err != nil {
			return nil, err
		}
		result.Repos = append(result.Repos, repo)
	}
	return result, nil
}

// exportRepo exports the held graph. Embedded base entries are identical in
// every graph, so only the local repo carries them.
func (gf *GraphFinder) exportRepo(ctx context.Context, repoID string, local bool, decay model.DecayFunc, now time.Time) (query.ExportRepo, error) {
	g := gf.graph
	wip, err := gf.WIPMarkers()
	if err != nil {
		return query.ExportRepo{}, err
	}
	repo := query.ExportRepo{RepoID: repoID, Local: local, WIP: wip, LoadIssues: g.LoadIssues}

	var arrivals map[string]time.Time
	if h, dir := gf.finder.gitHistory, g.GraphDir(); h != nil && dir != "" && h.InWorkTree(ctx, dir) {
		if repo.Revision, err = h.HeadRevision(ctx, dir); err != nil {
			return query.ExportRepo{}, err
		}
		if arrivals, err = h.FileArrivals(ctx, dir); err != nil {
			return query.ExportRepo{}, err
		}
	}

	for _, e := range g.Entries {
		if e.Embedded && !local {
			continue
		}
		entry := query.ExportEntry{
			Entry:        e,
			Status:       g.DerivedStatus(e),
			ClosedBy:     g.ClosedBy[e.ID],
			SupersededBy: g.SupersededBy[e.ID],
			Topics:       g.EffectiveTopics(e),
			Heat:         model.HeatScore(g, e, decay, now),
			InDegree:     int(model.InDegreeScore(g, e)),
		}
		if key, ok := g.DisplayID(e.ID); ok && key != e.ID {
			entry.FullID = key
		}
		if rel, err := model.IDToRelPath(e.ID); err == nil {
			entry.LandedAt = arrivals[filepath.ToSlash(rel)]
		}
		if entry.Attachments, err = gf.exportAttachments(e); err != nil {
			return query.ExportRepo{}, err
		}
		repo.Entries = append(repo.Entries, entry)
	}
	return repo, nil
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
