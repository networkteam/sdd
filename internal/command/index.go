package command

import "github.com/networkteam/sdd/internal/model"

// BuildIndexCmd warms up the search index by chunking, embedding, and
// upserting every entry on disk. The command is idempotent — re-running
// Build over an up-to-date index is a no-op when Force is false (entries
// whose hash and fingerprint match the manifest are skipped); Force=true
// re-embeds everything regardless. Nothing else is deleted: versions
// accumulate until `sdd index gc` (DropIndexVersionsCmd) removes them.
type BuildIndexCmd struct {
	// Force re-embeds every entry even when the manifest already records
	// an up-to-date row set, replacing the entry's versions under the current
	// derivation rule and leaving versions of other rules in place.
	Force bool

	// OnPlanned reports the number of entries needing indexing after selection.
	OnPlanned func(totalEntries int)

	// OnBatchStart is called before each embedding round-trip with the entry
	// IDs in that batch and their combined chunk count. Optional; names the
	// work in flight for a live status note. It does not advance the bar — the
	// bar advances only as entries complete (OnEntryIndexed), so it never reads
	// done before the work is.
	OnBatchStart func(entryIDs []string, chunkCount int)

	// OnEntryIndexed is called once per entry after its rows are upserted, with
	// the entry's chunk count. Progress advances by one published entry.
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
	// entry count to index, the authoritative progress total.
	OnPlanned func(totalEntries int)

	// OnBatchStart mirrors BuildIndexCmd's callback — fired before each
	// embedding round-trip with the batch's entry IDs and combined chunk count,
	// naming the work in flight.
	OnBatchStart func(entryIDs []string, chunkCount int)

	// OnEntryIndexed mirrors BuildIndexCmd's callback — the per-entry chunk
	// count; the bar advances by one entry as publication completes.
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
	OnPhase func(phase model.Phase)

	// OnPlanned fires once per repo, after that repo's skip pass, with the
	// entry count to index for it. The caller accumulates these into a
	// running total — the bar's denominator grows as each repo is reached,
	// because member work is only known after its cache is fresh.
	OnPlanned func(entries int)

	// OnBatchStart mirrors BuildIndexCmd's callback — fired before each
	// embedding round-trip with the batch's entry IDs and combined chunk
	// count, naming the work in flight.
	OnBatchStart func(entryIDs []string, chunkCount int)

	// OnEntryIndexed mirrors BuildIndexCmd's callback — the per-entry chunk
	// count; the bar advances by one entry as publication completes.
	OnEntryIndexed func(entryID string, chunkCount int)
}

// DropIndexVersionsCmd deletes stored index versions by group — the `sdd index
// gc --drop` path, the only deletion besides a force rebuild (d-tac-c9c). Groups
// are the names `sdd index gc` reports (index.VersionGroups); the stale
// selector expands to every droppable group. Cleanup is best effort: a group
// this store does not hold is skipped and reported, only the current group is
// refused.
type DropIndexVersionsCmd struct {
	Groups []string

	// OnDropped reports the outcome once, also when nothing matched.
	OnDropped func(DroppedIndexVersions)
}

// DroppedIndexVersions is one store's drop outcome.
type DroppedIndexVersions struct {
	Versions int
	Chunks   int
	// Bytes the removed rows occupied on disk.
	Bytes int64
	// Missing lists requested groups this store did not hold.
	Missing []string
}
