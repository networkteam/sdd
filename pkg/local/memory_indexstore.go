package local

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	app "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/application/types"
)

// MemorySearchIndexStore is a process-local mechanical vector index. It is
// useful for embedded compositions that want root-owned lazy reconciliation
// without operating a separate persistent index service.
type MemorySearchIndexStore struct {
	mu        sync.RWMutex
	chunks    map[app.IndexNamespace]map[string]app.IndexedChunk
	published map[app.SearchEntryVersion]bool
}

func NewMemorySearchIndexStore() *MemorySearchIndexStore {
	return &MemorySearchIndexStore{chunks: map[app.IndexNamespace]map[string]app.IndexedChunk{}, published: map[app.SearchEntryVersion]bool{}}
}

func (s *MemorySearchIndexStore) Manifest(_ context.Context, namespace app.IndexNamespace) ([]app.StoredChunkRef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.chunks[namespace]
	result := make([]app.StoredChunkRef, 0, len(items))
	for _, item := range items {
		result = append(result, app.StoredChunkRef{ID: item.Chunk.ID, Revision: item.Chunk.Revision, ContentHash: item.Chunk.ContentHash})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// IndexedEntries reports one ref per stored (entry, version) pair — the
// optional entry-manifest capability the application uses for monotonic,
// per-version reconciliation. Distinct (entry-id, entry-hash) pairs, sorted for
// deterministic output.
func (s *MemorySearchIndexStore) IndexedEntries(_ context.Context, namespace app.IndexNamespace) ([]app.StoredEntryRef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var refs []app.StoredEntryRef
	for version := range s.published {
		if version.Namespace == namespace {
			refs = append(refs, app.StoredEntryRef{EntryID: version.EntryID, EntryHash: version.EntryHash})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].EntryID != refs[j].EntryID {
			return refs[i].EntryID < refs[j].EntryID
		}
		return refs[i].EntryHash < refs[j].EntryHash
	})
	return refs, nil
}

func (s *MemorySearchIndexStore) Reconcile(ctx context.Context, namespace app.IndexNamespace, _ string, upserts []app.IndexedChunk, deletes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validateDimensions(namespace, upserts); err != nil {
		return err
	}
	if s.chunks[namespace] == nil {
		s.chunks[namespace] = map[string]app.IndexedChunk{}
	}
	for _, id := range deletes {
		if item, ok := s.chunks[namespace][id]; ok {
			delete(s.published, app.SearchEntryVersion{Namespace: namespace, EntryID: item.Chunk.EntryID, EntryHash: item.Chunk.EntryHash})
		}
		delete(s.chunks[namespace], id)
	}
	for _, item := range upserts {
		s.chunks[namespace][item.Chunk.ID] = cloneIndexedChunk(item)
		s.published[app.SearchEntryVersion{Namespace: namespace, EntryID: item.Chunk.EntryID, EntryHash: item.Chunk.EntryHash}] = true
	}
	return nil
}

func (s *MemorySearchIndexStore) validateDimensions(namespace app.IndexNamespace, rows []app.IndexedChunk) error {
	dims := 0
	for _, item := range s.chunks[namespace] {
		dims = len(item.Vector)
		break
	}
	for _, row := range rows {
		if len(row.Vector) == 0 {
			return fmt.Errorf("sdd: empty vector")
		}
		if dims == 0 {
			dims = len(row.Vector)
		}
		if len(row.Vector) != dims {
			return fmt.Errorf("sdd: inconsistent vector dimensions")
		}
		for _, v := range row.Vector {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("sdd: non-finite vector")
			}
		}
	}
	return nil
}

func cloneIndexedChunk(row app.IndexedChunk) app.IndexedChunk {
	row.Vector = append([]float32(nil), row.Vector...)
	row.Chunk.Breadcrumb = append([]string(nil), row.Chunk.Breadcrumb...)
	return row
}

func (s *MemorySearchIndexStore) EntryPublished(ctx context.Context, version app.SearchEntryVersion) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.published[version], nil
}

func (s *MemorySearchIndexStore) PublishEntry(ctx context.Context, version app.SearchEntryVersion, rows []app.IndexedChunk) error {
	if err := types.ValidateEntryPublication(version, rows); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.published[version] {
		return nil
	}
	if err := s.validateDimensions(version.Namespace, rows); err != nil {
		return err
	}
	if s.chunks[version.Namespace] == nil {
		s.chunks[version.Namespace] = map[string]app.IndexedChunk{}
	}
	for _, row := range rows {
		s.chunks[version.Namespace][row.Chunk.ID] = cloneIndexedChunk(row)
	}
	s.published[version] = true
	return nil
}

func (s *MemorySearchIndexStore) Nearest(_ context.Context, namespaces []app.IndexNamespace, vector []float32, limit int) ([]app.ScoredChunkHit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []app.ScoredChunkHit
	for _, namespace := range namespaces {
		for _, item := range s.chunks[namespace] {
			if !s.published[app.SearchEntryVersion{Namespace: namespace, EntryID: item.Chunk.EntryID, EntryHash: item.Chunk.EntryHash}] {
				continue
			}
			if len(vector) != len(item.Vector) {
				return nil, fmt.Errorf("sdd: query vector has %d dimensions, want %d", len(vector), len(item.Vector))
			}
			score, err := cosineSimilarity(vector, item.Vector)
			if err != nil {
				return nil, err
			}
			result = append(result, app.ScoredChunkHit{
				Namespace: namespace, ChunkID: item.Chunk.ID, EntryID: item.Chunk.EntryID,
				EntryHash: item.Chunk.EntryHash,
				Revision:  item.Chunk.Revision, ContentHash: item.Chunk.ContentHash, Score: score,
				Body: item.Chunk.Body, Breadcrumb: item.Chunk.Breadcrumb, Depth: item.Chunk.Depth,
				IsSummary: item.Chunk.IsSummary, IsAttachment: item.Chunk.IsAttachment,
				SourceAttachmentPath: item.Chunk.SourceAttachmentPath,
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].ChunkID < result[j].ChunkID
		}
		return result[i].Score > result[j].Score
	})
	if limit >= 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func cosineSimilarity(left, right []float32) (float64, error) {
	if len(left) != len(right) {
		return 0, fmt.Errorf("sdd: vector dimension mismatch")
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l, r := float64(left[i]), float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, nil
	}
	score := dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0, fmt.Errorf("sdd: invalid vector score")
	}
	return score, nil
}
