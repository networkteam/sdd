// Package application owns SDD's protocol-neutral runtime, public request and
// result types, and the infrastructure ports implemented by consumers.
//
// # Acquired graph reads
//
// Each application read acquires its source through SnapshotReader when the
// graph store implements it. Branch selection never calls TargetAcquirer.
// An empty branch selects the runtime's current authority, independently of
// DefaultBranch. Show, View and Search preserve their request signatures;
// ReadAttachmentRequest also carries Branch and BranchFromSession. MCP forwards
// the session branch for attachment pages. Each page is an independent read.
//
// AcquiredSnapshot.Config is the sole source of effective read configuration.
// A nonnil value supplies committed language and dependency declarations from
// the acquired revision. Application copies those settings for the operation;
// it never mutates the source configuration or the shared ProjectRuntime.
// SnapshotData.Config is retained document data, not an alternative effective
// configuration input. Hosts populating both must derive them from one source;
// application does not interpret SnapshotData.Config as runtime settings.
//
// Nil Config explicitly retains runtime configuration, including local
// overrides. It never represents a configuration read failure. Revision-backed
// hosts read their selected tree with ReadProjectConfigFS and return missing or
// malformed configuration as an acquisition error. Credentials, providers, index
// configuration and write routing remain runtime composition. Read declarations
// never grant access: each project and dependency is authorized independently.
//
// Stores lacking SnapshotReader retain unpinned current-authority reads only.
// Named-branch, exact and causal selections fail rather than silently weakening
// their guarantees. Acquisition failures never trigger a Current fallback.
// Search preparation still requires pinned sources for all selected projects.
//
// Application owns acquired resources until their last source-dependent read,
// joining release failures with operation errors. Returned canonical snapshots
// are materialized immutable values and need no live lease. Show/workflow
// dependency graphs are materialized before they escape acquisition. Discovery
// iterators instead borrow the caller's lease until iteration finishes or stops.
// SearchTarget must be consumed within PrepareSearch; retain descriptors for
// jobs, never a target or its iterator. Exact indexing retains its existing
// publication shortcut and fails when the recorded source cannot be acquired.
//
// Writes retain the existing GraphStore.Apply contract, fresh-read revalidation,
// retry limit and recovery behavior. The preparation revision remains provenance,
// not a newly imposed write precondition. Acquired read configuration introduces
// no new write validations or changes to write-time configuration precedence.
//
// # Local composition
//
// FilesystemGraphStore pins graph and attachment bytes in memory and deliberately
// returns nil Config: language/dependency settings and local overrides continue
// to come from runtime composition. Its revision covers the graph directory,
// not committed repository configuration. Named local branches resolve registered
// worktrees through GitWorktreeAcquirer.ReadFactory without mutation acquisition;
// the local CLI uses its existing configuration resolver for graph-directory
// overrides and validates project identity. No local configuration precedence
// changes. An unscoped FilesystemGraphStore still rejects a nonempty branch.
// Local exact sources survive while retained by that store instance, not process
// restart. Durable consumers must provide their own reproducible source access.
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
// For branch-aware hosts, implement SnapshotReader on the runtime's GraphStore
// for current, named, exact and causal selections; no read factory belongs in
// the mutation port. Return immutable Config from the same selected source.
// Acquire graph and attachment access together; an operation's release must
// not evict resources used by another lease. Cached graphs may be shared by
// project/revision, but authorization and operation configuration stay separate.
// Changes to loader settings require a distinct cache identity. Keep source
// availability for queued work independently of active leases.
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
