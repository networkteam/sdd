# Local mutation targets and recovery

Engine writes carry an immutable project-and-branch authority. The project is the workflow session's project; the branch is concrete before durable intent is recorded. Ordinary captures use the committed `default_branch` setting. Implementation workflows instead carry explicit `baseBranch` and `workBranch` values: WIP coordination targets base, while implementation captures and the closing done target work.

The local adapter never treats process cwd as authority. For each short graph operation it validates the branch, exact-matches it to one entry from `git worktree list --porcelain -z`, rechecks symbolic HEAD, loads that checkout's SDD configuration, and constructs checkout-scoped graph and Git finalizer adapters. SDD does not create, switch, merge, or remove branches or worktrees.

## Durable apply lifecycle

A prepared intent retains the concrete target, structured entry documents, canonical bytes, graph revision, mutation batch identity and digest, and staged-blob ownership. Target acquisition is released before pre-flight and summary model calls. After intent persistence, the engine reacquires the target, validates the retained structured facts against its fresh graph, and calls the storage-neutral `GraphStore.Apply` CAS operation. Target-scoped finalizers are idempotent.

Pending intent never runs automatically during startup, session resume, orientation, view, catch-up, or an unrelated write. Read surfaces only report that action is available.

## Recovery states and actions

The projection answers three separate questions in three fields, so none of them
has to encode the others.

**State** answers only whether delivery was reached, and has three values:

- `delivered`: the batch landed and finalization is proven — at least one recorded finalizer outcome, all successful. Nothing is owed;
- `pending`: delivery is not proven. This is exactly the actionable condition; nothing else is actionable and no pending item is not;
- `abandoned`: a participant decided to stop pursuing delivery.

**Reason** qualifies a state that does not explain itself. For `pending` it names
what delivery waits on; for `abandoned`, which decision ended it; for `delivered`
it is empty.

- `outcome-unknown`: no definitive canonical outcome and no reconciliation establishing one;
- `not-applied`: the batch is definitively absent;
- `finalization-owed`: the batch landed, but either a recorded finalizer outcome failed or no finalizer outcome is recorded at all, which means none ran;
- `discarded`: abandoned as definitively absent;
- `abandoned-unknown`: abandoned without claiming absence.

Applied state itself comes from the recorded outcome: the canonical apply outcome
when it is definitive, otherwise a recovery attempt's reconciliation.

**Recovered** is provenance, not state: it records that recovery machinery — a
reconciliation or a verb — touched this mutation. A write that simply succeeded is
`delivered` with the flag unset; one that needed help is `delivered` with it set.
Both are equally delivered, which is why this is a flag and not a state.

Every recovery action resolves current authorization using the actor, original owner and session, concrete target, and a distinct verb. It then reacquires and reconciles batch ID plus digest before acting:

- `reconcile` is a nonterminal refresh used by interactive clients before they present a verb; it records current evidence but never applies, finalizes, discards, abandons, or binds;
- `apply` revalidates the retained structured facts and retries CAS only from definitely not applied;
- `discard` terminally releases a definitely absent batch;
- `finalize-retry` retries unfinished idempotent finalizers only after application is established;
- `abandon-unknown` is allowed only after recorded failed acquisition or non-definitive reconciliation;
- `bind-target` is reserved for legacy version-1 intent. Intent without sufficient structured facts fails with migration-required and must be explicitly recaptured.

Terminal audit records retain the original owner/session, recovery actor, target, batch identity and digest, verb, reason, and reconciliation evidence. Abandoned entry identities remain available for a future grooming lens; recovery never creates graph entries itself.

## Local command

Run `sdd recover` in a terminal to inspect actionable items and choose an allowed recovery verb with explicit confirmation. An unknown item is reconciled first so the menu reflects current evidence; `reconcile` itself is not exposed as a user-selectable terminal action. `sdd recover --history` shows closed audit history. Non-interactive recovery requires `--session`, `--mutation`, `--verb`, and `--yes`; `bind-target` additionally requires `--branch`.
