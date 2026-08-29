# Automatic-serve size audit (2026-08-29)

Delegated code audit of every surface where the engine or MCP server pushes content to the agent without an explicit pull. Question: which pushed content scales with the graph or with session state, and what bound exists today. Swept: `internal/engine`, `application`, `internal/mcpapp`, and the shipped procedure specs.

## Systemic findings

1. **No serve-level size accounting exists.** `Session.serveWith` / `renderUnit` (`internal/engine/instance.go:412`, `:583`) compose lanes and inject results with no size tracking. The only bound mechanism is a per-inject `maxBytes` argument, implemented by exactly one query, `viewLayout` (`application/workflow_registry.go:88`). `entryChains`, `topicLabels`, `factIndex`, `procedureList`, `sessionInfo`, `generatedSummary` accept no bound argument.

2. **The pull path is capped, the push path is not.** `guardViewSize` (40,000 bytes, honest notice naming `n(K)` paging as recovery) applies only at the `view` tool (`mcpapp/tools.go:977`). The `viewLayout` inject calls `w.app.View` directly (`application/workflow_registry.go:84`) and bypasses it: the same layout is capped when pulled, unbounded when pushed.

3. **The 25KB door budget has no runtime enforcement, only a test, and the fixture misses the unbounded lanes.** `TestDoorPayloadUnder25KB` (`mcpapp/server_test.go:2613`) builds a 60-entry graph with no fact entries, no actors, no focus, no WIP markers, no procedures, so it exercises only the lanes that are already bounded.

4. **Cut honesty is inconsistent.** Three styles ship: `capOnLineBoundary` appends "… (lane truncated to fit its byte cap)" with no pointer and no naming of the loss (`application/workflow_registry.go:665`); `guardViewSize` names the loss and the recovery (`mcpapp/tools.go:1009`); `capCollected` names each dropped field (`mcpapp/tools.go:1046`). Only the latter two are honest cuts.

## Detail on the previously known spots

- **Catch-up is not six bounded lanes.** One `maxBytes: 28000` applies over the concatenated six-section render (`d-prc-cat` spec, line 20; `capOnLineBoundary(result.Sections, …)`). The cut lands at the tail, so late sections (open-and-warm, wip) starve first, and the one generic notice never names which sections vanished. The first lane, `focus:brief`, is itself uncapped and can consume the whole budget.
- **Engage chains are depth-bounded only.** `BuildShowTree` (`internal/model/show_tree.go:78`) limits depth (up 3 / down 2) but not node count; a hub entry with 100 refd-by edges expands fully. Explore is worse: `targets` is a caller-supplied `list<entry-id>`, N primaries each with a full body. The depth frontier does point at the rest (`TruncatedRef`, rendered at `internal/presenters/show.go:191`): honest at the frontier, unbounded before it.
- **`topicLabels`** returns every distinct topic label across active entries as bare lines (`application/workflow_registry.go:150`), no cap.

## Inventory (risk-ordered)

| # | Surface | What grows | Bound today | Honest cut? | Evidence |
|---|---|---|---|---|---|
| 1 | engage/explore/evaluate/implementation `entryChains` inject | chain nodes (unbounded fan-out at depth ≤3/2); explore also N primaries × full body | none (depth only) | partial: depth frontier names hidden IDs | `d-prc-eng.md:31`, `d-prc-exp.md:27`, `d-prc-evl.md:33`, `d-prc-imp.md:35`; `application/workflow_registry.go:97`; `internal/model/show_tree.go:331` |
| 2 | shell `principles` framing lane (NEW) | active facts under `principles/interactive`, rendered as full bodies | none | no cut | `d-prc-dlg.md:30`; `internal/presenters/render_bodies.go:22` |
| 3 | catch-up compose, all six lanes | entries + `expand(refs)` sub-lines (per-entry ref count uncapped even under `n(8)`) | one 28,000-byte cap over the concatenation | no: generic notice, no pointer, silently starves tail lanes | `d-prc-cat.md:20`; `application/workflow_registry.go:88,647`; `internal/presenters/render_list.go:33` |
| 4 | shell `focus` framing lane; catch-up lane 1 (NEW) | active focus entries × involvement targets | none (`focus` macro emits no `n()`) | no cut | `d-prc-dlg.md:33`; `internal/query/macros.go:268` |
| 5 | shell `participants` lane; bootstrap `readiness` lane 1 (NEW) | active actors × bound roles | none (`participants` macro emits no `n()`; readiness caps its other 3 lanes at `n(6)`) | no cut | `d-prc-dlg.md:34`; `internal/query/macros.go:299,116` |
| 6 | groom `sweep`, `wip` inject (NEW) | active WIP markers | none (`wip` macro emits no `n()`) | no cut | `d-prc-grm.md:23`; `internal/query/macros.go:316` |
| 7 | shell junction, `factIndex` inject (NEW) | active indexed facts, one line each | none | no cut | `d-prc-dlg.md:23,49`; `application/workflow_registry.go:57` |
| 8 | shell junction, `procedureList` inject (NEW) | live procedure chains, one signature line each | none | no cut | `d-prc-dlg.md:22,60`; `application/application.go:345` |
| 9 | shell info block, recovery notices (NEW) | pending unrecovered writes, one line each | none | no cut | `d-prc-dlg.md:46`; `application/recovery.go:190` |
| 10 | capture `assemble`, `topicLabels` lane | distinct topic labels across active entries | none | no cut | `d-prc-cap.md:49,175`; `application/workflow_registry.go:150` |
| 11 | `noHandleError` rejection (NEW) | sessions with open work, handle + quoted label each | none | no cut | `mcpapp/tools.go:430` |
| 12 | `resume_session` state projection (NEW) | running instances, one full ServeResult each | partial: `Collected` capped (2000/value, 8000/instance); instance count and instruction bytes uncapped; `fullReplay` clears dedup | Collected yes; rest no | `mcpapp/tools.go:1338,1013`; `application/workflow.go:613` |
| 13 | shell serve `open_threads` block (NEW) | this session's open instances, one line each | none | no cut | `mcpapp/tools.go:1364` |
| 14 | capture `guideFindings` + `findings` units (NEW) | LLM findings, one multi-field line each | none at any layer | no cut | `d-prc-cap.md:216,238`; `internal/llm/preflight.go:580`; `application/workflow_registry.go:338,451` |
| 15 | terminal serve `produced` (NEW) | engine-written store values incl. full findings as JSON | none (asymmetric with resume's `capCollected`) | no cut | `internal/engine/instance.go:387`; `mcpapp/tools.go:1219` |
| 16 | `abandon` teardown `discarded_threads` (NEW) | discarded instances, one line each | none | no cut | `mcpapp/tools.go:649` |
| 17 | capture playback draft block (NEW) | first serve head/tail-bounded (6+3 lines); adjust rounds serve a full unified diff; list fields render every item | partial | no cut on diff/lists | `internal/engine/draftserve.go:19,101,82` |
| 18 | gate diagnostics (`refsInspected`, `draftValidates`) | draft refs / structural findings in the rejection | bounded in practice by draft size (agent-supplied) | yes: names each ID and the recovery | `internal/engine/predicates.go:238,270` |
| 19 | shell framing, graph-health block | graph warnings + unreadable entries | 5 lines + "(… and N more)" | yes | `application/workflow.go:944` |
| 20 | chooser payloads / report schemas | spec-derived only; no graph-derived enums | n/a | n/a | `internal/engine/schema.go:14` |

## Bounded correctly today (the pattern to copy)

- Graph-health block: 5 lines plus a named remainder count (`application/workflow.go:944`).
- Resume projection `capCollected`: per-value and per-instance caps with named omissions (`mcpapp/tools.go:1013`).

## Out of scope, noted

`list_sessions` is an explicit pull and uncapped (every session with open work plus every open instance). `search` defaults to 8 hits, `view` guards at 40,000 bytes, `read_attachment` pages at 65,536 bytes: pulls, bounded. `emptyViewMessage` inlines every known participant canonical, pull-only.