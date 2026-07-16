# Restore persistent MCP vector search — implementation specification

## 1. Objective and release boundary

Restore phrase and hybrid search over MCP by reconnecting `sdd serve` to the existing machine-global chromem index. A fresh server process must answer from an index previously built by the CLI without embedding graph documents again. When one immutable graph entry is added, only that entry's chunks are embedded once; the next restart performs no graph-document embedding.

This patch closes the release-blocking regression `s-tac-ex4`. The verification-process retrospective is deliberately deferred until after the patched release. The existing cross-repository embedder-selection gap `s-tac-qbk` is also outside this patch; current embedder selection remains unchanged.

## 2. Governing graph decisions and evidence

The design was checked with these CLI queries, as requested:

- `sdd search --query 'machine-global persistent vector index immutable entries graph revisions' --limit 10`
- `sdd search --query 'one machine-global content-addressed vector index shared by checkout worktrees connected cache' --limit 10`

The implementation must preserve:

- `d-cpt-e1i`: graph entries are immutable.
- `d-cpt-65i`: graph revisions are mutation-concurrency tokens; they are not vector-index freshness tokens.
- `d-cpt-6cq`: one machine-global, content-addressed vector index per repository and embedding fingerprint, shared across checkout, worktrees, and cache.
- `d-tac-wjq`: embedding execution and search-index storage remain separate public ports, with SDD owning compatibility rules and reusable conformance tests.
- `s-tac-96a`: persistent chromem search is the established behavior.
- `s-tac-cjr` / commit `5038f85`: the machine-global store directory and locking primitives already exist and are the base to reuse.

## 3. Current regression, grounded in code

### Production wiring

`cmd/sdd/serve.go:buildLocalApplication` constructs one `localadapter.NewMemorySearchIndexStore()` and injects it into the base project and connected-repository runtimes. Every new MCP process therefore starts with an empty vector store.

`cmd/sdd/search.go:resolveIndexStore` already resolves the correct persistent location through `index.RepoKey` and `index.StoreDir`. MCP must use the same address calculation instead of a second storage topology.

### Application reconciliation

`application/vector_search.go` derives its desired chunks after request filters and superseded-entry exclusion, compares the store's graph-wide `Revision`, deletes stored chunks absent from that request-shaped set, and rejects hits with a different revision. These behaviors contradict immutable-entry accumulation: an unrelated graph write, a status change, or a narrower query filter can invalidate or delete otherwise valid vectors.

### Existing persistence surface

`internal/index/index.go` already persists chunk identity, entry identity, fingerprint, vector, body, breadcrumb, summary/attachment flags, and attachment path. `internal/index/store.go` already supplies machine-global store directories, locks, and write sessions. `internal/handlers/handler_index.go` already derives summary, body, and Markdown-attachment chunks and computes a state hash from entry content, summary, and attachment bytes. The repair must reuse and consolidate these surfaces rather than introduce a parallel index.

## 4. Required index semantics

1. For a repository plus embedding fingerprint, the persistent vector index is the monotonic union of chunks derived from immutable graph entries.
2. Graph revision is ignored for vector validity. Revision remains relevant only to graph mutation concurrency.
3. Query filters, open/closed status, supersession visibility, and repository selection are applied at read time. They never delete or invalidate stored vectors.
4. Existing rows are retained. Only graph entries not represented in the index are derived and embedded.
5. A stored hit whose entry no longer exists in the currently loaded graph is ignored safely at query time. The patch does not perform destructive cleanup.
6. CLI `--force` / rebuild behavior remains the explicit repair path for corruption or intentional replacement.
7. Embedding dimensions are checked where vectors meet the store, including persisted-store compatibility, while lazy embedders may report zero before their first call.

## 5. Public port evolution and patch-release compatibility

Keep the v0.16.x public API source-compatible. Add fields and optional capabilities; do not remove or change the type of existing exported fields.

Add an optional capability implemented by persistent stores:

    type SearchIndexEntryManifest interface {
        IndexedEntries(context.Context, IndexNamespace) ([]StoredEntryRef, error)
    }

    type StoredEntryRef struct {
        EntryID string
    }

Extend `CanonicalChunk` with persisted citation and identity data needed by both adapters:

- `Body`
- `Breadcrumb`
- `Depth`
- `IsSummary`
- `IsAttachment`
- `SourceAttachmentPath`
- `EntryHash`

Extend `ScoredChunkHit` with:

- `EntryID`
- `Body`
- `Breadcrumb`
- `Depth`
- `IsSummary`
- `IsAttachment`
- `SourceAttachmentPath`

Existing `Revision` fields remain for source compatibility but are deprecated in comments and ignored by reconciliation and hit validity. `EntryHash` uses the same definition as the CLI manifest state hash.

## 6. Shared chunk derivation

Extract the pure chunk derivation and entry-state hashing currently embedded in `internal/handlers/handler_index.go` into one internal helper used by both CLI indexing and application vector search.

The helper accepts a `model.Entry`, an attachment reader, and the configured splitter. It returns deterministic chunks with stable chunk IDs, citation metadata, and the entry hash. It must preserve the current CLI behavior for:

- summary chunks;
- body chunks;
- Markdown attachment chunks;
- breadcrumb/depth metadata;
- attachment source paths;
- content hashing over entry content, summary, and attachment bytes.

The application-side attachment reader pages through `GraphStore.ReadAttachmentPage` so MCP and CLI derive the same content. This removes the current parity defect where `application/deriveApplicationChunks` omits attachment content.

## 7. Persistent local adapter

Add `local.PersistentSearchIndexStore`, backed by `internal/index`, and bind it to the expected project/repository identity and cache root. The request fingerprint selects `index.StoreDir`; opened stores validate project identity, fingerprint, metric, and dimensions.

Required behavior:

- `IndexedEntries` reads the existing v1 manifest and returns indexed entry identities without migration or rebuilding.
- If manifest information is incomplete, compatibility fallback may inspect existing chunk IDs/rows, including chromem `Collection.GetByID`, to recover entry membership. It must not trigger document embedding.
- `Reconcile` ignores revision and does not accept request-driven deletes. It groups upserts by entry, acquires the repository/fingerprint write lock, reopens the latest store inside the lock, applies upserts, and writes the existing v1 manifest.
- `Nearest` opens a fresh read snapshot for each operation and maps `internal/index.Hit` back to `ScoredChunkHit` with complete citation data.
- The adapter must not retain a long-lived in-memory chromem snapshot inside the long-running MCP server because CLI or another process can update the shared index.

Close the current open/load race by adding lock-before-load helpers in `internal/index` (names illustrative):

    func ReadStore(ctx context.Context, dir string, fn func(*Index) error) error
    func WriteStore(ctx context.Context, dir string, fn func(*Index) error) error

The invariant is mandatory: the relevant lock is acquired before loading the store snapshot and held through the operation/save. The CLI handler and persistent adapter should share this helper rather than reimplement locking.

## 8. Application search algorithm

For every phrase or hybrid search:

1. Resolve the embedding specification and fingerprint.
2. Resolve the repository namespace.
3. Ask the optional entry-manifest capability for already indexed entry IDs.
4. Walk the complete current graph independently of request filters and status.
5. Derive and embed only absent entries, including Markdown attachments, then persist their chunks.
6. Embed the query once.
7. Run nearest-neighbor search over the persistent store.
8. Resolve each hit's `EntryID` against the current graph; ignore missing entries.
9. Apply repository, type, kind, layer, status, include-superseded, and embedded-entry rules at read time.
10. Render citations from stored hit metadata.
11. For hybrid mode, fuse the vector and text result sets exactly as today.

For third-party stores that implement the existing `SearchIndexStore` but not `SearchIndexEntryManifest`, keep a compatibility reconciliation path based on existing chunk identity. It must still ignore graph revision and must never issue deletes derived from request filters.

## 9. Production wiring

In `cmd/sdd/serve.go`:

- remove production construction of `MemorySearchIndexStore`;
- calculate the base repository key exactly as the CLI does: `index.RepoKey(cfg.RepoID, filepath.Dir(sddDir))`;
- calculate a connected repository key from its repository ID according to the existing connected-repository storage contract;
- construct a persistent adapter for every project-aware runtime;
- let the embedding fingerprint select the final `StoreDir` inside the adapter;
- retain `MemorySearchIndexStore` only for tests/examples and public adapter consumers that explicitly choose it.

Keep local/global embedder resolution unchanged in this patch. Resolving `s-tac-qbk` would broaden the release repair and requires its own decision.

## 10. Verification contract

### Adapter conformance

Extend `sddtest` conformance so the same suite can run against memory and persistent stores. Cover:

- reopen persistence;
- entry-manifest reporting;
- dimension mismatch;
- complete citation metadata round-trip;
- monotonic reconciliation without revision invalidation;
- no filter-shaped deletion.

### Application tests

Add deterministic counter-based tests:

- fully pre-indexed graph: zero document embeddings and exactly one query embedding;
- unrelated graph revision change: zero document embeddings;
- one new immutable entry: only that entry's chunks embedded;
- restart after that addition: zero document embeddings;
- narrower filters/status changes: no deletes and no re-embedding;
- Markdown attachment hit preserves source citation;
- persisted row absent from current graph is ignored.

### Production-path regression test

Add a subprocess-level test around the real command wiring, using a fake Ollama-compatible HTTP server and a real temporary XDG cache:

1. Create a temporary graph and configuration.
2. Seed the index through the real CLI indexing path.
3. Start a fresh `sdd serve`, issue phrase search, assert expected result, one query embedding, and zero document embeddings.
4. Start a second fresh server and assert the same zero-document behavior.
5. Add one graph entry, start a third server, and assert only the new entry's document chunks are embedded.
6. Start a fourth server and assert zero document embeddings.
7. Assert MCP and CLI resolve the same machine-global `StoreDir`, proving the production path does not silently fall back to memory.

Use request counters and inspected store identity, not elapsed-time thresholds.

### Release gate

Before publishing the patch release:

- `go vet ./...`
- `go test ./...`
- tests in `examples/extendingsdd`
- `golangci-lint run ./...`
- from a fresh real agent/MCP session, run phrase search against the real repository twice and confirm no graph-document embedding batch occurs.

## 11. Expected file map

- `application/search_ports.go`: additive port fields and optional entry-manifest capability.
- `application/vector_search.go` and tests: monotonic entry-based freshness and read-time filtering.
- `local/persistent_indexstore.go` and tests: persistent adapter.
- `internal/index/index.go`, `internal/index/store.go`: fresh lock-before-load operations and row/manifest access.
- `internal/handlers/handler_index.go`: consume shared derivation and shared locked write path.
- shared internal chunk-derivation helper and tests.
- `cmd/sdd/serve.go` and production wiring/subprocess tests.
- `sddtest/conformance.go`: reusable persistence/monotonicity contract.

## 12. Explicitly rejected alternatives

- Calling the legacy finder directly from MCP, which would collapse the public port boundary.
- Copying the disk index into a per-process memory index.
- Treating graph revision as vector freshness.
- Deleting vectors because a request filter or status excludes them.
- Accepting one cold rebuild after upgrade; the existing v1 index must remain immediately usable.
- Folding the verification retrospective into this release-blocking patch.