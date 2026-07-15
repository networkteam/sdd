# Local mutation targets and recovery

Engine writes carry an immutable project-and-branch authority. The project is the workflow session's project; the branch is concrete before durable intent is recorded. Ordinary captures use the committed `default_branch` setting. Implementation workflows instead carry explicit `baseBranch` and `workBranch` values: WIP coordination targets base, while implementation captures and the closing done target work.

The local adapter never treats process cwd as authority. For each short graph operation it validates the branch, exact-matches it to one entry from `git worktree list --porcelain -z`, rechecks symbolic HEAD, loads that checkout's SDD configuration, and constructs checkout-scoped graph and Git finalizer adapters. SDD does not create, switch, merge, or remove branches or worktrees.

## Durable apply lifecycle

A prepared intent retains the concrete target, structured entry documents, canonical bytes, graph revision, mutation batch identity and digest, and staged-blob ownership. Target acquisition is released before pre-flight and summary model calls. After intent persistence, the engine reacquires the target, validates the retained structured facts against its fresh graph, and calls the storage-neutral `GraphStore.Apply` CAS operation. Target-scoped finalizers are idempotent.

Pending intent never runs automatically during startup, session resume, orientation, view, catch-up, or an unrelated write. Read surfaces only report that action is available.

## Recovery states and actions

The durable projection distinguishes:

- `unknown`: no definitive canonical outcome;
- `not-applied-awaiting-decision`: the batch is definitively absent;
- `applied-finalization-pending`: canonical state landed but finalization is incomplete;
- `discarded`: definitively absent and terminally discarded;
- `abandoned-unknown`: terminally acknowledged without claiming absence;
- `recovered`: applied and fully finalized.

Every recovery action resolves current authorization using the actor, original owner and session, concrete target, and a distinct verb. It then reacquires and reconciles batch ID plus digest before acting:

- `apply` revalidates the retained structured facts and retries CAS only from definitely not applied;
- `discard` terminally releases a definitely absent batch;
- `finalize-retry` retries unfinished idempotent finalizers only after application is established;
- `abandon-unknown` is allowed only after recorded failed acquisition or non-definitive reconciliation;
- `bind-target` is reserved for legacy version-1 intent. Intent without sufficient structured facts fails with migration-required and must be explicitly recaptured.

Terminal audit records retain the original owner/session, recovery actor, target, batch identity and digest, verb, reason, and reconciliation evidence. Abandoned entry identities remain available for a future grooming lens; recovery never creates graph entries itself.

## Local command

Run `sdd recover` in a terminal to inspect actionable items and choose an allowed recovery verb with explicit confirmation. `sdd recover --history` shows closed audit history. Non-interactive recovery requires `--session`, `--mutation`, `--verb`, and `--yes`; `bind-target` additionally requires `--branch`.
