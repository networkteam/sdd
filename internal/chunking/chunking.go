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
func DeriveChunks(ctx context.Context, entry *model.Entry, splitter *textsplitter.Splitter, attachments AttachmentReader) ([]Chunk, error) {
	out := make([]Chunk, 0, 8)

	if sc, ok := splitter.SummaryChunk(entry.Summary); ok {
		out = append(out, Chunk{ChunkID: index.SummaryChunkID(entry.ID), Chunk: sc})
	}

	bodyOut, err := splitter.Split(textsplitter.SplitInput{
		Markdown:     model.ResolveAttachmentLinks(entry.Content, entry.ID),
		EntrySummary: entry.Summary,
	})
	if err != nil {
		return nil, fmt.Errorf("split body for %s: %w", entry.ID, err)
	}
	for i, c := range bodyOut.Chunks {
		out = append(out, Chunk{ChunkID: index.BodyChunkID(entry.ID, i), Chunk: c})
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
			out = append(out, Chunk{ChunkID: index.AttachmentChunkID(entry.ID, attRel, i), Chunk: c})
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
