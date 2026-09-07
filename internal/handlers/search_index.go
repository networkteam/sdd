package handlers

import (
	"context"

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
	publisher, complete := h.Store.(EntryPublisher)
	inputs := func(yield func(indexInput, error) bool) {
		for _, entry := range entries {
			version := types.SearchEntryVersion{Namespace: h.Namespace, EntryID: entry.ID, EntryHash: hashes[entry.ID]}
			if version.EntryHash == "" {
				hash, err := chunking.EntryStateHash(ctx, entry, h.Attachments)
				if err != nil {
					yield(indexInput{}, err)
					return
				}
				version.EntryHash = hash
			}
			if complete {
				present, err := publisher.EntryPublished(ctx, version)
				if err != nil {
					yield(indexInput{}, err)
					return
				}
				if present {
					continue
				}
			}
			if !yield(indexInput{entry: entry, version: version}, nil) {
				return
			}
		}
	}
	if complete {
		skip = nil
	}
	err := indexStream(ctx, inputs, h.Embedder, h.Attachments, textsplitter.NewSplitter(), skip,
		func(ctx context.Context, entry *model.IndexWork) error {
			if complete {
				if err := types.ValidateEntryPublication(entry.Version, entry.Rows); err != nil {
					return err
				}
				if err := publisher.PublishEntry(ctx, entry.Version, entry.Rows); err != nil {
					return err
				}
			} else {
				if len(entry.Rows) == 0 {
					return nil
				}
				if err := h.Store.Reconcile(ctx, h.Namespace, h.Revision, entry.Rows, nil); err != nil {
					return err
				}
			}
			h.entries++
			h.chunks += len(entry.Rows)
			if h.cmd.OnEntryIndexed != nil {
				h.cmd.OnEntryIndexed(entry.Version.EntryID, len(entry.Rows))
			}
			return nil
		}, nil)
	if err != nil {
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
