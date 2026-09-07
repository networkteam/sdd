# Acquired graph views: delivery evidence

Implements 20260906-234056-d-tac-bae with the write-preservation refinement 20260906-234852-d-tac-vdu.

## API and configuration

- `AcquiredSnapshot.Config *ProjectConfig` supplies immutable committed configuration for one source revision. The application copies language and dependency declarations into an operation-local runtime value. It does not modify the shared runtime or the supplied configuration.
- `SnapshotData.Config` remains stored document data. It is not interpreted as another effective configuration source. An adapter populating both representations must derive them from the same source.
- `Config == nil` explicitly selects runtime configuration compatibility. Source configuration errors propagate; they do not select compatibility. Runtime credentials, providers, index settings and write routing remain composition concerns.
- `ReadAttachmentRequest` adds `Branch` and `BranchFromSession`. MCP forwards both. Paging has no new revision parameter.
- `SnapshotReader`, `SnapshotReadQuery`, `GraphStore.Apply`, Show/View/Search signatures and consumer search preparation remain intact.
- The local `GitWorktreeAcquirer` gains a `ReadFactory` and implements `AcquireSnapshot`. A read-only composition can omit its mutation factory. The CLI composes this reader with its current filesystem store, preserving current-authority selection and registered-worktree branch validation.

## Caller and lifetime audit

| Caller | Source ownership |
| --- | --- |
| Show, View, Lint, Search | Shared acquisition helper; release after operation, including error paths. |
| Attachment read | Home source establishes effective dependency scope; selected owner source supplies both entry membership and page bytes; release after paging. |
| CurrentSnapshot, Info, Procedures | Acquire, materialize immutable value or operation configuration, release. |
| Workflow graph access | Cache materialized views only within an operation; next operation invalidates the cache. View/entry-chain/procedure injections use the selected graph instead of reacquiring. |
| Show/workflow dependency graphs | Materialize authorized dependency graphs before the resulting resolver can escape acquisition. No live source resources hide in lazy graph resolution. |
| Read-side dependency closure | Use acquired home configuration on the session branch and acquired intermediate configuration; check access separately. |
| ReconcileSearchIndex | Graph, hashing and attachment reads share the lease through reconciliation. |
| Search preparation, coverage and retrieval | Same authorized acquired home and dependency sources, with callback-scoped target access. A resumed iterator checks expiry before the next source read. |
| DiscoverSearchEntries | Existing caller-owned lease; caller consumes or stops the iterator before releasing it. |
| IndexSearchEntry | Existing exact-revision acquisition and already-published shortcut retained. |
| Mutation preparation | Acquire the target snapshot and release its storage resources once materialized. Preparation revision remains provenance. |
| Apply retries | Each attempt obtains a fresh materialized snapshot through the helper. Existing revalidation, expected-revision apply, three-attempt retry and recovery logic remain. |

The only application `GraphStore.Current` call outside a pinned wrapper is the explicit legacy current-authority compatibility branch. Named-branch and causal requests reject legacy stores; exact indexing also requires SnapshotReader. An acquisition failure does not fall back to Current.

## Regression evidence

- `TestAcquiredReadsUseReadAuthorityAndRelease`: Show, View, text search, attachment, snapshot, procedure and info reads use acquired sources. The mutation acquisition port refuses access and live Current/attachment reads fail if called.
- `TestAcquiredConfigurationIsPerOperationAndHasOneAuthority`: acquired configuration overrides read settings, empty dependencies remove the runtime list, nil compatibility restores runtime settings, and conflicting SnapshotData.Config does not become another authority.
- `TestSourceConfigurationCannotGrantDependencyAccess`: a source dependency declaration cannot override an access denial.
- `TestAttachmentOperationPinsSourceAcrossBranchAdvance`: two readers observe their own revision and attachment bytes while the branch advances.
- `TestWorkflowServeUsesOneViewAndRefreshesNextOperation`: one framing uses one source; the next operation sees the newer source and configuration.
- `TestAcquiredReadFailuresKeepCleanupErrors`, `TestAcquiredReadCancellationReleasesSource`: operation errors, partial acquisition and cancellation release their source and preserve cleanup errors.
- `TestLegacyReadsRejectBranchAndCausalSelection`: current reads remain available while unsupported guarantees fail explicitly.
- `TestGitWorktreeReadAcquisitionWithoutMutationFactory`: real registered branches remain readable with no mutation factory; unregistered branches fail.
- Existing search/index tests cover fixed preparation targets, exact-source publication, retained-source lifetime, restart and publication shortcuts, indexing interruption, cursor scope and incomplete coverage.
- Existing interleaved-capture, prepared-transition, retry-exhaustion and summary-replacement tests preserve today's write behavior. Unused unavailable dependencies do not become a new write gate.
- `ExampleAcquiredSnapshot` demonstrates loading graph and committed configuration from one immutable filesystem tree and pairing its attachment reader with the acquired snapshot.

## Adoption

Hosted compositions implement SnapshotReader on the runtime graph store, return configuration from the selected source, preserve branch/exact/causal selection and retain source availability independently of active leases. Cache loaded immutable graphs by project/revision and loader configuration, never by session; keep authorization separate. Released leases must not invalidate another reader.

Local language and dependency configuration, including local overrides, keep their runtime precedence through explicit nil Config. Named-branch graph-directory lookup continues using the local configuration resolver. The local filesystem revision pins graph/attachment bytes, not committed repository configuration, and historical sources are not durable across process restart. No local configuration precedence change is introduced.

The broader whole-graph CAS removal is not part of this delivery. Consumer write semantics, provider batching, publication identity and scheduling responsibilities remain unchanged.

## Verification and status

Implementation commit: `0701f288` on `codex/acquired-graph-views`.

- `devbox run test`: passed root module and `examples/extendingsdd`.
- `go vet ./...` through Devbox: passed.
- `devbox run lint`: passed; only existing test-package convention warnings.
- Focused `go test -race` for acquired reads, source configuration, concurrent attachment reads, workflow view reuse, local branch reads and interleaved captures: passed.
- `devbox run build`: passed.
- Fresh binary `sdd view --layout 'rank(by(date)):n(3):brief:as-list'`: passed.
- `git diff --check`: passed.

At completion preparation, the implementation is committed locally, not merged, pushed or released. The implementation procedure owns the subsequent landing status.
