# Delegate sessions: design record

Working record from the dialogue of 2026-08-26/27 that produced the delegation
directive. Holds the settled mechanism in full, the shapes weighed and set
aside, and what stays open. The directive carries the decision; this carries the
walk.

## What was observed first

A session dispatched a sub-agent to research a set of entries and hand the
findings back, so the wide reading stayed out of the outer context. The delegate
worked through the `sdd` CLI and `grep` (20260826-222535-s-cpt-dkv). A probe
then established that a dispatched sub-agent shares its parent's MCP connection,
so it is already attached to the parent's session and can drive it, and its
reads land in the parent's read set (20260826-225922-s-cpt-pxq).

Verified in source during the dialogue:

- `mcpapp/session.go` keys attachment on the MCP transport connection.
- `mcpapp/tools.go` `attachedSession` requires the connection be bound to the
  session a work tool names. This is a plain binding check, not consent.
- Consent lives only in `resume_session`: `userWords` for a session this
  connection did not open, `takeover: true` for an actively driven one.
- `mcpapp/tools.go` `startSession` reopens the live session on an already-bound
  connection and drops the `shell` argument; the unbound branch binds the new
  session and leaves the previous one. Neither branch opens an isolated session
  for a delegate.
- `internal/engine/session.go` records the read set at session level by design,
  so a dispatching parent's inspections count for the children it seeds.
- `internal/engine/predicates.go` `refsInspected` evaluates exactly that set.

## The settled mechanism

**Fork on task-class dispatch.** Task class already means a delegate a move
dispatches, kept off the offered moves. It now also means the procedure runs in
a forked child session. The class decides, so forking is never agent discretion
and no new tool is needed.

**What the child inherits.** The parent's branch binding and project target,
plus a lineage pointer to the parent session and the dispatching instance.
Nothing else: empty read set, no instances, its own shell.

**Non-interactive as a declared session property.** The child is marked
non-interactive, and procedure text and choosers may branch on that property. A
user chooser on a live path of a procedure reachable in a non-interactive
session is a spec error rather than a runtime surprise. Capture cannot reach
confirm there, so "a delegate cannot capture" needs no prohibition; it falls
out. Procedures written for autonomous use keep their own path, which is what
intake needs (20260603-172628-d-cpt-fbi).

**Handle-only drive.** Opening a child never moves the connection's binding, and
the parent stays attached where it is. A non-interactive session is driven by
handle: `resume_session` on one requires no `userWords` and rebinds nothing, and
the work tools skip the binding check for it. Interactive sessions are untouched.

**Return transition.** The child's final transition delivers declared values
into declared fields of the dispatching instance and completes the child, which
then concludes. The parent's step does not move. That distinction is what
separates the engine delivering a result the parent asked for from the delegate
driving the parent. Delivery must land regardless of where the parent currently
is, and an undeliverable delivery fails loudly or arrives as a named notice
rather than being dropped.

**No engine-side blocking, no engine-side notification.** The parent is never
suspended waiting for a child. Whether the host waits for the sub-agent or
backgrounds it is the host's business, and the harness carries completion
notification. The engine's only obligation is that the value is present when the
parent next looks and that its serve shows the parent the state it holds, which
the handover fidelity invariant already requires.

## Call sequence

Parent, in session S at shell instance i_1:

    start_procedure { canonical: "explore", session: "S", parent: "i_1",
                      params: { targets: [...], goal: "..." } }
    -> { session: "C", instance: "j_1", execution: "fork-required",
         handoff: "hand these two handles to a disposable delegate" }

The receipt carries handles, not the step serve. The injected target chains must
not come back here, or the parent pays the context the delegation exists to save.

The parent dispatches a sub-agent through its host with exactly two strings, C
and j_1.

Delegate:

    resume_session { session: "C" }
    -> the inspect serve: mission, goal verbatim, target chains in full,
       report_schema. No userWords. The connection stays bound to S.

    show / search as needed        (free reads, recorded in C, not in S)

    next { session: "C", instance: "j_1",
           report: { widenReport, inspectedIds } }      -> compress

    next { session: "C", instance: "j_1",
           report: { briefing } }                       -> return transition:
       briefing delivered into i_1's declared field, C completes and concludes,
       and the serve tells the delegate its work is delivered and to stop.

Parent continues in dialogue. S's read set holds none of the delegate's reads,
so a later capture in S must inspect the load-bearing entries itself before it
can write.

## Shapes weighed and set aside

**A plain fresh session for the delegate, no fork.** Rejected on branch
binding. An unbound session resolves the graph against the configured default
branch, and reads against the wrong tree return present entries as absent, a
silence already misdiagnosed once as a broken tool
(20260728-004132-s-tac-cjt). A delegate researching for a worktree-bound
dialogue would produce a briefing that is wrong in a way nothing reports.
Inheriting branch authority is therefore not optional, which forces a fork
rather than a fresh open.

**Passing the parent's own session handle to the delegate.** Rejected. It grants
a read-only delegate the full write authority of the dialogue that dispatched
it, which is the gap being answered.

**Widening the attachment rule by one case: a connection attached to a session
may also drive that session's children.** Rejected in favour of handle-only for
non-interactive sessions. Three reasons. It is enabling, not protective, so it
reads like a safety mechanism while providing none. A delegate that dispatches
its own delegate produces a grandchild, unreachable under a children-only rule,
so nested delegation breaks. And in a hosted deployment where the delegate has
its own connection the rule never fires, leaving the delegate holding a handle
it cannot use. Handle-only is host-neutral, nests for free, and stops leaning on
connection-keying that the session contract set out to retire.

**Host-carried results: the delegate's text returns through the harness'
sub-agent result channel.** Rejected as the contract, kept as a convenience. It
returns untyped prose nothing validates, it is lost if the parent's context
compacts even though the child's log holds it, and it only works on a host with
such a channel at all, which a cron runner or a hosted worker dispatcher does
not have.

**Parent collects the result: the child completes and waits until the parent
reads it.** Rejected in favour of the return transition. It requires keeping the
child alive as an uncollected thread, and it leaves the result living in session
scaffolding, which by contract carries robustness but never record-grade
durability (20260727-224047-d-cpt-u8o). Pushing on the child's final transition
lands the value in the session that will use it and lets the child conclude at
once.

**Engine-side completion notification to the parent.** Rejected as harness
territory, on the same boundary that keeps the worktree lifecycle out of the
CLI. The harness can wait on a sub-agent or notify on a background one; the
engine only has to have the value present and served faithfully.

**Auto-seeding the parent's procedure with the result rather than a declared
field on the dispatching instance.** Not adopted. Coupling the two procedures
would make each delegate's shape a dependency of its caller's. A declared field
keeps them independent.

## Accepted limit

The delegate shares the parent's connection and the surface cannot tell one
caller from another, so nothing structurally stops a delegate from reaching the
parent's session. It is never told the parent handle, but an argument-less
resume discovers it, and that call cannot simply be removed because it is the
compaction escape for real dialogues. The design removes every reason for a
delegate to make it. That is mitigation, not prevention, and it sits on the
recorded guest-surface ceiling where enforcement reaches the artifact and the
stored state but never the agent's process (20260629-204630-s-cpt-1dz).

## Left open

- Whether a non-interactive session appears in `list_sessions` at all while it
  runs, and what a stalled or abandoned child leaves behind.
- What concluding a parent does to children still running under it.
- Whether the read-set isolation should be relaxed for anything, and if the
  parent should be able to inspect a child's `widenReport` and `inspectedIds`
  for auditability without those reads counting as its own evidence.
- Whether the explore procedure's first step should still inject the target
  chains, given the delegate can fetch them itself and the injection is the one
  part of the mission the parent would otherwise pay for.
