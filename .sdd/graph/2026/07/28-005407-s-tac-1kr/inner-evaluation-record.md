# Inner evaluation record — d-tac-irt recovery projection fix

Inner lens (verification) for the recovery-projection fix. Run by Claude across
three review lenses plus a fourth independent verification pass, each a separate
agent with no access to the others' conclusions. The outer lens is recorded
separately in s-tac-in2 and was driven by Christopher against the live store.

Scope note: the plan reserved its live-store criterion (AC 5) for Christopher and
forbade implementation and inner evaluation from claiming it. Two agents observed
34 → 0 incidentally while verifying other things; both were instructed to report
it as incidental and did. This record does not claim that criterion.

## Method

Red-then-green with the two steps deliberately given to separate agents, so the
agent writing the failing tests had no ability to make them pass and could not
author a test that passed trivially.

- Step 1 (`ababca5`) — fixtures extracted verbatim from three real live-store
  session logs, plus the failing tests. Test-only scope, enforced.
- Step 2 (`524d3f6`) — the projection fix, with the tests explicitly off-limits.

Then three review lenses in parallel, followed by one verification pass over the
corrected state.

## Lens 1 — acceptance criteria

Verdict: 1, 2, 3, 4, 6 met; 7 NOT met as written; 5 not claimed.

Reproduced rather than trusted. Both red tests were confirmed to fail on the
pre-fix tree for exactly the stated reasons, using scratch copies via
`git archive` so the worktree was never mutated. Fixture provenance was verified
byte-for-byte against the source logs, with the documented reproduction command
re-run and the output confirmed sha256-identical; exactly one derived byte was
found (the `Succeeded: true → false` flip for the boundary fixture) and it is
documented. The integration test's non-vacuity guard is real:
`ListRecoveries(includeClosed=true)` must return exactly 3 first.

AC 7 failed on the nested module: `golangci-lint` exited 1 on a pre-existing
`QF1011` in `examples/extendingsdd/adapters_test.go`, identical on the merge-base
and not introduced by this work. Fixed during the correction pass at Christopher's
direction. Worth noting the exit code is easy to miss — piping into `tail` reports
tail's status, not the linter's.

## Lens 2 — adversarial correctness

Verdict: the fix over-suppressed. Confirmed, and worse than the implementer's own
flag.

`finalizationOwed()` returned true only on a *recorded* failure, so zero recorded
finalizer outcomes read as success. The implementer flagged this as a narrow crash
window. It is not narrow. Because `finishTransition` enters the finalizer loop
only from a definitive apply (`application/transition.go:153`), any item whose
state comes from `attempt.Reconciled` necessarily has no finalizer record at all —
so `finalizationOwed()` is guaranteed false on that branch and
reconcile-says-applied *always* projected non-actionable.

The consequence chain, traced and executed end to end:

1. `ListRecoveries(includeClosed=false)` filters non-actionable items.
2. `selectRecoveryItem` searches only that filtered list.
3. Therefore no CLI path reaches `finalize-retry` — not even explicit
   `--session/--mutation/--verb`.
4. `mcpapp` exposes no recovery verb at all, so no alternate route exists.

Result: a graph entry on disk with its git commit genuinely owed and no
machine-readable trace that it was owed. A silent-loss shape, worse in kind than
the noisy defect being fixed.

The implementer's counter-argument — that treating absence as owed would strand
targets configuring no finalizers — was refuted structurally: the terminal append
at `transition.go:178` sits outside the finalizer loop, so a zero-finalizer target
closes as `recovered` via its terminal and never reaches the branch. The
allowance protected nothing.

Also found and confirmed at this lens: multi-finalizer partial completion projects
as finalized (embedder-only today; the shipped CLI configures exactly one), and
staged-blob retention leaks for every suppressed item.

## Lens 3 — proportionality and architecture

Verdict: the production fix is proportionate and clean; 49 added / 15 removed
lines, no new verb, no migration, no store write, nothing speculative.

Independently reached the same over-suppression finding as lens 2, by a different
route — that the rule was broader than the defect required.

Judged and cleared: purity holds (`recoveryItem` and both helpers are pure, value
receivers, no I/O); the new `RecoveryAppliedFinalized` state is warranted, since
reusing `RecoveryRecovered` would falsely claim a recovery decision nobody made;
altitude is right, because `RecoveryState`/`RecoveryItem`/`ApplyState` are
exported `application` types and `internal/model` holds no session or mutation
concepts at all; the test package split is idiomatic and additionally forced by an
import cycle; the in-test envelope reader does not violate single-path, since the
store's real decode path is exercised by the integration test.

Flagged: two test comments narrating the development sequence ("primary red test",
"the coming fix"), which the comment rule forbids. And `extract_fixtures.go` sits
under `testdata/`, invisible to build, vet, lint and test — it compiles today but
nothing will notice when it stops, and it re-declares event codes that already
exist in `application`. The repo's idiomatic pattern for an expensive manual step
is a build-tagged test.

## Correction pass (`57e4741`)

Delivery now requires positive proof: a recorded outcome that succeeded. No record
at all means no finalizer ran, keeping the item actionable. Two tests pin the
boundary from the silent side — an applied intent with its finalizer record
removed, and one reconciled to applied — both derived by subtraction from the real
stranded fixture rather than hand-assembled.

Alongside: `recordedApplyState` short-circuits only on the two definitive states;
`recoveryTargetLabel` names an unrecorded target instead of rendering a bare `@`;
the nested-module lint offender fixed; the two comments recast. A fixture-helper
parameter was dropped because `unparam` flagged it once the new call sites landed.

## Verification pass over the corrected state

Verdict: the must-fix works. Traced by reading and confirmed by execution.

Executed end to end: after reconcile, state is `applied-finalization-pending`,
actionable, listed, and `RecoverMutation(finalize-retry)` succeeds with the
finalizer actually invoked. Against the tree with only `recovery.go` reverted to
`524d3f6`, the same tests fail — so the reachability regression was real and is
closed. Both new boundary tests confirmed non-vacuous the same way.

A 1200-row decision table (version × apply × attempt × finalizers × terminal),
`ababca5` vs the corrected state, found 80 divergent rows in two classes:

- 24 rows — the intended divergence: applied with at least one recorded successful
  finalizer and no terminal → `applied-finalized`, non-actionable.
- 56 rows — v1-unbound items now report their true outcome-derived state instead
  of a forced `unknown`, while keeping `Actionable` and `LegacyUnroutable`. No
  behavioural consequence: `recoveryVerbs` keys on `LegacyUnroutable` first and
  reconciliation gating is unchanged. Only the rendered string moves, toward
  accuracy.

Terminal-present rows: zero divergence. Toolchain: all six exit codes 0 across
both modules.

Clarification the pass established, not a defect: for a legacy v1 unbound item
`recoveryVerbs` returns only `bind-target`, so the route for a stranded legacy
item is two invocations — `bind-target` then `finalize-retry`. That route was
confirmed usable against a real v1 fixture intent.

## Residual, decided rather than fixed

An applied mutation whose finalizers all succeeded and were recorded, but whose
staged-blob release or terminal append then failed, still projects
`applied-finalized` and non-actionable. The verification pass proposed gating on
prepared-transition version to separate "v1 never wrote terminals" from "v2 never
reached its terminal".

Christopher's call, and the reasoning: under d-cpt-u8o the entry exists and the
git commit succeeded, so the mutation *is* delivered and `applied-finalized` is
the honest projection. The unwritten terminal is session bookkeeping, which that
directive holds to robustness rather than record-grade recovery. What genuinely
remains owed is the staged-blob retention — already an accepted case of d-cpt-msg
with a sweeper as its intended answer. Recorded as s-tac-gtx rather than answered
with a version branch here, since re-arming the notice would reimport the
machinery this work removed.

## Process findings

The inner and outer lenses each caught what the other could not, and the ordering
mattered. Four review agents had passed the first fix; the over-suppression was
found by an adversarial lens explicitly told to hunt for silent suppression, not by
conformance checking — and the plan's own AC 2 was fully satisfiable by the
over-suppressing code, because it named a recorded *failure* as the boundary while
the actual breach was *silence*. That is the plan-level finding recorded in
s-tac-in2, and it recurred one level down: the correction covered the window
before the finalizer record and left the window after it, which the verification
pass then found. Three instances of the same error shape in one work item.

Two prior claims by the author were corrected by reading the code rather than
reasoning about it: the revision CAS is bounded-retry-then-discard, not a
permanent trap; and `ReplaceSummary` is an in-place write, so "nothing here is an
update-in-place" was false. Both corrections changed the resulting contract.
