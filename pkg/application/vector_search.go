package application

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

// vectorSearch answers a phrase or hybrid search against the machine-global
// persistent index. The index is the monotonic union of chunks derived from
// immutable graph entries (d-cpt-e1i); graph revision is never a freshness
// token (d-cpt-65i). Every search:
//
//  1. resolves the embedding fingerprint and repository namespace;
//  2. optionally reconciles the index so missing entries are derived and embedded
//     once (including Markdown attachments) — nothing is ever deleted here;
//  3. embeds the query once and runs nearest-neighbor search;
//  4. resolves each hit's entry against the current graph and applies filter,
//     status, supersession, and embedded-entry rules at read time;
//  5. renders citations from stored hit metadata; fuses with text for hybrid.
func (r *ProjectRuntime) vectorSearch(ctx context.Context, snapshot *Snapshot, attachments GraphStore, request query.SearchQuery) (*query.SearchResult, error) {
	namespace, err := r.indexNamespace()
	if err != nil {
		return nil, err
	}

	var hashes map[string]string
	if request.SyncMode != query.SearchSyncNone {
		hashes, err = r.currentEntryHashes(ctx, snapshot.graph.Entries, attachments)
		if err != nil {
			return nil, err
		}
		if err := r.reconcileSearchSnapshot(ctx, snapshot, attachments, namespace, hashes, ReconcileSearchIndexCmd{}); err != nil {
			return nil, err
		}
	}

	embedded, err := r.options.Embedder.Embed(ctx, embed.Request{Purpose: embed.PurposeQuery, Texts: []string{request.Phrase}})
	if err != nil {
		return nil, err
	}
	if len(embedded.Vectors) != 1 || len(embedded.Vectors[0]) == 0 {
		return nil, fmt.Errorf("sdd: embedder returned an invalid query vector")
	}

	limit := request.EffectiveLimit()
	// Oversample chunks so the per-entry roll-up still fills top-N after
	// read-time filtering — a single large entry can own many chunks.
	chunkLimit := limit * 10
	if chunkLimit < 50 {
		chunkLimit = 50
	}
	hits, err := r.options.SearchIndex.Nearest(ctx, []IndexNamespace{namespace}, embedded.Vectors[0], chunkLimit)
	if err != nil {
		return nil, err
	}

	if request.SyncMode == query.SearchSyncNone {
		candidates := candidateSet(snapshot.graph, request, r.options.ExcludeEmbeddedFromIndex)
		var entries []*model.Entry
		for _, hit := range hits {
			if entry, ok := candidates[hit.EntryID]; ok && hit.EntryHash != "" {
				entries = append(entries, entry)
				delete(candidates, hit.EntryID)
			}
		}
		hashes, err = r.currentEntryHashes(ctx, entries, attachments)
		if err != nil {
			return nil, err
		}
	}
	vector := r.vectorResult(snapshot.graph, request, hits, hashes)
	if len(request.Terms) == 0 {
		return vector, nil
	}
	text, err := findTextForHybrid(ctx, snapshot.graph, request)
	if err != nil {
		return nil, err
	}
	return hybridResult(text, vector, limit), nil
}

// Reconciliation needs the full graph; no-sync search verifies only candidate
// versions so unrelated attachments do not contribute to search I/O.
func (r *ProjectRuntime) currentEntryHashes(ctx context.Context, entries []*model.Entry, store GraphStore) (map[string]string, error) {
	attachments := graphStoreAttachmentReader{store: store}
	hashes := make(map[string]string, len(entries))
	for _, entry := range entries {
		if !chunking.IncludeEntry(entry, r.options.ExcludeEmbeddedFromIndex) {
			continue
		}
		hash, err := chunking.EntryStateHash(ctx, entry, attachments)
		if err != nil {
			return nil, err
		}
		hashes[entry.ID] = hash
	}
	return hashes, nil
}

// vectorResult rolls chunk hits up to per-entry results, resolving each hit's
// entry against the current graph and applying the stale-version, filter,
// status, supersession, and embedded-entry rules at read time. A stored hit
// whose entry no longer exists, is filtered out, or belongs to a superseded
// version is ignored — the store is never mutated here.
func (r *ProjectRuntime) vectorResult(graph *model.Graph, request query.SearchQuery, hits []ScoredChunkHit, hashes map[string]string) *query.SearchResult {
	candidates := candidateSet(graph, request, r.options.ExcludeEmbeddedFromIndex)
	byEntry := map[string]*query.SearchEntry{}
	for _, hit := range hits {
		entry, ok := candidates[hit.EntryID]
		if !ok {
			continue
		}
		// Stale-version gate: keep the hit only when its version equals the
		// entry's current state. A hit whose version the store could not
		// resolve (empty EntryHash) is not filtered — a third-party store
		// without version identity degrades to entry-presence only.
		if hit.EntryHash != "" && hit.EntryHash != hashes[hit.EntryID] {
			continue
		}
		score := float32(hit.Score) * publicStatusMultiplier(graph.DerivedStatus(entry).Kind)
		row := byEntry[entry.ID]
		if row == nil {
			row = &query.SearchEntry{Entry: entry, Score: score}
			byEntry[entry.ID] = row
		}
		if score > row.Score {
			row.Score = score
		}
		if request.EffectiveMaxCitations() > 0 {
			row.Citations = append(row.Citations, query.Citation{
				Breadcrumb:           hit.Breadcrumb,
				Snippet:              publicSnippet(hit.Body),
				SourceAttachmentPath: hit.SourceAttachmentPath,
				IsSummary:            hit.IsSummary,
				IsAttachment:         hit.IsAttachment,
				Score:                score,
			})
		}
	}
	result := &query.SearchResult{Mode: query.SearchModeVector}
	for _, row := range byEntry {
		sort.Slice(row.Citations, func(i, j int) bool { return row.Citations[i].Score > row.Citations[j].Score })
		if len(row.Citations) > request.EffectiveMaxCitations() {
			row.Citations = row.Citations[:request.EffectiveMaxCitations()]
		}
		result.Entries = append(result.Entries, *row)
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Score > result.Entries[j].Score })
	if len(result.Entries) > request.EffectiveLimit() {
		result.Entries = result.Entries[:request.EffectiveLimit()]
	}
	return result
}

// candidateSet applies the read-time filter (type/layer/kind), the
// supersession gate, and the embedded-entry rule, keyed by entry ID for hit
// resolution. Entries not represented (filtered, superseded, excluded, or
// gone) are simply absent, so their stored hits are ignored.
func candidateSet(graph *model.Graph, request query.SearchQuery, excludeEmbedded bool) map[string]*model.Entry {
	out := map[string]*model.Entry{}
	for _, entry := range graph.Filter(request.Filter) {
		if !chunking.IncludeEntry(entry, excludeEmbedded) {
			continue
		}
		if !request.IncludeSuperseded && graph.DerivedStatus(entry).Kind == model.StatusSupersededBy {
			continue
		}
		out[entry.ID] = entry
	}
	return out
}

// graphStoreAttachmentReader reads attachment bytes by paging the GraphStore,
// so the application derives the same attachment content the CLI reads from
// disk. It is a pure read over the canonical graph authority.
type graphStoreAttachmentReader struct {
	store AttachmentPageReader
}

// attachmentPageSize is the page size for assembling an attachment's full
// bytes; attachments are small (Markdown design notes), so one or two pages
// cover them.
const attachmentPageSize = 1 << 20

func (r graphStoreAttachmentReader) ReadAttachment(ctx context.Context, entry *model.Entry, relPath string) ([]byte, error) {
	filename := path.Base(relPath)
	var content []byte
	offset := int64(0)
	for {
		page, err := r.store.ReadAttachmentPage(ctx, entry.ID, filename, offset, attachmentPageSize)
		if err != nil {
			return nil, err
		}
		content = append(content, page.Content...)
		if !page.More {
			break
		}
		if page.NextOffset <= offset {
			return nil, fmt.Errorf("sdd: attachment %s made no progress at offset %d", relPath, offset)
		}
		offset = page.NextOffset
	}
	return content, nil
}

func findTextForHybrid(ctx context.Context, graph *model.Graph, request query.SearchQuery) (*query.SearchResult, error) {
	return finders.NewSearchFinder(finders.SearchFinderOptions{Graph: graph}).Search(ctx, query.SearchQuery{
		Terms: request.Terms, Filter: request.Filter, IncludeSuperseded: request.IncludeSuperseded,
		Limit: request.Limit, MaxCitationsPerEntry: request.MaxCitationsPerEntry,
	})
}

func hybridResult(text, vector *query.SearchResult, limit int) *query.SearchResult {
	rows := map[string]*query.SearchEntry{}
	for _, source := range []struct {
		result *query.SearchResult
		weight float32
	}{{text, 1}, {vector, 1}} {
		for rank, entry := range source.result.Entries {
			row := rows[entry.Entry.ID]
			if row == nil {
				copy := entry
				copy.Score = 0
				row = &copy
				rows[entry.Entry.ID] = row
			}
			row.Score += source.weight / float32(60+rank+1)
			row.Citations = append(row.Citations, entry.Citations...)
		}
	}
	result := &query.SearchResult{Mode: query.SearchModeHybrid}
	for _, row := range rows {
		result.Entries = append(result.Entries, *row)
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Score > result.Entries[j].Score })
	if len(result.Entries) > limit {
		result.Entries = result.Entries[:limit]
	}
	return result
}

func publicStatusMultiplier(kind model.StatusKind) float32 {
	switch kind {
	case model.StatusSupersededBy:
		return 0.25
	case model.StatusClosedBy, model.StatusCascadeClosedBy:
		return 0.5
	default:
		return 1
	}
}

func publicSnippet(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if len(body) > 180 {
		return body[:177] + "..."
	}
	return body
}
