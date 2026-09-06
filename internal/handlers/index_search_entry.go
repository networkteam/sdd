package handlers

import (
	"context"
	"fmt"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
	"github.com/networkteam/sdd/pkg/application/types"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/slogutils"
)

type EntryPublisher interface {
	EntryPublished(context.Context, types.SearchEntryVersion) (bool, error)
	PublishEntry(context.Context, types.SearchEntryVersion, []types.IndexedChunk) error
}

type SearchEntryHandler struct {
	Store       EntryPublisher
	Embedder    embed.Embedder
	Entry       *model.Entry
	Attachments chunking.AttachmentReader
}

func (h SearchEntryHandler) Index(ctx context.Context, cmd command.IndexSearchEntryCmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := cmd.Entry.Version
	published, err := h.Store.EntryPublished(ctx, key)
	if err != nil || published {
		return err
	}
	attachments := &chunking.CachedAttachments{Reader: h.Attachments}
	hash, err := chunking.EntryStateHash(ctx, h.Entry, attachments)
	if err != nil {
		return err
	}
	if hash != key.EntryHash {
		return fmt.Errorf("sdd: pinned entry content does not match descriptor")
	}
	chunks, err := chunking.DeriveChunks(ctx, h.Entry, hash, textsplitter.NewSplitter(), attachments)
	if err != nil {
		return err
	}
	rows := make([]types.IndexedChunk, len(chunks))
	if len(chunks) > 0 {
		texts := make([]string, len(chunks))
		for i, chunk := range chunks {
			texts[i] = chunk.Chunk.Text
		}
		result, err := h.Embedder.Embed(ctx, embed.Request{Purpose: embed.PurposeDocument, Texts: texts})
		if err != nil {
			return err
		}
		if len(result.Vectors) != len(chunks) {
			return fmt.Errorf("sdd: embedder returned %d vectors for %d chunks", len(result.Vectors), len(chunks))
		}
		for i, chunk := range chunks {
			rows[i] = types.IndexedChunk{Chunk: chunking.CanonicalChunk(h.Entry.ID, hash, chunk), Vector: result.Vectors[i]}
		}
	}
	if err := types.ValidateEntryPublication(key, rows); err != nil {
		return err
	}
	if err := h.Store.PublishEntry(ctx, key, rows); err != nil {
		return err
	}
	slogutils.FromContext(ctx).DebugContext(ctx, "published search entry", "project", key.Namespace.Project, "entry", key.EntryID, "hash", key.EntryHash, "chunks", len(rows))
	if cmd.OnPublished != nil {
		cmd.OnPublished(key.EntryID, len(rows))
	}
	return nil
}
