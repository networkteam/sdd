# Export brief: public project-aware SDD application and shared MCP surface

## Purpose of this file

This is a handoff for a separate SDD session in `github.com/networkteam/sdd`.
It should be used to dialogue about and capture an OSS implementation plan.

The general requirement is to make the existing SDD MCP application usable as
an importable, extensible Go library. External applications should be able to
provide project resolution, identity, persistence, search, attachment, and
command adapters while reusing the exact MCP tools, procedures, served
knowledge, engine behavior, and write gates used by `sdd serve`.

## Suggested capture shape in the OSS graph

- Type: decision
- Kind: plan
- Layer: tactical
- Confidence: medium
- Suggested topics: `portability/mcp`, `implementation/architecture`,
  `project/routing`
- Suggested OSS refs:
  - builds on `20260703-002223-s-tac-kd2`, the shipped workflow MCP shell;
  - grounded in `20260702-174833-d-cpt-3yw`, the in-process workflow-engine
    architecture;
  - related to `20260629-211231-d-cpt-8k5`, the Go in-process runtime verdict;
  - related to `20260610-232719-d-tac-47d`, while keeping transport
    authentication outside the reusable application package.

The OSS session should search and read those entries in full before capture.
It should refine conflicting older plans if necessary, but it should not
reframe this work as a new CLI-parity MCP server: the mature workflow MCP
surface already exists and is the implementation being extracted.

## Extension requirement

An external Go application must be able to import
`github.com/networkteam/sdd`, inject a multi-project implementation, and mount
the shared Streamable HTTP MCP handler inside its own HTTP stack. The local
`sdd serve` command must compose the same application with one implicit local
project.

Once a project is selected, local and external compositions must see one
compatible tool/procedure contract and one source of served SDD knowledge.
There must not be a second implementation of the MCP tools or engine
transitions.

When an external composition supplies identity and access adapters, each
request re-derives the current principal and project access. A session stores
immutable principal and project identities, but neither that binding nor a
cached runtime is proof of current access. Read-only principals may start and
use ordinary dialogue sessions; graph-mutating engine commands fail at the
write gate unless write access is freshly resolved.

## Current code evidence

The existing implementation is behaviorally close but has the wrong import
boundary:

- `internal/mcpserver` owns the mature tool schemas, served instructions,
  session binding, engine registry, free reads, write-gate commands, JSONL
  replay, stdio, and Streamable HTTP.
- `mcpserver.Options` requires concrete internal `*handlers.Handler` and
  `*finders.Finder` values plus one process-wide `GraphDir`, `SessionsDir`,
  `*repos.Registry`, and searcher.
- `Server.newShellSession`, resume, framing, procedure loading, registry
  queries, and all free reads close over that one graph and filesystem session
  store.
- `internal/engine` already has useful seams (`Graphs`, `EventSink`) and
  invalidates graph state after mutating commands, but its types are internal.
- Cross-repository reads already work through the current graph/repository
  machinery and must be preserved behind an authorization-aware dependency
  resolver.
- The current HTTP method owns a static bearer guard. Reusable transport
  authentication must instead compose outside the shared MCP application and
  expose authenticated identity as current per-request metadata.

An external Go module cannot import `internal/mcpserver`, and simply moving it
to a public directory would expose local concrete types and filesystem
assumptions. The plan must extract ports before declaring the API stable.

## Proposed public package boundary

Use two small public packages rather than exporting the existing internal
structs wholesale:

### `github.com/networkteam/sdd` (package `sdd`)

The obvious protocol-neutral SDD application facade. It owns opaque project
snapshots, the concrete SDD project runtime, dialogue-engine orchestration,
and narrow dependency ports. It may use `internal/model`, `internal/engine`,
finders, handlers, and presenters internally, but none of those types may
appear in its exported API. It does not import the MCP SDK.

### `github.com/networkteam/sdd/mcpapp`

The single shared MCP application. It owns tool registration and descriptions,
request-to-project routing, MCP-connection/session binding, read breadcrumbs,
served-once behavior, and mapping typed application errors to MCP results. It
depends on the root `sdd` package, not on a particular external application.

`cmd/sdd/serve.go` becomes a thin local composition root over these packages.
Existing local implementations may remain under `internal/` and be adapted to
the public ports.

The dependency direction is `mcpapp` → root `sdd` → internal implementation.
The root facade may translate public DTOs and ports into internal interfaces;
internal packages must not import the root package in a way that creates a
cycle. The important isolation is protocol-neutral SDD versus MCP-specific
composition, not an additional generically named `app` layer.

## Proposed import surface

The following is design-level Go, not code to copy verbatim. The captured plan
should preserve the responsibilities and type safety even if names sharpen.

```go
// package sdd

type Principal struct {
    // Subject is an opaque, stable identity in the composing application.
    // An external composition supplies its stable subject; local composition
    // uses a stable local subject. SDD does not interpret external roles.
    Subject     string
    Participant string
}

// RequestIdentity is protocol-neutral current-request authentication data.
// mcpapp translates the MCP transport's per-request token metadata into this
// value; the root package never imports MCP SDK types or infers identity from arbitrary
// context values.
type RequestIdentity struct {
    Subject    string
    Scopes     []string
    Attributes map[string]any
}

type PrincipalResolver interface {
    ResolvePrincipal(ctx context.Context, identity RequestIdentity) (Principal, error)
}

type ProjectID string

type ProjectRef struct {
    ID          ProjectID
    DisplayName string
}

type Access int

const (
    AccessRead Access = iota
    AccessWrite
)

type ProjectState string

const (
    ProjectReady          ProjectState = "ready"
    ProjectActionRequired ProjectState = "action_required"
    ProjectUnavailable    ProjectState = "unavailable"
)

type ProjectAction struct {
    ID           string
    DisplayName  string
    State        ProjectState
    ActionURL    string // optional; safe for the user
    Reason       string // optional, safe for the user
}

type ProjectSummary struct {
    ProjectRef
    SourceID    string
    CanRead     bool
    CanWrite    bool
    State       ProjectState
}

type ProjectList struct {
    Actions  []ProjectAction
    Projects []ProjectSummary
}

// ProjectResolver is called for every MCP request that touches a project.
// An access-aware implementation must derive authorization from ctx;
// Principal is binding/audit data, not authority.
type ProjectResolver interface {
    ListProjects(ctx context.Context, principal Principal) (ProjectList, error)
    ResolveProject(
        ctx context.Context,
        principal Principal,
        project ProjectID,
        required Access,
    ) (*ProjectRuntime, error)
}

// ProjectRuntime is a concrete, opaque SDD runtime created with NewProjectRuntime.
// External code configures it with ports but does not reimplement the engine
// or MCP behavior.
type ProjectRuntime struct { /* private */ }

type ProjectRuntimeOptions struct {
    Project      ProjectSummary
    Snapshots    SnapshotSource
    Dependencies DependencyResolver
    Embeddings   EmbeddingExecutor
    SearchIndex  SearchIndexStore
    Mutations    MutationStore
    Finalizers   []MutationFinalizer
    StagedBlobs  StagedBlobStore
    Attachments  CanonicalAttachmentReader
    LLM          LLMExecutor
}

func NewProjectRuntime(opts ProjectRuntimeOptions) (*ProjectRuntime, error)

// Snapshot is immutable and opaque outside the root package. It carries the parsed graph,
// its project/revision identity, and any data needed by the engine/read side.
// No internal model types are exported.
type Snapshot struct { /* private */ }

type SnapshotSource interface {
    Current(ctx context.Context) (*Snapshot, error)
    Invalidate(project ProjectID)
}

// SnapshotData is the canonical, structured input contract. It represents
// stored entry/configuration documents, not SDD's indexed/traversal model.
// Exact fields follow the public graph schema and remain owned by SDD.
type SnapshotData struct {
    Project   ProjectID
    Revision  string
    Config    ProjectConfigDocument
    Entries   []EntryDocument
}

type EntryDocument struct {
    LogicalPath string
    Frontmatter EntryFrontmatter
    Body        string
}

// BuildSnapshot is the single in-memory construction path. It validates the
// canonical documents and builds the opaque indexed graph used by SDD.
func BuildSnapshot(ctx context.Context, data SnapshotData) (*Snapshot, error)

// LoadSnapshotFS is a convenience I/O adapter. It parses canonical files into
// SnapshotData and delegates to BuildSnapshot; it contains no parallel graph
// construction logic.
func LoadSnapshotFS(
    ctx context.Context,
    project ProjectID,
    revision string,
    fsys fs.FS,
    graphDir string,
) (*Snapshot, error)

// A graph may name connected repositories, but resolution is supplied by the
// composition. Access-aware resolution rechecks authorization for every target.
type DependencyResolver interface {
    ResolveDependency(
        ctx context.Context,
        principal Principal,
        from ProjectID,
        repoID string,
    ) (*Snapshot, error)
}
```

`SnapshotData` is intentionally not a public copy of `model.Graph`: it contains
canonical stored documents only, without indexes, resolved chains, derived
status, traversal state, or mutable graph internals. A non-filesystem adapter
may supply it directly; filesystem parsing delegates to the same builder. This
keeps one validation/construction path while avoiding a requirement to
materialize Markdown or `fs.FS` for every persistence backend.

The runtime exposes SDD-owned application operations, not replaceable read or
command services. It owns graph semantics, procedure discovery/parsing,
rendering, read logging, pre-flight, entry construction, summary replacement,
WIP behavior, write gates, and engine transitions. External composition may
authorize before returning a runtime and may supply infrastructure ports, but
it does not implement SDD operations such as `Show`, `NewEntry`, or
`ReplaceSummary`.

The public request/result DTOs should be the same protocol-neutral data the MCP
tools need, while the implementation remains in SDD:

```go
type InfoRequest struct{}
type ViewRequest struct { Layout string; Repos []string; AllRepos bool }
type ShowRequest struct { IDs []string; Up, Down int }
type SearchRequest struct { /* current search fields plus cross-repo selection */ }
type ReadAttachmentRequest struct { ID, Name string; Offset int64; MaxBytes int }

// Every project-scoped structured result contains ProjectRef. Concrete result
// DTOs use a `Project ProjectRef` field; project-independent capability and
// registry results do not.
type ProjectScopedResult struct {
    Project ProjectRef
}

type ProjectScope struct {
    Principal Principal
    Project   ProjectID
    Snapshot  *Snapshot
}

func (r *ProjectRuntime) Info(ctx context.Context, scope ProjectScope, req InfoRequest) (InfoResult, error)
func (r *ProjectRuntime) View(ctx context.Context, scope ProjectScope, req ViewRequest) (ViewResult, error)
func (r *ProjectRuntime) Show(ctx context.Context, scope ProjectScope, req ShowRequest) (ShowResult, error)
func (r *ProjectRuntime) Search(ctx context.Context, scope ProjectScope, req SearchRequest) (SearchResult, error)
func (r *ProjectRuntime) ReadAttachment(ctx context.Context, scope ProjectScope, req ReadAttachmentRequest) (ReadAttachmentResult, error)

// Dialogue/engine methods are likewise concrete SDD behavior. Exact method
// names may differ, but external code supplies stores/effects rather than an
// alternate engine command implementation.
func (r *ProjectRuntime) NewDialogue(/* binding and event sink */) (*Dialogue, error)
```

Infrastructure extension points sit below those operations:

```go
// EmbeddingExecutor performs model execution only. SDD prepares canonical
// document/query text and owns chunking, hashes, and use of the vectors.
type EmbeddingExecutor interface {
    Spec(ctx context.Context) (EmbeddingSpec, error)
    Embed(ctx context.Context, inputs []EmbeddingInput) ([]EmbeddingVector, error)
}

type EmbeddingSpec struct {
    Fingerprint string
    Dimensions  int
}

type EmbeddingInput struct {
    ID   string
    Text string
}

type EmbeddingVector struct {
    ID     string
    Values []float32
}

type IndexNamespace struct {
    Project     ProjectID
    Fingerprint string
    Dimensions  int
    Metric      string
}

// CanonicalChunk is produced by SDD. It is a storage DTO, not permission or
// ranking policy, and contains enough identity to detect stale hits.
type CanonicalChunk struct {
    ID          string
    EntryID     string
    Ordinal     int
    Revision    string
    ContentHash string
    Text        string
}

type IndexedChunk struct {
    Chunk  CanonicalChunk
    Vector []float32
}

type StoredChunkRef struct {
    ID          string
    Revision    string
    ContentHash string
}

type ScoredChunkHit struct {
    Namespace   IndexNamespace
    ChunkID     string
    Revision    string
    ContentHash string
    Score       float64 // raw score/distance under Namespace.Metric
}

// SearchIndexStore performs storage mechanics only. Reconcile atomically
// applies SDD's calculated upserts/deletes. Nearest must filter to exactly the
// supplied namespaces before ranking and returns uninterpreted scored hits.
type SearchIndexStore interface {
    Manifest(ctx context.Context, namespace IndexNamespace) ([]StoredChunkRef, error)
    Reconcile(
        ctx context.Context,
        namespace IndexNamespace,
        revision string,
        upserts []IndexedChunk,
        deleteIDs []string,
    ) error
    Nearest(
        ctx context.Context,
        namespaces []IndexNamespace,
        vector []float32,
        limit int,
    ) ([]ScoredChunkHit, error)
}

// MutationStore owns concurrency and canonical persistence effects. Begin must
// expose the latest snapshot. SDD validates and constructs MutationBatch;
// Apply persists the whole batch atomically with optimistic revision checking.
type MutationStore interface {
    Begin(ctx context.Context, principal Principal, project ProjectID) (Mutation, error)
    Reconcile(
        ctx context.Context,
        principal Principal,
        project ProjectID,
        mutationID string,
        batchDigest string,
    ) (ApplyResult, error)
}

type Mutation interface {
    Snapshot() *Snapshot
    Apply(
        ctx context.Context,
        expectedRevision string,
        batch MutationBatch,
        blobs StagedBlobReader,
    ) (ApplyResult, error)
    Abort(ctx context.Context) error
}

type MutationBatch struct {
    ID      string // stable idempotency/reconciliation identity chosen by SDD
    Digest  string // SDD-owned digest of the canonical batch
    Files   []FileChange // entries, WIP, and other canonical files
    Attachments []AttachmentMaterialization
    Message string
    Author  Author
}

type ApplyState string

const (
    MutationNotApplied ApplyState = "not_applied"
    MutationApplied    ApplyState = "applied"
    MutationUnknown    ApplyState = "unknown"
)

type ApplyResult struct {
    State    ApplyState
    Revision string
}

// Finalizers run only after canonical persistence succeeded. They may create
// an audit record, publish, enqueue indexing, or perform another post-persist
// effect, but they cannot redefine or roll back MutationBatch.
type MutationFinalizer interface {
    Name() string
    Finalize(ctx context.Context, applied AppliedMutation) error
}

type AppliedMutation struct {
    Project  ProjectID
    BatchID  string
    Revision string
}

// MutationError preserves the durable outcome when an operation fails.
// Callers must never blindly retry Applied or Unknown outcomes.
type MutationError struct {
    State    ApplyState
    Revision string
    Stage    string // apply or finalize
    Err      error
}

type BlobDigest struct {
    Algorithm string
    Value     string
}

type BlobOwner struct {
    Subject string
    Session SessionID
}

type StagedBlob struct {
    ID       string
    Owner    BlobOwner
    Digest   BlobDigest
    Size     int64
    Filename string
    CreatedAt time.Time
}

type SweepResult struct {
    DeletedBlobIDs []string
    DeletedBytes   int64
}

// StagedBlobStore owns immutable session-lifetime content. IDs and reads are
// owner-scoped even when an implementation deduplicates bytes internally.
type StagedBlobStore interface {
    Stage(
        ctx context.Context,
        owner BlobOwner,
        filename string,
        content io.Reader,
    ) (StagedBlob, error)
    Stat(ctx context.Context, owner BlobOwner, id string) (StagedBlob, error)
    Open(ctx context.Context, owner BlobOwner, id string) (io.ReadCloser, error)
    Retain(ctx context.Context, owner BlobOwner, retentionID string, ids []string) error
    Release(ctx context.Context, owner BlobOwner, retentionID string) error
    SweepUnretained(
        ctx context.Context,
        before time.Time,
        limit int,
    ) (SweepResult, error)
}

// SDD supplies Apply with an owner-scoped reader limited to the blob IDs named
// by the batch; the mutation adapter never receives a general blob-store key.
type StagedBlobReader interface {
    Open(ctx context.Context, id string) (io.ReadCloser, error)
}

// SDD constructs canonical target identity and expected blob facts. Apply
// verifies them and atomically materializes these bytes with all other files.
type AttachmentMaterialization struct {
    BlobID        string
    Digest        BlobDigest
    Size          int64
    SourceName    string
    CanonicalPath string
}

type AttachmentPage struct {
    Filename   string
    Content    []byte
    Offset     int64
    NextOffset int64
    TotalSize  int64
    More       bool
    Digest     BlobDigest
}

// CanonicalAttachmentReader is the authorized, path-free read mechanic for
// materialized attachments. SDD resolves entry/name semantics before calling.
type CanonicalAttachmentReader interface {
    ReadPage(
        ctx context.Context,
        scope ProjectScope,
        entryID string,
        filename string,
        offset int64,
        maxBytes int,
    ) (AttachmentPage, error)
}

type LLMPurpose string

const (
    LLMPurposePreflight LLMPurpose = "preflight"
    LLMPurposeSummary   LLMPurpose = "summary"
)

type LLMRequirements struct {
    StructuredOutput bool
    MinContextTokens int
    MinOutputTokens  int
    Required         []string // extensible neutral capability names
}

// LLMRoutingContext is opaque adapter-routing metadata. It is not prompt
// content and must not contain credentials.
type LLMRoutingContext struct {
    Project   ProjectID
    Subject   string
    RequestID string
}

type LLMRequest struct {
    Purpose        LLMPurpose
    SystemPrompt   string
    UserPrompt     string
    OutputSchema   json.RawMessage
    Requirements   LLMRequirements
    Routing        LLMRoutingContext
    IdempotencyKey string
    PromptDigest   string
}

type LLMCapabilities struct {
    ExecutorFingerprint string
    StructuredOutput    bool
    ContextTokens       int
    OutputTokens        int
    Supported           []string
}

type LLMUsage struct {
    InputTokens       int64
    OutputTokens      int64
    CachedInputTokens int64
}

type LLMResponse struct {
    Raw                 []byte
    ExecutorFingerprint string
    ModelID             string
    FinishReason        string
    Usage               LLMUsage
}

// LLMExecutor selects provider/model per purpose and performs execution only.
// It never returns SDD domain verdicts such as approved/rejected or findings.
type LLMExecutor interface {
    Capabilities(
        ctx context.Context,
        purpose LLMPurpose,
        routing LLMRoutingContext,
    ) (LLMCapabilities, error)
    Execute(ctx context.Context, request LLMRequest) (LLMResponse, error)
}
```

Attachments have two explicit lifetimes. Session staging stores immutable,
content-addressed, owner-scoped blobs and returns stable identity plus digest,
size, and filename metadata. Canonical attachments exist only after SDD places
`AttachmentMaterialization` records into a mutation batch and that batch is
applied. `Mutation.Apply` opens blobs through the scoped reader, verifies size
and digest, and atomically persists the attachment bytes with the entry/WIP and
other graph files. It never trusts a mutable temporary path.

Before appending transition intent, SDD retains every referenced staged blob
under the mutation ID. The intent records the stable blob IDs and expected
digests/sizes, not the content. Retention remains until the transition has a
durable terminal reconciled outcome (including a definitively not-applied or
applied-with-finalizer-failure outcome). Unknown outcomes, missing outcomes,
and incomplete reconciliation retain their blobs. Session abandonment must
reconcile outstanding intents before releasing their retention IDs. Cleanup
is an atomic store operation, never a `List` followed by `Delete` in
application code. `SweepUnretained` may delete only blobs created before its
cutoff that have no durable retention record at the deletion transaction's
commit point. Retention records are keyed by owner plus mutation ID, and retain
versus sweep must be serialized so a stale candidate list cannot delete a blob
that is concurrently retained. The sweep result exposes only deleted opaque
blob IDs and aggregate bytes for diagnostics, never content or adapter-private
paths. `Release` after a terminal reconciled outcome only makes blobs eligible
for a later policy-driven sweep; it need not delete synchronously. Canonical
reads go through the paged, path-free reader after project access is resolved.

If intent append fails normally after `Retain`, SDD releases that retention.
A process crash in the retain-before-intent window may leave a durable orphaned
retention. The generic sweeper must preserve it rather than guessing that it is
safe to expire; an explicit reconciliation or administrative repair operation
may release it after proving no durable intent can reference it. This chooses a
bounded operational leak over attachment loss in the cross-store crash window.

SDD owns LLM semantics above `LLMExecutor`: complete prompt rendering,
purpose-specific response contracts, optional structured-output schema,
parsing, mechanical checks, finding classification, summary fidelity, and the
decision whether a write gate opens. The adapter selects provider/model for the
declared purpose and returns raw output plus neutral usage/execution metadata.
Before execution, SDD compares the declared purpose requirements with
`Capabilities` and refuses an incompatible selection. It also verifies the
actual executor fingerprint returned by the response. Provider failures,
timeouts, missing capabilities, and malformed SDD output fail loud; the adapter
cannot turn them into an approved/rejected domain result. Idempotency keys are
stable for the relevant transition/purpose, while prompt digest prevents reuse
for different rendered input.

SDD owns the complete retrieval semantics above these ports: canonical chunk
derivation, content hashes, embedding input text, lazy manifest reconciliation,
text/regex matching, vector and text hybrid fusion, entry/type/layer filters,
authorized cross-project fan-out, ranking, stale-hit rejection, citation
construction, and rendering. `SearchIndexStore` never selects accessible
projects or interprets entries. For a shared index, SDD supplies the exact
already-authorized namespace set and the store applies that filter before
nearest-neighbor ranking.

For a read-only composition, `MutationStore` is absent or fail-closed. At each
mutating engine transition, `mcpapp` calls
`ResolveProject(..., AccessWrite)` again before SDD begins the mutation. The
runtime resolved for a prior read is never reused as authorization evidence.
Policy wrappers may control runtime resolution and infrastructure effects, but
they do not replace SDD application semantics.

`Apply` means durable in the adapter's canonical authority, not merely written
to a staging workspace. In a local filesystem composition, writing the graph
may be `Apply` while an automatic Git commit is a finalizer; commit failure is
reported as `MutationApplied` at stage `finalize`. In a composition where a
remote repository is canonical, commit/push belongs inside `Apply`; a failed
push is not applied merely because an ephemeral checkout changed. A finalizer
failure always surfaces, but it never makes an applied mutation appear rolled
back. `MutationUnknown` covers an indeterminate persistence result and requires
reconciliation by batch ID rather than an automatic retry.

For a given mutation ID, `Apply` is idempotent only when the batch digest also
matches; reusing an ID with different content fails. `Reconcile` queries the
canonical authority by mutation ID and digest and returns
applied/not-applied/unknown without applying anything. Finalizers are
individually idempotent by mutation ID plus finalizer name so recovery can
resume them after canonical persistence.

## Per-request identity, context, and runtime lifetime

This is a required refactor, not an optional implementation detail. Current
registry commands call handlers with `context.Background()`, and a session's
registry closures capture dependencies when the shell session is created. In
addition, the current MCP SDK retains arbitrary context values from the
initialization request rather than replacing them on later requests. These
behaviors could turn initialization identity or an old access result into
ambient authority.

- Engine advancement APIs (`Start`, `Report`, `Answer`, `Serve`, replay-driven
  continuation, or their public wrappers) must accept the current request
  context for cancellation, deadlines, tracing, and logging.
- The engine function context must carry that `context.Context`; registry
  queries and commands pass it to all ports. Production paths must not replace
  it with `context.Background()`.
- Identity and authorization data do not rely on arbitrary context values.
  `mcpapp` extracts current-request transport authentication metadata and passes
  a protocol-neutral `RequestIdentity` explicitly through principal/project
  resolution and engine authorization checks.
- A shell session stores only immutable `Principal.Subject` and `ProjectID`
  plus engine state. It must not retain a resolved runtime as authorization
  evidence.
- Registry functions close over a request-scoped runtime accessor, not one
  runtime value. Read functions resolve `AccessRead` for the current context;
  graph-mutating commands resolve `AccessWrite` at the transition itself.
- In-memory snapshots and runtime objects may still be cached as data or
  performance optimizations after authorization succeeds. Cache identity and
  authorization lifetime are separate concerns.
- A real stateful Streamable HTTP contract test sends two requests on one MCP
  session with the same subject but different scopes or identity attributes.
  The second request's sentinel must reach project resolution, an engine query,
  and a mutation authorization check. A request with a different subject must
  be rejected as a session-owner mismatch.
- Public HTTP composition is blocked until that test passes. If the SDK's
  per-request `TokenInfo`/headers cannot safely carry what is required,
  `mcpapp` adds a narrow private bridge or contributes an upstream hook; it
  never falls back to initialization-time identity.

## Session persistence contract

MCP connection presence remains in memory and disposable. Dialogue sessions
and engine events use an injected store. The store must not expose internal
engine types; persist engine events as versioned opaque JSON payloads while
keeping routing/ownership metadata structured.

```go
type SessionID string

type Session struct {
    ID             SessionID
    Subject        string    // immutable Principal.Subject
    ProjectBinding ProjectID // immutable selector; resolve before every use
    Participant    string
    Label          string
    Status         SessionStatus
    Version        uint64    // optimistic append position
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type SessionFilter struct {
    Subject string
    Status  []SessionStatus
}

type StoredEvent struct {
    Sequence     uint64
    CodecVersion uint32
    Payload      json.RawMessage // opaque engine event envelope
}

type EventToAppend struct {
    CodecVersion uint32
    Payload      json.RawMessage
}

// ProcedurePin is written into the instance-start event. NormalizedDocument
// is the exact SDD-normalized procedure used by that instance, not a canonical
// name to resolve again during replay.
type ProcedurePin struct {
    Canonical          string
    SourceEntryID      string
    NormalizedDocument []byte
    Digest             string
    SpecVersion        uint32
    RegistryVersion    uint32
}

type MigrationRequiredError struct {
    Session         SessionID
    Component       string // event codec, procedure spec, or registry contract
    StoredVersion   uint32
    Supported       []uint32
}

type SessionPatch struct {
    Label     *string
    Status    *SessionStatus
    UpdatedAt time.Time
}

type SessionStore interface {
    Create(ctx context.Context, session Session) error
    Load(ctx context.Context, id SessionID) (Session, []StoredEvent, error)
    List(ctx context.Context, filter SessionFilter) ([]Session, error)
    // Append assigns the next sequence and applies patch atomically. It fails
    // on expectedVersion mismatch and returns the new version on success.
    Append(
        ctx context.Context,
        id SessionID,
        expectedVersion uint64,
        event EventToAppend,
        patch SessionPatch,
    ) (newVersion uint64, err error)
}

type SessionIDGenerator interface {
    NewSessionID(now time.Time) (SessionID, error)
}
```

An external composition may supply its own session ID and persistence strategy.
Local composition may preserve current human-readable handles behind
`SessionIDGenerator` and a JSONL `SessionStore`, provided tool handles remain
strings. The application must reject resume/list/abandon when the current
principal does not own the session, and it must re-resolve the bound project
before replay or use. `Session.ProjectBinding` is persistence and routing data,
not necessarily the canonical ID returned to callers: a composition may retain
an immutable historical alias there, while authorization and public results use
the currently resolved `ProjectRuntime` and its canonical `ProjectRef`.

## Pinned procedures and replay migration

Starting every procedure instance persists an instance-start event containing
the exact normalized `ProcedurePin`. This applies independently to the session
shell and every dispatched submove. The canonical name remains discovery data
for new instances only; replay verifies the pin digest and reconstructs the
instance from the pinned document, never from the current graph or currently
embedded procedure with the same name.

Deterministic replay is versioned on three axes: event codec, normalized
procedure-spec format, and registry function contract. SDD owns codecs and
explicit migrations for all three. `Load`, `List`, and `Resume` apply supported
migrations only to the in-memory replay view and never append merely because a
session was read. The store remains an opaque ordered event store and never
interprets, rewrites, compacts, or migrates historical payloads itself.

When a successfully migrated session is next authorized to advance, its first
CAS-protected append records a versioned migration-acknowledgement audit event
before the new transition. The event names the original and target event-codec,
procedure-spec, and registry-contract versions for every migrated axis, plus a
stable migration-chain/version fingerprint. It acknowledges the replay
baseline used for subsequent execution; it does not claim that historical
payloads were rewritten. Replay recognizes an existing acknowledgement for the
same target baseline so retries do not append it repeatedly. No external effect
may occur between deciding that acknowledgement is required and durably
appending it. A crash after the acknowledgement but before the transition is
safe: replay observes the acknowledged baseline and the requested transition
may be attempted normally under session-version CAS.

Unsupported versions or a digest mismatch stop before advancement with a typed
migration-required or integrity error and append no acknowledgement. There is
no silent fallback to the current canonical procedure.

Pinned control-flow semantics do not grant stale authority. Resume first
resolves the current principal and project read access. Before any resumed
external effect, SDD resolves current write access, loads the current authority
snapshot, and applies current mechanical validation and write-gate checks. A
pinned procedure whose registry contract is no longer supported must be
explicitly migrated or abandoned rather than partially executed.

## Durable engine transition protocol

The session store and mutation authority are independently durable and cannot
share one transaction. Mutating engine transitions therefore use a required
intent/apply/outcome protocol:

1. SDD completes all nondeterministic semantic work, including any required
   LLM execution, and deterministically prepares a versioned canonical
   `PreparedTransition`. Preparation freezes:
   - instance identity, source step, selected transition, and deterministic
     engine-state delta;
   - registry operation identity and pinned contract version;
   - the canonically serialized `MutationBatch`, including its mutation ID and
     digest, generated IDs/timestamps, entry documents, WIP changes, attachment
     materialization metadata, message, and author;
   - expected authority revision;
   - ordered finalizer descriptors with stable identity/version and only the
     non-secret parameters needed to resume them; and
   - prepared result data needed to produce the same engine outcome.
2. It appends a `transition_intent` event with session expected-version CAS.
   Its payload is the complete canonical `PreparedTransition` envelope plus a
   digest over that envelope, not a set of inputs from which the transition is
   later reconstructed. Stable staged-blob IDs, digests, and sizes are included
   in attachment materialization metadata; blob bytes remain in the staged
   blob store. Winning this append claims the transition; a concurrent caller
   receives a session conflict before any canonical mutation effect.
3. SDD calls `Mutation.Apply` with the same mutation ID, digest, and expected
   authority revision.
4. It runs idempotent finalizers only when canonical apply succeeded.
5. It appends `transition_outcome` with a second expected-version CAS,
   recording applied/not-applied/unknown, resulting authority revision,
   finalizer status, and diagnostic error details.

Every engine advance, including a non-mutating one, appends with session
expected-version CAS so two connections cannot evolve one session from the
same state. Mutating advances additionally use the intent protocol above.

On replay, an intent without an outcome is never executed blindly. SDD first
calls `MutationStore.Reconcile(mutationID, batchDigest)`:

- `applied`: resume any incomplete idempotent finalizers and append the applied
  outcome without applying the mutation again;
- `not_applied`: the prepared batch may be applied again with the same ID only
  through the explicit recovery path, then its outcome is appended;
- `unknown`: stop the transition and surface reconciliation-required state;
  do not retry automatically.

Recovery decodes the persisted prepared envelope and may rerun current
authorization and mechanical/write-gate validation against its frozen batch,
but it must not regenerate transition targets, IDs, timestamps, summaries, LLM
prompts or results, mutation content, or prepared engine/result data. Missing
or incompatible registry/finalizer implementations fail with a typed
recovery-required error rather than substituting new behavior. If current
authority has advanced, the frozen expected revision conflicts; recovery does
not rebuild a new batch under the existing mutation ID.

Prepared-envelope codecs are explicitly versioned and SDD-owned. A supported
migration may deterministically construct an in-memory representation needed
to execute the old prepared transition, but the persisted canonical envelope,
original mutation ID, original canonical batch bytes, and original batch digest
remain immutable. Reconciliation always uses that original ID/digest pair. If
an upgrade cannot preserve those invariants and execute the same semantic
batch, replay stops with a typed migration-required error.

Failed applies and failed finalizers both produce diagnostic outcome events.
This keeps replay evidence complete and ensures an applied mutation never
appears rolled back merely because the outcome append or a finalizer failed.

## Proposed MCP application surface

```go
// package mcpapp

type Options struct {
    Principals sdd.PrincipalResolver
    Projects   sdd.ProjectResolver
    Sessions   sdd.SessionStore
    SessionIDs sdd.SessionIDGenerator
    Version    string
    LocalClient bool
}

type Application struct { /* private */ }

func New(opts Options) (*Application, error)

// Handler contains MCP protocol behavior only. Authentication middleware is
// composed outside it; mcpapp must obtain authenticated identity from a
// verified current-request channel, not initialization context.
func (a *Application) StreamableHTTPHandler() http.Handler

// Local stdio remains a composition of the same Application.
func (a *Application) RunStdio(ctx context.Context) error

// Retain an in-memory transport entry point for integration tests without
// requiring external consumers to depend on internal packages.
```

The shared handler must not own an external application's authentication
protocol. A composing HTTP stack may mount it behind middleware; `mcpapp`
translates verified per-request authentication metadata into the
`RequestIdentity` consumed by `PrincipalResolver`. `cmd/sdd` may preserve the
current static bearer-token wrapper for standalone local HTTP.

Avoid exposing the MCP SDK server as the primary API unless a concrete
composition need proves necessary. `http.Handler`, stdio, and a test transport
keep the dependency surface smaller and allow SDK changes inside the module.

## Tool and routing semantics

The surface remains the current workflow MCP surface with these additive,
generic project semantics:

1. Add `list_projects`.
   - An external composition may protect it with its authentication middleware.
   - It returns the accessible project subset plus generic action-required or
     unavailable states supplied by the resolver.
   - Required setup is represented generically as `action_required` with an
     optional safe URL; external credentials never cross MCP.

2. Add optional `project` to `start_session`.
   - Multi-project composition requires it.
   - Local composition may infer the sole project.
   - Zero or multiple selectable projects without an explicit value returns a
     typed project-required error.

3. Bind each session immutably to `Principal.Subject` and a project-binding
   selector, normally the canonical `ProjectID`.
   - Resolve that binding through `ProjectResolver` before every use; authorize
     and render results from the resolved project rather than copying the
     stored selector into public output.
   - Include `ProjectRef` in every session serve, nested/base serve, session
     descriptor, list result, and resume result.
   - `list_sessions` filters by principal and includes project.
   - `resume_session` and session teardown verify owner and current read access.

4. Add optional `project` to project-scoped free reads:
   - `search`, `view`, `show`, `read_attachment`, and graph-specific `info`;
   - outside a session, `project` is required unless one project can be inferred;
   - inside a session, the bound project is used and a differing explicit
     project is rejected rather than silently switching context.
   - every search, view, show, attachment, and graph-specific info structured
     result includes the resolved base `ProjectRef`.

5. Keep server capability/registry reads project-independent where possible
   and omit `ProjectRef` from those results.
   Procedure availability and framing remain project-derived once a session is
   bound because projects may supersede procedures and configure language.

6. Preserve cross-repository arguments and IDs.
   - The selected session/argument project is the base graph.
   - Each connected target resolves through `DependencyResolver` and therefore
     receives its own authorization check.
   - An unauthorized dependency is unavailable without leaking more metadata
     than an unknown dependency.
   - `all_repos` means all dependencies accessible through the resolver, never
     every project known to the composing application.

7. “Free read” continues to mean outside the workflow write gate, not outside
   authentication, project authorization, or audit logging.

8. Typed errors cover at least:
   - authentication required;
   - project required / project unavailable;
   - external action required (safe URL and action identity);
   - read denied;
   - write denied/read-only;
   - session ownership mismatch;
   - stale/conflicting session event append.

`ProjectRef.ID` is the stable application project identity. `DisplayName` is
presentation metadata and may change without changing session binding. Neither
field exposes source-system identifiers, repository URLs/paths, credentials,
roles, or authorization details. Cross-repository results still identify the
selected base project with `ProjectRef`; individual foreign entry IDs retain
their existing repository scoping.

The local composition does not invent or persist a project identity for a
local-only graph. While no canonical `repo_id` exists, its one implicit project
uses the composition-scoped sentinel `ProjectID("local")` and the repository
directory basename as `DisplayName`. The sentinel is not a graph identity, is
not globally unique, and cannot be used as a cross-repository address; none of
those properties are needed while `sdd serve` exposes exactly one implicit
local project. This fallback must not modify `.sdd/meta.json` or
`.sdd/config.yaml`.

Once the graph has a real canonical `repo_id`, the local composition uses that
as the project ID for new sessions and results. To keep replay valid across
that transition, the local session resolver continues to accept `"local"` as
an alias for its sole project when loading sessions created before the
`repo_id` existed. Those sessions retain `"local"` in their immutable stored
binding and their events are not rewritten. Listing, resuming, or otherwise
using them nevertheless returns the newly resolved canonical `ProjectRef`, and
authorization operates on that resolved project. Only canonical `repo_id`
values participate in cross-repository references.

The first implementation can render connection-required as a structured tool
error with a URL. URL elicitation can be added when the MCP SDK/client support
is validated without changing the application error.

## Compatibility rules

- Current local tool names and current non-project arguments remain valid.
- Local single-project behavior remains implicit; callers are not forced to
  send a synthetic project ID.
- A local-only graph persists no synthetic `project_id` or fallback `repo_id`;
  the local composition uses the non-persisted `"local"` sentinel until a
  canonical `repo_id` exists, and preserves it only as a replay alias afterward.
- Existing served procedure knowledge, schemas, chooser behavior, write gates,
  read logging, and result text remain generated by one implementation.
- Additive project fields may change generated JSON schemas but must not fork
  local and external schema definitions.
- Every project-scoped structured result adds `ProjectRef`, while existing
  local human-readable result text remains byte-for-byte stable unless project
  context is required to explain an error or reorient the user.
- `sdd serve --transport http --auth-token ...` remains supported as the local
  standalone composition until an OSS decision explicitly changes it.
- No exported signature contains a type from an `internal/` package.
- No public runtime contract requires a persistent host filesystem path.
- Remote attachment results never expose local paths.
- Access decisions and external credentials are never stored in engine events.

## Implementation slices

### Slice A — freeze parity and public contract

- Add a real stateful Streamable HTTP spike over MCP SDK v1.6.1: initialize
  with one subject, then send later requests with the same subject and changed
  scopes/attributes. Prove the latest values reach project resolution, an
  engine query, and mutation authorization; prove a changed subject is
  rejected. This is a blocker for declaring the public HTTP application viable.
- Decide from the spike whether supported per-request SDK metadata is
  sufficient, a narrow private `mcpapp` bridge is needed, or an upstream SDK
  hook should be contributed. Keep that mechanism out of the root `sdd` package and prohibit
  initialization-context fallback.
- Add contract tests that snapshot the current tool set, schemas, descriptions,
  core response shapes, served-once behavior, and a complete capture replay.
- Add external-package compile tests (`package sdd_test` and
  `package mcpapp_test`) proving the intended APIs can be imported without
  internal packages and the root package does not compile against MCP SDK types.
- Settle root `sdd`/`mcpapp` DTOs, typed errors, and the interfaces above before
  moving behavior.
- Document API stability expectations: public but pre-1.0/evolving until an
  external multi-project composition validates it.

### Slice B — extract the protocol-neutral runtime

- Introduce opaque `Snapshot`, `SnapshotSource`, the concrete project runtime,
  and infrastructure ports for dependencies, embedding execution, index
  storage, mutations, attachments, LLM execution, and sessions.
- Define purpose/capability-driven `LLMExecutor`; keep prompt construction,
  parsing, findings, summaries, and gate decisions inside SDD.
- Split immutable owner/session-scoped staged blobs from canonical paged
  attachment reads; materialize verified staged refs only through MutationBatch.
- Split atomic canonical `MutationStore.Apply` from post-persist finalizers and
  preserve applied/not-applied/unknown outcomes through typed errors.
- Add the session-CAS transition intent/apply/outcome protocol and mandatory
  mutation reconciliation before recovering an incomplete transition.
- Pin each instance's normalized procedure plus digest/spec/registry versions;
  add SDD-owned event/procedure migration and fail-closed unsupported replay.
- Thread operational request context plus explicit `RequestIdentity` through
  engine advancement and registry execution; remove production
  `context.Background()` calls from graph queries/commands.
- Make registry closures use a request-scoped runtime accessor so a session
  binding never caches authorization.
- Adapt current graph loaders/finders/handlers/search/repositories behind the
  SDD-owned local runtime without changing behavior.
- Add the structured `SnapshotData` plus single SDD-owned `BuildSnapshot` path;
  make the `io/fs` and existing path loaders parse into it. Add a structured
  non-filesystem fixture proving no host filesystem or Markdown materialization
  is required.
- Keep the engine and model internal; translate only through public DTOs.

### Slice C — extract the shared MCP application

- Move/refactor `internal/mcpserver` behavior into public `mcpapp` without
  duplicating tool registrations.
- Expose the transport-neutral Streamable HTTP handler, stdio runner, and test
  connection seam.
- Move static bearer authentication to the local `cmd/sdd` composition.
- Make `cmd/sdd/serve.go` a thin single-project adapter and pass all parity
  tests.

### Slice D — add project routing and immutable session binding

- Add `list_projects`, project selection/inference, project fields on free
  reads, and project-bearing session descriptors.
- Add compact `ProjectRef` to every project-scoped structured result while
  preserving local human-readable text compatibility.
- Resolve principal and project on every project-scoped request.
- Introduce the session store and ID generator ports; implement JSONL/local
  parity and opaque event replay.
- Persist procedure pins on instance start and prove replay remains
  deterministic after the current procedure with that canonical changes.
- Include session version in application serves/bindings and reject concurrent
  advances through expected-version append rather than in-memory assumptions.
- Reject project switching, session ownership mismatches, and current-access
  loss explicitly.

### Slice E — authorization-aware cross-repo and read-only proof

- Route every connected-repository resolution through `DependencyResolver`.
- Provide a fake external composition with at least two principals, two base
  projects, one connected dependency, and differing read/write permissions.
- Prove read-only principals can start sessions and use dialogue/read tools.
- Prove mutating procedure transitions re-resolve `AccessWrite` and fail at the
  write gate without writing.
- Prove an external Go module can mount the handler behind middleware, preserve
  current per-request identity, select a project, start/resume a session, and execute
  `info`, `view`, `search`, `show`, and `read_attachment`.

### Slice F — release handoff

- Run full unit/integration tests, lint, local stdio smoke, and local HTTP smoke.
- Add package documentation and a minimal external composition example.
- Release/tag a version that an external Go module can import without a
  committed local `replace` directive.
- Record the OSS plan’s done signal with the release/commit and the exact
  public package version.

## Acceptance criteria for the captured OSS plan

- [ ] A non-OSS Go module imports public SDD application packages and mounts
      the exact workflow MCP surface without importing any `internal` package.
- [ ] The root `github.com/networkteam/sdd` package is the protocol-neutral
      facade and has no package dependency on the MCP SDK; only `mcpapp` owns
      MCP-specific tools, request metadata, and transports.
- [ ] Local `sdd serve` and an injected multi-project composition use the same
      tool registrations, schemas, procedures, served knowledge, engine
      transitions, and write gates.
- [ ] SDD owns standard reads, graph semantics, rendering, procedure loading,
      pre-flight, entry creation, summary replacement, WIP behavior, and write
      orchestration; external applications supply infrastructure and access
      adapters rather than replacement `ReadService`/`CommandService` logic.
- [ ] Search exposes `EmbeddingExecutor` and `SearchIndexStore` mechanics only;
      SDD owns chunks, hashes, reconciliation, lexical matching, hybrid fusion,
      filters, authorized namespace fan-out, ranking, citations, and rendering,
      with no replaceable `SearchService`.
- [ ] Staged attachments are immutable and owner/session-scoped with stable ID,
      digest, size, and filename metadata; mutation intent retains and records
      those refs, Apply verifies/materializes them atomically, and canonical
      reads are authorized, paged, and path-free.
- [ ] Staged blobs referenced by missing or unknown mutation outcomes cannot be
      swept; release occurs only after a durable terminal reconciled outcome or
      reconciled session abandonment.
- [ ] Staged-blob cleanup is an atomic `SweepUnretained` store operation that
      deletes only old blobs with no retention at commit, serializes concurrent
      retain versus sweep, and returns only opaque deleted IDs plus aggregate
      bytes. A retained pre-intent crash orphan requires explicit
      reconciliation and is never expired heuristically.
- [ ] `LLMExecutor` selects provider/model per declared purpose only after
      satisfying SDD-owned minimum capabilities and returns raw output plus
      neutral execution/usage metadata; SDD owns prompts, schemas, parsing,
      mechanical checks, findings, summaries, and write-gate verdicts.
- [ ] LLM capability mismatch, timeout, execution failure, or malformed output
      fails loud, and the actual executor/model fingerprint is auditable.
- [ ] Canonical mutation batches apply atomically against an expected revision;
      post-persist finalizer failures surface as applied failures, never as
      rollback, while indeterminate apply outcomes prohibit blind retries.
- [ ] Mutating transitions durably append intent before apply and typed outcome
      afterward; replay reconciles an orphaned intent by mutation ID and digest
      before any retry, and all engine advances use session-version CAS.
- [ ] The intent payload is a versioned, canonically serialized
      `PreparedTransition` containing the frozen engine delta, exact mutation
      batch, expected revision, resumable finalizer descriptors, and prepared
      result data. Recovery never regenerates nondeterministic or semantic
      content, and envelope migration cannot change the original mutation ID,
      canonical batch bytes, or batch digest used for reconciliation.
- [ ] Failed transitions and finalizers leave diagnostic outcome events, while
      concurrent connections cannot claim the same session transition.
- [ ] `list_projects`, explicit/implicit project selection, and immutable
      principal/project session binding behave as specified.
- [ ] A local-only graph uses the non-persisted, composition-scoped `"local"`
      project ID and directory basename for presentation without changing graph
      metadata; a later canonical `repo_id` becomes the ID for new sessions,
      while the old sentinel remains the unchanged stored replay alias for
      existing sessions. Listing and resuming those sessions returns the
      resolved canonical `ProjectRef` without rewriting stored events.
- [ ] Every project-scoped serve, session descriptor/resume, read, search,
      attachment, and graph-info structured result contains only the stable
      application project ID plus display name; project-independent results
      omit it and local rendered text remains compatible.
- [ ] Every project-scoped request resolves current principal and access; every
      mutation separately re-resolves write access at the engine write gate.
- [ ] Current per-request identity reaches project resolution, engine queries,
      and mutation authorization explicitly; operational context reaches ports,
      and no production path substitutes initialization identity,
      `context.Background()`, or a runtime authorized on an earlier request.
- [ ] Read-only users can start/resume sessions and use dialogue plus all read
      capabilities without gaining mutation access.
- [ ] Free reads work outside a session only with a resolved project and cannot
      switch the base project of an active session.
- [ ] Cross-repository reads resolve only authorized dependencies and preserve
      existing short/full ID semantics without leaking unauthorized projects.
- [ ] Session persistence is injected, stores immutable owner/project metadata
      plus replayable opaque events, and retains a JSONL local adapter.
- [ ] Each instance replays from its digest-verified pinned normalized
      procedure, never a current canonical lookup; event codec, procedure spec,
      and registry contract versions migrate explicitly in SDD or stop with a
      typed migration-required error before advancement.
- [ ] Stores remain opaque and perform no codec/procedure migration, while
      resumed effects still pass current access, authority revision,
      mechanical validation, and write-gate checks.
- [ ] Load, list, and resume perform supported replay migrations purely in
      memory. The next authorized advance appends exactly one CAS-protected
      migration-acknowledgement event before its transition, recording source
      and target versions without rewriting historical payloads or performing
      an external effect first.
- [ ] Local single-project compatibility is covered by contract tests and does
      not require callers to provide a project argument.
- [ ] The stateful Streamable HTTP identity spike passes before the public HTTP
      application is declared viable. MCP SDK types remain outside the root `sdd` package; local
      static bearer protection stays in local composition, while external
      authentication protocols remain outside the reusable application package.
- [ ] Public APIs contain no internal Go types, external credentials,
      application-specific identity types, persistence implementation types,
      source-system types, or required persistent filesystem paths.
- [ ] The release provides package documentation, an external composition
      example, and a version consumable by another Go module.

## Explicit non-goals of this OSS plan

- Implementing a particular external authentication or authorization system.
- Defining application-specific project catalogs, identities, roles, or policy.
- Implementing a particular source-control, database, object-storage, job, or
  vector-search backend.
- Making SDD itself a multi-tenant authorization authority.
- Exposing a second set of application-specific MCP tools.
- Stabilizing every internal model/engine type as a public SDK.

## Questions the OSS capture dialogue should pressure-test

1. Can the root `sdd` facade wrap the existing internal implementation without
   import cycles, while keeping all MCP SDK imports confined to `mcpapp`?
2. Does `SnapshotData` expose only canonical stored-document fields while
   carrying enough information for both filesystem and structured sources,
   with all validation and derived graph construction remaining inside SDD?
3. Are the proposed infrastructure ports below the correct semantic boundary,
   or does any port still require an external application to reproduce SDD
   behavior rather than supply storage, execution, or policy effects?
4. Which event-codec, procedure-spec, and registry-contract migrations must the
   first public version support, and what stable fingerprint identifies an
   already acknowledged migration chain so a retry cannot duplicate its audit
   event?
5. Can MCP SDK per-request token metadata carry all required identity and
   authorization inputs, or does `mcpapp` need a narrow private bridge/upstream
   hook? The Slice A test decides; initialization-time context is disallowed.
6. Do tests prove that, after a canonical `repo_id` appears, a legacy session
   retains the composition-scoped `"local"` stored binding without event
   rewrites while list, resume, authorization, and subsequent results use the
   resolved canonical `ProjectRef`?
7. Which existing local result snapshots must allow additive project fields,
   and which should remain byte-for-byte stable?
8. Which stable registry-operation and finalizer descriptor versions must the
   first implementation retain so an old prepared transition either resumes
   with exactly compatible behavior or fails with a typed recovery-required
   error?
9. What explicit reconciliation or operator repair workflow can prove that a
   retention left by a crash between `Retain` and intent append has no durable
   intent before releasing it, without weakening atomic sweep safety?

These questions are implementation-design choices inside the committed
architecture. They should sharpen the plan, not reopen the decision to share
one importable project-aware MCP application.
