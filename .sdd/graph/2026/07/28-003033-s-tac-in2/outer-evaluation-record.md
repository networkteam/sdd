# Outer evaluation record — d-tac-irt recovery projection fix

Run 2026-07-28 by Christopher, driving the engine and the CLI against the real
machine-global live session store (not a copy). Lens: outer only.

## Scope

Outer lens only. What outer concretely checks here is taken from the plan's own
AC #5: does a real run against the accumulated live session store report zero
pending-write notices where it previously reported thirty-four? Under s-prc-ogp
this is the one check no amount of artifact review substitutes for, and the plan
explicitly reserves it for Christopher — neither implementation nor inner
evaluation may claim it.

Also in scope: whether the delta is attributable to the fix rather than to a
store that moved (the recent store-locality work, d-tac-ln1) or a binary without
the fix. That false-pass risk is what most of the evidence below addresses.

Explicitly uncovered: the inner lens. ACs 1–4, 6, 7 — the outcome-before-version
ordering test, the finalizer-boundary tests, fixture provenance review, the
no-store-write / no-new-verb diff review, and the toolchain runs — are not judged
here. No inner evaluation of this fix exists in the graph.

## Evidence

### 1. Fresh engine session against the live store

Session `s_20260728-001625-530a9ab9` opened at 00:16:25 against the live store.
The opening serve carried zero pending-write notices, where a fresh session
previously surfaced thirty-four. Christopher's own observation and attestation;
he drove the run, as AC #5 requires.

### 2. `info` — the purpose-built recovery surface — reports nothing actionable

MCP `info` returned `{participant, search, version}` with no recovery section.
CLI `./bin/sdd info` likewise printed no recovery section.

This is a positive zero, not a silent surface. The recovery notices are wired
into both framing paths:

- `application/application.go:49` — `Recovery: renderRecoveryNotices(recoveries.Items)`
  in the info framing
- `application/application.go:136` — `if notices := renderRecoveryNotices(recoveries.Items); notices != ""`
  for the session serve

Each populates only when the render is non-empty, so an absent section means
`renderRecoveryNotices` returned empty — zero actionable items.

### 3. `recover --history` at HEAD: exactly the plan's census, all delivered

`./bin/sdd recover --history` over the same store returns 114 items in exactly
two states:

```
     80 recovered
     34 applied-finalized
```

Thirty-four `applied-finalized` — matching the plan's figure exactly. The same
intents now project as delivered and non-actionable instead of
`unknown · target binding required`. Nothing sits in an actionable state.

### 4. Attribution — no false pass

The store is unambiguously the accumulated live one. It holds all three sessions
the plan named:

- `s_20260714-095955-885b3c45` — history items 1–10
- `s_20260715-165703-e25be2e2` — history items 17–27
- `s_20260716-121551-d740f1c6` — history items 33–35

plus four further sessions carrying stranded intents that the plan did not
enumerate, all now `applied-finalized`:

- `s_20260714-140752-f45e6448` — items 11–12
- `s_20260715-000534-3ece90a2` — items 13–16
- `s_20260715-173937-f595aa69` — items 28–29
- `s_20260715-174231-837b0614` — items 30–32

So the clean result is not a store that moved after the locality work.

### 5. Binary attribution — judgment holds at HEAD

`PATH` resolves `sdd` to the worktree build
(`.claude/worktrees/irt-recovery-projection/bin/sdd`). That binary was rebuilt at
00:20, after the session opened at 00:16 — so the engine serving this session
carries the `524d3f6` state, not the newest commit `57e4741` (authored 00:20:04).

Re-running `info` and `recover --history` with the HEAD binary reproduces the
identical clean result. The judgment therefore holds at HEAD, not only at the
state the running server carries.

## Judgment

Validated. The fix is the right thing and it works on the real corpus: the
thirty-four irremovable notices are gone, the exact census is accounted for as
delivered rather than merely suppressed, and the store was left untouched —
consistent with the read-side-only commitment and with d-cpt-msg's two durable
conditions. AC #5 is met.

## Observation 1 — the plan's boundary criterion named failure, not silence

The first fix (`524d3f6`) over-suppressed. `finalizationOwed` treated *zero*
recorded finalizer outcomes as success, so an applied mutation whose finalizers
never reported projected as delivered. Because `ListRecoveries` filters
non-actionable items and `selectRecoveryItem` searches only that list, the
reconciliation path became unreachable: any item whose state comes from a
recovery attempt's reconciliation necessarily has no finalizer record, so
reconcile-says-applied always projected non-actionable. The result was a graph
entry on disk with its git commit owed and no machine-readable trace that it was
owed — a silent-loss shape, worse in kind than the noisy defect being fixed.

Corrected in `57e4741`: delivery now requires positive proof — a recorded outcome
that succeeded. No record at all means no finalizer ran, which keeps the item
actionable. Two tests pin the boundary from the silent side (an applied intent
with its finalizer record removed, and one reconciled to applied), both derived
from the real stranded fixture.

The finding is a gap in the plan, not only in the code. The plan named the
boundary the fix must not cross as "a recorded finalizer **failure** must keep
the item actionable" — it named failure, and the actual breach was **silence**:
no finalizer record at all. An AC written against the failure case alone passes
while the absent-record case regresses, which is exactly what happened — AC #2
was fully satisfiable by the over-suppressing code. This is the run-it-once
discipline (s-prc-ogp) applying one level up: the plan's own boundary criterion
was under-specified, and no amount of conformance to it would have caught this.

## Observation 2 — census count right, session attribution wrong

The plan's body states that "the three sessions holding them are
s_20260714-095955-885b3c45, s_20260715-165703-e25be2e2 and
s_20260716-121551-d740f1c6." The store holds seven such sessions (§4 above). The
total of thirty-four was correct; its attribution to sessions was not.

Harmless here, because the fix is class-wide rather than instance-targeted — it
teaches the projection to read recorded state rather than enumerating known
instances, so an undercounted session list changes nothing about its reach. But
it is a reminder that a hand-audited enumeration inside a plan body is evidence
of a different grade than the count it accompanies, and that a fix scoped to the
enumeration rather than the class would have missed four sessions.
