package types

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model/vectors"
)

// SearchEntryVersion is the publication and deduplication key. Revision is
// deliberately absent: identical content in different snapshots shares work.
type SearchEntryVersion struct {
	Namespace IndexNamespace
	EntryID   string
	EntryHash string
}

// SearchEntryDescriptor identifies exact retained input, including attachments.
// The host must retain SourceRevision until all work using it is finished.
type SearchEntryDescriptor struct {
	Version        SearchEntryVersion
	SourceRevision string
}

type IndexSearchEntryCmd struct {
	Entry       SearchEntryDescriptor
	OnPublished func(entryID string, chunks int)
}

// ValidateEntryPublication validates the entire write before a store changes.
func ValidateEntryPublication(version SearchEntryVersion, chunks []IndexedChunk) error {
	if version.Namespace.Project == "" || version.Namespace.Fingerprint == "" || version.Namespace.Metric != "cosine" || version.EntryID == "" || version.EntryHash == "" {
		return fmt.Errorf("sdd: incomplete entry version")
	}
	seen := make(map[string]bool, len(chunks))
	embeddings := make([][]float32, len(chunks))
	for i, row := range chunks {
		if row.Chunk.EntryID != version.EntryID || row.Chunk.EntryHash != version.EntryHash || row.Chunk.ID == "" || seen[row.Chunk.ID] {
			return fmt.Errorf("sdd: invalid or duplicate chunk identity %q", row.Chunk.ID)
		}
		seen[row.Chunk.ID] = true
		embeddings[i] = row.Vector
	}
	return vectors.Validate(embeddings, len(chunks))
}
