# `sdd view` — design dialogue

## 1. Problem & approach

Catch-up at 423 entries (May 2026) consumes 32% of session context and exceeds 3 minutes (per `s-cpt-jq7`). The bottleneck is synthesis: the existing playbook rewrites pre-computed summaries into narrative across 40+ entries on every session start. The deferred tiered catch-up condition from `d-prc-t5m` is met.

Two approaches considered:

**A. Refactor `sdd status` with a `--layout` flag.** Compact CLI surface but introduces flag-default-changes that risk surprising existing scripts and the existing skill. Forces deciding what bare `sdd status` means going forward.

**B. New command `sdd view` (chosen).** Build the new pipeline architecture under a fresh command name. Leave `sdd status` untouched. Migrate skill usage via the catch-up sweep activity. Deprecation of `sdd status` becomes a separate decision once `sdd view` is well-adopted.

The new-command path keeps Plan 2 scope as build-not-refactor, ships zero breakage during build, makes the migration moment explicit (the sweep), and gives `sdd status` an exit ramp without forcing the question now.

The pipeline architecture itself is "primitives all the way down": every concern (source, filter, transform, aggregate, rank, page, render) is a composable function. Macros are pure sugar over canonical pipelines. This avoids the "specialized section with internal render logic" pattern that breaks composability.

## 2. Pipeline architecture

```
source → filter* → transform* → aggregate? → rank? → page? → render
```

Each stage is a primitive function. Pipeline composes left-to-right. Render is always the terminus.

**Modifier semantics:**

- Filters stack additively (intersection). Multiple `kind(...)` filters intersect; `kind(K1, K2)` within one filter is disjunction.
- Non-filter modifiers (rank, page, name, render) apply last-write-wins per modifier kind. Macros expand first; user modifiers append.

**Macro expansion example:**

```
top(20):rank(in-degree):name(structural)

  expands first to: active:n(20):rank(heat(exp-14d)):as-list
  user modifiers append: active:n(20):rank(heat(exp-14d)):as-list:rank(in-degree):name(structural)
  final after last-write-wins: active:n(20):rank(in-degree):as-list:name(structural)
```

## 3. Grammar specification

```
layout    := entry ("," entry)*
entry     := func (":" func)*
func      := name ("(" arg-list? ")")?
arg-list  := arg ("," arg)*
arg       := func | value
value     := number | identifier | string
name      := [a-zA-Z][a-zA-Z0-9_-]*
```

Parser tracks paren depth across function calls, allowing nested calls like `rank(heat(linear-30d))`. Comma at depth 0 = entry separator. Comma at depth > 0 = arg separator.

## 4. Vocabulary reference

### Sources

| Function | Args | Semantics |
|---|---|---|
| `source(graph)` | — | All graph entries (default if no source specified) |
| `source(wip)` | — | Active WIP markers from `.sdd/graph/wip/` |

### Filters

| Function | Args | Semantics |
|---|---|---|
| `active` | — | Entries not closed and not superseded |
| `closed` | — | Entries closed by another entry |
| `superseded` | — | Entries replaced by another same-kind entry |
| `kind(K[, K2, ...])` | one or more kind names | Disjunction within one filter (any of); intersection if multiple `kind()` filters |
| `layer(L)` | layer name (`stg`, `cpt`, `tac`, `ops`, `prc`) | Entries at that layer |
| `since(spec)` | ISO date or duration | Entries created on/after the resolved date |
| `topic(L)` | topic label | Entries with topic L (inline `topics:` or annotation membership), prefix-match on path components |

### Transforms

| Function | Args | Semantics |
|---|---|---|
| `expand(field)` | frontmatter field name | Per row, explode the field's list value into sub-rows |

### Aggregates

| Function | Args | Semantics |
|---|---|---|
| `group(by:field)` | field name | Group result rows by field; output shape becomes grouped |

`count`, `agg(...)` — reserved, future plans.

### Rank

| Function | Args | Semantics |
|---|---|---|
| `rank(<algorithm>)` | algorithm function call | Sort result by computed score descending |

Algorithms (used inside `rank`):

| Function | Args | Formula |
|---|---|---|
| `heat(decay)` | optional decay name; default `exp-14d` | `Σ over incoming refs of decay(now - ref_source.created_at)` |
| `in-degree` | — | `count of incoming refs`; ignores decay |
| `mult(decay)` | optional decay name; default `exp-14d` | `heat(decay) × in-degree` |
| `add(decay)` | optional decay name; default `exp-14d` | `normalized(heat(decay) + in-degree)` |
| `log(decay)` | optional decay name; default `exp-14d` | `heat(decay) × log(1 + in-degree)` |
| `by(date)` | — | Sort by entry creation timestamp; descending by default |

Decay functions (used inside algorithm calls):

| Name | Formula |
|---|---|
| `exp-7d` | `2^(-age_days/7)` |
| `exp-14d` | `2^(-age_days/14)` |
| `exp-30d` | `2^(-age_days/30)` |
| `linear-7d` | `max(0, 1 - age_days/7)` |
| `linear-14d` | `max(0, 1 - age_days/14)` |
| `linear-30d` | `max(0, 1 - age_days/30)` |
| `none` | `1` (no age effect) |

### Page

| Function | Args | Semantics |
|---|---|---|
| `n(N)` | integer | Limit to first N rows after rank |

`offset(O)` — reserved, future.

### Output

| Function | Args | Semantics |
|---|---|---|
| `name(string)` | header label | Override section header text |

### Render

| Function | Args | Input shape | Output |
|---|---|---|---|
| `as-list` | — | flat list | One line per entry: ID, type/kind/layer, score (if ranked), summary |
| `as-grouped` | — | grouped result | One sub-section per group, listing entries within |
| `as-focus-block` | — | focus entries with `expand(involvement)` | Per focus: header + window + per-target lines with state |
| `as-participants-block` | — | actor entries | Per actor canonical: actor + bound active roles |
| `as-wip-list` | — | wip markers | One line per WIP marker |

`as-table(...)` and other render shapes — reserved, future.

## 5. Macro definitions (canonical pipelines)

| Macro | Expansion |
|---|---|
| `top(N)` | `active:n(N):rank(heat(exp-14d)):as-list` |
| `focus` | `kind(focus):active:expand(involvement):as-focus-block` |
| `topic(L)` | `topic(L):rank(heat(exp-14d)):as-list` |
| `decisions` | `active:kind(plan,directive,activity,contract,aspiration):group(by:kind):as-grouped` |
| `signals` | `active:kind(gap,question):group(by:kind):as-grouped` |
| `insights` | `active:kind(insight):since(30d):rank(by(date)):as-list` |
| `done` | `kind(done):since(30d):rank(by(date)):as-list` |
| `aspirations` | `active:kind(aspiration):as-list` |
| `contracts` | `active:kind(contract):as-list` |
| `participants` | `active:kind(actor):as-participants-block` |
| `wip` | `source(wip):as-wip-list` |

Macros expand at parse time; user-supplied modifiers in the layout entry append to the expansion. Last-write-wins resolves conflicts per modifier kind.

## 6. Focus-block state derivation

For each involvement target in an active focus entry:

```
resolved_actors = involvement.actors if present
                  else focus.actors if present
                  else []
resolved_when   = involvement.when if present
                  else focus.when if present
                  else null
heat            = ranking_score(target)            // configured rank algorithm
target_status   = derived_status(target)            // existing graph derivation

if target_status not in {"active", "open"}:
    omit from focus block
elif resolved_actors == []:
    state = "pull-available"
elif heat < stalled_threshold:
    state = "stalled"
else:
    state = "driving"
```

`stalled_threshold` configurable via `stalled(value)` modifier on the focus section. Default observed during plan implementation.

`as-list` (with `top(N)` macro context) deduplicates entries already shown in any focus block in the same layout, so the warm-but-undeclared surface is what surfaces in `top` after focus consumption.

## 7. Render contracts

Each render expects a particular input shape:

- `as-list` — flat list of entries
- `as-grouped` — grouped result (after `group(by:...)` aggregation)
- `as-focus-block` — list of focus entries with their involvement triples expanded
- `as-participants-block` — list of actor entries
- `as-wip-list` — list of WIP markers

If a layout produces a shape that doesn't match the chosen render, the executor returns a clear error: `render shape mismatch: as-grouped expects grouped result, got flat list (did you forget group(by:...)?)`.

## 8. CQRS scaffolding (per `d-cpt-ah1`)

```
internal/
├── command/                            # no commands (no writes)
├── query/
│   ├── view.go                         # ViewQuery struct
│   └── layout_parser.go                # grammar parser → Layout AST
├── handlers/                           # no handlers (no writes)
├── finders/
│   ├── view.go                         # pipeline executor
│   ├── ranking.go                      # rank algorithm dispatch
│   ├── decay.go                        # decay function dispatch
│   ├── aggregation.go                  # group(by:field)
│   └── transforms.go                   # expand(field)
├── model/
│   ├── layout.go                       # Layout, Section, Function, Result
│   ├── ranking.go                      # algorithm and decay types
│   └── render_shape.go                 # render input shape contract
└── presenters/
    ├── view.go                         # render dispatcher
    ├── render_list.go                  # as-list
    ├── render_grouped.go               # as-grouped
    ├── render_focus.go                 # as-focus-block
    ├── render_participants.go          # as-participants-block
    └── render_wip.go                   # as-wip-list

cmd/sdd/
└── view.go                             # thin CLI shell (parses --layout, dispatches)
```

`ViewQuery` carries the parsed `Layout` AST. The finder walks the AST applying primitives to produce a `Result` (which can be `FlatList`, `Grouped`, or `Specialized` per render needs). The presenter dispatches based on render-name and input shape, returning rendered text.

## 9. Open implementation questions

These are settled inside plan implementation, not before:

- **Error message wording.** Specifics for "unknown function," "render shape mismatch," "invalid layout," and "missing required arg" surface during implementation; user-facing clarity over engineering precision.
- **Test fixture strategy.** Use the live local graph (current 427-entry SDD repo) for smoke testing, or build a synthetic fixture? Live graph is most realistic but couples tests to graph evolution; synthetic fixture is hermetic but maintenance overhead. Decide during test design.
- **Performance target.** No fixed numeric target; "acceptable on a 500-entry graph" is the qualitative target. Measured during smoke testing and recorded in findings.

## 10. Out-of-scope items (with rationale)

- **`offset(O)` paging.** Top-N is sufficient for v1 catch-up. Offset would enable scrolling, useful for `as-table` exports — defer until that arrives.
- **`count` aggregation.** Group-by satisfies catch-up needs (kind grouping for decisions/signals). Counts useful for analytics — separate use case.
- **`as-table(...)`.** Tabular rendering is a different consumer (e.g., spreadsheet export). Catch-up uses prose-line rendering.
- **`agg(...)` envelope.** General aggregation language premature; concrete `group(by:field)` covers immediate needs.
- **`source(topics)` and `as-tree`.** Topic-tree rendering is a separate exploration; grammar accommodates it as future macro.
- **`sdd list` rank flags.** `sdd list` stays a simple filter+list tool. Topic filtering (Plan 1's scope) exposes the `topic(L)` filter primitive on `sdd list`; full rank composition lives on `sdd view`.
- **`sdd status` deprecation.** Defer to a future directive once `sdd view` is well-adopted by the skill and humans. No anxiety needed during build.
- **Persistent ranking index/cache.** v1 is pure read-time computation. If performance proves limiting at 1000+ entries, a separate plan adds an index.

## 11. Sample invocations per catch-up move

### Light catch-up

```bash
sdd view --layout=focus,top(25)
```

Shows active focuses with state-derived involvement, then top 25 entries by heat (excluding focus targets).

### Full catch-up

```bash
sdd view --layout=focus,aspirations,contracts,decisions,signals,insights,done,participants
```

Shows focuses + all kind-grouped sections + recent insights + recent done + participants block. Closest to current `sdd status` output, plus focus block.

### Topic drill

```bash
sdd view --layout=focus,topic(catch-up-scaling)
```

Shows active focuses + entries clustered under "catch-up-scaling" topic, ranked by heat.

### Eval-mode comparison (one-off)

```bash
sdd view --layout=top(20):name(heat):rank(heat),top(20):name(degree):rank(in-degree),top(20):name(mult):rank(mult)
```

Shows three top-20 lists side by side using different rank algorithms, each with its own header.

### Custom: light + WIP markers

```bash
sdd view --layout=focus,top(25),wip
```

Lightweight view plus active WIP markers.

### Pure structural ranking

```bash
sdd view --layout=top(25):rank(heat(none))
```

Heat with no decay collapses to weighted in-degree — useful for "what's structurally central regardless of recency."
