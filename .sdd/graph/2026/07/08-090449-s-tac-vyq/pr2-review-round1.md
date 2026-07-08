# PR #2 review — round 1: cross-repo references (branch worktree-cross-repo-refs)

Reviewer: Christopher (GitHub review comments on https://github.com/networkteam/sdd/pull/2, 2026-07-07).
Analysis and remediation dialogue: Christopher + Claude, session 2026-07-08.

Scope: architecture review of the cross-repo references implementation (nine commits, recorded as done 20260708-010505-s-tac-94a). Code-level review only — the feature was not exercised in this round, so the closing done's functional claims stand on the implementing session's own record; Christopher's outer evaluation (validation in use) is still pending. All findings are layering, encapsulation, and duplication debt. Merge held until remediated.

## The seven comments, verbatim

1. `cmd/sdd/repo.go:112` — "Could be moved to the model package. Rule: any code that can should be moved down as far as possible."
2. `cmd/sdd/search.go:131` — "This puts a lot of detailed code into the CLI layer. It looks finder shaped -> we should find a way to move more of the implementation into a finder method."
3. `internal/finders/graphsource_multigraph_test.go:18` — "This feels hacky to override these env vars. Can't we make the config and cache home make configurable via config that is passed from the CLI layer down to the finder?"
4. `internal/finders/search_multi_test.go:36` — "See other comment - this again duplicates the env setup without any explanation. I'd favor an explicit config forward pass here."
5. `internal/mcpserver/tools.go:934` — "This is a duplication with the same function used in the CLI surface! Unify and export from model package."
6. `internal/repos/cache.go:33` — "I'd move this to the CLI layer (where deps are built and resolved) and rather pass an explicit config struct to the code in this package."
7. `internal/repos/cache.go:179` — "AFAIK we had some nice interfaces to abstract the actual Git calls away in other places. It seems odd to add this function here (we must have other code that already executes Git) - so we should rather unify it."

## Theme A — duplicated pure helper (comments 1 + 5)

`crossRepoIDsIn` (cmd/sdd/repo.go:112) and `crossRepoIDsOf` (internal/mcpserver/tools.go:934) are character-identical pure functions collecting distinct repo IDs from cross-repo (`<repo-id>:<entry-id>`) arguments, both already built on `model.SplitCrossRepoID`.

Rules violated: push-logic-down (pure computation two layers too high) and single-path (duplicated across the CLI and MCP surfaces).

Remediation: one function in `internal/model` next to `SplitCrossRepoID` (e.g. `model.CrossRepoIDs(ids []string) []string`); both call sites collapse onto it.

## Theme B — ambient environment resolution inside internal/repos (comments 3, 4, 6)

Root cause sits deeper than the flagged tests: `repos.CacheRoot()` and `repos.ConfigPath()` read `$XDG_CACHE_HOME` / `$XDG_CONFIG_HOME` as hidden globals, and package-level `repos.LoadConfig()` / `CacheDir()` are called from eleven sites across handlers (`handler_repo.go`), finders (`graphsource.go` memberGraphLoader, `search_multi.go`), and the CLI. The finder's member-graph loader silently reading env plus a user-global file is a purity leak in the read side. The `t.Setenv` in the two tests is the symptom, not the disease. Same drift family as the open read-side dependency leak (20260507-130356-s-tac-m09): dependencies consumed ambiently / couriered, instead of owned at the composition root.

Remediation (agreed):
- `repos.Locations{ConfigPath, CacheRoot}` value; `repos.DefaultLocations()` keeps the XDG convention **inside the repos package** (documented and testable in one place) — composition roots (CLI bootstrap, MCP bootstrap) decide *when* to apply it.
- **Two abstractions, not one** — the split follows the codebase's read/write seam:
  - `repos.Registry` — pure reads only: `Connected`, `CacheDir`, `IsCloned`, `GraphDir`, `IndexDir`, `DeclaredRepoID`, config load. Injected into `finders.Options`.
  - `repos.Manager` — the side-effectful lifecycle: `EnsureCloned`, `CooldownPull`, `ForcePull`, add/remove-with-save. Wraps the Registry plus the git dependency. Injected into `handlers.Options`.
- Reason for the split: finder purity becomes **type-enforced instead of discipline-enforced** — a finder physically cannot clone or pull because its dependency has no such method.
- Config reads stay lazy per call inside the Registry (not snapshotted at construction), so a long-lived MCP server sees an `sdd repo add` from another terminal without restart.
- Tests construct `Locations` over temp dirs; all `t.Setenv` of XDG variables goes away.

## Theme C — ad-hoc git execution (comment 7)

`runGit` in `internal/repos/cache.go:179` shells out directly, bypassing the established pattern: consumer-defined git interfaces (`handlers.Committer` / `Brancher` / `Mover` / `Puller`, `finders.GitSyncer`) with exec-based implementations wired at the CLI layer. Christopher additionally flagged the accumulated raw `exec.Command` git code in `cmd/sdd/main.go` and `cmd/sdd/sync.go` as the same smell.

Remediation (agreed):
- New `internal/git` adapter package: the low-level run-git-fold-stderr-into-error helper, plus the concrete implementations of all consumer-defined git interfaces, plus the loose helpers currently in `main.go` (repo root, `user.name`, `remote get-url`).
- Interfaces stay defined with their consumers (Go convention); `internal/repos` gets its own narrow interface (`Clone(ctx, url, dir)`, `PullFFOnly(ctx, dir)`) implemented by the adapter and held by `repos.Manager`.
- Deliberately **no** single universal Git interface — sync predicates, committing, and cache clone/pull have different consumers; unification happens at the pattern level and the shared low-level runner.
- Scope decision: migrating `main.go`'s and `sync.go`'s existing exec code into `internal/git` is folded into this branch ("now's the best time").
- Future option this enables: swapping the adapter internals to go-git, confined to one package with every interface unchanged — possibly partial (plumbing like clone/pull first; `merge-tree --write-tree` may stay a shell-out). Cost framing: go-git is a heavy dependency tree; what it buys is dropping the "git binary on PATH" requirement. Enabled, not decided.

## Theme D — finder-shaped computation in the CLI (comment 2)

`populateRepoIndexLint` (cmd/sdd/search.go:131) and its sibling `populateIndexLint` (line 101) compute `query.LintResult` fields — config → manifest → fingerprint drift counts — in the CLI layer. Pure reads that belong beside `Finder.Lint`.

Remediation: move both into the finder; the CLI keeps only flag → `model.EmbeddingConfig` resolution. With Theme B's Registry injected into the finder, the cross-repo part needs no CLI-side wiring at all. The deliberate degrade-silently behavior (lint never blocks on index machinery being absent) moves with the code.

## Verdict

At the code level, no functional defects surfaced in reading — the findings are architecture debt. This round neither ran nor exercised the feature: the closing done's acceptance verification stands on the implementing session's own record, and Christopher's outer evaluation (trying the feature against a real second graph) is still pending. Merge waits on the four remediations, whose architecture is committed as a directive on top of this evaluation.
