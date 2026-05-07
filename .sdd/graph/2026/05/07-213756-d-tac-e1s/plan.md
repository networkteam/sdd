# `not(<filter>)` filter negation primitive

## Alternatives considered

### A — general `not(<filter>)` primitive (chosen)

Wraps any supported inner filter and applies its inverse semantic.

```
top(20):not(kind(contract,aspiration)):rank(in-degree)
```

- Pros: zero parser change (nested function args already supported by `rank(by(date))`, `group(by(kind))`); symmetric across filter primitives; user composes per-section without macro authority creep; extends naturally to `not(layer(stg))`, `not(topic(parked))`.
- Cons: requires a small per-primitive negation slot in `sectionSpec`; needs explicit decision on which inner filters are supported (we constrain to set-shaped filters in v1).

### B — negation tokens within `kind(...)`

Inline `!` tokens, e.g. `kind(!contract, !aspiration)`.

- Pros: tightest syntax for the specific common case (excluding kinds in `top(N)` catch-up).
- Cons: needs a new token class in the parser (`!` prefix on identifiers); mixing positive and negative in the same call (`kind(plan, !directive)`) is ambiguous when chained with other `kind(...)` intersections; doesn't generalize to other filter primitives, creating asymmetry.

### C — macro-level default exclusions

Bake `not-kind(contract, aspiration)` into the `top(N)` macro expansion.

- Pros: simplest patch; matches the surfaced pain point exactly.
- Cons: opinionated — macro author decides what "actionable" means; doesn't help custom layouts that need exclusion outside the `top(N)` shape; users can't override (top bakes `active` already; layering another bake makes intent unclear); kicks the same design choice into a future moment when another macro needs the same treatment.

## Comparison

| | A. `not(<filter>)` | B. `kind(!K)` | C. Macro bakes |
|---|---|---|---|
| Parser change | None | New token class | None |
| Generality | All set-shaped filters | `kind` only | Per-macro |
| User authority | Composes per-section | Composes per-section | Macro author |
| Cost | small executor slot per primitive | parser + mixed-mode rejection | one-line edit |

## Scope boundary

**In v1:** `not(kind(...))`, `not(layer(...))`, `not(topic(...))`. Pure set-shaped filters where the inverse semantic is unambiguous.

**Deferred (own decision later):**

- `not(active)` — the "non-active" set splits into closed and superseded with different semantic weight; worth its own dialogue.
- `not(since(spec))` — meaningful as "older than spec" (a real catch-up move: "what hasn't been touched in 30d?"); inverting a temporal cutoff has a clear answer but deserves explicit naming rather than a mechanical not-flip.

**Disallowed:**

- Nested `not(not(...))` — error rather than involute. A reader scanning a layout shouldn't have to track parity. Future relaxation is cheap.

## Implementation sketch

Single CQRS slice, finder-layer only — no command, no model change.

1. **Parser** (`internal/query/layout_parser.go`) — no change. `not(kind(...))` already parses as a function with one nested-function arg.

2. **Executor** (`internal/finders/view.go`):
   - Add `not` to `knownFunctions`.
   - Add `case fn.Name == "not"` in `parseSectionFunction`. Validates exactly one argument, that argument is `ArgKindFunc`, and the inner function name is one of `kind`, `layer`, `topic`. Otherwise returns the listed-supported-set error.
   - Dispatches the inner call into negated buckets on `sectionSpec`:
     - `excludeKindSets [][]model.Kind` (mirrors the positive `kindFilters` shape; multiple `not(kind(...))` calls intersect their exclusions)
     - `excludeLayer model.Layer` (last-write-wins, matching positive `layer()` semantics)
     - `excludeTopicPrefix model.TopicPath` (last-write-wins)

3. **Application order** — apply exclusions in the same filter pass, immediately after each positive counterpart:
   - After `filterByKinds`, drop entries whose Kind matches any `excludeKindSets` entry.
   - When `excludeLayer` is set, drop entries at that layer (separate from the positive `Layer` field on `GraphFilter`).
   - When `excludeTopicPrefix` is set, drop entries whose effective topic set has it as a component-wise prefix.

4. **Tests** — extend `view_test.go` (or a new `view_not_test.go`):
   - Three positive negation tests (kind/layer/topic), each verifying composition with a positive filter on the same primitive.
   - One unsupported-inner test (`not(active)`, `not(since(...))`, `not(not(kind(...)))` — each emits the listed-supported-set error).
   - One arity test (`not()`, `not(kind(plan), kind(directive))`).

5. **Doc** — `internal/bundledskills/claude/sdd/references/cli-reference.md`:
   - One row in the Filters table: `| not(<filter>) | Excludes entries matched by the inner filter. Supported inner: kind, layer, topic |`.
   - One worked example near the existing `top(N):rank(in-degree)` example: `sdd view --layout='top(20):not(kind(contract,aspiration))'`.
   - Then rebuild + reinstall to refresh `.claude/skills/...`.

Single commit, single PR shape.
