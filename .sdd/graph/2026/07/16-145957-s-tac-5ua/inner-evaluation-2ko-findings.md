# Inner evaluation: local mutation-target and recovery branch (d-tac-2ko)

Branch `codex/2ko-local-mutation-recovery`, commits `4d3fe65` + `14032dc`, 32 files, +1902/−227, evaluated 2026-07-16 against the plan 20260715-170623-d-tac-2ko, its attached design contract, and AGENTS.md rules. Scope: inner lens only (architecture and code review); outer validation not covered by this run. Method: mechanical gates run directly; three independent Opus review passes (architecture, code correctness, delivery coverage); both major findings re-confirmed by direct code inspection.

## Mechanical gates

- gofmt: clean
- go vet ./...: clean
- go test ./...: all 26 packages green
- golangci-lint (pinned): 0 issues

These match the verification claims of the preliminary done 20260716-144647-s-tac-mvd. Its one over-claim: "retained structured facts support … cross-branch attachments" — the code exists, but no test drives attachment bytes across home staging into a target apply.

## Confirmed defects (fix before landing)

### 1. MAJOR — Fail-loud violation: target-release error swallowed on recovery non-apply paths

`application/recovery.go:213-217`: the deferred `_ = acquired.Release()` discards release errors on the `discard`, `abandon-unknown` (→ `terminalRecovery`), and `finalize-retry` (→ `finishTransition`) paths. Only `RecoveryApply` hands ownership off correctly (`release = false`, delegating to `applyOnAcquired`'s error-joining defer). Contradicts the design's "cleanup failure fail loudly" rule and the project's no-silent-fallback rule. Currently masked because the local acquirer's `Release` is a no-op; a future connected adapter performing clone removal there would fail silently. Sibling paths do it right: `applyOnAcquired` (`transition.go:102-107`), `snapshotMutationTarget` (`write_api.go`), `runtime.acquire` (`runtime.go`) all `errors.Join` the release error. Fix is mechanical.

### 2. MAJOR — Interactive `sdd recover` dead-ends on the primary crash-mid-write scenario

`cmd/sdd/recover.go:216-227` + `application/recovery.go:252-256`, `475-487`: a crash after intent append (`transition.go:86`) but before outcome persistence leaves the projection at `unknown`. The interactive verb menu's default branch offers only `abandon-unknown` for unknown-state items — but `RecoverMutation` reconciles first, the local `Reconcile` returns definitive `MutationNotApplied` (`local/local_graphstore.go:385-402`), and the abandon-unknown gate (requires reconciled state `unknown`) rejects the verb with a forbidden-after-`not_applied` error. The gate itself is safe (never wrongly abandons an applied mutation) and the flow self-heals: the failed attempt records a `recovery_attempt` event with the reconciled state, so a second `sdd recover` run reclassifies to `not-applied-awaiting-decision` and offers apply/discard; `--verb apply` also bypasses. But the primary interactive path throws a confusing error on exactly the crash recovery exists for. Root cause: no reconcile-only API, so the CLI guesses verbs from a stale projection.

### 3. Functional gap vs plan — recovery projections wired into `view` only

`application/application.go:117-129`: only `View` calls `listRecoveriesRuntime`. `Info` (orientation) and catch-up are not wired, though the design (slice 6) and AC 11 require all three. The one wired surface uses correct host-neutral language.

## Minor findings and notes

- **Orphaned staged-blob retention window, no GC** (`application/transition.go:83-92`): `Retain` precedes intent append per design ordering; a crash between them leaves a retention keyed by batch ID that no session intent references — never surfaced, never released.
- **Recovery-apply reruns CAS against the original expected revision** (`transition.go:115`, `recovery.go:239`): when not-applied was caused by a graph conflict, the graph has advanced and recovery-`apply` fails CAS every time; only discard-then-recapture works. Consistent with graph-state CAS by design, but the menu still offers `apply` for such items → repeatable `ErrorGraphConflict`.
- **Revalidation DeepEqual fragility** (`application/revalidation.go:35-41`): recovery-path documents round-trip through JSON while fresh parses come from YAML; today all frontmatter values are strings so `reflect.DeepEqual` holds, but any future numeric/boolean/timestamp frontmatter value would break recovery-apply with a divergence error. No test covers the path.
- **`recovery_terminal` on the happy path** (`transition.go:165-173`): every ordinary successful apply appends a terminal event with verb `apply`, so every ordinary capture appears as "recovered" in `sdd recover --history`. Functionally correct; naming conflates first apply with recovery.
- **`view` cost** (`application/application.go:116`): every `View` fully replays every session's event log to derive recovery states — O(sessions × events) on a hot path; candidate for caching/indexing.
- **Empty-target → default fallback softness** (`workflow_registry.go` `mutationTarget` + `write_api.go` `resolveMutationTarget`): a future template regression dropping a `captureBranch` seed would silently capture on the default branch rather than failing loud. Today prevented by the `hasBaseBranch`/`hasWorkBranch` gates and dispatch-seeding tests.
- **`init` seeds `default_branch` from the current branch** (`cmd/sdd/main.go`): initializing on a feature branch commits that branch as the repo-wide default until edited; existing values never overwritten, detached HEAD fails loud.
- **Finalizer commit not branch-pinned** (`local/git_finalizer.go:71-73`): `git -C <checkout> commit` lands on whatever is checked out; relies on the host not switching mid-operation — design-accepted.
- **Cosmetic**: successful finalize-retry prints `state = applied` instead of `recovered` (`cmd/sdd/recover.go:95-99`).

## Verified sound (positive confirmation)

- MutationTarget validated (non-empty branch, session project) before intent persistence; cwd never consulted for authority; defaults resolved during preparation (`application/target.go:13-25`, `transition.go:69,187-196`, `write_api.go`, `runtime.go`).
- One engine-owned apply orchestration; workflow submits facts only; acquisition released before preflight/LLM and reacquired for revalidate → CAS apply → outcome → finalizers; release guaranteed (panic-safe defers) on every path (`write_api.go`, `transition.go`).
- `GraphStore` untouched; conformance suite passes unmodified; expected-revision CAS remains the sole concurrency primitive.
- Local acquisition: `check-ref-format` → NUL-split `worktree list --porcelain -z` parsing (handles spaces/newlines, bare, detached, prunable) → exact `refs/heads/<name>` match (no prefix collisions) → exactly-one → HEAD recheck; zero/multiple/detached/changed-HEAD fail loudly; paths rediscovered per acquisition; no branch-mutating git ops anywhere.
- Six-state derivation correct across all crash boundaries; applied-vs-finalization-pending preserved across restart (apply record persisted under lock before `Apply` returns); crash-after-Apply never mistaken for pre-Apply — test-proven (`TestPreparedTransitionRecoversUnknownApplyAndFinalizer`, `TestPreparedTransitionPersistsNotAppliedAndRejectsStaleBinding`).
- Verb gating enforced on the reconciled state in `RecoverMutation`, not the UI; applied forbids discard; unknown forbids apply/discard; abandon-unknown evidence recorded durably before the terminal event.
- Concurrent recovery processes serialized by session-store version CAS + per-file flock.
- Blob release only when durable state permits; terminal states don't leak; intent-append failure joins retention-release errors (test-proven).
- v1 intents projected legacy-unroutable, never infer a target; bind-target authorized + audited, refuses missing structured facts with `ErrorMigrationRequired`; digest target-independent.
- No automatic replay: read surfaces never acquire/reconcile (`TestReadSurfacesNeverReplayPendingMutation`); transaction recovery only rolls back, never forward.
- CQRS respected in the `internal/` touches; no new operational stderr writes; comments explain current why.

## Delivery coverage

Slices 1–5 shipped (partial tests), slice 6 partial (CLI shipped; orientation/catch-up projections missing; no CLI/presenter tests), slice 7 partial (docs shipped; CLI/presenter/attachment test categories absent).

Acceptance criteria: AC 8 met at core (recovery-authorization contents test-proven); AC 1–7, 9–13 partial — code present, proofs trailing.

Verification matrix (19 rows): **proven** — directory-bound target adapters (row 4), crash-window distinction (row 11), recovery-authorization contents (row 14), GraphStore conformance unchanged (row 18). **Partial** — cwd exclusion (indirect), acquisition matching (multiple-match/changed-HEAD untested), release-on-failure, write-path targets, capture seeding (in-place only driven; same-directory and worktree modes unexercised), no-replay (read lanes only; startup/resume/unrelated-write unproven), abandon-unknown negatives, restart survival of audit/blob state, actionable/closed-history projection (`unknown`/`applied-finalization-pending` unasserted). **Unproven** — empty-branch/apply-time-default rejection (row 1), structured revalidation assertions (row 6), attachment bytes crossing staging → target apply (row 7), discard-forbidden-after-applied/unknown negatives (row 12), legacy-v1 fixtures + bind-target verb (row 16), remote-capability non-exposure guard (row 19); the `apply` and `bind-target` recovery verbs and the entire `recover` CLI have no test coverage.

## Judgment

Sound at the core, not yet landable. No blockers, no data-corruption paths. Fix the two confirmed defects and the projection gap, then close the verification matrix before the closing done.
