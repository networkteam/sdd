# SDD graph-access architecture review

Read-only review of graph loading/holding across CLI, MCP server, engine, and handlers (Claude, Opus sub-agent, 2026-07-07). All line numbers verified against the working tree at review time.

## 1. Complete inventory of graph load / hold / read sites

`finders.Finder.LoadGraph(dir)` (`internal/finders/graph.go:21`) is the **single IO loader** — it walks the dir, parses entries, joins embedded base procedures, builds `model.NewGraph`. Every other finder read (`Search`, `View`, `Show`, `Preflight`, `ReadAttachment`) takes an already-built `*model.Graph` in its query struct and does no IO. So the whole system has exactly one place that reads graph bytes off disk; the question is only *who calls it, how often, and who holds the result*.

| # | Site | Layer | Lifetime | Notes |
|---|------|-------|----------|-------|
| 1 | `cmd/sdd/main.go:301` `loadGraph` helper → `Finder.LoadGraph` | CLI | per-command (process) | called by `show` (`main.go:433`), `lint` (`main.go:859`), `search` (`cmd/sdd/search.go:307`) |
| 2 | `cmd/sdd/view.go:261` | CLI | per-command | direct `Finder.LoadGraph` |
| 3 | `cmd/sdd/search.go:192` | CLI | per-command | `index` pre-pass via injected `Reader.LoadGraph` |
| 4 | `internal/handlers/handler_new_entry.go:74` | handler | per-write | loads to validate + resolve refs, then writes |
| 5 | `internal/handlers/handler_rewrite.go:32` | handler | per-write | |
| 6 | `internal/handlers/handler_lint_fix.go:16` | handler | per-write | |
| 7 | `internal/handlers/handler_summarize.go:31` | handler | per-write | |
| 8 | `internal/handlers/handler_wip_start.go:22` | handler | per-write | |
| 9 | `internal/handlers/handler_index.go:133` | handler | per-write | |
| 10 | `internal/mcpserver/tools.go:960` (search), `:1026` (view), `:1041` (show), `:1066` (read_attachment) | MCP free reads | **per-call**, discarded after render | each free read reloads a *fresh, separate* graph via `s.finder.LoadGraph(s.graphDir)` |
| 11 | `internal/mcpserver/tools.go:405` `newShellSession` | MCP engine shell | **per-session, held, mutable** | `engine.New(registry, graph)` → stored as `Engine.Graph` |
| 12 | `internal/mcpserver/tools.go:799` `resumeSession` | MCP engine shell | **per-session, held, mutable** | fresh load at resume; replay applies logged state only (no re-read) |
| 13 | `internal/mcpserver/registry.go:459` `refreshGraph` → reassigns `ss.engine.Graph` | MCP engine shell | **per-write & per-advance** | called after `newEntry` (`registry.go:429`), after `replaceSummary` (`:246`), and at the *start* of `startProcedure` (`tools.go:479`) and `next` (`tools.go:541`) |

In-memory holders of the loaded graph (no IO of their own):
- `engine.Engine.Graph` field — `internal/engine/session.go:132-138` (the held snapshot; mutated by #13).
- `Context.Graph` — `internal/engine/registry.go:35-45`; built fresh per call by `funcContext` as `&Context{… Graph: s.engine.Graph …}` (`internal/engine/instance.go:161-163`). This is a *read of the live `Engine.Graph` pointer*, so it reflects the latest `refreshGraph`.
- `shellSession.framingGraph` — `internal/mcpserver/session.go:42`; pointer-identity cache key for the rendered framing (`renderFraming`, `tools.go:1215-1236`), not a separate load.

Not a holder: catch-up narrative is just `view` layouts (finder-rendered, no long-lived graph); pre-flight takes `Graph` in `PreflightQuery` (no load of its own); summarize/lint load inside their handlers (#5-#9).

**So the "three places" are:** (a) CLI/handler per-command loads, (b) MCP free-read per-call loads, (c) the engine session's held-and-refreshed snapshot. During one live MCP session with a free `show` interleaved, **two-to-three full-disk graph builds coexist** (the session snapshot + each free read's throwaway).

## 2. Post-write freshness inside an engine session — verdict: **NOT a defect (correct, but fragile)**

Concrete trace of a capture write through `next` → `Answer`:

1. `next` (`tools.go:529`) refreshes the graph at entry (`tools.go:541` → `refreshGraph` → `ss.engine.Graph = <fresh>`), then calls `ss.sess.Answer(...)` (`tools.go:552`) or `Report`.
2. `Answer` (`session.go:491`): if the answered option has `opt.Call` it runs `runCommand(inst, opt.Call)` (`session.go:571-574`); a gate-`op` write runs the same way from `cascade` (`instance.go:199-204`).
3. `runCommand` (`instance.go:273`) builds `ctx := s.funcContext(inst)` — capturing the *current* `s.engine.Graph` — then runs `newEntry` → `s.runNewEntry(ctx, ss)`.
4. `runNewEntry` (`registry.go:331`) dispatches `s.handler.NewEntry(...)` (writes file + commits), writes `entryId` into the store, then **calls `s.refreshGraph(ss)` as its last act** (`registry.go:429`), replacing `ss.engine.Graph` with a graph that includes the new entry.
5. Control returns to `Answer`/`cascade`, which then run `transitionTo` → `cascade` → `serve`. **Every** subsequent guard eval, inject query, and unit render rebuilds `Context` via `s.funcContext(inst)` (`instance.go:162`), reading the *now-refreshed* `s.engine.Graph`.

Because `Session.engine` and `shellSession.engine` are the same `*Engine` (set in `NewSession`, `session.go:177-186`), and `funcContext` reads the live pointer on every call, the refresh is visible to everything downstream in the same advance. So:
- The fidelity-review step's `generatedSummary` inject (`registry.go:186-204`, `ctx.Graph.ByID[id]`) **sees** the just-created entry.
- A subsequent `procedureList`/predicate on the shell junction serve (after the move ends, `next` → `serveShell`, `tools.go:562-564`) evaluates against the fresh graph.
- `renderFraming`'s pointer-keyed cache (`tools.go:1216`) correctly invalidates because the pointer changed.

`replaceSummary` follows the same discipline (`registry.go:246`). WIP commands (`wipStart/Done/Remove`) don't refresh — correctly, since WIP markers live under `.sdd/graph/wip/` and are skipped by `LoadGraph` (`graph.go:28`), so they never affect the snapshot.

`resumeSession` and parked flows: resume loads fresh (`tools.go:799`) and replay applies logged state directly with no side effects (`session.go:630-645`), so the snapshot matches disk at resume; later advances refresh at `next`/`startProcedure` entry. `abandonSession` (`tools.go:680`) needs no graph. No inconsistency.

**Why it's fragile (smell, not defect):** the freshness contract is *manual and distributed* — every write command must remember to end with `s.refreshGraph(ss)`. A future write command that forgets would silently serve a stale snapshot for the rest of its transition, with no compile-time or test guard. The correctness today rests on two commands each remembering one line.

**Minor staleness seams (acceptable, worth naming):** `refreshGraph` is bolted to specific entry points, not to serving. Serves that do *not* pass through `next`/`startProcedure` render against the last-refreshed snapshot:
- re-knocking `start_session` on an open door re-serves framing/openThreads/procedureList without refreshing (`tools.go:425-437`);
- `park` and `abandon` call `serveShell` (`tools.go:633,670`) without refreshing.

These only lag *external* writes (another connection/CLI) since the last advance, and only for orientation material — not decision-gating state — so they are defensible as-is. But they show the refresh is coupled to advance verbs rather than being a property of "reading the graph."

## 3. Per-pattern layering judgment (against AGENTS.md CQRS rules)

- **`Finder.LoadGraph` as the one loader — CONFORMS, good.** Loading is a pure read; it lives on the read side (`finders`). Every other finder method is graph-in/result-out. This is already the clean seam the refactor should build on.
- **CLI per-command load — FINE-AS-IS.** Load at the composition root, pass the `*model.Graph` into finder queries. Idiomatic; process is short-lived so "per-command" is the whole lifetime.
- **Handlers loading internally via `Reader.LoadGraph` — CONFORMS.** The `handlers` package doc explicitly sanctions "validation against loaded state" (`handler.go:1-5,24-29`). A handler loading to validate + resolve refs before a write is expected, not a query back-door.
- **MCP free reads reloading per call — CONFORMS to layering, but wasteful.** They go finder→finder correctly (`tools.go:960-1066`). The smell is efficiency: a full disk walk + parse + base-procedure join per free read, and a *separate* copy from the session snapshot. Under MultiGraph (N member repos) this per-call cost multiplies.
- **Engine holding a mutable `Engine.Graph` + shell reassigning it via `refreshGraph` — the DRIFT; pragmatic-but-undocumented with a fragility smell.** It is *not* a hard CQRS-rule break: the engine performs no side effect (the reload is still `Finder.LoadGraph`, a read, invoked from the shell), and the shell is the legitimate composition/lifecycle layer. But it contradicts the engine's own self-description — "pure Go over data … side-effectful commands come in through the registry" (`session.go:127-133`) — by having the engine *hold read-side state* whose freshness is maintained by imperative external reassignment scattered across write commands and advance verbs. Conceptually, graph *loading and freshness* belong to the read side (a finder), surfaced to the engine as a value it reads, not a field an outside actor pokes.

Net: no rule is *broken*, but the read side's ownership of "what the current graph is" has leaked into a mutable engine field plus a hand-maintained refresh protocol in the shell. That is exactly the seam MultiGraph will stress.

## 4. Recommended target seam + minimal refactoring (severity-ranked)

**Target: one graph-access seam every consumer holds instead of calling `Finder.LoadGraph` directly.** Introduce a read-side `GraphSource` (finder-owned) whose single job is "give me the current graph," with request/advance-scoped memoization and one invalidation trigger. MultiGraph then slots in *only* inside that source's load implementation.

Suggested shape (names illustrative):
- `finders`: a `GraphSource` with `Current(ctx) (*model.Graph, error)` that today wraps `LoadGraph(dir)` and memoizes the result until invalidated. Tomorrow it assembles `model.MultiGraph` (local + cached member graphs keyed by repo-id) — the finder-owned load path. The side-effectful part (clone/pull member repos into the cache) stays a **handler**, which on completion calls the source's `Invalidate()`. This keeps "read builds the composite graph / write mutates the member cache" on the correct sides of CQRS.
- `engine`: replace the `Engine.Graph` field with a small provider interface defined in `engine` (to avoid the import cycle), e.g. `type Graphs interface { Current() (*model.Graph, error) }`. `funcContext` (`instance.go:162`) resolves through it. The provider memoizes per advance and is invalidated by `runCommand` after any command that declares graph writes — so post-write freshness within a transition becomes automatic and centralized, deleting the scattered `refreshGraph` calls (`registry.go:429`, `:246`, `:459-466`) and the entry-point refreshes (`tools.go:479`, `:541`). This is the read-through view refreshed per transition.
- MCP free reads (`tools.go:960-1066`): obtain the graph from the same source with per-request memoization, so one MCP request that reads several times (or a resume rehydrating several serves) pays one MultiGraph build.
- CLI + handlers: retarget from `Finder.LoadGraph`/`Reader.LoadGraph` to `GraphSource.Current` — a mechanical swap; per-command lifetime is unchanged.

**Severity ranking:**

1. **SMELL (highest value) — mutable `Engine.Graph` + hand-maintained `refreshGraph` protocol.** Fix by the read-through provider with invalidate-on-write. Removes the latent "forgot to refresh" bug class, deletes ~4 scattered refresh call sites, and gives MultiGraph exactly one insertion point (`GraphSource.Current`). This is the change that pays for itself before MultiGraph lands.
2. **SMELL — redundant full-graph builds per free read and the session snapshot living beside them.** Fix with per-request memoization behind the same source. Low risk; matters more once each build fans out to member repos.
3. **FINE-AS-IS — CLI per-command loads and handler internal loads.** Correct and idiomatic. Do not restructure their *pattern*; only retarget the call to the seam so MultiGraph reaches them for free. Do not add caching to the CLI (short-lived process).
4. **FINE-AS-IS — the orientation-only staleness at `start_session` re-knock / `park` / `abandon` serves.** Once graph access is read-through-with-invalidation, these get current-on-read for free; not worth a dedicated fix on their own, since they only lag external writes for non-gating framing material.

**Explicitly not worth changing:** the free reads being separate from the engine snapshot at the *conceptual* level (they should stay ungated and independently loadable); WIP commands not refreshing (correct — WIP is outside the graph); replay applying logged state without re-reading (correct and side-effect-free).

Key files for the refactor: `internal/finders/graph.go` (loader → source), `internal/engine/session.go` + `instance.go` + `registry.go` (Graph field → provider, funcContext read-through), `internal/mcpserver/registry.go:459` + `tools.go:405,479,541,799,960-1066` (delete refreshGraph, route reads through the source), `cmd/sdd/serve.go:59-89` (wire the source at the composition root).