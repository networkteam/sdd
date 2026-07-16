package application

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/textsplitter"
)

// vectorSearch answers a phrase or hybrid search against the machine-global
// persistent index. The index is the monotonic union of chunks derived from
// immutable graph entries (d-cpt-e1i); graph revision is never a freshness
// token (d-cpt-65i). Every search:
//
//  1. resolves the embedding fingerprint and repository namespace;
//  2. reconciles the index so entries absent from it are derived and embedded
//     once (including Markdown attachments) — nothing is ever deleted here;
//  3. embeds the query once and runs nearest-neighbor search;
//  4. resolves each hit's entry against the current graph and applies filter,
//     status, supersession, and embedded-entry rules at read time;
//  5. renders citations from stored hit metadata; fuses with text for hybrid.
func (r *ProjectRuntime) vectorSearch(ctx context.Context, snapshot *Snapshot, request query.SearchQuery) (*query.SearchResult, error) {
	if r.options.Embeddings == nil || r.options.SearchIndex == nil {
		return nil, errVectorUnavailable
	}
	spec, err := r.options.Embeddings.Spec(ctx)
	if err != nil {
		return nil, err
	}
	if spec.Fingerprint == "" {
		return nil, fmt.Errorf("sdd: embedding executor returned invalid spec")
	}
	namespace := IndexNamespace{Project: r.options.Project.ID, Fingerprint: spec.Fingerprint, Metric: "cosine"}

	// The current state hash of every entry that belongs in the store, computed
	// once: reconcile compares it against stored versions for presence, and the
	// read-time filter compares it against each hit's version.
	hashes, err := r.currentEntryHashes(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	if err := r.reconcileVectorIndex(ctx, snapshot, namespace, hashes); err != nil {
		return nil, err
	}

	queryVectors, err := r.options.Embeddings.Embed(ctx, []EmbeddingInput{{ID: "query", Text: request.Phrase, Purpose: EmbeddingQuery}})
	if err != nil {
		return nil, err
	}
	if len(queryVectors) != 1 || len(queryVectors[0].Values) == 0 {
		return nil, fmt.Errorf("sdd: embedding executor returned invalid query vector")
	}

	limit := request.EffectiveLimit()
	// Oversample chunks so the per-entry roll-up still fills top-N after
	// read-time filtering — a single large entry can own many chunks.
	chunkLimit := limit * 10
	if chunkLimit < 50 {
		chunkLimit = 50
	}
	hits, err := r.options.SearchIndex.Nearest(ctx, []IndexNamespace{namespace}, queryVectors[0].Values, chunkLimit)
	if err != nil {
		return nil, err
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

// versionKey identifies one stored (entry, version) pair for the presence
// check — a changed entry's new hash reads as absent even when a prior version
// of the same entry ID is present.
type versionKey struct {
	entryID   string
	entryHash string
}

// currentEntryHashes computes the state hash of every graph entry that belongs
// in the store, once per search. Attachment bytes are part of the hash, paged
// through the GraphStore exactly as chunking derives them. Reconcile compares
// these against stored versions for presence; the read-time filter compares
// them against each hit's version.
func (r *ProjectRuntime) currentEntryHashes(ctx context.Context, snapshot *Snapshot) (map[string]string, error) {
	attachments := graphStoreAttachmentReader{store: r.options.Graph}
	hashes := make(map[string]string, len(snapshot.graph.Entries))
	for _, entry := range snapshot.graph.Entries {
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

// reconcileVectorIndex ensures every graph entry version that belongs in the
// store is embedded and persisted. It walks the FULL current graph independent
// of request filters and status. A store advertising the entry-manifest
// capability reconciles on (entry, version) presence: a missing pair embeds
// that entry once and ADDS a version — never deleting another. A third-party
// store without the capability falls back to chunk-identity comparison. Both
// ignore graph revision and never issue deletes.
func (r *ProjectRuntime) reconcileVectorIndex(ctx context.Context, snapshot *Snapshot, namespace IndexNamespace, hashes map[string]string) error {
	if manifestCap, ok := r.options.SearchIndex.(SearchIndexEntryManifest); ok {
		indexed, err := manifestCap.IndexedEntries(ctx, namespace)
		if err != nil {
			return err
		}
		present := make(map[versionKey]bool, len(indexed))
		for _, ref := range indexed {
			present[versionKey{ref.EntryID, ref.EntryHash}] = true
		}
		var absent []*model.Entry
		for _, entry := range snapshot.graph.Entries {
			if !chunking.IncludeEntry(entry, r.options.ExcludeEmbeddedFromIndex) {
				continue
			}
			if present[versionKey{entry.ID, hashes[entry.ID]}] {
				continue
			}
			absent = append(absent, entry)
		}
		return r.embedEntries(ctx, snapshot, namespace, absent, hashes, nil)
	}
	return r.reconcileByChunkIdentity(ctx, snapshot, namespace, hashes)
}

// reconcileByChunkIdentity is the compatibility path for third-party stores
// that implement SearchIndexStore but not SearchIndexEntryManifest. It derives
// chunks for the full graph and embeds those whose chunk ID is absent from the
// store — graph revision ignored, no deletes. Because chunk IDs are now
// version-qualified, a changed entry produces new IDs and its new version is
// embedded while old-version rows remain (monotonic).
func (r *ProjectRuntime) reconcileByChunkIdentity(ctx context.Context, snapshot *Snapshot, namespace IndexNamespace, hashes map[string]string) error {
	manifest, err := r.options.SearchIndex.Manifest(ctx, namespace)
	if err != nil {
		return err
	}
	stored := make(map[string]StoredChunkRef, len(manifest))
	for _, ref := range manifest {
		stored[ref.ID] = ref
	}
	var entries []*model.Entry
	for _, entry := range snapshot.graph.Entries {
		if !chunking.IncludeEntry(entry, r.options.ExcludeEmbeddedFromIndex) {
			continue
		}
		entries = append(entries, entry)
	}
	keep := func(chunk CanonicalChunk) bool {
		ref, ok := stored[chunk.ID]
		return ok && ref.ContentHash == chunk.ContentHash
	}
	return r.embedEntries(ctx, snapshot, namespace, entries, hashes, keep)
}

// embedEntries derives, embeds, and persists the chunks of the given entries
// under their current version (version-qualified chunk IDs from the precomputed
// hash). When skip is non-nil, chunks it returns true for are already stored
// and are not re-embedded (the compatibility path uses this; the entry-manifest
// path passes nil because it only ever hands over absent entries). Attachments
// are read through the GraphStore so MCP and the CLI derive identical content.
func (r *ProjectRuntime) embedEntries(ctx context.Context, snapshot *Snapshot, namespace IndexNamespace, entries []*model.Entry, hashes map[string]string, skip func(CanonicalChunk) bool) error {
	if len(entries) == 0 {
		return nil
	}
	attachments := graphStoreAttachmentReader{store: r.options.Graph}
	splitter := textsplitter.NewSplitter()

	canonical := map[string]CanonicalChunk{}
	var inputs []EmbeddingInput
	for _, entry := range entries {
		hash := hashes[entry.ID]
		if hash == "" {
			// Defensive: an entry not covered by the precomputed set. Hash it
			// now so its version identity is still correct.
			h, err := chunking.EntryStateHash(ctx, entry, attachments)
			if err != nil {
				return err
			}
			hash = h
		}
		chunks, err := chunking.DeriveChunks(ctx, entry, hash, splitter, attachments)
		if err != nil {
			return err
		}
		for _, c := range chunks {
			chunk := canonicalChunk(entry.ID, hash, c)
			if skip != nil && skip(chunk) {
				continue
			}
			canonical[chunk.ID] = chunk
			inputs = append(inputs, EmbeddingInput{ID: chunk.ID, Text: chunk.Text, Purpose: EmbeddingDocument})
		}
	}
	if len(inputs) == 0 {
		return nil
	}
	vectors, err := r.options.Embeddings.Embed(ctx, inputs)
	if err != nil {
		return err
	}
	if len(vectors) != len(inputs) {
		return fmt.Errorf("sdd: embedding executor returned %d vectors for %d inputs", len(vectors), len(inputs))
	}
	dims := 0
	upserts := make([]IndexedChunk, 0, len(vectors))
	for i, vector := range vectors {
		if vector.ID != inputs[i].ID || len(vector.Values) == 0 {
			return fmt.Errorf("sdd: embedding vector %d does not match its input", i)
		}
		if dims == 0 {
			dims = len(vector.Values)
		}
		if len(vector.Values) != dims {
			return fmt.Errorf("sdd: embedding vector %d has %d dimensions, want %d", i, len(vector.Values), dims)
		}
		upserts = append(upserts, IndexedChunk{Chunk: canonical[vector.ID], Vector: vector.Values})
	}
	return r.options.SearchIndex.Reconcile(ctx, namespace, snapshot.revision, upserts, nil)
}

// canonicalChunk builds the public CanonicalChunk carrying the citation and
// identity metadata a store persists for a derived chunk.
func canonicalChunk(entryID, entryHash string, c chunking.Chunk) CanonicalChunk {
	return CanonicalChunk{
		ID:                   c.ChunkID,
		EntryID:              entryID,
		ContentHash:          index.HashContent(c.Chunk.Text),
		Text:                 c.Chunk.Text,
		Body:                 c.Chunk.Body,
		Breadcrumb:           c.Chunk.Breadcrumb,
		Depth:                c.Chunk.Depth,
		IsSummary:            c.Chunk.IsSummary,
		IsAttachment:         c.Chunk.IsAttachment,
		SourceAttachmentPath: c.Chunk.SourceAttachmentPath,
		EntryHash:            entryHash,
	}
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
	store GraphStore
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
	return finders.NewSearchFinder(finders.SearchFinderOptions{}).Search(ctx, query.SearchQuery{
		Graph: graph, Terms: request.Terms, Filter: request.Filter, IncludeSuperseded: request.IncludeSuperseded,
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
