# Extending SDD from another Go module

This nested module compiles as `example.com/extendingsdd`, so it proves that an application can embed SDD without importing repository-internal packages.

The composition in `main.go` owns:

- authentication middleware and token validation;
- principal, project, access, and dependency resolution, and the session-continuation policy (`AuthorizeSession`; SDD ships `application.OwnerOnly`);
- graph, LLM, embedding/index, and finalizer adapters per project, and one session store and one staged-blob store for the whole composition;
- the HTTP listener and deployment policy.

SDD owns graph semantics, workflow procedures, read rendering, pre-flight and summary behavior, durable transition recovery, and write gates. The composition imports the canonical `application` contracts and optional `local` adapters, constructs an `application.ProjectRuntime` per project, resolves them through `application.AccessResolver`, creates `application.Application` with the composition-wide session and staged-blob stores, and mounts `mcpapp.Server.Handler()` behind the MCP SDK's bearer middleware. A session handle is the session ID alone; the application reads a session's home project from its own record, so the server pins no project and serves every project the principal can reach.

For local development the module uses `replace github.com/networkteam/sdd => ../..`. A release consumer removes that replacement and pins a published SDD version.

## Search synchronization

Every `application.SearchRequest` requires `SyncMode`; omission is an error.
Use `SearchSyncNone` to search the existing index, `SearchSyncLocal` to first
index the selected project branch snapshot, or `SearchSyncAll` to also index
searched dependencies. Old callers that relied on synchronous indexing should
pass `SearchSyncAll`. Text-only searches still require a mode but do not index.
For MCP, the host makes this choice in `mcpapp.Options.SearchSyncMode`;
the example selects `SearchSyncAll`.

A host that configures both an embedder and an index store can maintain a
runtime's current graph index independently:

```go
err := runtime.ReconcileSearchIndex(ctx, sdd.ReconcileSearchIndexCmd{
    OnEntryIndexed: func(entryID string, chunkCount int) {
        log.Printf("indexed %s: %d chunks", entryID, chunkCount)
    },
    OnComplete: func(revision string, entriesIndexed, chunksStored int) {
        log.Printf("index at %s: stored %d entries, %d chunks",
            revision, entriesIndexed, chunksStored)
    },
})
if err != nil {
    return err
}
```

The callbacks are optional and run synchronously after persistence succeeds.
Reconciliation preserves existing versions and adds missing ones. A retry may
repeat embedding work that failed before persistence. The host authorizes this
call and schedules further runs; it does not require a request identity and
does not subscribe to graph changes. The revision identifies the snapshot read,
not a global freshness guarantee. Branch searches use their selected snapshot
and retain stale-version filtering even when synchronization is disabled.

Run its independent compile and port-surface checks from this directory:

```bash
go test ./...
```
