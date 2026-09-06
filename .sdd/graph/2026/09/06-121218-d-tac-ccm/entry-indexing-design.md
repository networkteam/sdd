# Entry indexing, provider batching and search coverage

## Why separate the units

Whole-project reconciliation currently derives every missing chunk, embeds the aggregate, then persists. A workload that exceeds a job deadline can repeatedly consume provider work without publishing any progress. Raising that deadline leaves the failure mode intact.

An entry version is the unit of durable scheduling, retry and atomic publication. A provider batch is a bounded transport unit shared by compatible concurrent callers. A search target is the fixed set of entry versions required by selected snapshots. None of these units requires a project job that waits for every child.

## Source and discovery

Reuse the public Snapshot type and GraphStore attachment paging. Add a read capability only where existing acquisition cannot express exact retained source or causal freshness. A snapshot lease pairs the graph with immutable attachment bytes and a release operation. The host retains source revisions until jobs finish and may share cached source objects across leases. An indexing job must reopen its exact revision; missing retained input is an explicit failure. It must never substitute a current branch. Search freshness instead selects a revision that includes the successful write and may include later changes.

Discovery returns a Go iterator over eligible entry requirements and their published status. It hashes one entry at a time and does not prepare a project-wide chunk slice. Cancellation, early termination and source errors stop iteration. Snapshot-based graph loading remains supported; this is not a constant-memory graph claim.

A cursor contains the source revision, index configuration and last entry ID. Stable ID ordering skips earlier entries before attachment reads or hashing. The host commits cursor advancement with bounded durable enqueue groups. Replaying a group is safe through version-key uniqueness. Queue keys use canonical project ID, entry ID, full entry-state hash and index configuration fingerprint. Revision locates source but does not prevent identical versions across branches sharing publication. Fingerprints must distinguish applicable embedding and derivation configurations.

## Entry publication

The application command indexes one explicit entry version. It checks published completion before loading source or embedding, verifies reproducible input, derives all chunks, embeds through the supplied client and publishes the complete version. The store capability atomically governs both completion and retrieval. Unpublished chunks cannot be returned by search. A zero-chunk entry still receives a completion record. Publication is idempotent under concurrent attempts, and a retry after publication but before acknowledgement skips embedding.

An oversized entry spans multiple bounded provider batches and publishes once. An interrupted entry may repeat its unfinished embedding. Completed siblings remain usable, and a failed entry does not prevent independently scheduled siblings from progressing. No within-entry checkpoint mechanism is included.

Local stores must validate entire writes before mutation. Disk serialization must not leave partially written live documents after process interruption. Completion is published only after all entry rows exist; retrieval excludes rows absent from published completion. Compatibility reconciliation remains available for existing stores, with its limits documented. The stronger public operation requires explicit publication support.

## Shared embedding batches

A process-local stateful decorator wraps one fixed provider configuration. It combines document chunks across callers, flushes on a size bound or the oldest queued chunk's waiting limit, and routes vectors to input positions. Later arrivals do not reset the timer. An underfilled tail flushes without another caller. Requests larger than the input buffer are admitted incrementally.

Active entry-job concurrency belongs to the consumer. The batcher owns buffering, cross-caller batching, size/window flushing, bounded dispatch, cancellation isolation and vector distribution. Byte and provider-specific token/payload bounds supplement chunk count. A single input that violates a provider constraint fails explicitly.

Reuse existing composition tools for adjacent policies. Inspection found that embed.Bounded already applies a per-call deadline and EmbedderFunc can route by Purpose. Place Bounded inside the batcher so the timeout applies to each actual provider call. Route document requests to the batcher and query requests to a separately composed client through EmbedderFunc. Both routes must use the same compatible vector space. Distinct rate limiters or concurrency limits belong to consumer composition if needed; a shared limiter must not accidentally restore a document backlog ahead of queries. The checkpoint's built-in query slot and timeout option should be removed if the composition examples prove these existing tools sufficient.

A caller can stop waiting without canceling a provider batch serving others. The batcher owns shared request lifetime, and the inner deadline decorator bounds provider calls. Provider failure reaches affected callers without an opaque durable retry layer. Shutdown releases queued and waiting callers with explicit outcomes. Test the composed timeout and shutdown behavior, not only the batcher in isolation.

Observe provider calls inside the decorator. Each actual batch records usage once. Batch identity and participating caller identifiers correlate jobs and attempts without attributing the whole batch to an arbitrary caller or multiplying totals. Job logs separately carry project, attempt, committed-entry progress, elapsed time and failure cause.

The starting values discussed were 32 chunks, a 10 ms window, two document provider calls and 128 buffered chunks; eight host entry workers, groups of 100 enqueues and a two-second coverage wait were integration starting points. These are provisional, not universal library defaults. A real exercise must establish provider concurrency and latency before recommending them.

## Consumer preparation and SDD-owned coverage

The shared SDD application is the sole authority for search coverage. It authorizes and selects the home and requested dependency snapshots first, establishes their required entry versions, and retains the same snapshot and attachment authorities through preparation, coverage reads and retrieval. Later writes do not move this target. Required descriptors are graph-derived values, not caller assertions. The consumer cannot replace target snapshots or mark entries covered.

Provide one preparation extension point in the existing application search path. The proposed narrow shape is an optional ApplicationOptions.PrepareSearch callback:

```go
PrepareSearch func(context.Context, SearchTarget) error
```

SearchTarget is a read-only view of the fixed target. Its proposed Entries method yields SearchEntryDescriptor values for the target's required versions. It exposes the selected project/revision identities and requested synchronization scope as read-only data needed by composition. The callback cannot mutate the target or write result metadata. Exact method names and the final signature must be demonstrated by executable external-consumer and local examples before they are finalized. Do not add a separate public prepare/execute search API alongside the callback.

Preparation may schedule durable work, index synchronously, wait under consumer policy, or return immediately. Returning nil means only that preparation returned normally. In particular, an ordinary consumer waiting-budget expiry returns nil without claiming completeness. A preparation failure remains an error. A consumer must distinguish its own elapsed waiting budget from external cancellation and provider/storage failures; catching every context deadline as ordinary expiry would hide failures.

After preparation, SDD reads published entry versions and derives actual complete/incomplete coverage for the fixed target. It retrieves against those same selected snapshots and owns structured result metadata and the readable incomplete-results notice. Incomplete zero-match differs from complete no-match. Query embedding remains necessary for semantic search, and no text fallback is added. Completion and retrieval both obey the store's atomic publication boundary.

Scheduling, queue states, retries, waiting budgets and interpreting worker failures stay in consumer composition. No callback result or queue status proves coverage. A local preparation callback can synchronously index required versions. An immediate-return callback still produces SDD-derived incomplete metadata when publication is missing.

Existing SearchSyncNone/Local/All behavior must have one deliberate preparation path. With no custom callback, the compatibility preparation honors the requested scope. With an explicitly configured callback, the callback is the preparation path and there is no second implicit reconciliation afterwards. Document that distinction and test it through both Application.Search and the MCP search tool; changing MCP to none alone is not the external consumer integration.

## Concrete composition proposals

These snippets demonstrate ownership and call flow using proposed target/callback names. They are design examples, not APIs already present in the checkpoint. Turn them into executable examples using the real application and local adapters before finalizing the signature. Application access, sessions and blob stores remain the existing composition inputs.

An external consumer composition durably submits required descriptors in bounded groups, then optionally waits for consumer-owned notifications:

```go
options.PrepareSearch = func(ctx context.Context, target sdd.SearchTarget) error {
    // Consumer method: commits enqueue groups and returns a request-scoped waiter.
    pending, err := scheduler.Enqueue(ctx, target.Entries())
    if err != nil {
        return err
    }
    defer pending.Close()
    timer := time.NewTimer(waitBudget)
    defer timer.Stop()
    select {
    case err := <-pending.Result():
        return err
    case <-timer.C:
        return ctx.Err()
    case <-ctx.Done():
        return ctx.Err()
    }
}
application, err := sdd.NewApplication(options)
// Handle err before composing the server.
server, err := mcpapp.New(mcpapp.Options{
    Application: application,
    SearchSyncMode: sdd.SearchSyncNone,
})
```

Here scheduler and pending are consumer-owned example types, not proposed SDD ports. A nil notification result means preparation is finished, not that the search is complete. The timer branch returns nil only while the parent context remains live. The waiter must have bounded resources and release them when preparation returns. The enqueue and waiting phases both respect request cancellation; the consumer reserves time for subsequent query embedding and retrieval. Durable jobs outlive this request and retain their source independently of the search lease. Integration tests must show that even a premature successful notification cannot make SDD report false completion.

A local composition uses the same callback and the existing entry-indexing operation:

```go
options.PrepareSearch = func(ctx context.Context, target sdd.SearchTarget) error {
    for entry := range target.Entries() {
        runtime, ok := runtimes[entry.Version.Namespace.Project]
        if !ok {
            return fmt.Errorf("selected project runtime unavailable")
        }
        if err := runtime.IndexSearchEntry(ctx, sdd.IndexSearchEntryCmd{
            Entry: entry,
        }); err != nil {
            return err
        }
    }
    return nil
}
application, err := sdd.NewApplication(options)
// The ordinary Application.Search and MCP search paths use this preparation.
```

This local example chooses synchronous preparation for all selected projects. The runtime map belongs to consumer composition and must contain only the authorized target's selected runtimes. Exact-source reads reuse retained snapshot authority, including attachments; they must not reopen a moving branch. Demonstrate this with real local adapters, including zero-chunk entries and cancellation, before accepting the signature. A missing exact-source capability is an integration failure to resolve in the shared implementation, not a reason to work around entry indexing in consumer code.

An embedding composition can use the existing boundary without adding query routing to the batcher:

```go
documents, err := embed.NewBatcher(lifetime,
    embed.Observed(embed.Bounded(provider, providerTimeout), sink), batchOptions)
// Handle err and arrange batcher shutdown at the consumer lifecycle boundary.
queries := embed.Observed(embed.Bounded(provider, queryTimeout), sink)
routed := embed.EmbedderFunc{
    Space: provider.Fingerprint(),
    Run: func(ctx context.Context, req embed.Request) (embed.Result, error) {
        switch req.Purpose {
        case embed.PurposeDocument:
            return documents.Embed(ctx, req)
        case embed.PurposeQuery:
            return queries.Embed(ctx, req)
        default:
            return embed.Result{}, fmt.Errorf("unsupported embedding purpose")
        }
    },
}
```

Provider concurrency safety is a prerequisite of sharing that provider instance. Otherwise compose distinct compatible clients. Measure provider concurrency and query latency with document work active; a separate route is not evidence of latency isolation by itself.

## Write-triggered integration

Start from the existing MutationFinalizer and GraphStore write adapter. The consumer decides whether a successful write triggers indexing. Its graph-write/recovery protocol must retain a durable scheduling intent across the interval between canonical commit and finalizer execution. The finalizer can idempotently deliver that intent; background recovery must finish delivery without requiring the originating MCP session to resume. The graph adapter must provide post-write freshness so later search observes a revision containing the write.

Use the consumer's transaction, outbox or recoverable write protocol to guarantee that delivery. An in-memory finalizer callback alone is insufficient. Periodic reconciliation is a repair path. No new event or subscription subsystem is requested.

## Checkpoint costs to resolve

Retain the useful primitives in 0af4e1d7, but assess two costs before completion. Per-entry EntryPublished checks repeatedly load the local manifest. Measure discovery and worker costs as entry count grows, and reuse a generation-validated manifest view or bounded bulk read where that removes repeated work without accepting stale completeness after publication. A view used before preparation must be refreshed for the authoritative post-preparation coverage read. Prefer existing cache and locking boundaries; justify any extra public capability with the measured need.

Persistent retrieval currently requests all candidates before discarding unpublished rows. Preserve invisibility while evaluating a publication-filtered cached read view or bounded candidate expansion. Compare query memory, candidate count and latency with both many committed rows and interrupted unpublished rows. Do not let unpublished rows fill the candidate limit and hide healthy published results. Neither ignoring unpublished rows for speed nor trusting queue completion is acceptable.

## Code responsibilities

Public types live under pkg/application and its public types package. Command and query structs retain the existing CQRS vocabulary. Handlers own embedding and publication side effects. Finders perform discovery and coverage reads. Pure identity, ordering, eligibility and coverage comparison belong below orchestration. The application composes ports and selected snapshots. CLI and MCP shells translate requests and present results. The embedding decorator owns transport batching; existing decorators and consumer routing supply adjacent policies. Only the shared SDD application derives search coverage.

## Validation and delivery

Implement and run the concrete external-consumer and local composition examples before finalizing the preparation callback signature. Exercise public behavior: lazy discovery and resumed scans; exact sources, attachments and configuration drift; concurrent idempotent publication and zero-chunk completion; invisible unpublished rows; invalid vectors and storage errors; oversized requests, tail flushes, cancellation and shutdown; usage counted per actual call; fixed dependency coverage and immediate write-then-search; normal preparation wait expiry, preparation errors and false consumer completion notifications; manifest read amplification and unpublished-candidate filtering costs.

Run a realistic persistent multi-entry, multi-batch workload across interrupted attempts. Show durable progress, skipped completed versions on restart, eventual completion and isolation of a failing entry. Run repository-required checks including the separate example module. Return the commit, actual release status, evidence and precise external consumer adoption checklist. Closing completion is recorded through the implementation move as a done signal whose text the user confirms.

