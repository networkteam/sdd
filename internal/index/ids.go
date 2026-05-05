package index

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ChunkIDPrefix returns the per-entry prefix all this entry's chunk IDs
// share — used by Index.DeleteEntry's whereDocument filter (chromem-go
// supports id-prefix matching at delete-time via the metadata).
func ChunkIDPrefix(entryID string) string {
	return entryID + "#"
}

// SummaryChunkID is the deterministic chunk ID for an entry's summary
// chunk. Re-indexing the same entry produces the same ID — the indexer
// upserts via id rather than delete-and-add for the summary.
func SummaryChunkID(entryID string) string {
	return entryID + "#summary"
}

// BodyChunkID is the deterministic chunk ID for the n-th body chunk of an
// entry. n is positional in the chunker's emit order.
func BodyChunkID(entryID string, n int) string {
	return fmt.Sprintf("%s#body-%d", entryID, n)
}

// AttachmentChunkID is the deterministic chunk ID for the n-th chunk of
// the named attachment under entryID. attachmentPath is the entry-relative
// attachment path.
func AttachmentChunkID(entryID, attachmentPath string, n int) string {
	// Hash the attachment path so chunk IDs stay reasonably bounded and
	// don't include path separators that some downstream consumers might
	// interpret. Six hex chars is enough to disambiguate at our scale.
	h := sha256.Sum256([]byte(attachmentPath))
	short := hex.EncodeToString(h[:3])
	return fmt.Sprintf("%s#attach-%s-%d", entryID, short, n)
}

// HashContent returns a stable hex sha-256 of the chunk's embedded text.
// Used as the per-row content_hash metadata so future builds can detect
// when a row's source content changed.
func HashContent(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
