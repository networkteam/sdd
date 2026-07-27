# Audit: 34 unclosable legacy recovery items

Run 2026-07-27 against the live session store at
`~/.local/state/sdd/sessions/github.com/networkteam/sdd/` (100 session files),
relocated out of `.sdd/sessions/` at 2026-07-27T07:23:53Z per `.sdd/sessions/.relocated`.

## Method

Replayed every `*.jsonl` session log, collecting per mutation batch ID:
the `mutation_intent` prepared shape (version, target, per-change
`Document` presence), the `mutation_outcome` apply state, and whether a
`recovery_terminal` event exists. Cross-checked each batch's logical path
against the working tree.

## Store-wide event census

```
workflow_event        2817
workflow_staged_blob    34
mutation_intent        109
mutation_outcome       109
finalizer_outcome      109
recovery_terminal       75
branchBound              1
branchCleared            1
```

109 intents, 109 outcomes — **every outcome is `applied`**. Only 75 terminals.
The 34-item difference is exactly the set surfacing as pending recovery.

## The 34 open intents

All share the same shape: `Version: 1`, `Target: null`, `bound: false`,
`outcome: applied`, and **every change carries `Document: null`** (v1 persisted
only `CanonicalBytes`).

```
s_20260714-095955-885b3c45.jsonl
  entry-20260714-103304-s-tac-rcv
  entry-20260714-105448-d-tac-kag
  summary-20260714-105448-d-tac-kag-b31c9504c179a8bc
  wip-start-20260714-134051-christopher      (wip/20260714-134051-christopher.md)
  entry-20260714-140424-s-tac-fco
  summary-20260714-140424-s-tac-fco-3e3f1cbe39781e38
  wip-done-20260714-134051-christopher       (wip/..., Delete: true)
  entry-20260714-145628-s-tac-58d
  entry-20260714-150222-s-ops-c5c
  entry-20260714-151550-s-ops-dc2

s_20260714-140752-f45e6448.jsonl
  entry-20260714-142231-s-tac-zdt
  entry-20260714-142440-s-tac-19f

s_20260715-000534-3ece90a2.jsonl
  entry-20260715-113417-s-cpt-vxd
  entry-20260715-170623-d-tac-2ko
  entry-20260715-173711-s-cpt-e8x
  summary-20260715-173711-s-cpt-e8x-5c8bdec60e3535e5

s_20260715-165703-e25be2e2.jsonl
  entry-20260715-172002-s-cpt-i5a
  summary-20260715-172002-s-cpt-i5a-62e2349d1105f447
  entry-20260715-173844-d-cpt-o0r
  entry-20260715-174037-d-cpt-eke
  entry-20260715-175731-d-cpt-p3k
  entry-20260715-181235-d-tac-c7b
  entry-20260715-181539-s-cpt-9xm
  entry-20260715-183740-d-cpt-jrt
  entry-20260716-144113-s-cpt-7ch
  entry-20260716-144333-s-prc-7ku
  entry-20260717-003144-d-cpt-ifn

s_20260715-173937-f595aa69.jsonl
  wip-start-20260715-174117-christopher      (wip/20260715-174117-christopher.md)

s_20260715-174231-837b0614.jsonl
  wip-start-20260715-180313-christopher      (wip/20260715-180313-christopher.md)
  entry-20260716-150952-s-tac-zxh
  wip-done-20260715-180313-christopher       (wip/..., Delete: true)

s_20260716-121551-d740f1c6.jsonl
  entry-20260716-122508-s-tac-xm3
  entry-20260716-133307-d-tac-9td
  summary-20260716-133307-d-tac-9td-c3f0f29181d118e2
```

Split: **29 entry/summary intents** on `YYYY/MM/*.md` paths, **5 wip intents**
on `wip/*.md` paths (2 of them deletes).

## Every write landed

Spot-checked logical paths — all present and committed, e.g.

```
.sdd/graph/2026/07/14-103304-s-tac-rcv.md
.sdd/graph/2026/07/14-105448-d-tac-kag.md
.sdd/graph/2026/07/15-173711-s-cpt-e8x.md
.sdd/graph/2026/07/16-133307-d-tac-9td.md
.sdd/graph/2026/07/16-150952-s-tac-zxh.md
.sdd/graph/2026/07/17-003144-d-cpt-ifn.md
```

plus matching records in `.sdd/graph/.sdd-runtime/applied/`. The `wip/` markers
from those dates are likewise gone (only `20260719-191635-christopher.md` remains),
i.e. the delete changes applied too.

## Code paths

Version constants — `application/transition.go:12-13`:

```go
LegacyPreparedTransitionVersion uint32 = 1
PreparedTransitionVersion       uint32 = 2
```

**1. No back-fill of the success terminal.** `application/transition.go:179-192`
appends `recovery_terminal` after a successful apply. That is younger than these
sessions; nothing wrote terminals for intents that applied before it existed.

**2. Actionable = no terminal.** `application/recovery.go:147` —
`if !includeClosed && !item.Actionable { continue }` — so a terminal-less intent
is listed forever regardless of its recorded outcome.

**3. State reported as `unknown`, not `applied`.** `application/recovery.go:602-607`:

```go
item.Actionable = true
if replay.prepared.Version == LegacyPreparedTransitionVersion && replay.bound == nil {
    item.LegacyUnroutable = true
    item.State = RecoveryUnknown
    return item
}
```

The legacy short-circuit returns *before* the `switch replay.apply.State` below it,
so the recorded `applied` outcome is never read. The rendered notice
(`recovery.go:168-174`) then prints `unknown · target binding required`.

**4. Non-bind verbs are refused.** `application/recovery.go:293-301` — legacy +
unbound + verb != bind-target → `ErrorMigrationRequired`.

**5. bind-target is refused too, for all 29 non-wip items.**
`application/recovery.go:408-412`:

```go
for _, change := range prepared.Batch.Changes {
    if !change.Delete && !strings.HasPrefix(filepathSlash(change.LogicalPath), "wip/") && change.Document == nil {
        return RecoveryResult{}, &ApplicationError{Code: ErrorMigrationRequired,
            Message: "legacy intent lacks structured facts required for safe target binding; recapture explicitly", ...}
    }
}
```

Real v1 intents have `Document: null` on every change. The guard therefore rejects
each of the 29 entry/summary items. Only the 5 `wip/` items pass it (path prefix
or `Delete: true`).

The escape it names — "recapture explicitly" — is wrong for this state: the entries
are already in the graph, so recapturing would duplicate them.

## What the v2 shape looks like by contrast

```
entry-20260726-195621-s-tac-0hg  Document: true  target={project: github.com/networkteam/sdd, branch: worktree-session-branch-binding}
entry-20260726-203525-s-tac-q8r  Document: true  target={project: github.com/networkteam/sdd, branch: main}
```

## Why the shipped tests passed

d-tac-2ko's acceptance criteria require legacy-v1 bind-target and its audit event,
and s-tac-spx reports "focused proofs for recovery apply and legacy bind-target"
plus "legacy-v1 audit" as closed. Those proofs necessarily used constructed v1
intents carrying `Document` values — a real v1 writer never produced one. The
guard and the actual legacy corpus were never brought together.

## Reachable end state (for reference, not prescription)

For the 5 wip items: bind-target → reconcile (returns `applied`) → finalize-retry
→ terminal. For the 29 others: no verb sequence terminates them under current code.
