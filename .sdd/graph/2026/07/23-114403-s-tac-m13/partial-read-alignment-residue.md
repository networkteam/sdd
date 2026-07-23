# Partial-read alignment residue — survey inventory and staged follow-ups

Source: read-only codebase survey of graph load-error handling (Opus agent, 2026-07-22, during the remediation round), updated with the merge-readiness reviewer's accepted-debt list after the GraphFinder unification (commit 898d0e7). Areas A and B of the survey were fixed by the unification; everything below is the open residue.

## Cross-repo / dependency graphs
- **MCP dependency-store swallow** — `application/application.go` `snapshotWithDependenciesFrom`: any error from a dependency's `Graph.Current(ctx)` degrades to `dependencyUnavailable()` with a generic message, discarding the cause. Since the unification, malformed *entries* no longer error (they load partially), so the swallow now hides only genuine store failures — but those are exactly the ones that should surface loudly. Pre-existing fail-loud violation, unchanged.
- **Member-graph load issues invisible** — a connected repo's graph loads partial-read (`LoadIssues` recorded on the member graph), but `Graph.Health()` and `finders.Lint` read only the local graph. Dependency problems reach no lint/health surface; a cross-repo ref into an unreadable entry resolves as "not found" with no trace of why. Decide: aggregate member issues into local health/lint, or declare cross-repo problems the dependency's own lint concern.

## Policy calls parked deliberately
- **WIP-marker strictness** — one malformed WIP marker still hard-fails reads that load markers (e.g. `sdd view`). Ephemeral status files, so strictness is defensible — but it is a silent divergence from the partial-read posture until explicitly decided. (Carved out of directive d-cpt-4y9.)
- **CLI health footers** — `sdd view`, `show`, `search`, `stats` operate on readable entries with no hint that N entries were dropped; `sdd index` silently excludes unreadable entries from its indexed/skipped counts. Lint is the designated surface, but a user debugging "why isn't my entry showing up" gets no pointer. Decide per surface whether a one-line health footer is wanted.

## Code residue from the unification (accepted debt)
- `WIPListQuery.GraphDir` still carries a directory (outside the six migrated read queries).
- `ViewResult`/`ShowResult` still carry `Graph *model.Graph` for presenter-side derived attributes; a handful of `snapshot.graph` pokes remain in application (model-level calls: `AllParticipants`, `ByID`, `ProcedureChains`, vector-search entry iteration, write-path validation) — query inputs are pure, results/presenters are the remaining surface.
- `buildGraph` re-marshals structured frontmatter to YAML through the single `ParseEntry` gate; direct map decoding is a possible later optimization behind the same seam.

## Reviewer cosmetics
- Dangling-ref rejection for a target that exists but is unreadable reports "(entry not found)" — the block is correct (fail-loud holds), the message doesn't distinguish unreadable from absent.
- The capture playback refs line (lowercase JSON keys) has no output-level test assertion — the index sub-block is asserted end to end; the refs line is proven only by type-level reasoning.

## Documented tradeoff (not actionable, recorded for awareness)
Write-time mechanical invariant checks run against a graph that may be missing unreadable entries; a conflict hidden inside an unreadable file surfaces only once the file is repaired. Mitigated by health visibility at every session open and in lint.
