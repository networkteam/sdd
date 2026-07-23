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

	// OnPlanned is called once, after the skip pass decides what to embed and
	// before the first round-trip, with the total chunk count across all
	// entries to be embedded. Optional; the authoritative progress total —
	// embedding time scales with chunks, and this count comes from the same
	// skip logic that produces the work, so the bar's denominator matches what
	// actually runs.
	OnPlanned func(totalChunks int)

	// OnBatchStart is called before each embedding round-trip with the entry
	// IDs in that batch and their combined chunk count. Optional; names the
	// work in flight for a live status note. It does not advance the bar — the
	// bar advances only as entries complete (OnEntryIndexed), so it never reads
	// done before the work is.
	OnBatchStart func(entryIDs []string, chunkCount int)

	// OnEntryIndexed is called once per entry after its rows are upserted, with
	// the entry's chunk count. Optional; advances the progress bar by that many
	// chunks as work completes.
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
	// OnPlanned mirrors BuildIndexCmd's callback — fired once with the total
	// chunk count to embed, the authoritative progress total.
	OnPlanned func(totalChunks int)

	// OnBatchStart mirrors BuildIndexCmd's callback — fired before each
	// embedding round-trip with the batch's entry IDs and combined chunk count,
	// naming the work in flight.
	OnBatchStart func(entryIDs []string, chunkCount int)

	// OnEntryIndexed mirrors BuildIndexCmd's callback — the per-entry chunk
	// count that advances the bar as work completes.
	OnEntryIndexed func(entryID string, chunkCount int)

	// OnComplete is called once after lazy-fill finishes, with the count
	// of entries that were re-embedded.
	OnComplete func(indexed int)
}

// BuildConnectedIndexesCmd drives progress for filling one or more connected
// repos' member indexes — the eager `sdd index --repo/--all-repos` path and
// the fill half of a cross-repo search's prepare step. Each repo's fill is a
// LazyFill under the shared embedder unless Force is set, in which case every
// member entry re-embeds; the callbacks aggregate across repos (the caller
// accumulates OnPlanned totals rather than resetting per repo).
type BuildConnectedIndexesCmd struct {
	// Force re-embeds every member entry, mirroring `sdd index --force` for
	// the local index: it repairs a stale or corrupt connected store rather
	// than only filling what a lazy reconcile would touch. Search never
	// forces — only the explicit `sdd index --repo/--all-repos --force` sets it.
	Force bool

	// OnRepoStart fires before each repo's fill begins, naming the repo
	// whose member index is about to be reconciled. Optional; lets the
	// caller label the work in flight per repo.
	OnRepoStart func(repoID string)

	// OnPhase reports the active stage as the fill moves from freshening caches
	// (syncing) to embedding (indexing). Optional; the CLI maps it onto the
	// footer label so the transition is phase-true (never "indexing" while only
	// a cache pull is running). Emitted only when work actually happens.
	OnPhase func(phase Phase)

	// OnPlanned fires once per repo, after that repo's skip pass, with the
	// chunk count to embed for it. The caller accumulates these into a
	// running total — the bar's denominator grows as each repo is reached,
	// because member work is only known after its cache is fresh.
	OnPlanned func(chunks int)

	// OnBatchStart mirrors BuildIndexCmd's callback — fired before each
	// embedding round-trip with the batch's entry IDs and combined chunk
	// count, naming the work in flight.
	OnBatchStart func(entryIDs []string, chunkCount int)

	// OnEntryIndexed mirrors BuildIndexCmd's callback — the per-entry chunk
	// count that advances the bar as work completes.
	OnEntryIndexed func(entryID string, chunkCount int)
}
