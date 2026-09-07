package handlers

import (
	"context"
	"fmt"
	"iter"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
	"github.com/networkteam/sdd/pkg/application/types"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

type indexInput struct {
	entry   *model.Entry
	version types.SearchEntryVersion
}

func indexBatchSize(emb embed.Embedder) int {
	if local, ok := emb.(IndexEmbedder); ok && local.BatchSize > 0 {
		return local.BatchSize
	}
	return 32
}

func prepareIndexEntry(ctx context.Context, input indexInput, reader chunking.AttachmentReader, splitter *textsplitter.Splitter, skip func(types.CanonicalChunk) bool) (*model.IndexWork, error) {
	attachments := &chunking.CachedAttachments{Reader: reader}
	hash, err := chunking.EntryStateHash(ctx, input.entry, attachments)
	if err != nil {
		return nil, err
	}
	if input.version.EntryHash != "" && hash != input.version.EntryHash {
		return nil, fmt.Errorf("sdd: pinned entry content does not match descriptor")
	}
	input.version.EntryHash = hash
	chunks, err := chunking.DeriveChunks(ctx, input.entry, hash, splitter, attachments)
	if err != nil {
		return nil, err
	}
	prepared := &model.IndexWork{Version: input.version}
	for _, chunk := range chunks {
		canonical := chunking.CanonicalChunk(input.entry.ID, hash, chunk)
		if skip == nil || !skip(canonical) {
			prepared.Rows = append(prepared.Rows, types.IndexedChunk{Chunk: canonical})
		}
	}
	return prepared, nil
}

func indexStream(ctx context.Context, inputs iter.Seq2[indexInput, error], emb embed.Embedder, reader chunking.AttachmentReader, splitter *textsplitter.Splitter, skip func(types.CanonicalChunk) bool, publish func(context.Context, *model.IndexWork) error, onBatch func([]string, int)) error {
	items := func(yield func(model.IndexItem, error) bool) {
		for input, err := range inputs {
			if err == nil {
				err = ctx.Err()
			}
			if err != nil {
				yield(model.IndexItem{}, err)
				return
			}
			entry, err := prepareIndexEntry(ctx, input, reader, splitter, skip)
			if err != nil {
				yield(model.IndexItem{}, err)
				return
			}
			if len(entry.Rows) == 0 {
				if err := publish(ctx, entry); err != nil {
					yield(model.IndexItem{}, err)
					return
				}
			}
			for i := range entry.Rows {
				if !yield(model.IndexItem{Owner: entry, Position: i}, nil) {
					return
				}
			}
		}
	}
	for batch, err := range model.Batches(items, indexBatchSize(emb)) {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		work := model.IndexBatch(batch)
		if onBatch != nil {
			onBatch(work.EntryIDs(), len(batch))
		}
		result, err := emb.Embed(ctx, embed.Request{Purpose: embed.PurposeDocument, Texts: work.Texts()})
		if err != nil {
			return err
		}
		completed, err := work.Complete(result.Vectors)
		if err != nil {
			return err
		}
		for _, entry := range completed {
			if err := types.ValidateEntryPublication(entry.Version, entry.Rows); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := publish(ctx, entry); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
