# Merge-readiness review: remediation round on worktree-base-fact-index (commit 898d0e7)

Fresh-eyes adversarial review (Opus agent) over the full working-tree diff against 2405003, plus independent gate runs by the orchestrating session. Verdict: **MERGE-READY**, no blockers.

## Gates (reviewer's own runs, confirmed independently by the session)
- `go vet ./...` clean · `go test ./...` all packages ok · `go test -race ./internal/engine/...` ok · `golangci-lint run ./...` 0 issues · `examples/extendingsdd` builds and tests against the additive public API · gofmt clean · binary smokes (`sdd lint`, `sdd view --layout='active:indexed:as-list'`) pass · hybrid-fusion test stable across repeated separate-process runs post-fix.

## Deliverables verified (present and correct, not just claimed)
1. **Fact-index strictness**: `IsIndexed` single rule; `FilterIndexed` and `Graph.IndexedFacts` agree; `IndexedFacts` error return removed; malformed enrollment = load warning + quiet exclusion; write paths still hard-reject. Tests strengthened (parity, retired-fact exclusion, topicRaw round-trip).
2. **Engine store JSON normalization**: `StoreValueCloner`/reflection machinery fully deleted (zero residue); `normalizeStoreValue` (write-time marshal/unmarshal, loud error on non-marshalable) + `copyStoreValue` (container-recursion read isolation); transact→validate→commit atomicity preserved; `WriteEngine` propagates errors; `time.Time` normalizes to RFC3339, no panic; capture playback template on lowercase JSON keys, index sub-block asserted end to end.
3. **`factIndex` DTO**: `application.FactIndexRow{ID, Title, Topic string}` — no `model.TopicPath` leak.
4. **Partial-read end to end**: `LoadGraph` and `LoadSnapshotFS`/`BuildSnapshot` record load issues instead of aborting; `sdd lint` renders an unreadable-entries section counted in the exit code; host-neutral graph-health framing block in the opening serve (verified: names issues, never a CLI command); MCP session opens over a malformed entry and reports it.
5. **Search determinism**: score-desc-then-ID-asc tie-breaks in `runVector` and `rrfFuse`; root cause of the flake was untied sorts over map iteration hitting exact RRF score ties (proven with instrumentation; 3/30 failures pre-fix with two distinct wrong winners; 0/25 post-fix).
6. **GraphFinder architecture**: `DocumentSource` union contract (raw canonical bytes XOR structured frontmatter+body; unreadable documents as data); `GraphFinder` holds graph + WIP; six read-side query structs pure intent, all call sites migrated, no halfway injection variants; `BuildSnapshot`/`LoadSnapshotFS` thin adapters; public API additive only (`SnapshotData.Unreadable`, `DocumentIssue`, `Snapshot.Health()`); source-parity test proves raw vs structured sources converge on identical entries/warnings/health.

## Findings (all low / non-blocking)
- Stray untracked 25 MB example binary (deleted before commit; gitignore entry still worth adding).
- Dangling-ref rejection for an unreadable target reads "(entry not found)" — correct block, imprecise message.
- The playback refs-line key change has no output-level test assertion (index sub-block is asserted; refs line proven only by type-level reasoning).

## Documented tradeoff (accepted)
Write-time mechanical invariant checks run against a graph that may be missing unreadable entries; a conflict hidden in an unreadable file surfaces only after the file is fixed. Mitigation: the degraded state is loudly visible at every session open and in lint before a write is trusted.

## Accepted debt (staged follow-ups)
- `ViewResult`/`ShowResult` still carry `Graph` for presenter-side derived attributes; remaining `snapshot.graph` pokes in application (model-level calls, not query-carried data).
- `buildGraph` re-marshals structured frontmatter through the single `ParseEntry` gate (direct map decoding is a future optimization).
- WIP marker strictness kept hard (parked policy decision).
- Dependency-store error swallow (`dependencyUnavailable()`) pre-existing and unchanged; member-graph load issues not yet aggregated into local health/lint.
- `WIPListQuery.GraphDir` untouched (outside the six migrated read queries).
