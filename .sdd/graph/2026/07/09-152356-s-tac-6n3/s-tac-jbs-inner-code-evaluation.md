# Inner code evaluation — s-tac-jbs

Evaluated work: 20260709-141700-s-tac-jbs, slice 4 terminal-UX wiring for cross-repo clone and connected-index builds.

Scope: inner/code verification only. Outer validation-in-use was deliberately left to the other session Christopher named.

## Graph context inspected

- 20260709-141700-s-tac-jbs — evaluated done signal.
- 20260708-010505-s-tac-94a — original cross-repo references implementation.
- 20260708-090449-s-tac-vyq and pr2-review-round1.md — prior inner code review and architecture yardsticks.
- 20260708-151819-s-tac-cr0 and cross-repo-outer-evaluation.md — first outer validation and real-use gaps slice 4 was meant to address.
- 20260708-185833-d-tac-nhx — config/state placement plan and ACs 12-13 for slice 4.
- 20260708-164711-s-tac-k50 — no-progress cross-repo operations gap.
- 20260708-184318-d-cpt-6cq — placement directive behind slices 1-3.
- 20260703-200000-d-prc-evl — evaluate procedure.

## Code inspected

Commit 09ecee8 and current HEAD around:

- cmd/sdd/search.go
- cmd/sdd/repo.go
- cmd/sdd/serve.go
- internal/command/index.go
- internal/handlers/handler_repo.go
- internal/handlers/handler_repo_test.go
- internal/finders/search_multi.go
- internal/repos/config.go
- internal/repos/manager.go
- internal/index/store.go
- internal/index/index.go
- internal/cliout/tui/model.go
- internal/cliout/tui/interactive.go

## Judgment

The implementation is mostly sound, but not clean. The main slice shape holds: connected index building is handler-side command work, progress is callback-driven through command structs, the CLI owns transient UI wiring, repo add runs through the output coordinator, and finders remain pure reads. The previous architecture remediations from s-tac-vyq / s-tac-7lv mostly held.

Three findings surfaced.

## Finding 1 — MCP cross-repo vector search can use a different embedder than CLI

The CLI deliberately uses crossRepoEmbedder for --repo/--all-repos, making the user-global embedding config win so every selected connected index shares one vector space. Evidence: cmd/sdd/search.go crossRepoEmbedder loads the global config and prefers gcfg.Embedding (lines 131-145), and the search/index actions select it when crossRepo is true (search around 423-428, index around 190-194).

The MCP searcher does not follow that rule. cmd/sdd/serve.go buildServeSearcher uses buildEmbedder unconditionally at server startup (lines 126-156), then lazyFillSearcher.Search passes that same l.emb to PrepareCrossRepoSearch for cross-repo queries (lines 170-180). If a repo-local embedding override exists while a global cross-repo embedding provider is configured, CLI and MCP can build/read different (repo-id, fingerprint) stores for the same cross-repo query and return inconsistent vector results.

This violates the original CLI/MCP co-equal surface and the single shared embedder design.

## Finding 2 — sdd index --repo/--all-repos --force does not force connected repo indexes

cmd/sdd/search.go defines --force as re-embedding every entry and passes Force only to the local BuildIndexCmd (around line 265). internal/command/index.go BuildConnectedIndexesCmd has no Force field (lines 67-92). internal/handlers/handler_repo.go BuildConnectedIndexes always calls ih.LazyFill for member indexes (lines 409-414).

A user trying to repair a fingerprint-matching but corrupt or stale connected index with sdd index --repo X --force will not re-embed that member index.

## Finding 3 — text-only sdd search --repo can still have opaque first-time cache work

cmd/sdd/search.go sets showView true for any crossRepo query (line 524) and runs the TUI as label "indexing" with StreamLogs false (around 588-589). But internal/handlers/handler_repo.go routes text-only cross-repo prepare through EnsureReposFresh with no progress callbacks (lines 347-352). repos.Manager logs clone at Info (manager.go lines 55-61), which footer-only search hides unless warning or error.

This is not the original 35-minute vector-index hang, but a long first text search that needs to clone or pull a connected cache can still be ambiguous: the footer says indexing while no index work happens and clone/pull info is hidden.

## Verification run

- go test ./internal/handlers -run 'TestBuildConnectedIndexes|TestRepoAdd' — passed.
- go test ./internal/finders -run 'TestMultiSearch|TestGraphSource|TestSearch' — passed.
- go vet ./... — passed.
- go test ./... — passed.
- golangci-lint run ./... — passed with 0 issues after rerunning outside the sandbox so it could access the Go module/build cache.
- Worktree was clean.

Not exercised: the TTY footer was not visually inspected from this agent context, and no live connected index build was triggered to avoid a long Ollama embedding run.
