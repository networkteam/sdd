# Index store compatibility across binary generations

Every stored version belongs to one entry-hash rule. A reader serves only versions under its own rule, so two rules in one store are two overlays that never mix in results. Only deletes cross the line.

## Generations

| | A: v0.17.0 | B: main before this directive | C: v0.18.0 |
|---|---|---|---|
| Hash rule | H0, no prefix | H1, `sdd-entry-derivation-v1` prefix | H1 |
| Chunk IDs | `#v-<8 hex>#` | `#v-<full hash>#` | as B |
| `sdd index` | adds; GC drops non-current versions older than 14 days | as A | adds only; prints store size |
| `--force` | replaces all versions of the entry | as A | drops only versions under its own rule |
| Search reconcile | embeds missing, persists once after the full set | embeds missing, publishes per entry | as B |
| Deletes | GC and force | GC and force | `sdd index gc --drop`, named group only |

A and C are distinguishable without a new field: an 8-hex version segment is H0, a full hash is H1. C records the derivation rule per version going forward and classifies older versions by chunk-ID shape.

## Pairs on one store

A with B. Readers correct on both sides. Each side's `sdd index` evicts the other side's versions past the window; each side's `--force` wipes them at once. After eviction A re-embeds its whole overlay ahead of every query and loses it on cancel. This is 20260910-182907-s-tac-ecp.

A with C. Readers correct. C never deletes unasked, so A's overlay survives while A runs. Remaining hazard: a v0.17.0 CLI running `sdd index` against a store C wrote to for over 14 days. Upgrading replaces the CLI and servers never collect, so the path is closed in practice.

B with C. Same rule, same IDs. B's GC drops only old H1 branch versions. B becomes C on rebuild.

## Command surface

```
sdd index gc                              report, this repo's store
sdd index gc --all-repos                  report, this repo plus every connected repo
sdd index gc --repo <id>                  report, one connected repo
sdd index gc --drop v0                    one group
sdd index gc --drop stale                 every version not current for this checkout
sdd index gc --all-repos --drop stale     the same across all connected stores
```

Report per store: groups by derivation rule, the current rule split into `current` and `branch`, each with version count, entry count, oldest, newest, size. Nothing is deleted without `--drop`. Runs under the exclusive write lock.

## Migration

1. This machine: restart the MCP server on a main build. The store already holds the H1 overlay.
2. Build C: additive manifest field, no file-level bump, no store migration.
3. Release v0.18.0: a user's first C run embeds everything under H1 once (the derivation bump's cost, not GC's). H0 stays, so a running v0.17.0 server keeps working until restarted.
4. `sdd index gc` shows the H0 group; drop it once no v0.17.0 process remains. Until then the store is roughly double.

Unchanged: read-time hash filtering, store directory keyed by fingerprint, file-level manifest format.
