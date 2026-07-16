# Persistent MCP index amendment — design note

Amends the implementation specification attached to d-tac-9td (persistent-mcp-vector-index-repair.md). Where this note and the spec conflict, this note governs.

## Slice boundary

Slice 1 (release patch): spec sections 3–4 and 6–11 as written, plus the embedded-entry rules below. The presence check stays entry-ID-based against the existing v1 manifest. Accepted gap, closed by slice 2: an entry whose state changed (summary regeneration, mechanical fix) is present-by-ID and therefore not re-indexed via MCP; CLI lazy-fill still repairs it via the hash check on any CLI search or index run.

Slice 2: version-aware freshness, garbage collection, generation cache, connected-store force repair.

## Embedded-entry rules (slice 1)

The shared chunk-derivation contract carries index-time inclusion rules explicitly:

- Local/base store: embedded (binary-shipped) entries are included — the coverage condition of d-cpt-dtv (finders treat static and on-disk entries identically).
- Connected-repo stores: embedded entries are excluded (the CLI cache indexing's excludeEmbedded semantics) so base facts embed once per machine, not once per connected repo.

## Version-qualified chunk identity (slice 2)

- Chunk IDs gain a version segment derived from the entry-state hash (same definition as the CLI manifest hash: entry content + summary + attachment bytes): `entryID#v-<hash8>#summary`, `entryID#v-<hash8>#body-N`, `entryID#v-<hash8>#attach-<p6>-N`.
- Legacy (unqualified) chunk IDs remain valid, interpreted as the entry's sole pre-existing version owned by the hash the v1 manifest records. No migration, no rebuild.
- The manifest evolves from one state per entry to a per-entry version set: `entries[entryID] = versions[{hash, fingerprint, chunkIDs, indexedAt}]`. Legacy manifests load as one version. The manifest is a sidecar file, not public API.
- `StoredEntryRef` carries `EntryID` and `EntryHash`; the presence check is per (entryID, entryHash) pair. A missing pair derives and embeds that entry only, adding a version — never deleting another. This replaces delete-on-change (UpsertEntry old-chunk deletion) on both the CLI and MCP lazy paths.
- Read-time hit validity: the hit's entry resolves in the current graph AND the hit's version hash equals the current entry's state hash. Stale-version hits are ignored like absent-entry hits. The current hash is computed once per hit entry, cacheable per snapshot revision.
- Consequence: two branches holding different versions of one entry each find their own version present — no flip-flop, bounded store growth.

## Garbage collection (slice 2)

- Runs opportunistically inside existing write sessions (already under the exclusive flock) — no background job, no new locking surface. Reads never delete.
- Policy: drop versions that are neither the writer's current version nor recently indexed (retention window; implementation picks and records the default).
- Accepted risk: revisiting a stale branch after collection re-embeds that branch's changed entries — bounded and self-healing, since vectors are derived data.
- Sanctioned delete paths, complete list: (1) version GC under the write lock, (2) explicit CLI force/rebuild. Request- or filter-driven deletes remain forbidden.

## Generation-checked snapshot cache (slice 2)

- Every write session bumps a store write generation (manifest hash or a small generation marker written under the exclusive lock).
- Read path: acquire read lock → stat generation → reuse the held snapshot if unchanged, reload if changed → query → release.
- Steady state: lock + stat + in-memory query (milliseconds); a full reload only after actual writes.
- Verification is counter-based per the plan's testing style: a snapshot-load counter asserts no reload while the store is unchanged and exactly one reload after a write.

## Connected-store force repair (slice 2)

`sdd index --repo/--all-repos --force` currently silently downgrades to lazy-fill for member repos (s-tac-chy): BuildConnectedIndexesCmd carries no force field. Thread force through so the explicit rebuild path covers every store the serve runtimes use.

## Rejected alternatives

- **Long-lived in-memory snapshot without a generation check** — serves stale results after any other process writes; chromem has no change notification.
- **Delete-on-change on the lazy path (status quo)** — with one shared store and per-(entryID, ordinal) chunk IDs, two branch versions flip-flop: each search deletes the other branch's rows and re-embeds its own.
- **Content-addressed chunk IDs without entry-version grouping** — per-chunk hashes minimize churn but leave no cheap way to answer "which stored versions of this entry exist" for presence checks and GC; version grouping keeps the manifest the single source.
- **GC on the read path** — reads stay pure; deletion authority stays with write sessions and explicit rebuild.
- **Rebuild-only cleanup (no GC)** — parks unbounded growth on a manual step nobody runs; regular GC with re-index risk is cheaper.
