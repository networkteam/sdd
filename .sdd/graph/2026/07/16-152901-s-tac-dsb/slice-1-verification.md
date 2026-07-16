# Slice 1 verification note — persistent MCP vector index repair

## Review (coordinator, first-hand)

- Diff reviewed commit by commit against the plan spec (d-tac-9td attachment) and the amendment (d-tac-js8 attachment). Faithful on all slice-1 clauses; slice-2 scope correctly fenced off.
- Public API: additive only (CanonicalChunk/ScoredChunkHit citation fields, optional SearchIndexEntryManifest capability, ExcludeEmbeddedFromIndex option, ErrorInvalidArgument code). v0.16.x source compatibility preserved.
- Fail-loud checks: persistent Reconcile rejects request-driven deletes with an error; unknown search filter values return typed ApplicationError instead of silently matching nothing.
- Lock-before-load: WriteStore/ReadStore acquire the flock before loading the chromem snapshot and hold through the operation; the old open-then-lock race (index.Open before WriteSession) is closed and the racy WriteSession method removed. Manifest sidecar saves are atomic (temp+rename), so unlocked presence reads are torn-read safe.
- CLI/MCP hash parity: both paths route through internal/chunking (same chunk IDs, same entry-state hash over content + summary + attachment bytes), so the shared v1 manifest never triggers redundant cross-path re-embedding.

## Judgment calls ratified

1. WriteSession removed rather than kept alongside WriteStore — internal API, all callers migrated.
2. ExcludeEmbeddedFromIndex as a public runtime option — carries the embedded-entry rule (include locally, exclude in connected stores per d-cpt-dtv).
3. Persistent adapter errors on deletes instead of honoring them — enforces the no-deletes contract loudly.
4. Nearest oversampling aligned with the CLI finder (max(limit*10, 50)) for consistent recall.

## Defect found during validation and fixed on the branch

Live Codex session: filtered MCP search (type: "s") returned "(no matches)" while the CLI succeeded. Root cause: publicGraphFilter cast type raw (layers got abbreviation expansion, types did not) — captured as s-tac-wyu, fixed in 971988f (ParseTypeFilter/ParseLayerFilter/IsKnownKind with loud errors). Pre-existing defect, masked until the spec-gate (s-tac-szl) and cold re-embed (s-tac-ex4) regressions were cleared. Lesson recorded: the initial smoke covered only unfiltered queries.

## Smoke measurements (real repository graph, ~145 MB store, local ollama)

| Scenario | Before | After |
|---|---|---|
| Fresh MCP server, first phrase search | ~8 min (full re-embed) | 11.3s (one-time incremental top-up, persisted) |
| Second fresh server, same query | ~8 min again | 0.79s, zero document embeds |
| Filtered search type=s kind=gap | (no matches), silent | expected gaps returned |
| Unknown kind ("gaps") | (no matches), silent | typed error in 0.14s |

## Gates

go vet ./... clean; go test ./... 26 packages ok; golangci-lint 0 issues; examples/extendingsdd ok. Re-verified on main after merge f4bbf5f.
