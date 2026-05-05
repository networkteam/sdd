package command

// BuildIndexCmd warms up the search index by chunking, embedding, and
// upserting every entry on disk. Existing rows for re-indexed entries are
// dropped before the new rows land. The command is idempotent — re-running
// Build over an up-to-date index is a no-op when Force is false (entries
// whose hash and fingerprint match the manifest are skipped); Force=true
// re-embeds and re-upserts everything regardless.
type BuildIndexCmd struct {
	// Force re-embeds every entry even when the manifest already records
	// an up-to-date row set.
	Force bool

	// OnEntryIndexed is called once per entry after its rows are upserted.
	// Optional; intended for CLI progress output.
	OnEntryIndexed func(entryID string, chunkCount int)

	// OnEntrySkipped is called for entries whose manifest record matches
	// the current state (skipped under Force=false). Optional.
	OnEntrySkipped func(entryID string)

	// OnComplete is called once after the build finishes successfully,
	// with the totals indexed and skipped. Optional.
	OnComplete func(indexed, skipped int)
}

// LazyFillIndexCmd reconciles the index against entries currently on disk:
// any entry not yet indexed (or whose stored hash/fingerprint differs from
// the configured embedder) is re-embedded and upserted. Used by sdd
// search before the query so cold-start cost on a fresh clone or branch
// switch is paid lazily rather than requiring an explicit warm-up.
type LazyFillIndexCmd struct {
	// OnEntryIndexed mirrors BuildIndexCmd's callback.
	OnEntryIndexed func(entryID string, chunkCount int)

	// OnComplete is called once after lazy-fill finishes, with the count
	// of entries that were re-embedded.
	OnComplete func(indexed int)
}
