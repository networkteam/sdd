# Spec — MCP server in sdd

Foundation work for the hosted SDD sandbox: expose the existing CLI surface as MCP tools so any agent runtime (Mastra first, others later) can read and write the graph through structured protocol rather than shell execution. Implementing the conceptual directive `20260525-204836-d-cpt-t3j`.

## Goals & Requirements

### Goals

- Run sdd as an MCP server, in-process within the existing Go binary, reusing the current handler/finder/presenter layer.
- Expose all read operations as MCP tools: `status`, `info`, `view`, `show`, `list`, `search`, `lint`.
- Expose all write operations as MCP tools: `new` (with attachments and pre-flight findings), `summarize`, `wip_start`, `wip_done`, `wip_list`.
- Expose sync as MCP tools (`sync_status`, `sync_rebase`) so the agent never invokes git directly.
- Two transports: stdio (local connections, Claude Code MCP integration, local dev) and streamable HTTP (remote hosted runtime).
- Local binding default for HTTP (127.0.0.1). Authentication deferred to a follow-up plan.
- Attachment handling via a two-step upload-token pattern.
- Structured JSON output for write tools (so agents can parse pre-flight findings).
- Reuse internal packages directly: no shelling to `sdd`, no parallel git wrapper.

### Non-goals (this plan)

- Authentication / authorization on HTTP transport — deferred.
- Skill content portability — separate plan per the directive (this plan only addresses the graph-access half of the portability gap).
- Updating SKILL.md to call MCP tools instead of `sdd ...` commands — separate skill-content work.
- Streaming intermediate progress notifications during long-running tool calls — block-and-return for v1.
- Per-call graph dir override / multi-tenant hosting — fixed at server start.
- Exposing `sdd_index` and `sdd_init` as MCP tools — maintenance operations, not dialogue operations.

## Architecture & Design Decisions

### 1. Library choice — `modelcontextprotocol/go-sdk`

**Decision:** Use `github.com/modelcontextprotocol/go-sdk` v1.6.1 (or latest 1.x).

**Reasoning:** Prior project experience confirmed the SDK supports both transports cleanly. The typed-args pattern (`mcp.AddTool[Args]` with `jsonschema` struct tags driving the input schema) matches Go idioms and gives us tool schemas for free from struct definitions. Grounded in upstream examples at `github.com/modelcontextprotocol/go-sdk/blob/v1.6.1/examples/server/hello/main.go` (stdio) and `examples/http/main.go` (streamable HTTP).

### 2. Package layout — `internal/mcpserver/`

**Decision:** New package `internal/mcpserver/` holds the server constructor and tool registration. `cmd/sdd/serve.go` is the thin CLI wrapper that builds the handler/finder dependencies and starts the transport.

**Reasoning:** Matches the existing pattern: `cmd/sdd/main.go` is thin; logic lives under `internal/`. The MCP server is an additional transport layered onto the same CQRS surface — it builds command/query structs and dispatches them to the existing handlers and finders. Grounded by CLAUDE.md "Library first: Domain logic lives in `internal/`."

### 3. CLI surface — `sdd serve` subcommand

**Decision:** New top-level subcommand:

```
sdd serve [--transport stdio|http]
          [--addr 127.0.0.1:8080]
          [--attach-ttl 30m]
```

Defaults:
- `--transport stdio`
- `--addr 127.0.0.1:8080` (HTTP only; bind to loopback)
- `--attach-ttl 30m`

**Reasoning:** Mirrors urfave/cli/v3 pattern in `cmd/sdd/main.go`. Local binding default per the user's transport-phasing answer. PoC shape — no extra guardrails on external binding; operators who need a different address can set `--addr` and accept that auth is not yet implemented.

### 4. Tool naming — `sdd_<verb>` snake_case

**Decision:** Tools are namespaced with the `sdd_` prefix and use snake_case:

| Tool | CLI parity | Dispatches to | Args struct |
|---|---|---|---|
| `sdd_status` | `sdd status` | query → finder | `StatusArgs` |
| `sdd_info` | `sdd info` | query → finder | `InfoArgs` |
| `sdd_view` | `sdd view --layout=...` | query → finder | `ViewArgs` |
| `sdd_show` | `sdd show <id> [...]` | query → finder | `ShowArgs` |
| `sdd_list` | `sdd list --type ... --layer ... --kind ... --topic ...` | query → finder | `ListArgs` |
| `sdd_search` | `sdd search --term ... --query ...` | query → finder | `SearchArgs` |
| `sdd_lint` | `sdd lint` | query → finder | `LintArgs` |
| `sdd_new` | `sdd new <type> <layer> ...` | command → handler | `NewArgs` |
| `sdd_summarize` | `sdd summarize` | command → handler | `SummarizeArgs` |
| `sdd_wip_start` | `sdd wip start ...` | command → handler | `WipStartArgs` |
| `sdd_wip_done` | `sdd wip done ...` | command → handler | `WipDoneArgs` |
| `sdd_wip_list` | `sdd wip list` | query → finder | `WipListArgs` |
| `sdd_sync_status` | (new — internalizes today's sync check) | query → finder | `SyncStatusArgs` |
| `sdd_sync_rebase` | (new — internalizes `git pull --rebase` after clean check) | command → handler | `SyncRebaseArgs` |
| `sdd_attach_upload` | (new — see decision 6) | command → handler | `AttachUploadArgs` |

**Reasoning:** Standard MCP convention is `<server>_<tool>` so tool names stay unambiguous when an agent has multiple MCP servers attached. Names map 1:1 to existing CLI verbs. The **Dispatches-to** column fixes each tool on the CQRS axis: side-effecting tools are commands routed to handlers; pure reads are queries routed to finders. The mcpserver layer itself never mutates state — it only translates protocol to command/query dispatch.

### 5. Tool result shape — presenter-text vs structured-JSON (orthogonal to CQRS)

**Decision:** Output shape is a separate axis from the command/query classification in decision 4. A tool can be a query that returns structured JSON (`sync_status`) or a command that returns structured JSON (`new`). The output-shape rule:

- **Presenter-text tools** (`status`, `info`, `view`, `show`, `list`, `search`, `lint`): return a single `mcp.TextContent` rendered by the existing presenters. Byte-for-byte CLI parity. The agent mirrors the output to the user without re-rendering.
- **Structured-JSON tools** (`new`, `summarize`, `wip_start`, `wip_done`, `sync_status`, `sync_rebase`, `attach_upload`): return structured JSON content. The agent acts on the fields programmatically (findings severity, blocked state, sync state, returned token/ID).
- **`sdd_new` shape:**
  ```json
  {
    "ok": true,
    "blocked": false,
    "id": "20260525-184931-s-cpt-z7l",
    "path": ".sdd/graph/2026/05/25-184931-s-cpt-z7l.md",
    "summary": "...",
    "findings": [
      {"severity": "low", "code": "entry-quality-opening", "message": "..."}
    ]
  }
  ```
  When `blocked` is true (any `high` finding present and `skip_preflight` not set), the entry is not written; `id`, `path`, `summary` are empty strings; the agent reads findings and decides whether to revise or retry with `skip_preflight: true`.
- **`sdd_sync_status`:** structured JSON `{ "state", "remote_ahead", "local_ahead", "conflict_paths", "reason" }`.
- **`sdd_sync_rebase`:** structured JSON `{ "ok", "state", "message" }`.
- **`sdd_attach_upload`:** structured JSON `{ "token", "expires_at", "size_bytes" }`.

**Reasoning:** Presenter-text tools keep parity with the CLI so an agent can mirror output to the user without re-rendering. Structured-JSON tools are acted on programmatically — the agent makes decisions based on the fields (findings severity, blocked state, sync state, returned ID/token). A duplicate text block would be derivable from the structured fields and adds maintenance burden without benefit. The axis is deliberately independent of CQRS: `sync_status` is a query (finder) but returns JSON because the agent decides what to do from the state; `show` is a query but returns text because the agent passes it through. The pre-flight findings shape mirrors what the CLI emits on stderr today (severity + code + message) so future readers see the same structure.

### 6. Attachment handling — two-step upload-token pattern (command → handler)

**Decision:**

`sdd_attach_upload` has a side effect (it stores attachment bytes under a token), so it is a **command dispatched to a handler** — not ad-hoc state in the mcpserver transport layer. `sdd_new` references uploaded tokens; the `NewEntry` handler resolves them at write time.

**Command + handler:**

```go
// internal/command/attach_upload.go
type AttachUploadCmd struct {
    Filename string
    Content  []byte
    OnUpload func(token string, expiresAt time.Time)
}

// internal/handlers/handler_attach_upload.go
func (h *Handler) AttachUpload(ctx context.Context, cmd command.AttachUploadCmd) error {
    token, expiresAt := h.attachments.Put(cmd.Filename, cmd.Content)
    if cmd.OnUpload != nil {
        cmd.OnUpload(token, expiresAt)
    }
    return nil
}
```

**Store interface (injected handler dependency, same pattern as `Committer`):**

```go
// internal/handlers/handler.go
type AttachmentStore interface {
    Put(filename string, data []byte) (token string, expiresAt time.Time)
    Take(token string) (filename string, data []byte, ok bool)
}
```

The store is **transport-agnostic and pluggable** — an interface so the backing can change later. The default implementation is filesystem-backed in `.sdd/tmp/`, the same directory the existing `saveStdinAttachment` retry mechanism already uses (`internal/handlers/stdin_save.go`), living in a new `internal/attachstore/` package. Both the CLI and `sdd serve` construct it and inject it into the Handler via `Options`. Tokens map to `.sdd/tmp/<token>` files; a TTL sweep (default 30m via `--attach-ttl`, mtime-based) prunes stale entries. A future in-memory or remote backing drops in behind the same interface.

Because the existing `saveStdinAttachment` does the same `.sdd/tmp/` staging, the store generalizes it: the rejected-stdin retry path and the MCP upload-token path become one mechanism rather than two.

**Extensions the abstraction enables:**

- *(a) Unify the CLI stdin-save retry path onto the store* — removes the duplicate `.sdd/tmp/` staging logic in `saveStdinAttachment`, keeping current CLI UX. **In scope for this plan** — clean design forbids two staging mechanisms.
- *(b) A CLI upload-token surface* — `sdd attach upload` returning a token plus `sdd new --attach-token <token>`. Because `AttachUploadCmd` is a transport-agnostic command, the CLI surface is a thin `*cli.Command` over the same handler. Eases multi-attachment + long-description ergonomics (today: one `-` stdin slot, temp-file juggling). **Deferred to a separate follow-up plan**, which must also update `references/cli-reference.md` and the `/sdd` SKILL.md so agents learn the new attachment flow.

**Token consumption lives in the `NewEntry` handler, on the write path only.** `command.Attachment` gains a `Token` field. When set, the handler resolves it via `h.attachments.Take(token)` — but only *after* pre-flight passes, immediately before writing the entry. A blocked pre-flight returns without taking the token, so the agent retries with the same token. The dispose-on-consume side effect stays in the handler; pre-flight only needs attachment-presence (token-ref count), not content.

**MCP tool surface:**

```go
type AttachUploadArgs struct {
    Filename string `json:"filename" jsonschema:"target filename inside the entry's attachment directory"`
    Content  string `json:"content" jsonschema:"attachment content as text"`
}

type AttachmentRef struct {
    Token  string `json:"token" jsonschema:"token from sdd_attach_upload"`
    Target string `json:"target,omitempty" jsonschema:"override target filename (defaults to filename from upload)"`
}
```

The `sdd_attach_upload` tool builds an `AttachUploadCmd` and dispatches it to the handler, capturing the token via `OnUpload`. The `sdd_new` tool maps `AttachmentRef`s into `command.Attachment{Token, Target}` entries — it never touches the store directly.

**Reasoning:** `attach_upload` mutates state → command → handler, per CQRS. The mcpserver layer stays pure dispatch and never mutates anything itself. Putting token consumption in the `NewEntry` handler (not the MCP tool) keeps the dispose-on-consume side effect in the domain layer and preserves the retry-after-block benefit. The CLI today saves stdin attachments to `.sdd/tmp/` and outputs the filename for retry; the token pattern is the MCP equivalent without filesystem leakage. Binary attachment support deferred — agents don't reliably encode binary on the fly, and the vast majority of SDD attachments are markdown / plain text.

### 7. Pre-flight integration — same finder, same blocking semantics

**Decision:** `sdd_new` calls the existing pre-flight finder via the existing `handlers.NewEntry` handler. Blocking semantics match CLI: `high` severity findings block creation unless `skip_preflight: true` is set. The findings list returned in the structured result preserves the existing severity / code / message shape from `internal/llm/preflight_templates/`.

**Reasoning:** Same data the CLI currently emits on stderr; same handler invoked; no logic duplication. Grounded in existing `internal/handlers/handler_new_entry.go` and `internal/finders/preflight.go`.

### 8. Sync as MCP tools — replaces direct git invocation by the agent

**Decision:** Two tools:

- **`sdd_sync_status`** — runs the existing `query.SyncStatusQuery` via `finders.Finder.SyncStatus`. Returns the `model.SyncStatus` struct as JSON (`{ state, remote_ahead, local_ahead, conflict_paths, reason }`).
- **`sdd_sync_rebase`** — dispatches `SyncRebaseCmd` to the handler. The handler checks sync state and only rebases on a safe state. The outcome flows back via the command's `OnResult` callback; the tool serializes it to JSON `{ ok, state, conflict_paths, message }`. Dirty tree → `{ ok: false, state: "dirty" }`; predicted conflict → `{ ok: false, state: "conflict-predicted", conflict_paths: [...] }`; clean → `{ ok: true, state: "rebased" }`.

A new handler method (see decision 14 for the dependency-injection shape):

```go
// internal/handlers/handler_sync_rebase.go (new file)
func (h *Handler) SyncRebase(ctx context.Context, cmd command.SyncRebaseCmd) error

// internal/command/sync_rebase.go
type SyncRebaseResult struct {
    Ok            bool
    State         model.SyncState
    ConflictPaths []string
    Message       string
}
type SyncRebaseCmd struct {
    OnResult func(result SyncRebaseResult)
}
```

The handler reads sync status via `Reader.SyncStatus` (extended Reader interface — see decision 14). On an unsafe state it reports the refusal through `cmd.OnResult` and never calls `Rebase`. Only on `fast-forward` / `clean-rebase` does it invoke `Rebaser.Rebase(ctx)`, then reports the outcome via `cmd.OnResult`. The handler returns `error` only for system failures (git unavailable, repo corrupted) — refusals are normal outcomes, not errors.

**Reasoning:** Per the conceptual directive `d-cpt-t3j` — the sync-check flow becomes an MCP tool so the agent doesn't need direct git access. Read side already exists as a finder/query; only the rebase action needs a new handler.

### 9. Long-running operations — block by default

**Decision:** `sdd_lint`, `sdd_search`, `sdd_new` (pre-flight LLM), and `sdd_summarize` block the tool call until complete. No background / status-poll pattern.

**Reasoning:** Aligns with CLI behavior. The SDK supports intermediate notifications but adding them requires careful design; v1 favors simplicity. `sdd_index` is deliberately excluded from the tool set per Non-goals — it's a maintenance operation rarely needed at dialogue time, and the search index lazy-fills on `sdd_search` calls.

### 10. Single graph per server invocation

**Decision:** The graph directory is fixed at server start time, resolved from `resolveSDDDir()` walking up from the CWD. No per-call graph dir override.

**Reasoning:** Existing convention: one `.sdd/` per repo. Per-call override would require deeper refactor (passing graph dir through every command/query) for ambiguous v1 benefit. Hosted multi-tenancy is future work and will likely use one server-per-tenant rather than one server multiplexing graphs.

### 11. Graph state managed by `GraphFileSyncer`

**Decision:** The cached graph lives behind a `GraphFileSyncer` component that owns initial load, file watching, and reload coordination. The Server uses it through a narrow interface:

```go
syncer.Current() *model.Graph   // lock-free read of latest snapshot
syncer.MarkDirty()              // request a reload (coalescing)
```

**Properties:**

- **Single source of truth: the files on disk.** The cached graph is a snapshot; cache invalidation is triggered by "files changed," not "this code path wrote them." Own writes, terminal `git pull`, parallel sessions, `sdd_sync_rebase` — all go through the same reload path.
- **At most one reload in flight.** A new `MarkDirty()` while a reload is running cancels the in-flight load and starts a fresh one; the canceled result is discarded. Bursts (e.g. `git pull` writing many files) collapse to one effective reload of the freshest state.
- **Reads are lock-free, and the reference swap is the only synchronized point.** Readers call `syncer.Current()` which loads an `atomic.Pointer[model.Graph]`. The swap-in after a reload is the only write to that pointer; the atomic load/store makes concurrent access safe without a mutex (a plain mutex around the field would also work — atomic is the lighter choice). **Invariant: a `*model.Graph` is immutable once constructed** — no tool handler mutates the graph in place, so an in-flight reader holding a snapshot is never affected by a concurrent reload swapping in a newer one.
- **fsnotify watcher** on the graph directory triggers `MarkDirty()` on relevant events. Watcher quirks (event drops on certain filesystems, partial-write events) are tolerated: `MarkDirty()` is idempotent, the next event recovers, and write tools also call `MarkDirty()` explicitly for immediacy.
- **Cancellation is propagated to disk I/O.** The Reader's `LoadGraph` signature takes `context.Context` (project rule: all I/O bearing paths accept ctx); the finder implementation checks `ctx.Done()` at iteration boundaries inside its directory walk.
- **No write lock on tool dispatch — committed, not hedged.** Concurrent `sdd_new` calls operate on the same snapshot, run pre-flight in parallel, and serialize naturally on git's index lock during commit. This matches the existing CLI, where each invocation is a separate process with no cross-process write lock. The concurrent-write race is bounded — one call's pre-flight may not observe another's in-flight entry — and accepted for this plan. No speculative mutex.

**Reasoning:** Reloading the entire graph on every tool call wastes work and contradicts the in-memory-graph benefit. Wrapping reload behind a syncer encapsulates the channel/goroutine machinery so the Server stays a thin tool-registration layer. The file-watcher model handles **all** mutation sources uniformly — including the case the previous RWMutex design missed: external changes from a terminal `git pull` or a parallel session. The trade-off is a new dependency (`fsnotify`) and a new component to test.

### 12. Process lifecycle — context cancellation handles shutdown

**Decision:** `sdd serve` listens for SIGINT/SIGTERM, cancels the root context, which terminates the transport. For HTTP, additionally shuts down `http.Server` with a 5s grace period. For stdio, transport exits when stdin closes.

**Reasoning:** Standard Go pattern; the SDK's server `Run(ctx, transport)` respects context cancellation.

### 13. Logging — slog via context

**Decision:** `sdd serve` configures slog the same way other CLI commands do (verbose / extra-verbose flags inherited from the root command). The MCP server propagates a context with `slogutils.FromContext(ctx)` for each tool call so handlers log uniformly.

**Reasoning:** Matches CLAUDE.md logging convention: "Use `log/slog`; retrieve the logger via `slogutils.FromContext(ctx)`."

### 14. `Rebaser` interface — separate single-method interface in `internal/handlers/`

**Decision:** New interface `Rebaser` in `internal/handlers/handler.go`:

```go
// Rebaser runs `git pull --rebase` on the current branch. Callers do
// not need to perform safety checks: the handler that uses Rebaser
// reads sync status first and refuses unsafe rebases before invoking.
type Rebaser interface {
    Rebase(ctx context.Context) error
}
```

The Handler struct gains a `rebaser Rebaser` field; the `Options` struct gains `Rebaser Rebaser`. Production implementation lives in `cmd/sdd/sync.go` as `gitRebaserImpl{}` alongside the existing `gitSyncerImpl{}`, shelling out to `git pull --rebase` and returning errors verbatim.

The Reader interface also gains a `SyncStatus(ctx, q) (model.SyncStatus, error)` method so the handler can check safety without reaching into the finder layer directly:

```go
type Reader interface {
    LoadGraph(ctx context.Context, dir string) (*model.Graph, error)
    LoadWIPMarkers(ctx context.Context, dir string) ([]*model.WIPMarker, error)
    Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error)
    SkillStatus(ctx context.Context, q query.SkillStatusQuery) (*query.SkillStatusResult, error)
    SyncStatus(ctx context.Context, q query.SyncStatusQuery) (model.SyncStatus, error) // new
}
```

`*finders.Finder` already implements `SyncStatus`; the interface extension just exposes it through the handler-side abstraction.

**Why a separate `Rebaser`, not an extension of `Brancher`:**

- `handlers.Brancher` operates on named branches (`Checkout`, `BranchMerged`, `DeleteBranch`). Rebase operates on the current branch by convention. Different mental model — mixing dilutes Brancher's purpose.
- `finders.GitSyncer` is read-side (under `internal/finders/`). Rebase is a write action with side effects and must live where handlers are.
- The existing pattern is small capability-focused interfaces (`Committer` = commit; `Brancher` = branch ops; `Mover` = git mv). `Rebaser` = rebase fits the same shape.

**Pre-flight inside the handler, not the caller.** The handler invokes `Reader.SyncStatus` itself before calling `Rebaser.Rebase`. If status is anything other than `fast-forward` or `clean-rebase`, the handler reports the refusal through `SyncRebaseCmd.OnResult` and `Rebase` is never called — it returns `error` only for system failures. The MCP tool can still surface state via `sdd_sync_status` first (for the agent's dialogue), but the handler doesn't trust the caller — defense in depth, costs one extra `MergeTreePredict` per rebase attempt, acceptable for a rare operation.

**Reasoning:** Matches the existing single-purpose-interface decomposition in `internal/handlers/`. Putting the safety check in the handler keeps the "is rebase safe?" decision inside the write-side domain layer — per CQRS, side-effecting domain logic lives in handlers, never in callers. The MCP tool dispatch is a thin transport; if it carried the safety check, the same logic would have to be duplicated for any future caller (CLI, hosted runtime, automation).

## Implementation Changes

### New files

- `internal/mcpserver/server.go` — `Server` struct, `New(ctx, opts) (*Server, error)`, `RunStdio(ctx) error`, `RunHTTP(ctx, addr) error`. Owns the syncer, attach store, and registered tools.
- `internal/mcpserver/graph_syncer.go` — `GraphFileSyncer` with `Start`, `Current`, `MarkDirty`. Wraps fsnotify watcher + reloader goroutine + atomic graph pointer.
- `internal/mcpserver/tools_read.go` — registers status, info, view, show, list, search, lint tools. Each tool handler calls `s.syncer.Current()` for the graph snapshot.
- `internal/mcpserver/tools_write.go` — registers new, summarize tools. Each write tool calls `s.syncer.MarkDirty()` after the handler returns successfully.
- `internal/mcpserver/tools_wip.go` — registers wip_start, wip_done, wip_list tools. WIP starts/dones call `MarkDirty()`.
- `internal/mcpserver/tools_sync.go` — registers sync_status (query → finder), sync_rebase (command → handler) tools. `sync_rebase` calls `MarkDirty()` after a successful rebase.
- `internal/mcpserver/tools_attach.go` — registers attach_upload tool; dispatches `command.AttachUploadCmd` to the handler. Owns no state.
- `internal/attachstore/filesystem.go` — filesystem-backed `handlers.AttachmentStore` impl staging in `.sdd/tmp/`, pluggable behind the interface. Constructed in `cmd/sdd/` and injected into the Handler on both the CLI and serve paths.
- `internal/mcpserver/args.go` — typed args structs (`StatusArgs`, `ShowArgs`, `NewArgs`, `AttachUploadArgs`, etc.) with jsonschema tags.
- `internal/mcpserver/result.go` — helpers for serializing structured result types as JSON into `*mcp.CallToolResult`. Shared types like `Finding` and structured-tool result envelopes live here.
- `internal/mcpserver/server_test.go` — table-driven tests covering each tool's happy and unhappy paths against an in-memory fixture graph.
- `internal/mcpserver/graph_syncer_test.go` — unit tests for the syncer (initial load, cancellation under burst, watcher event handling, reload-failure tolerance).
- `cmd/sdd/serve.go` — `serveCmd() *cli.Command` defining the subcommand, flag parsing, dependency wiring (incl. constructing the `AttachmentStore` and injecting it into the Handler), transport startup.
- `internal/handlers/handler_sync_rebase.go` — new handler method for `git pull --rebase`; reads `SyncStatus` and refuses unsafe states.
- `internal/handlers/handler_attach_upload.go` — new `Handler.AttachUpload` command method.
- `internal/command/sync_rebase.go` — `SyncRebaseCmd` with `OnResult func(SyncRebaseResult)`; `SyncRebaseResult { Ok bool, State model.SyncState, ConflictPaths []string, Message string }`.
- `internal/command/attach_upload.go` — `AttachUploadCmd` struct with `OnUpload` callback.
- `docs/mcp-server.md` — reference documentation: tools and input schemas, transport options, configuration, attachment upload-token flow, structured result shape for write tools.

### Modified files

- `cmd/sdd/main.go` — register `serveCmd()` in the root command's `Commands` slice. Update existing callers of `loadGraph(cmd)` to pass `ctx` through to the new `Reader.LoadGraph(ctx, dir)` signature.
- `cmd/sdd/wip.go` (and any other caller of `LoadWIPMarkers`) — pass `ctx` through to the new ctx-bearing signature.
- `go.mod` / `go.sum` — add `github.com/modelcontextprotocol/go-sdk` v1.6.1 (or latest 1.x) and `github.com/fsnotify/fsnotify` (latest stable).
- `internal/handlers/handler.go` — **breaking signature change to `Reader` interface**: `LoadGraph(ctx context.Context, dir string)` and `LoadWIPMarkers(ctx context.Context, dir string)`. Project rule: I/O bearing paths take `context.Context`. Single shape — no contextless variant. Also extend `Reader` with `SyncStatus(ctx, q) (model.SyncStatus, error)` (already implemented by `*finders.Finder`) so the rebase handler can check safety without reaching into the finder layer. Add new `Rebaser` interface (decision 14) and `AttachmentStore` interface (decision 6); add `rebaser Rebaser` and `attachments AttachmentStore` fields on `Handler` + matching `Options` fields.
- `internal/command/new_entry.go` — `Attachment` struct gains a `Token string` field. When set, the `NewEntry` handler resolves it via `AttachmentStore.Take` on the write path (after pre-flight passes).
- `internal/handlers/handler_new_entry.go` — resolve token-bearing attachments via the store immediately before writing the entry; pre-flight only inspects attachment-presence (token-ref count), not content.
- `cmd/sdd/sync.go` — new `gitRebaserImpl{}` type implementing `handlers.Rebaser` next to the existing `gitSyncerImpl{}`. Shells `git pull --rebase` and returns errors verbatim.
- `internal/finders/graph.go` and `internal/finders/wip.go` — implement the ctx-bearing methods; check `ctx.Done()` at iteration boundaries inside directory walk / parse loops so the syncer's cancellation propagates.

### Structural code — server skeleton

```go
// internal/mcpserver/server.go

package mcpserver

import (
    "context"
    "log/slog"
    "net/http"
    "time"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    "github.com/networkteam/sdd/internal/finders"
    "github.com/networkteam/sdd/internal/handlers"
)

type Options struct {
    Handler   *handlers.Handler
    Finder    *finders.Finder
    GraphDir  string
    AttachTTL time.Duration
    Version   string
    Logger    *slog.Logger
}

type Server struct {
    mcp    *mcp.Server
    syncer *GraphFileSyncer
}

func New(ctx context.Context, opts Options) (*Server, error) {
    s := &Server{
        syncer: NewGraphFileSyncer(opts.GraphDir, opts.Finder, opts.Logger),
    }
    if err := s.syncer.Start(ctx); err != nil {
        return nil, err
    }
    s.mcp = mcp.NewServer(&mcp.Implementation{
        Name:    "sdd",
        Version: opts.Version,
    }, nil)

    // The attachment store is constructed in cmd/sdd/serve.go and injected
    // into opts.Handler; the mcpserver no longer owns it. Tool registration
    // is pure dispatch: queries → finder, commands → handler.
    registerReadTools(s, opts.Finder)
    registerWriteTools(s, opts.Handler, opts.Finder)
    registerWIPTools(s, opts.Handler, opts.Finder)
    registerSyncTools(s, opts.Handler, opts.Finder)
    registerAttachTool(s, opts.Handler)

    return s, nil
}

// RunStdio blocks until the stdio transport closes or ctx is cancelled.
func (s *Server) RunStdio(ctx context.Context) error {
    return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// RunHTTP serves over streamable HTTP at addr. Returns when ctx is cancelled.
func (s *Server) RunHTTP(ctx context.Context, addr string) error {
    handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
        return s.mcp
    }, nil)
    httpServer := &http.Server{Addr: addr, Handler: handler}
    // Shutdown plumbing: goroutine watching ctx, http.Server.Shutdown with 5s grace.
    // (Implementation detail, not specified here.)
    return httpServer.ListenAndServe()
}
```

### Structural code — `GraphFileSyncer`

```go
// internal/mcpserver/graph_syncer.go

package mcpserver

import (
    "context"
    "errors"
    "log/slog"
    "sync/atomic"

    "github.com/networkteam/sdd/internal/handlers"
    "github.com/networkteam/sdd/internal/model"
)

// GraphFileSyncer encapsulates graph state: initial load, fsnotify-driven
// invalidation, and a "at-most-one-cancelable-load-in-flight" reload loop.
// Readers get lock-free access via Current(); any code path that mutates
// the graph (own writes, sync rebase, external file changes) drives one
// uniform reload path via MarkDirty().
type GraphFileSyncer struct {
    graphDir string
    reader   handlers.Reader
    logger   *slog.Logger

    current   atomic.Pointer[model.Graph]
    requestCh chan struct{}
}

func NewGraphFileSyncer(graphDir string, reader handlers.Reader, logger *slog.Logger) *GraphFileSyncer {
    return &GraphFileSyncer{
        graphDir:  graphDir,
        reader:    reader,
        logger:    logger,
        requestCh: make(chan struct{}, 1),
    }
}

// Start performs the initial load and launches the reloader and watcher
// goroutines. Returns once the initial load completes. Goroutines stop
// when ctx is canceled.
func (s *GraphFileSyncer) Start(ctx context.Context) error {
    g, err := s.reader.LoadGraph(ctx, s.graphDir)
    if err != nil {
        return err
    }
    s.current.Store(g)
    go s.runReloader(ctx)
    go s.runWatcher(ctx)
    return nil
}

// Current returns the latest cached graph snapshot. Lock-free; safe to
// call concurrently from any tool handler. The returned *model.Graph is
// immutable after construction; in-flight readers keep their pointer
// safely while a newer snapshot is swapped in.
func (s *GraphFileSyncer) Current() *model.Graph {
    return s.current.Load()
}

// MarkDirty requests a reload. If a load is in flight it is canceled and
// a fresh one starts (latest signal wins). Bursts collapse to one
// pending request.
func (s *GraphFileSyncer) MarkDirty() {
    select {
    case s.requestCh <- struct{}{}:
    default:
        // already pending
    }
}

func (s *GraphFileSyncer) runReloader(ctx context.Context) {
    var cancelInFlight context.CancelFunc
    done := make(chan loadResult, 1)
    loading := false

    for {
        select {
        case <-ctx.Done():
            if cancelInFlight != nil {
                cancelInFlight()
            }
            return

        case <-s.requestCh:
            if loading {
                cancelInFlight()
            }
            loadCtx, cancel := context.WithCancel(ctx)
            cancelInFlight = cancel
            loading = true
            go func() {
                g, err := s.reader.LoadGraph(loadCtx, s.graphDir)
                done <- loadResult{graph: g, err: err}
            }()

        case res := <-done:
            loading = false
            cancelInFlight = nil
            if errors.Is(res.err, context.Canceled) {
                continue
            }
            if res.err != nil {
                s.logger.Warn("graph reload failed", "err", res.err)
                continue
            }
            s.current.Store(res.graph)
        }
    }
}

// runWatcher emits MarkDirty on relevant *.md events under graphDir.
// (fsnotify setup, recursive watch, filter elided — implementation
// detail not load-bearing for the spec.)
func (s *GraphFileSyncer) runWatcher(ctx context.Context) {
    // ...
}

type loadResult struct {
    graph *model.Graph
    err   error
}
```

### Structural code — tool registration pattern

```go
// internal/mcpserver/tools_read.go (excerpt)

type ShowArgs struct {
    IDs        []string `json:"ids" jsonschema:"entry IDs to show (full or short form)"`
    MaxDepth   int      `json:"max_depth,omitempty" jsonschema:"upstream/downstream expansion depth (default 4)"`
    Downstream bool     `json:"downstream,omitempty" jsonschema:"include downstream entries"`
}

func registerReadTools(s *Server, f *finders.Finder) {
    mcp.AddTool(s.mcp, &mcp.Tool{
        Name:        "sdd_show",
        Description: "Show one or more entries with their upstream summary chain and (optionally) downstream entries.",
    }, func(ctx context.Context, req *mcp.CallToolRequest, args ShowArgs) (*mcp.CallToolResult, any, error) {
        g := s.syncer.Current()
        result, err := f.Show(query.ShowQuery{
            Graph:      g,
            IDs:        args.IDs,
            MaxDepth:   pickDepth(args.MaxDepth),
            Downstream: args.Downstream,
        })
        if err != nil {
            return nil, nil, err
        }
        var buf bytes.Buffer
        presenters.RenderShow(&buf, result)
        return &mcp.CallToolResult{
            Content: []mcp.Content{&mcp.TextContent{Text: buf.String()}},
        }, nil, nil
    })

    // ... other read tools follow the same pattern.
}
```

### Structural code — `sdd serve` subcommand

```go
// cmd/sdd/serve.go (new file)

func serveCmd() *cli.Command {
    return &cli.Command{
        Name:  "serve",
        Usage: "Run an MCP server exposing the sdd graph as tools",
        Flags: []cli.Flag{
            &cli.StringFlag{Name: "transport", Value: "stdio", Usage: "transport: stdio or http"},
            &cli.StringFlag{Name: "addr", Value: "127.0.0.1:8080", Usage: "HTTP listen address (only for transport=http)"},
            &cli.DurationFlag{Name: "attach-ttl", Value: 30 * time.Minute, Usage: "attachment upload token TTL"},
        },
        Action: func(ctx context.Context, cmd *cli.Command) error {
            // Build handler + finder + git syncer using existing helpers (loadConfig, newRunner, gitCommitterFunc, gitBrancher, gitMover).
            // Construct *mcpserver.Server (initial graph load happens here).
            // Dispatch to RunStdio or RunHTTP based on --transport.
            // ...
        },
    }
}
```

## Test Cases

Tests live in `internal/mcpserver/server_test.go` and `cmd/sdd/serve_test.go`. Read fixtures from existing `internal/finders/testdata/`.

### Layer: internal/mcpserver

| Test | Setup | Action | Expected |
|---|---|---|---|
| `TestStatusToolMatchesCLI` | fixture graph, in-memory server | call `sdd_status` | `TextContent` byte-equals output of `presenters.RenderStatus` on the same graph |
| `TestShowToolMultipleIDs` | fixture with two chained entries | call `sdd_show` with `ids: [id1, id2]` | both groups rendered, `---` separator present |
| `TestListWithFilters` | fixture graph | call `sdd_list` with `type: "d", layer: "tac", kind: "plan"` | text content lists only matching entries |
| `TestSearchTextMode` | fixture graph | call `sdd_search` with `term: ["mcp"]` | matching entries with citations |
| `TestViewMacro` | fixture graph | call `sdd_view` with `layout: "top(5)"` | top-5 section rendered |
| `TestNewToolHappyPath` | fixture, mock pre-flight returning no `high` findings | call `sdd_new` with valid args | structured result `ok=true blocked=false id≠"" findings=[]`, entry file present on disk |
| `TestNewToolPreflightBlocks` | fixture, mock pre-flight returns one `high` finding | call `sdd_new` without `skip_preflight` | result `ok=false blocked=true findings=[{severity:"high",...}]`, no entry file written |
| `TestNewToolSkipPreflightOverride` | same as above | call `sdd_new` with `skip_preflight: true` | entry written; result `ok=true blocked=false findings=[{severity:"high",...}]` (findings still reported, not enforced) |
| `TestNewToolWithAttachment` | empty server | `sdd_attach_upload(filename, content)`, then `sdd_new(attachments: [{token}])` | token returned with `expires_at`; entry written with attachment file at correct path; token removed from store |
| `TestAttachTokenExpiry` | server with `attach_ttl=10ms` | upload, sleep 50ms, attempt consume on `sdd_new` | `sdd_new` returns structured error: `attachment token expired or unknown` |
| `TestAttachTokenReusedAfterPreflightBlock` | fixture, pre-flight blocks first call | upload, `sdd_new` blocks, revise args, `sdd_new` succeeds with same token | first call leaves token in store; second call succeeds and removes it |
| `TestSyncStatusFastForward` | mock GitSyncer reporting fast-forward, N=3 | call `sdd_sync_status` | structured result `state="fast-forward" remote_ahead=3` |
| `TestSyncRebaseDirtyWorkingTree` | mock GitSyncer reporting dirty tree | call `sdd_sync_rebase` | structured result `ok=false state="dirty"`, no rebase attempted |
| `TestSyncRebaseConflictPredicted` | mock predicting conflict in `path/x.md` | call `sdd_sync_rebase` | result `ok=false state="conflict-predicted" conflict_paths=["path/x.md"]` |
| `TestSyncRebaseClean` | mock predicting clean rebase | call `sdd_sync_rebase` | rebase invoked; result `ok=true state="rebased"` |
| `TestWipStartAndDone` | fixture graph | `sdd_wip_start(...)`, then `sdd_wip_done(...)` | marker created, then removed; results structured |
| `TestConcurrentReadsAreServedInParallel` | fixture graph; in-memory server | fire N concurrent `sdd_status` calls | all complete in parallel (no serialization; reads are lock-free) |
| `TestWriteTriggersReloadOnSuccess` | fixture graph | call `sdd_new`, then immediately `sdd_status` | status output includes the new entry (MarkDirty + reload completes before status reads Current) |
| `TestExternalFileWriteTriggersReload` | fixture graph; server running | write a new entry file directly to the graph dir from the test (simulating a parallel session) | within debounce + reload window, `sdd_status` reflects the new entry |
| `TestSimulatedGitPullTriggersReload` | fixture graph; server running | rename in many `*.md` files at once (simulating a git pull burst) | a single reload runs; final state observed reflects all files |

### Layer: internal/mcpserver/GraphFileSyncer (unit)

| Test | Setup | Action | Expected |
|---|---|---|---|
| `TestSyncerInitialLoadFailurePropagates` | reader stub returns error on `LoadGraph` | call `Start(ctx)` | error returned; syncer state empty; no goroutines leaked |
| `TestSyncerHappyPath` | reader stub returns a graph | `Start`, then `Current()` | returns the loaded graph |
| `TestSyncerCancelsInFlightLoadOnNewRequest` | reader stub blocks on a channel inside `LoadGraph` until released | `Start` succeeds with fast load; then 5 rapid `MarkDirty()` calls while load blocks | 4 of the in-flight loads observe `ctx.Err() == context.Canceled`; final load completes and is the snapshot stored |
| `TestSyncerToleratesReloadFailure` | reader stub returns nil + error on second `LoadGraph` | `Start`, then `MarkDirty()`, then `Current()` | `Current()` still returns the initial snapshot; warn log emitted |
| `TestSyncerStopsOnContextCancel` | running syncer | cancel root ctx | both goroutines exit within 100ms; no leaks |

### Layer: cmd/sdd (integration)

| Test | Setup | Action | Expected |
|---|---|---|---|
| `TestServeStdioE2E` | `sdd serve --transport stdio` against fixture graph; MCP client over stdio | call `sdd_status` | response text matches CLI `sdd status` byte-for-byte |
| `TestServeHTTPE2EHappyPath` | `sdd serve --transport http --addr 127.0.0.1:0` (auto-port) | MCP client via streamable HTTP, call `sdd_show <id>` | response matches CLI `sdd show <id>` |
| `TestServeShutdownOnSIGINT` | running server | send SIGINT | server shuts down within 5s, returns zero exit code |

## Open items to resolve before implementation

None remaining.
