# Local capture-target and explicit recovery design

## Scope

The first delivery makes every local engine graph mutation branch-correct in in-place, same-directory branch, and Git worktree workflows. It does not implement remote Git synchronization, connected-repository mutation, or logical branching for non-Git stores.

The session remains bound to its home project. A mutation carries a separate, immutable target authority:

```go
type MutationTarget struct {
    Project ProjectID
    Branch  string
}

type PreparedTransition struct {
    Version               uint32
    Target                MutationTarget
    ExpectedGraphRevision string
    Batch                 MutationBatch
    BlobOwner             BlobOwner
    BlobIDs               []string
}
```

For this delivery, `Target.Project` must equal the session project. `Branch` must be concrete and non-empty before the intent is persisted. An ordinary capture resolves configured `defaultBranch` during preparation. An implementation move receives agent-supplied `baseBranch` and `workBranch`; cwd may be presented as a locator suggestion but never selects authority.

Expected graph revision is graph-state CAS, not Git lineage identity. A deleted and recreated same-name branch with identical graph content is deliberately indistinguishable in this delivery. No branch-generation token is added.

## Apply-shaped mutation lifecycle

Handlers and workflow commands submit immutable mutation facts to one engine-owned orchestration. They never receive a target runtime or invoke a target GraphStore directly.

1. Resolve fresh write authority for the concrete target.
2. Briefly acquire the target and read the canonical snapshot.
3. Release the acquisition before preflight or LLM execution.
4. Build the structured `EntryDocument`, canonical bytes, batch digest, attachment materializations, and expected graph revision deterministically.
5. Retain the named staged blobs in the session home runtime.
6. Append the durable mutation intent.
7. Reacquire the target.
8. Re-run application-owned structured validation against the fresh target snapshot without regenerating content.
9. Call `GraphStore.Apply(expectedRevision, batch, homeBlobReader)`.
10. Persist the definitive outcome when known.
11. Run target-scoped idempotent finalizers.
12. Persist each finalizer outcome and release retained blobs when the durable state permits it.
13. Release the acquisition on every path.

A future connected adapter may implement ephemeral clone acquisition, refresh, revalidation, non-force push, clone removal, and successful-push cache refresh behind this orchestration. Clone removal is unconditional cleanup; cache refresh follows successful push. Applied-but-unfinalized must never be reported as not applied.

`GraphStore` remains unchanged and storage-neutral. `BlobOwner` and `BlobIDs` are the cross-target content references. The home staged-blob store supplies the reader passed to target Apply; applied canonical state fully materializes attachments and never depends on staging afterward.

## Local target acquisition

The runtime instance wires an internal target-acquisition port. The local Git adapter:

1. Validates the supplied branch with `git check-ref-format --branch`.
2. Runs `git -C <server-checkout> worktree list --porcelain -z`.
3. Exact-matches `branch refs/heads/<name>`.
4. Requires exactly one matching registered checkout.
5. Rechecks `git -C <path> symbolic-ref --quiet HEAD` against the expected full ref.
6. Loads the checkout's SDD configuration and constructs its graph adapter and target-scoped finalizers.
7. Executes directory-bound Git operations such as `git -C <path> add/commit`.
8. Releases the acquisition after the short operation.

Detached checkouts do not match. A branch that exists but is not checked out does not match. Zero or multiple matches, changed HEAD, invalid configuration, or cleanup failure fail loudly while the durable workflow/recovery state remains. Checkout paths are rediscovered for every acquisition and never persisted.

Mode behavior:

- In-place: `baseBranch == workBranch`; one checkout serves all mutations.
- Same-directory branch: create WIP while base is checked out; after the host switches, captures and done target work; after the host switches back, merge and WIP cleanup target base. Only the branch needed by the current mutation must be checked out.
- Worktree: base and work resolve to their separate registered checkouts. WIP targets base; implementation captures and done target work; landing and marker cleanup target base.

SDD never creates, switches, merges, removes, or otherwise owns branches or worktrees.

## Workflow routing

The implementation procedure persists explicit `baseBranch` and `workBranch` as durable move state. It explicitly seeds `captureBranch <- workBranch` at every capture dispatch junction; there is no ambient inheritance rule. WIP creation/removal targets base. Implementation captures and the closing done target work. The session's home project and identity are not rebound.

Ordinary writes also carry a concrete target. Any write path that cannot produce one before intent persistence fails validation.

## Durable recovery state machine

No pending mutation is replayed automatically—not on startup, session resume, orientation, catch-up, view, or an unrelated write.

The read projection exposes these durable states:

1. `unknown`: intent exists without a definitive outcome.
2. `not-applied-awaiting-decision`: reconciliation definitively found no application and a human decision remains.
3. `applied-finalization-pending`: canonical apply succeeded but one or more finalizers or their outcome persistence remain incomplete.
4. `discarded`: definitively not applied and terminally discarded.
5. `abandoned-unknown`: recovery was terminally abandoned without claiming not-applied.
6. `recovered`: applied and fully finalized.

Every recovery attempt freshly authorizes the current actor, acquires the named target when possible, and reconciles by batch ID plus digest before executing a verb.

- `apply`: permitted only from definitively not-applied; repeats structured validation and expected-revision CAS through the shared apply orchestration.
- `discard`: permitted only from definitively not-applied; appends a terminal audited event and releases retained blobs without deleting the intent.
- `finalize-retry`: permitted only after applied; retries only unfinished idempotent finalizers.
- `abandon-unknown`: permitted only after a durable failed acquisition attempt or a reconciliation that remained non-definitive. It appends a terminal audit event and releases blobs while the historical apply outcome remains unknown.
- `bind-target`: legacy-v1 recovery verb that appends a concrete target binding without editing the original intent, after which normal reconciliation can begin.

If reconciliation establishes applied, discard is forbidden. If it remains unknown, ordinary apply/discard are forbidden. A crash after Apply but before outcome persistence is therefore never mistaken for a pre-Apply crash.

## Recovery authorization and audit

Recovery authority is resolved at recovery time; nothing is inherited from the original SessionBinding. Original principal and session are provenance.

A recovery access request contains:

- current actor;
- target project and branch;
- verb: apply, discard, finalize-retry, abandon-unknown, or bind-target;
- original principal and session.

The resolver can distinguish self-recovery from cross-principal recovery and assign any policy it chooses. The engine does not assume ordered authorization tiers. Target-mutating recovery has at least the same target authority floor as a fresh write and is never an elevation path.

Engine invariants remain independent of authorization policy: reconcile before verbs; revalidate and CAS before apply; preserve immutable intent; and record both original ownership and current recovery actor.

Terminal audit events include target, mutation/batch ID, digest, original owner/session, recovery actor, verb, reason, and acquisition/reconciliation evidence. `bind-target` additionally records the bound concrete target and reason.

## Legacy prepared intents

Version-1 intents contain no branch and must never infer one from cwd or current defaults. The normal path lists them as legacy-unroutable until an authorized human performs `bind-target`. The original intent stays immutable.

If implementing safe target binding proves disproportionate, the accepted fallback is `ErrorMigrationRequired` followed by explicit recapture. Silent migration is forbidden.

## User-facing recovery

The first delivery provides an interactive local `sdd recover` command that can operate without a live MCP connection or workflow session. It lists actionable items, names actor/owner/target/state, and requires explicit confirmation for every verb. Closed history is queryable separately like an audit log.

Orientation, catch-up, and view include actionable recovery projections as free reads. Served text is host-neutral: for example, "a pending write awaits explicit recovery." It never names a host command.

After `abandon-unknown`, the item immediately leaves all pending-work lanes. Its full history remains explicitly queryable. Runtime recovery never auto-creates a graph entry.

A future grooming lens may compare entry IDs retained in abandoned-unknown intents with canonical graph content. Only a detected match becomes an ordinary candidate: entries from an abandoned-unknown write are present; keep or retire? The lens itself is deferred, but this delivery must retain the data it needs.

## Implementation slices

1. **Durable model and projection**
   - Add MutationTarget and bump the prepared-transition version.
   - Preserve structured EntryDocument facts in prepared batches.
   - Add recovery events, state derivation, listing/detail queries, dual attribution, and legacy projection.
   - Add serialization/replay tests for every state and crash boundary.

2. **Target acquisition**
   - Add the engine-internal scoped acquisition port.
   - Implement the local Git worktree resolver and directory-bound finalizer.
   - Prove release on success and every failure path.
   - Keep GraphStore unchanged.

3. **Write routing**
   - Route all application write APIs through concrete targets.
   - Resolve ordinary default branch before persistence.
   - Separate home session/staging stores from acquired target graph/finalizers.
   - Revalidate structured prepared content before CAS apply.

4. **Implementation workflow**
   - Add baseBranch/workBranch state and agent-choice prompts.
   - Treat cwd branch only as a suggestion.
   - Route WIP to base and capture/done to work through explicit junction seeding.
   - Cover in-place, same-directory, and worktree sequences.

5. **Recovery application**
   - Add distinct recovery authorization requests and all five verbs.
   - Reconcile before every verb.
   - Implement discard, finalizer retry, abandon-unknown, and legacy bind-target audit.
   - Never replay automatically.

6. **CLI and read surfaces**
   - Add interactive `sdd recover` actionable and history views.
   - Add actionable projections to orientation, catch-up, and view with host-neutral language.
   - Ensure terminal items leave action lanes.
   - Do not add automatic graph captures.

7. **Documentation and verification**
   - Document authority, lineage limitation, modes, state machine, and remote deferrals.
   - Add application tests, adapter fixtures, CLI interaction tests, workflow registry tests, presenter tests, and crash-window recovery tests.
   - Run formatting, unit tests, vet, lint, skill rendering/validation when procedure templates change.

## Verification matrix

Tests must prove:

- empty branches and apply-time defaults are rejected;
- cwd never selects or changes a target;
- exact one-checkout matching, zero/multiple/detached/changed-HEAD failures;
- target graph and target directory-bound finalizer are used;
- acquisition never spans preflight/LLM and always releases;
- structured revalidation uses prepared facts without content regeneration;
- attachment bytes cross from home staging and survive target apply;
- every write path carries a concrete target;
- explicit capture dispatch seeding and base/work routing in all three modes;
- no automatic replay through startup, resume, read lanes, or unrelated writes;
- crash before Apply and crash after Apply/before outcome are distinguished by reconciliation;
- discard cannot follow applied or unknown;
- abandon-unknown requires recorded non-definitive evidence and remains historically unknown;
- recovery authorization receives actor, owner/session, target, and distinct verb;
- terminal audit and blob-release behavior survive restart;
- v1 never infers a target and bind-target is authorized/audited;
- actionable lanes and closed history project the correct states;
- GraphStore conformance remains unchanged;
- remote connected writes and logical branching are not accidentally exposed.
