package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
	"github.com/networkteam/slogutils"
)

// IndexHandler is the side-effecting code path for building and lazily
// filling the search index. Per d-tac-lqr it owns chunking, embedding,
// and upserting; the SearchFinder is pure-read and consults the index
// via index.Index directly.
//
// Two operations:
//
//   - Build (sdd index): full warm-up over every entry on disk. Skips
//     entries whose manifest record is up-to-date unless Force is set.
//
//   - LazyFill (sdd search prelude): reconciles the manifest against
//     entries on disk — re-embeds entries that are missing, or whose
//     content hash / embedder fingerprint differs from the stored state.
type IndexHandler struct {
	graphDir   string
	indexDir   string
	embedder   llm.Embedder
	splitter   *textsplitter.Splitter
	indexStore *index.Index
	reader     Reader
	now        func() time.Time
	stderr     io.Writer
}

// IndexHandlerOptions configures NewIndexHandler. Required fields are
// GraphDir, IndexDir, Embedder, IndexStore, Reader. Splitter defaults to
// textsplitter.NewSplitter() with default options when nil; Now defaults
// to time.Now.
type IndexHandlerOptions struct {
	GraphDir   string
	IndexDir   string
	Embedder   llm.Embedder
	Splitter   *textsplitter.Splitter
	IndexStore *index.Index
	Reader     Reader
	Now        func() time.Time
	Stderr     io.Writer
}

// NewIndexHandler constructs an IndexHandler with the given dependencies.
func NewIndexHandler(opts IndexHandlerOptions) *IndexHandler {
	h := &IndexHandler{
		graphDir:   opts.GraphDir,
		indexDir:   opts.IndexDir,
		embedder:   opts.Embedder,
		splitter:   opts.Splitter,
		indexStore: opts.IndexStore,
		reader:     opts.Reader,
		now:        opts.Now,
		stderr:     opts.Stderr,
	}
	if h.splitter == nil {
		h.splitter = textsplitter.NewSplitter()
	}
	if h.now == nil {
		h.now = time.Now
	}
	if h.stderr == nil {
		h.stderr = os.Stderr
	}
	return h
}

// Build is the sdd-index warm-up. It loads the graph, derives chunks for
// every entry (skipping unchanged ones unless cmd.Force), embeds in
// batches across entries (one Embed call per outer batch), and upserts
// per-entry into the index. The manifest is saved after every batch — a
// crash mid-build leaves a partially-populated index that lazy-fill can
// finish later.
func (h *IndexHandler) Build(ctx context.Context, cmd *command.BuildIndexCmd) error {
	if cmd == nil {
		return errors.New("BuildIndexCmd is required")
	}
	return h.indexEntries(ctx, cmd.Force, cmd.OnPlanned, cmd.OnBatchStart, cmd.OnEntryIndexed, cmd.OnEntrySkipped, cmd.OnComplete)
}

// LazyFill is the sdd-search prelude — only entries missing from the
// manifest, or whose hash/fingerprint differs from current, are
// re-embedded.
func (h *IndexHandler) LazyFill(ctx context.Context, cmd *command.LazyFillIndexCmd) error {
	if cmd == nil {
		cmd = &command.LazyFillIndexCmd{}
	}
	onComplete := func(indexed, _ int) {
		if cmd.OnComplete != nil {
			cmd.OnComplete(indexed)
		}
	}
	return h.indexEntries(ctx, false, cmd.OnPlanned, cmd.OnBatchStart, cmd.OnEntryIndexed, nil, onComplete)
}

// indexEntries is the shared core for Build and LazyFill. The
// up-to-date check (manifest hash + fingerprint match) skips converged
// entries; force bypasses that check.
//
// Work is packed into buckets sized to the embedder's BatchSize so each
// EmbedDocuments call corresponds to a single transport round-trip on
// the model — progress callbacks fire per bucket (≈ per HTTP call)
// instead of waiting for one giant cross-entry batch to return. Each
// bucket holds entries whose total chunk count fits within BatchSize;
// an entry whose own chunks exceed BatchSize gets its own oversized
// bucket (the embedder splits internally).
//
// The manifest is saved after every bucket so a crash mid-build leaves
// a resumable state.
func (h *IndexHandler) indexEntries(ctx context.Context, force bool,
	onPlanned func(int), onBatchStart func([]string, int), onIndexed func(string, int), onSkipped func(string), onComplete func(int, int)) error {

	logger := slogutils.FromContext(ctx)

	g, err := h.reader.CurrentGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	manifest, err := index.LoadManifest(h.indexDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	fingerprint := h.embedder.Fingerprint()
	batchSize := h.embedder.BatchSize()
	if batchSize <= 0 {
		batchSize = 1 // defensive — no embedder should report 0, but a bucket of 1 still terminates
	}

	var (
		work    []entryWithChunks
		skipped int
	)

	for _, e := range g.Entries {
		hash, err := h.entryStateHash(e)
		if err != nil {
			logger.Warn("hash failure, skipping entry", "entry", e.ID, "err", err)
			continue
		}
		if !force {
			if state, ok := manifest.Entries[e.ID]; ok && state.Hash == hash && state.Fingerprint == fingerprint {
				logger.Debug("skipped, up to date", "entry", e.ID)
				if onSkipped != nil {
					onSkipped(e.ID)
				}
				skipped++
				continue
			}
		}
		chunks, err := h.deriveChunks(e)
		if err != nil {
			return fmt.Errorf("deriving chunks for %s: %w", e.ID, err)
		}
		work = append(work, entryWithChunks{entry: e, hash: hash, chunks: chunks})
	}

	// The authoritative progress total: total chunks across the work set,
	// from the same skip logic that produced it — so the bar's denominator
	// can't drift from what actually embeds. Reported before any round-trip.
	totalChunks := 0
	for _, w := range work {
		totalChunks += len(w.chunks)
	}
	if onPlanned != nil {
		onPlanned(totalChunks)
	}

	if len(work) == 0 {
		if onComplete != nil {
			onComplete(0, skipped)
		}
		return nil
	}

	indexed := 0
	bucketStart := 0
	bucketChunks := 0
	for i, w := range work {
		// Flush the current bucket when adding this entry's chunks would
		// exceed batchSize. Empty bucket case (single entry larger than
		// batchSize) takes the entry on its own — the embedder will
		// split internally on its way to the wire.
		if bucketChunks > 0 && bucketChunks+len(w.chunks) > batchSize {
			if err := h.indexBucket(ctx, work[bucketStart:i], manifest, fingerprint, onBatchStart, onIndexed); err != nil {
				return err
			}
			if err := manifest.Save(h.indexDir); err != nil {
				return fmt.Errorf("save manifest after bucket [%d:%d]: %w", bucketStart, i, err)
			}
			indexed += i - bucketStart
			bucketStart = i
			bucketChunks = 0
		}
		bucketChunks += len(w.chunks)
	}
	// Flush the trailing bucket.
	if bucketStart < len(work) {
		if err := h.indexBucket(ctx, work[bucketStart:], manifest, fingerprint, onBatchStart, onIndexed); err != nil {
			return err
		}
		if err := manifest.Save(h.indexDir); err != nil {
			return fmt.Errorf("save manifest after final bucket: %w", err)
		}
		indexed += len(work) - bucketStart
	}

	if onComplete != nil {
		onComplete(indexed, skipped)
	}
	return nil
}

// indexBucket embeds and upserts a single bucket of entries. All chunks
// across the bucket are embedded in one EmbedDocuments call — that's
// one transport round-trip on the model when the bucket is sized to
// the embedder's BatchSize. After the call returns, every entry in
// the bucket has all its embeddings ready and is upserted as a unit;
// the manifest entry follows.
func (h *IndexHandler) indexBucket(ctx context.Context, bucket []entryWithChunks,
	manifest *index.Manifest, fingerprint string,
	onBatchStart func([]string, int), onIndexed func(string, int)) error {

	logger := slogutils.FromContext(ctx)

	// Announce the batch (entry IDs + combined chunk count) before the
	// embedding round-trip so the view can name what's in flight while the
	// call runs. This does not advance the bar — that happens per entry as
	// work completes (report → onIndexed), so the bar never reads done before
	// the work is.
	bucketIDs := make([]string, len(bucket))
	bucketChunks := 0
	for i, w := range bucket {
		bucketIDs[i] = w.entry.ID
		bucketChunks += len(w.chunks)
	}
	if onBatchStart != nil {
		onBatchStart(bucketIDs, bucketChunks)
	}

	// report logs the entry at Info (the operational record routed to the
	// transient view or the leveled stderr handler) and fires the progress
	// callback. Keeps the log and the count advance together at each site.
	report := func(id string, n int) {
		logger.Info("indexed", "entry", id, "chunks", n)
		if onIndexed != nil {
			onIndexed(id, n)
		}
	}

	// Flatten chunks across the bucket while remembering which entry each
	// embedding belongs to.
	type ownedChunk struct {
		entryID string
		chunkID string
		chunk   textsplitter.Chunk
	}
	var allChunks []ownedChunk
	for _, w := range bucket {
		for _, c := range w.chunks {
			allChunks = append(allChunks, ownedChunk{entryID: w.entry.ID, chunkID: c.ChunkID, chunk: c.Chunk})
		}
	}

	if len(allChunks) == 0 {
		// Every entry in this bucket has no chunks (empty summary, no
		// body). Record them in the manifest anyway so the up-to-date
		// check skips them next pass.
		for _, w := range bucket {
			manifest.Entries[w.entry.ID] = index.EntryState{
				Hash:        w.hash,
				Fingerprint: fingerprint,
				ChunkIDs:    nil,
				IndexedAt:   h.now(),
			}
			report(w.entry.ID, 0)
		}
		return nil
	}

	texts := make([]string, len(allChunks))
	for i, c := range allChunks {
		texts[i] = c.chunk.Text
	}
	embeddings, err := h.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed bucket: %w", err)
	}
	if len(embeddings) != len(texts) {
		return fmt.Errorf("embedder returned %d embeddings for %d inputs", len(embeddings), len(texts))
	}

	rowsByEntry := map[string][]index.Row{}
	for i, c := range allChunks {
		rowsByEntry[c.entryID] = append(rowsByEntry[c.entryID], index.Row{
			EntryID:              c.entryID,
			ChunkID:              c.chunkID,
			Text:                 c.chunk.Text,
			Body:                 c.chunk.Body,
			Breadcrumb:           c.chunk.Breadcrumb,
			Depth:                c.chunk.Depth,
			IsSummary:            c.chunk.IsSummary,
			IsAttachment:         c.chunk.IsAttachment,
			SourceAttachmentPath: c.chunk.SourceAttachmentPath,
			ContentHash:          index.HashContent(c.chunk.Text),
			ModelFingerprint:     fingerprint,
			Embedding:            embeddings[i],
		})
	}

	for _, w := range bucket {
		rows := rowsByEntry[w.entry.ID]
		oldChunkIDs := manifest.Entries[w.entry.ID].ChunkIDs
		if err := h.indexStore.UpsertEntry(ctx, w.entry.ID, oldChunkIDs, rows); err != nil {
			return fmt.Errorf("upsert %s: %w", w.entry.ID, err)
		}
		newChunkIDs := make([]string, 0, len(rows))
		for _, r := range rows {
			newChunkIDs = append(newChunkIDs, r.ChunkID)
		}
		manifest.Entries[w.entry.ID] = index.EntryState{
			Hash:        w.hash,
			Fingerprint: fingerprint,
			ChunkIDs:    newChunkIDs,
			IndexedAt:   h.now(),
		}
		report(w.entry.ID, len(rows))
	}
	return nil
}

// entryWithChunks pairs an entry with its derived chunks and the hash
// recorded in the manifest. Lifted to a top-level type so indexBucket
// can take a slice without re-declaring the shape.
type entryWithChunks struct {
	entry  *model.Entry
	hash   string
	chunks []entryChunk
}

// entryChunk pairs a derived chunk with its deterministic chunk ID. Used
// internally so the index-write step can correlate chunks back to their
// stored row IDs after batch embedding.
type entryChunk struct {
	ChunkID string
	Chunk   textsplitter.Chunk
}

// deriveChunks produces every chunk this entry contributes to the index:
// the summary chunk, body chunks, and one chunk-set per .md attachment.
// Non-markdown attachments are skipped silently — chunking arbitrary
// binary or non-markdown text is out of scope for v1.
func (h *IndexHandler) deriveChunks(entry *model.Entry) ([]entryChunk, error) {
	out := make([]entryChunk, 0, 8)

	if sc, ok := h.splitter.SummaryChunk(entry.Summary); ok {
		out = append(out, entryChunk{ChunkID: index.SummaryChunkID(entry.ID), Chunk: sc})
	}

	bodyOut, err := h.splitter.Split(textsplitter.SplitInput{
		Markdown:     model.ResolveAttachmentLinks(entry.Content, entry.ID),
		EntrySummary: entry.Summary,
	})
	if err != nil {
		return nil, fmt.Errorf("split body for %s: %w", entry.ID, err)
	}
	for i, c := range bodyOut.Chunks {
		out = append(out, entryChunk{ChunkID: index.BodyChunkID(entry.ID, i), Chunk: c})
	}

	for _, attRel := range entry.Attachments {
		if !strings.HasSuffix(strings.ToLower(attRel), ".md") {
			continue
		}
		absPath := filepath.Join(h.graphDir, attRel)
		data, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read attachment %s: %w", attRel, err)
		}
		attOut, err := h.splitter.Split(textsplitter.SplitInput{
			Markdown:             string(data),
			EntrySummary:         entry.Summary,
			IsAttachment:         true,
			SourceAttachmentPath: attRel,
		})
		if err != nil {
			return nil, fmt.Errorf("split attachment %s: %w", attRel, err)
		}
		for i, c := range attOut.Chunks {
			out = append(out, entryChunk{ChunkID: index.AttachmentChunkID(entry.ID, attRel, i), Chunk: c})
		}
	}

	return out, nil
}

// entryStateHash combines the entry's body, summary, and each attachment's
// bytes into a single sha-256 digest. Stored in the manifest so a later
// build can detect changes without re-chunking and re-embedding.
func (h *IndexHandler) entryStateHash(entry *model.Entry) (string, error) {
	hh := sha256.New()
	hh.Write([]byte(entry.Content))
	hh.Write([]byte("\n--summary--\n"))
	hh.Write([]byte(entry.Summary))
	for _, attRel := range entry.Attachments {
		absPath := filepath.Join(h.graphDir, attRel)
		data, err := os.ReadFile(absPath)
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
