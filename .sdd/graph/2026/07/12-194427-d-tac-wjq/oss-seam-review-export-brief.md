# Export brief: seam review findings for the project-aware application plan

## Purpose of this file

This is a handoff for a separate SDD session in `github.com/networkteam/sdd`.
It carries the findings of an architecture review of the captured plan
`20260712-104530-d-tac-1mb` (public project-aware SDD application and shared
MCP surface), conducted from the perspective of designing a real external
multi-project composition against the proposed surface.

The OSS session should read that plan and its attachment in full, dialogue
these findings, and capture the agreed amendments — the session decides the
shape (refine vs. a revised plan). The findings sharpen the plan; none of
them reopens the decision to share one importable project-aware MCP
application. The plan's own pressure-test question 3 (are the proposed ports
the correct semantic boundary?) is the question this review answers.

## Review principle

The proposed surface follows interface segregation — no port is method-fat —
but it models SDD's internal pipeline stages rather than the systems of
record a composition actually owns. The consolidated principle: **ports stay
mechanically small and map 1:1 to real subsystems; invariants live in
SDD-owned orchestration plus a shipped conformance test suite per port**,
not in protocol-shaped method structure. Distrusting adapters via extra
lifecycle methods is the wrong trade when it enlarges the public API.

## Finding 1 — consolidate ports around systems of record

A composition owns six subsystems: identity/access, a canonical graph
store, a durable session log, staged scratch bytes, a retrieval index, and
model execution. The current ~fourteen interfaces consolidate to one port
family per subsystem:

- Merge `SnapshotSource` + `MutationStore`/`Mutation` +
  `CanonicalAttachmentReader` into one **`GraphStore`** (naming aligned with
  `SessionStore`/`StagedBlobStore`; "canonical authority" remains contract
  language, not a type name):

  ```go
  type GraphStore interface {
      Current(ctx) (*Snapshot, error)
      Apply(ctx, expectedRevision string, batch MutationBatch, blobs StagedBlobReader) (ApplyResult, error)
      Reconcile(ctx, mutationID, batchDigest string) (ApplyResult, error)
      ReadAttachmentPage(ctx, entryID, filename string, offset int64, maxBytes int) (AttachmentPage, error)
  }
  ```

  Attachment pages belong here because canonical attachment bytes are graph
  content. `Invalidate` disappears: the adapter learns the new revision from
  its own `Apply` and owns its snapshot cache.
- Merge `PrincipalResolver` + `ProjectResolver` + `DependencyResolver` into
  one access resolver — they answer one question ("who is this and what may
  they touch") and a real composition implements all three against the same
  data.
- `SessionIDGenerator` becomes an options field, not a port.
- `StagedBlobStore` drops `SweepUnretained` (and possibly `Stat`) from the
  SDD-facing surface: SDD never calls sweep, and retention records live in
  the adapter's own storage, so sweeping is composition-side operations
  work. The sweep-safety semantics (atomicity, retain-vs-sweep
  serialization, crash-orphan preservation) move to the port's documented
  contract and conformance suite.
- Weighed and deliberately left open: merging `EmbeddingExecutor` +
  `SearchIndexStore` into one semantic-index port. It would shrink the
  surface (vectors stop crossing the boundary at all), but SDD would lose
  central verification that query-time and index-time embedder fingerprints
  match — a mistake adapters will make independently. If merged, SDD should
  ship the compositor helper (embedder + store → index port). The OSS
  session should decide.

## Finding 2 — drop `Begin`/`Abort` and the lease window; CAS-only `Apply`

The lease held from `Begin` across validation reduces conflict *frequency*
among the composition's own writers only; it was never the correctness
mechanism. A writer outside the composition can always move the canonical
store (e.g. a direct push), so the expected-revision conflict/retry path
must exist and be tested regardless. Therefore:

- `Apply(expectedRevision, ...)` is the only concurrency primitive in the
  contract. A stale revision returns a definitive not-applied conflict; SDD
  re-runs the prepare cycle as a new transition.
- A composition may still serialize its own fetch-check-apply sequence with
  a lease **inside** `Apply` — an implementation detail held for the
  duration of storage operations, never across LLM calls, with no TTL/
  renewal/abort machinery in the public contract.
- Acknowledged cost: under same-project write contention, an optimistic
  retry wastes a prepare cycle including the pre-flight LLM call. Graph
  writes happen at human dialogue pace, so this is the rare case; the trade
  is reversible adapter-internally if contention ever becomes real.

## Finding 3 — conformance test suites as the enforcement mechanism

Ship `sddtest.Run<Port>Tests(t, adapter)` per port in the module. The local
adapters (filesystem GraphStore, JSONL SessionStore, staged-blob dir,
chromem index) are the first consumers, so local/external parity stops
being a separate test bed — it is the same suite run twice. This absorbs
much of what Slice E's fake composition intended, and replaces
method-structure enforcement as the way port invariants stay honest.

## Finding 4 — storage-neutral mutation batches

Replace `Files []FileChange` in `MutationBatch` with canonical
document-shaped changes (the `EntryDocument` representation `SnapshotData`
already uses for the read direction) **plus** SDD-rendered canonical bytes
per item. Filesystem/Git adapters write the bytes verbatim; a structured
store persists the documents directly and never parses Markdown back out of
file changes. Rendering stays SDD-owned in exactly one place, and the seam
becomes symmetric: structured documents in both directions, files only
where an adapter is file-based. WIP markers and other canonical files ride
the same shape (logical path + bytes, document form where one exists).

## Finding 5 — v1 ships single-version, fail-closed

Keep the version fields on stored events, procedure pins, and prepared
envelopes, but implement zero migrations in the first version: any
unsupported version stops with the typed migration-required error. This
removes the migration-acknowledgement events, chain fingerprints, and
envelope-migration rules from v1 implementation scope without changing the
contract; the machinery returns only when a second version of something
actually exists.

## Finding 6 — scope the intent/apply/outcome protocol as the price of split stores

The protocol remains required where the session store and canonical graph
store are independently durable — the expected first composition. Worth
recording as a future seam (not v1 scope): a composition whose session and
graph stores share one transactional database could provide an
atomic-transition capability interface that collapses intent/outcome into
a single transaction; SDD would detect and prefer it.

A composition-side note that belongs in the contract documentation: the
session-store-side consequences of an applied mutation (outcome event,
revision bookkeeping, and any job enqueue the composition performs) may
share one database transaction *with each other*, but never atomically with
the canonical apply — so finalizers stay individually idempotent because
recovery can replay them.

## Finding 7 — session binding: exclusive holder, fencing underneath

Reframe the concurrent-session defense (the gap in
`20260709-174818-s-cpt-c2i`): session-version CAS is the *primitive*, not
the fix. The user-level fix is an exclusive session binding, built entirely
on the existing CAS append plus session metadata — no new port methods.

- A session carries holder metadata: subject, MCP logical session ID,
  client info, bind generation, last activity. Holder history is recorded —
  this also resolves the liveness-visibility gap in
  `20260706-231416-s-tac-y2z` (listings can show "held by X, active N min
  ago").
- Bind/resume claims the session via a CAS-protected metadata update. A
  live holder yields a typed session-in-use error; taking over a live
  session is a user-visible decision, relayed verbatim. An expired binding
  is claimed silently but never invisibly (recorded, listed, and the
  displaced holder's next append fails the version check and receives a
  re-orienting response).
- Liveness layering: activity-based expiry is the base truth; transport
  lifecycle events (stdio process exit, HTTP DELETE of the MCP logical
  session) are early-release accelerators; SSE stream state is only a hint.
  Streamable HTTP guarantees no persistent connection, so no layer above
  the activity TTL may be a correctness dependency.
- State-aware TTLs: the expected silence depends on the step a session
  waits at. Procedure specs may declare per-step TTL overrides above
  chooser-kind defaults (user junction: minutes-scale; host-work/run-mode
  steps: hours-scale or none). This extends the single-source procedure
  spec: one spec feeds guide, gate, and now liveness.
- Identity stance: strong fencing, weak identity, graceful recovery. No
  durable agent-conversation identity exists across reconnects and context
  compaction; design for cheap re-binding with reorientation instead of
  strong holder identity.

## Also observed (candidate housekeeping signals)

- `read_attachment` cannot resolve entries in connected dependency repos
  (qualified or bare ID form) although `show` resolves them — already
  confirmed in an OSS-repo session; noted here for cross-reference.
- Procedure-start anchor params reject repo-qualified foreign IDs, so a
  review of a foreign entry must anchor locally. Worth deciding whether
  anchors should resolve across dependencies like refs do.

## Suggested handling in the OSS graph

Findings 1–6 amend the plan's public contract and its Slice A/B scope;
finding 7 amends the session-persistence and binding sections and adds a
small procedure-spec surface (per-step TTL). Suggested capture: one
refining decision over `20260712-104530-d-tac-1mb` (or a revised plan, the
session's call), with this brief attached as the review record.
