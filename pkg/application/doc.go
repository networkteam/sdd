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
package application
