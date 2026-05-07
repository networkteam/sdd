# `sdd view` plan — comparative findings & closing notes

**Plan:** 20260506-151345-d-tac-uww
**Augmenting directives closed alongside:** 20260507-101046-d-tac-3pq (group form), 20260507-110226-d-tac-jgi (AC 14 scope)
**Methodology:** Live SDD graph, 452 entries (129 decisions, 323 signals) as of 2026-05-07. All comparisons run against this graph; absolute scores reproducible by re-running each layout against the same commit.

## 1. Half-life sensitivity — heat with exp-7d / exp-14d / exp-30d

`top(20)` results overlap heavily across all three half-lives — the same ~12 entries dominate (the active type-system contract, the dialogue-first aspirations, the `sdd view` plan itself). Half-life affects **ordering** rather than membership at top(20).

| Half-life | Distinguishing characteristic |
|---|---|
| `exp-7d` | Surfaces *very* recent work prominently. Two slice-7-era process signals (`s-prc-fc0`, `s-prc-kaw` from 2026-05-07) appear in top-20 because their refs are days old; they fall off `exp-30d` entirely. Risk: noisy when commit days are dense. |
| `exp-14d` (default) | Balances recency against structural weight. The two-week window naturally cycles with most working sessions; targets stay surfaced long enough to act on but yield to truly new work after a fortnight. |
| `exp-30d` | Pulls structural anchors forward (`d-cpt-vt1`, `d-cpt-ah1`, `d-stg-beb`) — the long-living contracts and aspirations. Risk: catch-up reads as "what's important in general" rather than "what's hot now"; dilutes the catch-up function. |

**Default chosen: `exp-14d`** — matches the typical SDD session cadence (a few work-week pulses) and surfaces both fresh moves and stable anchors in roughly equal proportion. Users wanting a faster catch-up window can write `top(N):rank(heat(exp-7d))`; users wanting structural orientation can write `top(N):rank(heat(exp-30d))`.

## 2. Heat vs in-degree — recency vs structural prominence

`rank(in-degree)` ignores decay entirely: it counts incoming refs without temporal weighting. At top(20) on this graph, in-degree's ordering differs meaningfully from heat's — two contracts (`d-cpt-vt1`, `d-cpt-ah1`) and the foundational adoption gap (`s-stg-qg0`) climb because they accumulated refs over months; recent work (`s-tac-n8u`, `s-prc-nxy`, `s-cpt-0db` — all from May 5) drops several positions because their absolute ref counts are small even though those refs are very recent.

This is the catch-up vs orientation distinction in mechanical form: in-degree answers "what does the graph orbit around?" while heat answers "what's the current pulse?" Both are useful; catch-up is the latter, so the default rank is heat.

## 3. Composite algorithms — `mult`, `add`, `log`

`mult(exp-14d)` (heat × in-degree) emphasizes entries that are *both* recent and structurally prominent. On this graph, the top-5 reorders to `d-cpt-ah1`, `d-cpt-vt1`, `d-stg-beb`, `d-tac-uww`, `d-stg-qlt` — the high-in-degree contracts/aspirations rise. The product strongly penalizes entries with low in-degree even if their heat is high (recent work simply doesn't accumulate enough refs to compete).

`add(exp-14d)` (heat + in-degree, raw sum, slice 3) is closer to heat alone for top-20 because heat scores in the [0, 8] range dominate in-degree counts in the [0, 20] range only at the very top. The in-degree contribution mostly affects the lower half of any given top-N.

`log` was not exercised against the live graph — its damped product shape suits exploratory comparison rather than catch-up's "what's hot" framing.

**Defaults stay with `heat`**, not the composites. The composite signals are accessible via explicit override (`rank(mult)`) when a user wants the structural-recency intersection rather than recency alone. Documented in the design's algorithm vocabulary; no change.

## 4. Stalled threshold — the live graph doesn't discriminate

The current focus block (`d-tac-0qn`) declares three involvement targets:

- `d-tac-uww` — `actors: []` (pull-available, threshold-irrelevant)
- `d-tac-gvn` — closed (omitted from focus block per design §6)
- `d-tac-1du` — actors set; score 0.000 (no incoming refs)

Threshold sensitivity: at `stalled(0.5)`, `stalled(1.0)` (default), and `stalled(2.0)`, both observable targets render identically — `d-tac-uww` is pull-available regardless (resolved-actors empty short-circuits the threshold check), and `d-tac-1du` is stalled regardless (its score is exactly 0). The default is not exercised by the available test data.

**Default kept at `stalled(1.0)`** with a soft commitment to revisit once focus blocks accumulate involvement triples whose score lands in the 0.5–2.0 range. The qualitative anchor — "fewer than one fresh-ref-equivalent post-14d-decay" — remains a defensible starting point even without empirical pressure.

## 5. Slice-2 spec deviations corrected in slice 6 — carry forward

Two behaviours specified in slice 2 needed adjustment during slice 6 implementation:

1. **Multiple `kind()` filters now intersect**, not union. Slice 2 implemented multi-call kind() as union; this contradicted the d-tac-uww §2 last-write-wins-and-intersection rule. Fixed under commit 2aa0a26. Plan readers comparing the slice-2 implementation against §2 should treat the slice-6 fix as canonical.
2. **`render` no longer needs to be the syntactically last function.** Slice 2's strict last-position check rejected `top(N):rank(...)` (where `as-list` lands inside the macro expansion before `:rank(...)` appended). Slice 6 relaxes this to last-write-wins per primitive kind, matching §2's listing of render alongside rank/page/name. The "render is the terminus" semantic property holds via canonical bucket order (filter → rank → page → group/expand → render), not by source position.

Both corrections are documented inline in the executor comments. Future plan reviewers comparing implementation to text should read this finding before assuming slice 2's original behaviour.

## 6. Architecture stability across all eight slices

The slice-1 CQRS seam (parser → query → finder → presenter → CLI) carried through every subsequent slice unchanged. The same shape — `model.Layout` AST → `query.ViewQuery` → `Finder.View` → `query.ViewResult` → `presenters.RenderView` — accommodated grouped results, focus blocks, name() modifiers, source switching, and two new render shapes without architectural rework. Validates the d-cpt-ah1 CQRS decomposition mandate as a sound planning discipline for this category of work.

Pure scalar computation (HeatScore, decay funcs, since-spec parsing) lives in `internal/model/`. Orchestration (algorithm dispatch, pipeline composition, time injection, focus expansion, source-aware dispatch) lives in `internal/finders/`. The slice-3 split-pattern (entry-list manipulation in finders, scalar math in model) extended to slice 8's source-routing and participants-block construction without precedent rework.

The slice-7 hand-off note flagged a concern at `expandInvolvement` — entry-list manipulation that produces a scalar (HeatScore) — which slice 8 inherited without further drift. The split holds.

## 7. Auto-derive shipped (AC 14 closure per d-tac-jgi)

Slice 8 ships explicit `name(string)` *and* auto-derive: when no `name()` is supplied and the section carries a `rank()` specification, the executor synthesizes a header from the algorithm and decay. Examples:

- `top(N)` → `## Top by heat (exp-14d)`
- `top(N):rank(in-degree)` → `## Top by in-degree`
- `done` macro (uses `rank(by(date))`) → `## Most recent`

User-supplied `name("Custom")` always wins, including the empty-string clear (`name("")`) which removes any prior header. Sections without rank() and without name() render headerless (the slice-5/6 default for as-grouped, as-focus-block, etc.). Implementation lives at the executor boundary (`rankSpec.derivedHeader()` writes into `spec.nameValue` when nameSet is false), so SectionResult and renderer types stay free of rank context.

This closes both branches of AC 14 — explicit and auto-derive — completing d-tac-jgi.

## 8. CQRS leak surfaced — captured for follow-up

Slice 8's source(wip) work made visible that six read-side queries (`LintQuery`, `ListQuery`, `SearchQuery`, `ShowQuery`, `StatusQuery`, `ViewQuery`) carry `Graph *model.Graph` as a struct field, and `WIPListQuery` carries `GraphDir string`. The shell loads the graph and hands it to the finder per call; the finder consumes it as if it were declarative input. This conflates *what to read* (intent) with *the resolved data the read works against* (a dependency the finder should own).

The pattern was rejected for slice 8's natural extension (adding `WIPMarkers []*model.WIPMarker` to ViewQuery), but the existing peers retain the leak. Captured as gap signal `s-tac-m09` for a separate refactor — not in scope for d-tac-uww closure.

## 9. AC coverage

| AC | Status |
|---|---|
| 1, 2, 3, 4 | ✓ — `--layout` flag, bare-help, empty-error, `sdd status` untouched |
| 5, 6 | ✓ — grammar, paren-depth, intersection vs last-write-wins |
| 7 | ✓ — sources `graph` + `wip`; filters/transforms/aggregates/rank/page/output/render all implemented |
| 8 | ✓ — heat/in-degree/mult/add/log/by(date); in-degree ignores decay |
| 9 | ✓ — exp-/linear-{7,14,30}d, none |
| 10 | ✓ — since() ISO date + Nd/Nw/Nm/Ny with calendar arithmetic per spec |
| 11 | ✓ — all 11 macros implemented (top, focus, topic, decisions, signals, insights, done, aspirations, contracts, participants, wip) |
| 12 | ✓ — focus-block state derivation pull-available / stalled / driving; closed targets omitted; threshold via stalled() |
| 13 | ✓ — as-list dedup pass against any prior focus-block in same layout |
| 14 | ✓ — `name(string)` explicit and auto-derived from rank+decay (per d-tac-jgi) |
| 15, 16, 17 | ✓ — unknown function/algorithm/decay/render errors with valid-set; render-shape mismatch errors; layout-grammar errors with position |
| 18 | ✓ — CQRS decomposition per d-cpt-ah1 (parsing/query/finder/model/presenter/CLI) |
| 19 | ◐ — `topic(L)` filter is shared between `sdd view` and `sdd list --topic`; the example in the AC is satisfied. Other primitives (filters, rank, aggregation) are not yet exposed for cross-command reuse — captured as future opportunity rather than pushed into slice 8 |
| 20 | ✓ — unit tests cover layout parser edges, ranking, decay, since(), aggregation, state derivation, render-shape validation, source dispatch, auto-derive |
| 21 | ✓ — light/full/drill compositions smoke-tested live (this document); plus the comparison runs above |
| 22 | ✓ — `cli-reference.md` skill update with `sdd view` overview, grammar, vocabulary tables, macros, worked examples (commit df30578) |
| 23 | ✓ — closing done signal carries this findings attachment |

## 10. Defaults of record (revisable post-shipping per a separate directive)

| Setting | Value | Rationale |
|---|---|---|
| Top-N count for `top(N)` | User-supplied; macro requires N | No silent default |
| Default rank algorithm | `heat` | Pulse over orientation for catch-up |
| Default decay | `exp-14d` | Half-life matching working-session cadence |
| Default stalled threshold | `1.0` | "Under one fresh-ref-equivalent post-14d-decay"; not yet stress-tested by real data |

Any of these can be revisited via a follow-up directive once usage data accumulates.
