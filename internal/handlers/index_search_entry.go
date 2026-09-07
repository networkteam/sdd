package handlers

import (
	"context"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
	"github.com/networkteam/sdd/pkg/application/types"
	"github.com/networkteam/sdd/pkg/llm/embed"
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
	inputs := func(yield func(indexInput, error) bool) { yield(indexInput{entry: h.Entry, version: key}, nil) }
	return indexStream(ctx, inputs, h.Embedder, h.Attachments, textsplitter.NewSplitter(), nil,
		func(ctx context.Context, entry *model.IndexWork) error {
			if err := types.ValidateEntryPublication(entry.Version, entry.Rows); err != nil {
				return err
			}
			if err := h.Store.PublishEntry(ctx, entry.Version, entry.Rows); err != nil {
				return err
			}
			if cmd.OnPublished != nil {
				cmd.OnPublished(key.EntryID, len(entry.Rows))
			}
			return nil
		}, nil)
}
