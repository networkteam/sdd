package types

import (
	"fmt"
	"math"
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
	dims := 0
	for _, row := range chunks {
		if row.Chunk.EntryID != version.EntryID || row.Chunk.EntryHash != version.EntryHash || row.Chunk.ID == "" || seen[row.Chunk.ID] {
			return fmt.Errorf("sdd: invalid or duplicate chunk identity %q", row.Chunk.ID)
		}
		seen[row.Chunk.ID] = true
		if len(row.Vector) == 0 {
			return fmt.Errorf("sdd: empty vector for %s", row.Chunk.ID)
		}
		if dims == 0 {
			dims = len(row.Vector)
		}
		if len(row.Vector) != dims {
			return fmt.Errorf("sdd: inconsistent vector dimensions")
		}
		norm := float64(0)
		for _, v := range row.Vector {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("sdd: non-finite vector")
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return fmt.Errorf("sdd: zero vector")
		}
	}
	return nil
}
