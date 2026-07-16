package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/textsplitter"
)

type applicationChunk struct {
	canonical  sddCanonicalChunk
	body       string
	breadcrumb []string
	summary    bool
}

type sddCanonicalChunk struct {
	public CanonicalChunk
	entry  *model.Entry
}

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
	chunks, err := deriveApplicationChunks(snapshot, request)
	if err != nil {
		return nil, err
	}
	desired := make(map[string]applicationChunk, len(chunks))
	for _, chunk := range chunks {
		desired[chunk.canonical.public.ID] = chunk
	}
	manifest, err := r.options.SearchIndex.Manifest(ctx, namespace)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]StoredChunkRef, len(manifest))
	for _, ref := range manifest {
		stored[ref.ID] = ref
	}
	var (
		upserts []IndexedChunk
		inputs  []EmbeddingInput
		deletes []string
	)
	for id, ref := range stored {
		chunk, ok := desired[id]
		if !ok {
			deletes = append(deletes, id)
			continue
		}
		if ref.Revision == chunk.canonical.public.Revision && ref.ContentHash == chunk.canonical.public.ContentHash {
			continue
		}
		inputs = append(inputs, EmbeddingInput{ID: id, Text: chunk.canonical.public.Text, Purpose: EmbeddingDocument})
	}
	for id, chunk := range desired {
		if _, ok := stored[id]; !ok {
			inputs = append(inputs, EmbeddingInput{ID: id, Text: chunk.canonical.public.Text, Purpose: EmbeddingDocument})
		}
	}
	if len(inputs) > 0 {
		vectors, err := r.options.Embeddings.Embed(ctx, inputs)
		if err != nil {
			return nil, err
		}
		if len(vectors) != len(inputs) {
			return nil, fmt.Errorf("sdd: embedding executor returned %d vectors for %d inputs", len(vectors), len(inputs))
		}
		dims := 0
		for i, vector := range vectors {
			if vector.ID != inputs[i].ID || len(vector.Values) == 0 {
				return nil, fmt.Errorf("sdd: embedding vector %d does not match its input", i)
			}
			if dims == 0 {
				dims = len(vector.Values)
			}
			if len(vector.Values) != dims {
				return nil, fmt.Errorf("sdd: embedding vector %d has %d dimensions, want %d", i, len(vector.Values), dims)
			}
			upserts = append(upserts, IndexedChunk{Chunk: desired[vector.ID].canonical.public, Vector: vector.Values})
		}
	}
	if len(upserts) > 0 || len(deletes) > 0 {
		if err := r.options.SearchIndex.Reconcile(ctx, namespace, snapshot.revision, upserts, deletes); err != nil {
			return nil, err
		}
	}
	queryVectors, err := r.options.Embeddings.Embed(ctx, []EmbeddingInput{{ID: "query", Text: request.Phrase, Purpose: EmbeddingQuery}})
	if err != nil {
		return nil, err
	}
	if len(queryVectors) != 1 || len(queryVectors[0].Values) == 0 {
		return nil, fmt.Errorf("sdd: embedding executor returned invalid query vector")
	}
	limit := request.EffectiveLimit()
	hits, err := r.options.SearchIndex.Nearest(ctx, []IndexNamespace{namespace}, queryVectors[0].Values, limit*4)
	if err != nil {
		return nil, err
	}
	vector := vectorResult(snapshot.graph, request, desired, hits)
	if len(request.Terms) == 0 {
		return vector, nil
	}
	text, err := findTextForHybrid(ctx, snapshot.graph, request)
	if err != nil {
		return nil, err
	}
	return hybridResult(text, vector, limit), nil
}

func deriveApplicationChunks(snapshot *Snapshot, request query.SearchQuery) ([]applicationChunk, error) {
	splitter := textsplitter.NewSplitter()
	var chunks []applicationChunk
	for _, entry := range snapshot.graph.Filter(request.Filter) {
		if !request.IncludeSuperseded && snapshot.graph.DerivedStatus(entry).Kind == model.StatusSupersededBy {
			continue
		}
		if chunk, ok := splitter.SummaryChunk(entry.Summary); ok {
			public := CanonicalChunk{ID: index.SummaryChunkID(entry.ID), EntryID: entry.ID, Revision: snapshot.revision, ContentHash: index.HashContent(chunk.Text), Text: chunk.Text}
			chunks = append(chunks, applicationChunk{canonical: sddCanonicalChunk{public: public, entry: entry}, body: chunk.Body, summary: true})
		}
		output, err := splitter.Split(textsplitter.SplitInput{Markdown: model.ResolveAttachmentLinks(entry.Content, entry.ID), EntrySummary: entry.Summary})
		if err != nil {
			return nil, err
		}
		for ordinal, chunk := range output.Chunks {
			public := CanonicalChunk{ID: index.BodyChunkID(entry.ID, ordinal), EntryID: entry.ID, Ordinal: ordinal, Revision: snapshot.revision, ContentHash: index.HashContent(chunk.Text), Text: chunk.Text}
			chunks = append(chunks, applicationChunk{canonical: sddCanonicalChunk{public: public, entry: entry}, body: chunk.Body, breadcrumb: chunk.Breadcrumb})
		}
	}
	return chunks, nil
}

func vectorResult(graph *model.Graph, request query.SearchQuery, desired map[string]applicationChunk, hits []ScoredChunkHit) *query.SearchResult {
	byEntry := map[string]*query.SearchEntry{}
	for _, hit := range hits {
		chunk, ok := desired[hit.ChunkID]
		if !ok || hit.Revision != chunk.canonical.public.Revision || hit.ContentHash != chunk.canonical.public.ContentHash {
			continue
		}
		score := float32(hit.Score) * publicStatusMultiplier(graph.DerivedStatus(chunk.canonical.entry).Kind)
		row := byEntry[chunk.canonical.entry.ID]
		if row == nil {
			row = &query.SearchEntry{Entry: chunk.canonical.entry, Score: score}
			byEntry[chunk.canonical.entry.ID] = row
		}
		if score > row.Score {
			row.Score = score
		}
		if request.EffectiveMaxCitations() > 0 {
			row.Citations = append(row.Citations, query.Citation{Breadcrumb: chunk.breadcrumb, Snippet: publicSnippet(chunk.body), IsSummary: chunk.summary, Score: score})
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
