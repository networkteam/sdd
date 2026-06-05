# Cross-repo design revisions — decisions and rejected alternatives

Augments d-cpt-v8t. Captured from planning dialogue (Christopher, Claude). Conceptual decisions only; implementation, command surface, and acceptance criteria live in the cross-repo plan.

## Decisions

1. **Discovery in scope.** Cross-graph search across connected repos is in scope (the original directive deferred it). A reference you cannot discover or read across repos is not usable. Only who-cites-me backlinks stay deferred.

2. **Managed caches over user-maintained mapping.** sdd clones each connected repo into a homedir cache it owns (`~/.cache/sdd/<repo-id>/`, XDG-aware), pulled lazily. Reverses the original directive's user-maintained repo-id→checkout-path mapping — resolution and discovery work without the user wiring paths. Requires a registered clone URL per repo, distinct from the canonical repo-id (identity is decoupled from the git remote).

3. **Global-only embedder over global-default + per-repo override.** One embedder, in a global user-level config, indexes every connected repo. Cross-graph fusion needs one vector space. Existing per-repo embedding configs migrate to global. Rejected: per-repo override, because an overriding repo drops out of cross-graph search.

4. **Per-repo indexes merged by score over one combined index.** Each repo keeps its own index; the query is embedded once and scored against each selected index; results roll up per entry, tag with source repo-id, and merge into one list sorted by comparable cosine score (cosine scores are comparable across same-embedder indexes). Incremental, no coupling. Rejected: a single combined index — must rebuild on any repo change, couples repos, duplicates vectors.

5. **Local index stays in-repo; cache holds others.** The current repo keeps indexing itself at `.sdd/index/` (branch/worktree-sensitive, sees uncommitted entries). The cache holds only other connected repos (read-only, published default branch). A repo-id-keyed cache cannot represent branches/worktrees and would collide with `sdd wip --worktree`.

6. **Strict resolve-or-block over silent fallback.** A cross-repo ref must resolve at capture or capture blocks (per the universal invariant d-cpt-ba7). On cache miss, fetch fresh, then block only if genuinely absent from the other repo's default branch. Read surfaces show a visible `[unresolved]` marker for an already-captured ref whose repo is later removed. Rejected: silent syntactic-only fallback.

7. **Full cross-graph traversal over one-hop.** `sdd show` follows cross-repo chains fully, bounded by max-depth, with (repo-id, entry-id) dedup. Max-depth is the natural limiter and the cache loader already exists; the marginal cost over one-hop is the reentrant loader call plus the composite dedup key.

8. **Display: full canonical repo-id prefix, no status segment.** repo-id is canonical-for-all (a per-user alias would break that); status never crosses the boundary, so no remote status is rendered.

## Search flag shape (tactical — for the plan)

Local searched by default; `--repo <id>` (repeatable) adds repos; `--all-repos` spans current + all connected. Cross-graph hits carry the repo-id prefix; mismatched-embedder repos are excluded and flagged by `sdd lint`. Detail belongs in the cross-repo plan.
