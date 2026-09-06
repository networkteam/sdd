// Package application owns SDD's protocol-neutral runtime, public request and
// result types, and the infrastructure ports implemented by consumers.
//
// SearchRequest.SyncMode is required. Without ApplicationOptions.PrepareSearch,
// SearchSyncNone skips maintenance, SearchSyncLocal reconciles the selected
// home snapshot, and SearchSyncAll also reconciles searched dependencies.
// A supplied PrepareSearch callback owns preparation policy for the complete
// authorized SearchTarget. Its error-only result never asserts coverage.
// See the SearchTarget examples for synchronous and external composition.
//
// Semantic search derives coverage from published entry versions after
// preparation. This hashes the target's eligible entries and attachments even
// with SearchSyncNone. Retrieval verifies returned candidates against those
// same snapshots. Legacy adapters without SearchIndexEntryStore retain
// candidate-only verification without coverage metadata or custom preparation.
// Text-only search requires a mode but skips preparation and embedding coverage.
//
// ProjectRuntime.DiscoverSearchEntries streams revision-bound requirements;
// ProjectRuntime.IndexSearchEntry publishes one exact-source version atomically.
// ProjectRuntime.ReconcileSearchIndex remains a synchronous convenience.
// Consumers own authorization, durable source retention, scheduling and retries.
// Reconciliation adds versions; it does not watch for subsequent graph changes.
//
// # Consumer adoption
//
// Compose authorized project runtimes through the existing access resolver and
// register PrepareSearch once. MCP uses the same application. Every selected
// project needs SnapshotReader and SearchIndexEntryStore for custom preparation.
// Preserve SDD's Coverage and readable Notice in the consumer's search response.
//
// In the mutation finalizer or graph-write/recovery adapter, call
// AppliedMutation.AffectedEntryIDs. An empty result means no discovery job.
// Before enqueueing, durably retain a reproducible source and its attachments.
// This may be the finalized Git revision, rather than AppliedMutation.Revision
// from an earlier workspace apply. AffectedEntryIDs establishes no such guarantee.
// The consumer's write/recovery protocol must close any crash gap between commit,
// finalization and durable scheduling; a best-effort finalizer alone is insufficient.
//
// Queue selected IDs for write-triggered discovery and nil for cold search,
// periodic reconciliation or configuration changes. Acquire the exact retained
// source, then call DiscoverSearchEntries for either scope. Persist each cursor
// atomically with durable enqueueing or the record that published work needs no
// enqueue. Deduplicate indexing by full SearchEntryVersion, and run IndexSearchEntry
// with source retention through retries. Queue state never establishes coverage.
//
// Share one document batcher per embedding configuration and process. Compose
// query routing separately and provider deadlines and observation inside it.
// Configure explicit limits and measure provider/query latency in the consumer's
// workload; cross-process limits belong to the consumer. See embed.Batcher.
//
// Deploy publication-aware retrieval before asynchronous writers. The derivation
// schema participates in entry hashes, so prior rows can remain stored while
// current entries require fresh publication. Embedding configuration changes
// must change the fingerprint. Existing retention/rebuild tools own old-row cleanup.
package application
