// Package chunking is the single place a graph entry becomes index chunks.
// Both the CLI indexer (internal/handlers) and the application vector search
// derive chunks and entry-state hashes through here, so the shared persistent
// store holds identical content regardless of which path wrote it — the same
// chunk IDs, the same embedded text, the same entry hash.
//
// The chunk IDs and content hash come from internal/index; the splitting from
// internal/textsplitter. This package owns only the composition: how an entry
// and its Markdown attachments fan out into the summary chunk, body chunks,
// and per-attachment chunk sets that the index stores.
package chunking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
)

// AttachmentReader supplies the full bytes of one entry attachment named by
// its graph-relative path. The CLI reads from disk; the application pages the
// GraphStore. Both must yield identical bytes so derived chunks and the entry
// hash match across the two write paths that share one persistent store.
type AttachmentReader interface {
	ReadAttachment(ctx context.Context, entry *model.Entry, relPath string) ([]byte, error)
}

// Chunk pairs a derived chunk with its deterministic, stable ID.
type Chunk struct {
	ChunkID string
	Chunk   textsplitter.Chunk
}

// IncludeEntry reports whether an entry belongs in a store given the
// embedded-exclusion rule. The local/base store includes embedded
// (binary-shipped) entries — finders treat static and on-disk entries
// identically (d-cpt-dtv). Connected-repo stores exclude them so base facts
// embed once per machine, not once per connected repo.
func IncludeEntry(entry *model.Entry, excludeEmbedded bool) bool {
	return !excludeEmbedded || !entry.Embedded
}

// DeriveChunks produces every chunk an entry contributes to the index: its
// summary chunk, body chunks, and one chunk-set per Markdown attachment.
// Non-markdown attachments are skipped silently — chunking arbitrary binary
// or non-markdown text is out of scope for v1.
//
// entryHash is the entry's state hash (from EntryStateHash); new writes mint
// version-qualified chunk IDs (entryID#v-<hash8>#…) so a changed entry adds a
// version to the shared store rather than overwriting the old one. Both write
// paths (CLI indexer, application vector search) pass the same hash, so the
// derived IDs match across paths.
func DeriveChunks(ctx context.Context, entry *model.Entry, entryHash string, splitter *textsplitter.Splitter, attachments AttachmentReader) ([]Chunk, error) {
	out := make([]Chunk, 0, 8)

	if sc, ok := splitter.SummaryChunk(entry.Summary); ok {
		out = append(out, Chunk{ChunkID: index.SummaryChunkIDVersioned(entry.ID, entryHash), Chunk: sc})
	}

	bodyOut, err := splitter.Split(textsplitter.SplitInput{
		Markdown:     model.ResolveAttachmentLinks(entry.Content, entry.ID),
		EntrySummary: entry.Summary,
	})
	if err != nil {
		return nil, fmt.Errorf("split body for %s: %w", entry.ID, err)
	}
	for i, c := range bodyOut.Chunks {
		out = append(out, Chunk{ChunkID: index.BodyChunkIDVersioned(entry.ID, entryHash, i), Chunk: c})
	}

	for _, attRel := range entry.Attachments {
		if !strings.HasSuffix(strings.ToLower(attRel), ".md") {
			continue
		}
		data, err := attachments.ReadAttachment(ctx, entry, attRel)
		if err != nil {
			return nil, fmt.Errorf("read attachment %s: %w", attRel, err)
		}
		attOut, err := splitter.Split(textsplitter.SplitInput{
			Markdown:             string(data),
			EntrySummary:         entry.Summary,
			IsAttachment:         true,
			SourceAttachmentPath: attRel,
		})
		if err != nil {
			return nil, fmt.Errorf("split attachment %s: %w", attRel, err)
		}
		for i, c := range attOut.Chunks {
			out = append(out, Chunk{ChunkID: index.AttachmentChunkIDVersioned(entry.ID, entryHash, attRel, i), Chunk: c})
		}
	}

	return out, nil
}

// EntryStateHash combines the entry's body, summary, and each attachment's
// bytes into a single sha-256 digest. Stored in the manifest so a later build
// can detect changes without re-chunking and re-embedding. This is the
// definition CanonicalChunk.EntryHash and the CLI manifest hash share.
func EntryStateHash(ctx context.Context, entry *model.Entry, attachments AttachmentReader) (string, error) {
	hh := sha256.New()
	hh.Write([]byte(entry.Content))
	hh.Write([]byte("\n--summary--\n"))
	hh.Write([]byte(entry.Summary))
	for _, attRel := range entry.Attachments {
		data, err := attachments.ReadAttachment(ctx, entry, attRel)
		if err != nil {
			return "", fmt.Errorf("read attachment %s: %w", attRel, err)
		}
		hh.Write([]byte("\n--attach:"))
		hh.Write([]byte(attRel))
		hh.Write([]byte("--\n"))
		hh.Write(data)
	}
	return hex.EncodeToString(hh.Sum(nil)), nil
}

// DiskAttachmentReader reads attachments from a graph directory on disk. Used
// by the CLI indexer, whose entry.Attachments are graph-relative paths.
type DiskAttachmentReader struct {
	GraphDir string
}

func (r DiskAttachmentReader) ReadAttachment(_ context.Context, _ *model.Entry, relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.GraphDir, relPath))
}

// CachedAttachments keeps hashing and derivation on the same bytes within one
// entry operation. Its lifetime is bounded by that entry, not the whole graph.
type CachedAttachments struct {
	Reader  AttachmentReader
	content map[string][]byte
}

func (r *CachedAttachments) ReadAttachment(ctx context.Context, entry *model.Entry, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if content, ok := r.content[path]; ok {
		return content, nil
	}
	content, err := r.Reader.ReadAttachment(ctx, entry, path)
	if err != nil {
		return nil, err
	}
	if r.content == nil {
		r.content = map[string][]byte{}
	}
	r.content[path] = content
	return content, nil
}
