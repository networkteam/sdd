# Slice 2 verification note — version-aware index, GC, generation cache

## Review (coordinator, first-hand)

- Gates run personally in the worktree: go vet clean, go test all 26 packages ok, golangci-lint 0 issues, extendingsdd ok.
- Manifest schema evolution reviewed line by line: per-entry self-describing format (flat shape for single version, {"versions":[...]} for many); an untouched v1 manifest round-trips byte-for-byte — no migration, no rebuild. Covered by TestManifest_LegacyShapeRoundTrip and a hand-seeded legacy-store test.
- GC reviewed: pure CollectStaleVersions (keeps current-or-recent versions, drops the rest, removes empty entries); VersionRetention = 14d as a documented constant; delete + manifest save only inside the CLI write session.
- Generation cache reviewed: marker bumped only on actual mutation (dirty flag), stat under the read lock, per-store-dir caches behind a mutex, reload counter proves reuse-until-write.

## Judgment calls ratified

1. GC in CLI write sessions only — the app reconcile stays pure-add (it lacks full-graph hash knowledge). Boundary noted for serve-only deployments.
2. Read-time version filtering added to the CLI finder too — required once the store became additive; without it stale citations could serve for up to the retention window.
3. No pre-merge live smoke against the real store — correct discipline: the format is forward-only and the base binary was still slice-1; live smoke ran post-merge instead.
4. Reads serialized per adapter (mutex across the query) — acceptable for a local single-user server; revisit if read concurrency grows.

## Post-merge live smoke (real repository store, local ollama)

| Scenario | Result |
|---|---|
| Fresh server, first search | 46s — embedded only entries captured since last fill (one batch), persisted |
| Fresh server, filtered type=s kind=gap | expected gaps returned |
| Fresh server, shorthand type=d filter | decisions returned (top hit: the fresh d-cpt-vri) |
| Unknown layer "tct" | typed error naming accepted values, 0.13s |
| Fresh servers, warm reads | 0.88–1.16s, zero document embeds |

## Compatibility scope

Landing in place (no store-dir format versioning) is governed by d-cpt-vri: per-store monotonic binaries, prompt release after format-affecting merges, dir-versioning with copy-seed as the recorded escape hatch. The read-compat contract d-cpt-i2x (older formats always readable) is honored: v1 stores load unchanged forever; only old-binary-reads-new-format is scoped out.
