# Convergence contract — design record

Full record of the dialogue that produced the convergence contract, including the
enumerated failure-mode table, the accepted and not-accepted eventual-consistency
windows, and the code findings that grounded each judgment. The entry body carries
the commitments; this carries the reasoning and the evidence behind them.

## 1. Why recovery existed at all

A confirmed capture crosses independent durable boundaries that cannot be one
transaction:

1. The session records that the transition is ready.
2. The graph authority writes the entry and its attachments.
3. Locally, the Git commit happens afterwards as another fallible step.

If the process dies between them the engine cannot tell whether the graph write
happened, so it may lose a confirmed capture, duplicate it on retry, or report
completion while finalization failed. That is the sound core.

What was over-built: every crash window became a durable user-facing state with
its own verb, authorization, audit event, and legacy path. Branch targeting added
another dimension. Internal reconciliation and a user-facing recovery product
were conflated.

The reframe that resolved it (Christopher): sessions are disposable — their
lifetime is roughly an agent session, with compactions and resumes. Old sdd
sessions are not the durable thing. Durability belongs to captured entries.
Recorded separately as the calibration directive.

## 2. The primitive

A confirmed transition persists exactly one immutable declaration:

    target + operation ID + exact desired content + digest

The engine repeatedly asks the target to ensure that state exists. It never
regenerates content, never changes destination, never replays dialogue or LLM
work.

Pre-generating the ID alone is slightly insufficient, because recomputing
timestamps, summaries, or rendered content could produce different bytes. The
confirmed entry document is already available at that point, so persisting the
exact payload is straightforward and closes the gap.

Note on where crash-safety moved: the durable thing is now the declaration. A
declaration that fails to persist means the capture was never confirmed-durable,
and the user is still in the conversation to redo it. That is a strictly better
place for the problem — benign and unambiguous rather than uncertain.

## 3. Observed condition → behavior

| Observed condition | Behavior |
| --- | --- |
| ID absent | Write the exact content |
| ID present, digest matches | Success; this was a retry |
| Request timed out / process crashed | Retry the same declaration |
| ID present, digest differs | Fail loud (not a durable state) |
| Target unavailable | Remain pending; retry at the defined sweep trigger |
| Authorization lost | Stop pending; require renewed authority |
| Graph validation no longer passes | Pending with diagnostic; dialogue when the user next appears |
| Storage corruption / incompatible format | Fail loud as an operator problem |
| Git commit or other finalizer fails | Keep the graph write, report the failure, retry that finalizer independently |

Two durable conditions only: **pending delivery** and **delivered**. Transient
errors are diagnostics on pending delivery, never new states or verbs.

Why digest-mismatch is not a third durable state: entry IDs are timestamp plus
suffix and the bytes are frozen at confirm, so reaching it requires corruption or
an ID collision. A durable workflow state for it would be a branch only a
constructed test can ever produce — the same disease as the unreachable
`bindLegacyTarget` guard. Same reasoning demotes validation-no-longer-passes from
terminal to pending-with-diagnostic.

## 4. Accepted eventual-consistency windows

- The graph may contain the entry before the session records completion.
- Immutable attachments may exist before the entry publishes them.
- A closing entry may exist briefly before its WIP marker disappears.
- Git commits, indexes, caches, and notifications may lag canonical graph state.
- Duplicate delivery requests are normal.
- Orphan attachments may accumulate after a crash between materialization and the
  entry write. They are content-addressed and harmless, and want a sweeper.

## 5. Not accepted

- Reporting completion before the canonical entry is verified.
- Publishing an entry before its referenced attachments exist.
- Removing a WIP marker before its closing evidence exists.
- Writing different content under the same ID.
- Retargeting or regenerating a confirmed mutation during retry.
- Retrying after authorization loss without resolving access again.
- Discarding a confirmed capture because of contention.

## 6. Dropping atomic multi-document apply

The trade: drop `MutationBatch` multi-document atomicity as a public invariant;
use safely ordered, independently idempotent operations.

Evidence that it costs nothing today — exactly two construction sites, each with
exactly one `DocumentChange`, and nothing ever appends:

- `application/write_api.go:242` — entry capture (one change plus N attachments)
- `application/write_api.go:351` — `applyDocumentMutation`, used by
  `ReplaceSummary`, `StartWIP`, `FinishWIP`

`Changes []DocumentChange` has never held more than one element in production.

The declaration stays whole. Dropping atomic apply is not splitting entry from
attachments: `local/git_finalizer.go:54-59` already stages the entry path and
every attachment path into one commit, and that does not change. What changes is
only that the store need not apply the parts as one indivisible transaction.

Why ordering is sufficient: no operation is an update-in-place except summary
replacement (see §8). Intermediate states left by the ordering are garbage but not
lies — an orphan attachment, a WIP marker outliving its closing entry — and both
are groomable. The two states that would be lies are exactly what the ordering
forbids: an entry referencing a missing attachment, and a marker removed with no
closing evidence.

What is given up is rollback, and that is the point:

- `local/local_graphstore.go:156-171` — rollback on failed apply, revision
  re-derivation, apply-record rewrite, transaction cleanup
- `local/local_graphstore.go:116` — `recoverPendingTransactionsLocked()` sweeping
  transactions from a previous crash

That apparatus exists only to make multi-part apply all-or-nothing. Atomicity
forces rollback; convergence forces retry, which is the same code path as the
first attempt rather than a second inverse one.

The convergence primitive already exists in-tree, twice — the proposal makes it
the whole model rather than a special case inside an atomic one:

- `local/local_graphstore.go:119-129` — look up the apply record by batch ID,
  compare digest, return the prior result
- `local/git_finalizer.go:35-41` — grep for the `SDD-Mutation:` trailer, skip if
  already committed

Groom consequence: partial delivery becomes a normal observable graph state, so
groom needs to distinguish in-flight from abandoned. A pending-delivery record
naming its own operation ID gives that — groom skips whatever an in-flight
declaration claims.

## 7. Uniqueness at the write path, and dropping the revision CAS

"Derive applied by reading the target" is authoritative locally but not for a
hosted store, where a read can lag its own write, so *absent* is not proof of
absence. The guarantee is enforced at the write path by unique-ID-or-conflict; the
read is an optimization. Portability requirement: a store that cannot offer
unique-ID-or-conflict on write cannot host this contract.

On the whole-graph revision CAS at `local/local_graphstore.go:134`
(`ErrorGraphConflict` on any mismatch):

Correction made during the dialogue — this is *not* a permanent trap.
`application/transition.go:118-130` loops read-fresh → revalidate → apply up to
`maxApplyAttempts = 3` (line 25), re-deriving the snapshot each attempt. That is
the revalidated-fresh-revision behavior the session-model directive committed to,
and it is present.

The actual defect: after three lost races `discardContendedTransition`
(`transition.go:220`) discards the intent and returns "re-try the write". The
thing discarded is a confirmed capture, and because `ApplyPrepared` sits at the
end of `CreateEntry` (`write_api.go:265`) after pre-flight and summarize
(line 227), re-trying re-bills both LLM calls. Contention silently costs the user
a redo of work they already confirmed.

Why the CAS can go while revalidation stays: revalidation
(`application/revalidation.go`, called at `transition.go:123`) proves the refs
resolve and the graph still validates against the *fresh* snapshot. Because
entries are append-only, nothing another writer does afterwards can invalidate
that proof — another session adding an entry cannot break yours. Once
revalidation passes, revision equality contributes nothing but a chance to lose
the race. Keep the semantic check; drop the serialization artifact.

## 8. Summary replacement — the one carve-out

`ReplaceSummary` (`application/write_api.go:277`) rewrites an existing entry file
at the same logical path with new content. This is the only in-place entry write
in the system: `applyDocumentMutation` has exactly three callers, and the other
two (`StartWIP`, `FinishWIP`) touch `wip/` paths only. So same-ID-different-digest
is genuinely reachable here — two participants re-summarizing the same entry.

Commitments:

- Precondition is the entry file's current digest. No field-level precondition is
  needed, since "the file I read is still the file that is there" is sufficient
  when summary is the only mutable content.
- No auto-retry. The engine's `replaceSummary` command
  (`application/workflow_registry.go:160`) takes agent-authored `correctedSummary`
  text, not an LLM regeneration — so a retry is mechanically just re-delivering
  the same bytes against a fresh precondition. But auto-retrying makes the loser
  overwrite the winner, and if both sides auto-retry it ping-pongs. Retry is the
  caller's decision.
- The conflict response carries the current digest *and* the current summary text,
  so retry needs no extra read round-trip and no host-local command, and so
  "ignore" is an informed choice rather than a blind one.
- Ignore is a legitimate resolution here, unlike for an entry: a summary is a
  derived pointer, and someone else's is as valid as mine. But it must be the
  caller's decision, never a silent last-write-wins inside the store.
- A human-verified corrected summary must not be silently replaced by a machine
  regeneration.

Caller-specific judgment: in the capture fidelity path a conflict is near
impossible, since the entry was just written by that same transition, so a
conflict there indicates something genuinely wrong. A general re-summarize of an
older entry is the case where the agent should read the current value first.

## 9. Pre-contract intents

The contract requires the persisted exact content. Pre-contract (v1) intents never
stored a structured document, so they are not convergeable under it either. They
are discarded with a visible note; no recovery verb.

Two independent code paths carry the identical structured-document requirement,
which confirms the legacy corpus is unrecoverable by design rather than by one
bug:

- `bindLegacyTarget` — rejects any change that is neither a delete nor a `wip/`
  path when `Document` is nil
- `application/revalidation.go:33-35` — rejects an entry change with a nil
  `Document`, returning `ErrorMigrationRequired`

## 10. Where the contract lives

Not in `ReplaceSummary` as it stands. The preconditions belong to the unified
command-and-handler write path — otherwise they get encoded into a surface that is
slated to be folded.

The summarize CLI is a second instance of the same architecture drift, and in the
opposite direction from the new-entry case: `internal/handlers/handler_summarize.go`
loads the graph and writes updated frontmatter straight to disk, with no mutation
intent, no precondition, and no CAS — and `--all --force` does it across many
entries concurrently under an errgroup. That path is the more likely real-world
source of a lost summary than two engine sessions racing.

## 11. Acceptance bar

Convergence proven by killing the process mid-write against a real accumulated
store, not by a constructed declaration. Every pending declaration must be swept
at a defined trigger, and that sweep must be exercised by the real path — a
pending delivery nobody ever retries is how the notice pile came back.

## 12. Open at time of capture

- Who owns the retry sweep locally and at what trigger (next attach, next
  command) — named as a requirement, not yet chosen.
- Whether the summarize CLI path is folded into the same contract or explicitly
  declared out of scope.
- The sweeper for orphan attachments.
