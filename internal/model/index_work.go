package model

import (
	"github.com/networkteam/sdd/internal/model/vectors"
	"github.com/networkteam/sdd/pkg/application/types"
)

type IndexWork struct {
	Version types.SearchEntryVersion
	Rows    []types.IndexedChunk
}

type IndexItem struct {
	Owner    *IndexWork
	Position int
}

type IndexBatch []IndexItem

func (b IndexBatch) Texts() []string {
	texts := make([]string, len(b))
	for i, item := range b {
		texts[i] = item.Owner.Rows[item.Position].Chunk.Text
	}
	return texts
}

func (b IndexBatch) EntryIDs() []string {
	var ids []string
	for i, item := range b {
		if i == 0 || b[i-1].Owner != item.Owner {
			ids = append(ids, item.Owner.Version.EntryID)
		}
	}
	return ids
}

func (b IndexBatch) Complete(embeddings [][]float32) ([]*IndexWork, error) {
	if err := vectors.Validate(embeddings, len(b)); err != nil {
		return nil, err
	}
	for i, item := range b {
		item.Owner.Rows[item.Position].Vector = embeddings[i]
	}
	var completed []*IndexWork
	for _, item := range b {
		if item.Position == len(item.Owner.Rows)-1 {
			completed = append(completed, item.Owner)
		}
	}
	return completed, nil
}
