package chunking

import (
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/pkg/application/types"
)

func CanonicalChunk(entryID, entryHash string, c Chunk) types.CanonicalChunk {
	return types.CanonicalChunk{
		ID:                   c.ChunkID,
		EntryID:              entryID,
		ContentHash:          index.HashContent(c.Chunk.Text),
		Text:                 c.Chunk.Text,
		Body:                 c.Chunk.Body,
		Breadcrumb:           c.Chunk.Breadcrumb,
		Depth:                c.Chunk.Depth,
		IsSummary:            c.Chunk.IsSummary,
		IsAttachment:         c.Chunk.IsAttachment,
		SourceAttachmentPath: c.Chunk.SourceAttachmentPath,
		EntryHash:            entryHash,
	}
}
