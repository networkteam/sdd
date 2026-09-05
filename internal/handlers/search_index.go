package handlers

import (
	"context"
	"fmt"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
	"github.com/networkteam/sdd/pkg/application/types"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

type SearchIndexStore interface {
	Manifest(context.Context, types.IndexNamespace) ([]types.StoredChunkRef, error)
	Reconcile(context.Context, types.IndexNamespace, string, []types.IndexedChunk, []string) error
}
type SearchIndexEntryManifest interface {
	IndexedEntries(context.Context, types.IndexNamespace) ([]types.StoredEntryRef, error)
}
type SearchIndexHandler struct {
	Store           SearchIndexStore
	Embedder        embed.Embedder
	Graph           *model.Graph
	Revision        string
	Namespace       types.IndexNamespace
	Hashes          map[string]string
	Attachments     chunking.AttachmentReader
	ExcludeEmbedded bool
	cmd             command.ReconcileSearchIndexCmd
	entries, chunks int
}
type versionKey struct{ entryID, entryHash string }

func (h *SearchIndexHandler) complete(ctx context.Context, entries []*model.Entry, hashes map[string]string, skip func(types.CanonicalChunk) bool) error {
	if err := h.embedEntries(ctx, h.Namespace, entries, hashes, skip); err != nil {
		return err
	}
	if h.cmd.OnComplete != nil {
		h.cmd.OnComplete(h.Revision, h.entries, h.chunks)
	}
	return nil
}
func (h *SearchIndexHandler) Reconcile(ctx context.Context, cmd command.ReconcileSearchIndexCmd) error {
	slogutils.FromContext(ctx).DebugContext(ctx, "reconciling search index", "project", h.Namespace.Project)
	namespace, hashes := h.Namespace, h.Hashes
	h.cmd = cmd
	h.entries, h.chunks = 0, 0
	if manifestCap, ok := h.Store.(SearchIndexEntryManifest); ok {
		indexed, err := manifestCap.IndexedEntries(ctx, namespace)
		if err != nil {
			return err
		}
		present := make(map[versionKey]bool, len(indexed))
		for _, ref := range indexed {
			present[versionKey{ref.EntryID, ref.EntryHash}] = true
		}
		var absent []*model.Entry
		for _, entry := range h.Graph.Entries {
			if !chunking.IncludeEntry(entry, h.ExcludeEmbedded) {
				continue
			}
			if present[versionKey{entry.ID, hashes[entry.ID]}] {
				continue
			}
			absent = append(absent, entry)
		}
		return h.complete(ctx, absent, hashes, nil)
	}
	return h.reconcileByChunkIdentity(ctx, namespace, hashes)
}

func (h *SearchIndexHandler) reconcileByChunkIdentity(ctx context.Context, namespace types.IndexNamespace, hashes map[string]string) error {
	manifest, err := h.Store.Manifest(ctx, namespace)
	if err != nil {
		return err
	}
	stored := make(map[string]types.StoredChunkRef, len(manifest))
	for _, ref := range manifest {
		stored[ref.ID] = ref
	}
	var entries []*model.Entry
	for _, entry := range h.Graph.Entries {
		if !chunking.IncludeEntry(entry, h.ExcludeEmbedded) {
			continue
		}
		entries = append(entries, entry)
	}
	keep := func(chunk types.CanonicalChunk) bool {
		ref, ok := stored[chunk.ID]
		return ok && ref.ContentHash == chunk.ContentHash
	}
	return h.complete(ctx, entries, hashes, keep)
}

func (h *SearchIndexHandler) embedEntries(ctx context.Context, namespace types.IndexNamespace, entries []*model.Entry, hashes map[string]string, skip func(types.CanonicalChunk) bool) error {
	if len(entries) == 0 {
		return nil
	}
	attachments := h.Attachments
	splitter := textsplitter.NewSplitter()

	var pending []types.CanonicalChunk
	for _, entry := range entries {
		hash := hashes[entry.ID]
		if hash == "" {
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
			chunk := chunking.CanonicalChunk(entry.ID, hash, c)
			if skip != nil && skip(chunk) {
				continue
			}
			pending = append(pending, chunk)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	texts := make([]string, len(pending))
	for i, chunk := range pending {
		texts[i] = chunk.Text
	}
	embedded, err := h.Embedder.Embed(ctx, embed.Request{Purpose: embed.PurposeDocument, Texts: texts})
	if err != nil {
		return err
	}
	if len(embedded.Vectors) != len(pending) {
		return fmt.Errorf("sdd: embedder returned %d vectors for %d inputs", len(embedded.Vectors), len(pending))
	}
	dims := 0
	upserts := make([]types.IndexedChunk, 0, len(pending))
	for i, vector := range embedded.Vectors {
		if len(vector) == 0 {
			return fmt.Errorf("sdd: embedding vector %d is empty", i)
		}
		if dims == 0 {
			dims = len(vector)
		}
		if len(vector) != dims {
			return fmt.Errorf("sdd: embedding vector %d has %d dimensions, want %d", i, len(vector), dims)
		}
		upserts = append(upserts, types.IndexedChunk{Chunk: pending[i], Vector: vector})
	}
	if err := h.Store.Reconcile(ctx, namespace, h.Revision, upserts, nil); err != nil {
		return err
	}
	counts := make(map[string]int)
	for _, chunk := range pending {
		counts[chunk.EntryID]++
	}
	for _, entry := range entries {
		if count := counts[entry.ID]; count > 0 {
			h.entries++
			if h.cmd.OnEntryIndexed != nil {
				h.cmd.OnEntryIndexed(entry.ID, count)
			}
		}
	}
	h.chunks += len(pending)
	return nil
}
