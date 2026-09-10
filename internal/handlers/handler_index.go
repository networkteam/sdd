package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
	"github.com/networkteam/sdd/pkg/application/types"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/slogutils"
)

// IndexHandler is the side-effecting code path for building and lazily
// filling the search index. Per d-tac-lqr it owns chunking, embedding,
// and upserting; the SearchFinder is pure-read and consults the index
// via index.Index directly.
//
// Three operations:
//
//   - Build (sdd index): full warm-up over every entry on disk. Skips
//     entries whose manifest record is up-to-date unless Force is set.
//
//   - LazyFill (sdd search prelude): reconciles the manifest against
//     entries on disk — re-embeds entries that are missing, or whose
//     content hash / embedder fingerprint differs from the stored state.
//
//   - DropVersions (sdd index gc --drop): the only path that deletes stored
//     versions; Build and LazyFill only add.
type IndexHandler struct {
	graphDir    string
	indexDir    string
	embedder    IndexEmbedder
	splitter    *textsplitter.Splitter
	attachments chunking.AttachmentReader
	reader      Reader
	now         func() time.Time
	stderr      io.Writer
	// excludeEmbedded skips binary-scoped entries when indexing. Set for
	// connected-repo cache indexes: embedded entries are identical in every
	// member graph, so the local index alone covers them and cross-graph
	// search surfaces exactly one copy.
	excludeEmbedded bool
}

// IndexEmbedder carries the local composition size through CLI and MCP indexing.
type IndexEmbedder struct {
	embed.Embedder
	BatchSize int
}

// IndexHandlerOptions configures NewIndexHandler. Required fields are
// GraphDir, IndexDir, Embedder, Reader. Splitter defaults to
// textsplitter.NewSplitter() with default options when nil; Now defaults
// to time.Now. The store is loaded under the exclusive lock at write time
// (see index.WriteStore), so no pre-opened index is injected.
type IndexHandlerOptions struct {
	GraphDir string
	IndexDir string
	Embedder IndexEmbedder
	Splitter *textsplitter.Splitter
	Reader   Reader
	Now      func() time.Time
	Stderr   io.Writer
	// ExcludeEmbedded skips binary-scoped entries — set for connected-repo
	// cache indexes so embedded entries index only locally.
	ExcludeEmbedded bool
}

// NewIndexHandler constructs an IndexHandler with the given dependencies.
func NewIndexHandler(opts IndexHandlerOptions) *IndexHandler {
	h := &IndexHandler{
		graphDir:        opts.GraphDir,
		indexDir:        opts.IndexDir,
		embedder:        opts.Embedder,
		splitter:        opts.Splitter,
		attachments:     chunking.DiskAttachmentReader{GraphDir: opts.GraphDir},
		reader:          opts.Reader,
		now:             opts.Now,
		stderr:          opts.Stderr,
		excludeEmbedded: opts.ExcludeEmbedded,
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

// Build warms the index, persisting each complete entry before reporting progress.
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

func (h *IndexHandler) indexEntries(ctx context.Context, force bool,
	onPlanned func(int), onBatchStart func([]string, int), onIndexed func(string, int), onSkipped func(string), onComplete func(int, int)) error {

	g, err := h.reader.CurrentGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	// The whole write session — snapshot load, manifest read, skip pass,
	// embedding, upserts, manifest saves — runs under the store's exclusive
	// lock, acquired before the snapshot is loaded, so concurrent writers (a
	// CLI build racing the MCP server's reconcile) serialize instead of
	// interleaving chromem's per-document files.
	return index.WriteStore(ctx, h.indexDir, func(idx *index.Index) error {
		return h.indexEntriesLocked(ctx, idx, g, force, onPlanned, onBatchStart, onIndexed, onSkipped, onComplete)
	})
}

func (h *IndexHandler) indexEntriesLocked(ctx context.Context, idx *index.Index, g *model.Graph, force bool,
	onPlanned func(int), onBatchStart func([]string, int), onIndexed func(string, int), onSkipped func(string), onComplete func(int, int)) error {
	manifest, err := index.LoadManifest(h.indexDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	fingerprint := h.embedder.Fingerprint()
	currentHashes, err := h.currentEntryHashes(ctx, g)
	if err != nil {
		return err
	}
	var work []indexInput
	skipped := 0
	for _, entry := range g.Entries {
		hash, ok := currentHashes[entry.ID]
		if !ok {
			continue
		}
		if !force && manifest.Entries[entry.ID].HasVersion(hash, fingerprint) {
			skipped++
			if onSkipped != nil {
				onSkipped(entry.ID)
			}
			continue
		}
		work = append(work, indexInput{entry: entry, version: types.SearchEntryVersion{
			Namespace: types.IndexNamespace{Project: types.ProjectID(h.graphDir), Fingerprint: fingerprint, Metric: "cosine"}, EntryID: entry.ID, EntryHash: hash,
		}})
	}
	if onPlanned != nil {
		onPlanned(len(work))
	}
	inputs := func(yield func(indexInput, error) bool) {
		for _, input := range work {
			if !yield(input, nil) {
				return
			}
		}
	}
	indexed := 0
	err = indexStream(ctx, inputs, h.embedder, h.attachments, h.splitter, nil,
		func(ctx context.Context, entry *model.IndexWork) error {
			if err := h.publishIndexEntry(ctx, idx, manifest, entry, force); err != nil {
				return err
			}
			indexed++
			if onIndexed != nil {
				onIndexed(entry.Version.EntryID, len(entry.Rows))
			}
			return nil
		}, onBatchStart)
	if err != nil {
		return err
	}
	if onComplete != nil {
		onComplete(indexed, skipped)
	}
	return nil
}

// currentEntryHashes maps every indexable entry of the graph to its state hash
// — the checkout's current versions, which the skip pass and version grouping
// compare against.
func (h *IndexHandler) currentEntryHashes(ctx context.Context, g *model.Graph) (map[string]string, error) {
	hashes := make(map[string]string, len(g.Entries))
	for _, entry := range g.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !chunking.IncludeEntry(entry, h.excludeEmbedded) {
			continue
		}
		hash, err := chunking.EntryStateHash(ctx, entry, h.attachments)
		if err != nil {
			return nil, err
		}
		hashes[entry.ID] = hash
	}
	return hashes, nil
}

func (h *IndexHandler) publishIndexEntry(ctx context.Context, idx *index.Index, manifest *index.Manifest, entry *model.IndexWork, force bool) error {
	if err := types.ValidateEntryPublication(entry.Version, entry.Rows); err != nil {
		return err
	}
	rows := make([]index.Row, len(entry.Rows))
	ids := make([]string, len(rows))
	for i, row := range entry.Rows {
		c := row.Chunk
		ids[i] = c.ID
		rows[i] = index.Row{EntryID: c.EntryID, EntryHash: c.EntryHash, ChunkID: c.ID, Text: c.Text, Body: c.Body, Breadcrumb: c.Breadcrumb, Depth: c.Depth, IsSummary: c.IsSummary, IsAttachment: c.IsAttachment, SourceAttachmentPath: c.SourceAttachmentPath, ContentHash: c.ContentHash, ModelFingerprint: entry.Version.Namespace.Fingerprint, Embedding: row.Vector}
	}
	// Force replaces only this binary's own derivation rule: another rule's
	// versions belong to another binary sharing the store (d-tac-c9c).
	var old []string
	if force {
		old = manifest.Entries[entry.Version.EntryID].ChunkIDsOf(index.DerivationCurrent)
	}
	if err := idx.UpsertEntry(ctx, entry.Version.EntryID, old, rows); err != nil {
		return err
	}
	version := index.EntryVersion{Hash: entry.Version.EntryHash, Fingerprint: entry.Version.Namespace.Fingerprint, Derivation: index.DerivationCurrent, ChunkIDs: ids, IndexedAt: h.now()}
	if force {
		manifest.SetDerivationVersion(entry.Version.EntryID, version)
	} else {
		manifest.AddVersion(entry.Version.EntryID, version)
	}
	return manifest.Save(h.indexDir)
}

// DropVersions deletes the version groups cmd names from the store — the
// deliberate collection `sdd index gc --drop` runs (d-tac-c9c). Selection is
// resolved against the checkout's current hashes under the exclusive lock, so a
// concurrent writer cannot move a version between groups mid-drop.
func (h *IndexHandler) DropVersions(ctx context.Context, cmd *command.DropIndexVersionsCmd) error {
	if cmd == nil || len(cmd.Groups) == 0 {
		return errors.New("DropIndexVersionsCmd with at least one group is required")
	}
	g, err := h.reader.CurrentGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}
	return index.WriteStore(ctx, h.indexDir, func(idx *index.Index) error {
		manifest, err := index.LoadManifest(h.indexDir)
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		current, err := h.currentEntryHashes(ctx, g)
		if err != nil {
			return err
		}
		selected, err := manifest.SelectGroups(cmd.Groups, current)
		if err != nil {
			return err
		}
		chunkIDs, versions := manifest.DropGroups(selected, current)
		bytes := index.DocumentsSize(h.indexDir, chunkIDs)
		if len(chunkIDs) > 0 {
			if err := idx.DeleteEntry(ctx, chunkIDs); err != nil {
				return fmt.Errorf("deleting index rows: %w", err)
			}
		}
		if versions > 0 {
			if err := manifest.Save(h.indexDir); err != nil {
				return fmt.Errorf("save manifest after drop: %w", err)
			}
		}
		slogutils.FromContext(ctx).Info("dropped index versions", "groups", selected, "versions", versions, "chunks", len(chunkIDs))
		if cmd.OnDropped != nil {
			cmd.OnDropped(versions, len(chunkIDs), bytes)
		}
		return nil
	})
}
