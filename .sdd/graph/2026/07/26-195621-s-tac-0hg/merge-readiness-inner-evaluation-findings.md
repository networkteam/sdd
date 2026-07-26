# Inner evaluation: session branch-binding & store-locality branch — merge-readiness review

Evaluated: branch `worktree-session-branch-binding` at a57949c (base f2377f1), slices 4375b3a (store locality), b3bff10 (binding core), 7f307cb (binding consumption), 3bb2b7a (procedure prose). Diff: 77 files, ~16,212 insertions / 406 deletions.

Method: four independent review agents (Opus), each with a distinct lens — (1) overengineering/proportionality of the relocation core, (2) architecture/CQRS conformance against AGENTS.md, (3) acceptance-criteria fulfillment including the dialogue-settled migration refinements, (4) correctness/data-safety of the binding slices. Findings coordinated and re-rated centrally. Targeted test packages run green (`go test ./local/... ./mcpapp/... ./application/... ./internal/engine/... ./internal/git/... ./internal/index/... ./cmd/sdd/... ./internal/handlers/...`), `go vet` clean on checked packages.

Overall verdict: **merge-ready — zero blockers; all ACs and refinements verified — with one major proportionality finding (relocation core ~4–5x warranted weight) recorded as a follow-up refactoring gap rather than a merge blocker.**

## Lens 1 — Acceptance criteria: ALL VERIFIED

- **AC1 store locality**: XDG state root keyed by stable repo identity (`internal/repos/config.go:45-71`, `internal/git/stable_root.go:37-48`); init relocation (`handler_init.go`); serve non-blocking with standing notice in door/list/info (`serve.go:70-82`, mcpapp render); cross-worktree discovery (`storelocality_integration_test.go:TestRelocatedSessionIsListedResumedAndRecoveredAcrossWorktrees`).
- **AC2 binding core**: `bind_branch` set/clear with typed `branchBound`/`branchCleared` events and CAS append (`application/workflow.go:655-738`); declare-time exactly-one-checkout validation (`local/git_target.go:59-90`); clean `ErrorBranchUnavailable` where no branch concept; projection into door/resume/list (`mcpapp/tools.go`, `session.go:94`).
- **AC3 binding consumption**: effective-branch precedence with `captureBranch` override (`workflow_registry.go:374-390`); reads follow binding (`workflow.go:1118-1181`); empty-`baseBranch` WIP writes fail loud with exact message (`workflow_registry.go:192-195,214-217`); acquisition failures name the binding when it was the target's source (`application/target.go:97-110`).
- **AC4 procedure prose**: worktree-entry declare + closeout clear, `workTarget` as candidate default with explicit report still required, playback states effective target (`d-prc-imp.md:153,155,180`; `d-prc-cap.md`). Self-guarding requirement well-covered: dedicated tests prove the prose stays correct and `workBranch` stays required even when `worktreeMode` is preseeded wrong or the marker is suppressed (`implementation_test.go:TestImplementation_PreseededWorktreeModeIsSafeForNonWorktreeModes`, `...WorktreeClearGuidanceSurvivesMarkerSuppression`).
- **Migration refinements**: acknowledgement fires only on a pending durable identity transition (in-tree material, or global local/<hash> after repo_id appears); decline/non-interactive holds routing on the current key with persistent notice and re-prompt; tombstones written and marker state dominates material discovery (post-cutover leftovers never re-route — `storepaths_test.go:TestPostCutoverRoutingStaysOnDesiredStoreAcrossWorktreesWhenSourcesReappear`); repo_id A→B deferral honored — no rekey path exists, gap signal recorded (s-tac-vxs on the branch).
- Minor test-coverage edges (5): serve-starts-despite-leftovers asserted only at notice level; integration test doesn't positively assert resolved path under StateRoot; prompt-trigger tested piecewise not end-to-end; A→B no-op not positively pinned; special-file rejection covers symlink+fifo only (guard itself is type-agnostic).

## Lens 2 — Binding-slice correctness: SOUND (no blockers, no majors)

- CAS append semantics enforced (`local/local_sessionstore.go:204-247` version check under file lock; attachment-identity gate `session_runtime.go:132-144`); replay deterministic (metadata snapshot last-writer-wins; binding events audit-only in `decodeWorkflowEvents`).
- The previously-fixed state-poisoning class is NOT reintroduced: caches update only after confirmed writes (`workflow.go:731-737`, `workflow_registry.go:336-338`); durable appends copy from freshly-loaded metadata, never the in-memory cache.
- Minors: effective-branch precedence spread across three functions (`readTarget` field list vs `captureMutationTarget`) with a real drift seam — reads honor `workBranch` where captures don't; `resolvedCaptureBranch` pin shadows a mid-instance rebind (likely intended, deserves a comment); post-conflict in-memory branch briefly intermediate until reorientation (bounded, tested); bound free reads acquire a mutation target per call (possible serialization contention, not data loss).

## Lens 3 — Architecture conformance: clean except placement

- **MAJOR**: `cmd/sdd/storepaths.go` (358 lines + 721 test lines, `package main`) is a store-routing/transition policy engine — I/O, persisted-invariant validation, monotonic-cutover rules — operating exclusively on `local`-package types. Violates library-first/push-down; mechanically extractable into `local/` (three call sites: serve.go:245, recover.go:135, main.go:1415).
- MINOR: path-confinement rule defined twice (`confinedStatePath` in cmd/sdd vs `local_sessionrelocate.go:245,384,787` / `trusted_store.go:283-285`).
- MINOR: `serve.go:76-80` warn-and-continue on notice-inspection failure — borderline; the failure re-surfaces as a persistent client-visible notice on every serve, so continuously loud.
- CQRS wiring, application-layer purity (identity transformer, target errors), single-path `resolveCheckout` extraction, and slog usage all conformant.

## Lens 4 — Proportionality: the relocation core is 4–5x its warranted weight

Scope discipline HELD: no rejected/deferred machinery present (no A→B rekey, no alias/coexistence routing, no authority registry; the transition marker routes to exactly one store at a time). The excess is defensive machinery, not scope violation:

- **MAJOR M1 — two parallel transaction engines.** `local_sessionrelocate.go` (3,025 lines new) and `local_sessionmigration.go` (275→2,453 lines) each implement a full quarantine/proof/rollback/publish vocabulary with independent atomic-publish primitives (`publishNoClobber`/`publishNoClobberWithLink` vs inline `root.Rename`/`root.Link` sequences). At least three separate crash-safe publish paths exist including the store's `writeJSONAtomic`/`writeJSONAtomicRoot` pair.
- **MAJOR M2 — parallel "rooted" store reimplementation.** Every session/blob store method branches on `trustedStateRoot` into a near-complete second implementation (`createRooted`/`loadRooted`/`listRooted`/`appendRooted`, ~570 lines); production only constructs the rooted variant, leaving the path-based implementation dead outside tests/examples — a single-path rule violation.
- **MAJOR M3 — `trusted_store.go` (341 lines) TOCTOU/symlink-rebind hardening**: opens every ancestor as `os.Root`, pins with `os.SameFile`, revalidates the chain at five stages inside Relocate — against an adversary rebinding `~/.local/state/sdd` mid-operation. Not a credible threat model for a single-user offline CLI; one `os.OpenRoot` delivers the accepted containment fix. Related: `rooted_regular_*.go` (107 lines), `os.Root`+`SameFile` in `directory_sync_*` (82 lines).
- **MAJOR M4 — per-payload 4-state machine** (`planned→published→source_quarantined→source_deleted`), quarantine-then-finalize source deletion, and ~200 lines of rollback for already-byte-verified publishes — beyond crash-safe copy + atomic publish.
- MINOR: ~169 lines of `//go:build windows` files unbuildable under goreleaser (darwin+linux only); 3-state transition marker where routing distinguishes two; six-field marker re-derivation on every read path.

**Proportionate implementation estimate: ~1,200–1,600 non-test lines** (one engine, one shared crash-safe publish helper ~120 lines, 2-state marker, one store I/O path with `os.OpenRoot` containment, the pre-existing ~275-line format upgrader essentially unchanged).

**Ordered deletion/consolidation list:**
1. Replace `trusted_store.go` + `rooted_regular_*` + directory-sync pinning with a single `os.OpenRoot` (~530 lines removed).
2. Collapse the rooted/path-based store duplication to one implementation (~570 lines).
3. Merge the two transaction engines onto one shared publish helper; drop the per-payload state machine, quarantine dance, and published-payload rollback (bulk of the ~5,200 combined engine lines).
4. Delete Windows-only files (~169 lines, zero risk).
5. Collapse the transition marker to two states; trim read-path re-derivation.
6. Move routing policy from `cmd/sdd/storepaths.go` into `local/`, consolidating the confinement rule (fixes the architecture MAJOR alongside).
7. Consolidate effective-branch precedence into one shared resolution (fixes the binding-slice drift seam).

Outer validation (the worktree flow serving real sessions in use) is not covered by this run.