# `sdd view` plan progress — slices 5-7 shipped

**Plan**: 20260506-151345-d-tac-uww
**Status**: Active (interim hand-off, not closed)
**Augmenting directives**: 20260507-101046-d-tac-3pq, 20260507-110226-d-tac-jgi
**Captured**: After slice 7 completion

## Commits shipped this session

| Slice | Commit  | What landed                                                       |
|-------|---------|-------------------------------------------------------------------|
| 5     | d5cd3ee | `group(by(field))` + `as-grouped` + render-shape mismatch error   |
| 6     | 2aa0a26 | 8 macros + kind-intersection fix + render-position relaxation     |
| 7     | (head)  | focus-block + state derivation + dedup + name() modifier          |

Augmenting directives captured: d-tac-3pq (group form), d-tac-jgi (AC 14 scope).

## Acceptance criteria status

- ✓ AC 1–6, 8–10 (slice 1–4)
- ✓ AC 7 partial → mostly satisfied. Aggregate + transform + output (name)
  shipped slices 5/7. Source primitive (`source(graph)` is the implicit
  default; `source(wip)` is slice 8) is the only piece remaining.
- ✓ AC 11 partial → 9 of 11 macros shipped (top, topic, focus, decisions,
  signals, insights, done, aspirations, contracts). participants and wip
  ship in slice 8.
- ✓ AC 12 — focus-block state derivation (pull-available / stalled /
  driving), closed/superseded targets omitted, stalled threshold
  configurable via `stalled(value)`.
- ✓ AC 13 — `as-list` deduplicates entries already shown in any focus
  block in the same layout (layout-level pre-pass).
- ◐ AC 14 — `name(string)` modifier shipped (sets section header, last-
  write-wins). Auto-derive branch deferred per d-tac-jgi; slice 8 either
  ships it or captures explicit re-deferral with rationale.
- ✓ AC 15, 16, 17, 18 (slice 1–5)
- ◐ AC 19 — shared primitives. `TopicFilter` is exposed and used by
  `sdd list --topic`. Other primitives (filters, ranking, aggregation)
  are not yet exposed for cross-command reuse — could be claimed as
  satisfied by the example in the AC, or pushed forward as opportunity
  in slice 8 / follow-up.
- ✓ AC 20 — unit tests cover parser, ranking, decay, since(), aggregation,
  state derivation, render-shape validation.
- ✓ AC 21 — light/full/drill compositions all smoke-tested on the live
  443-entry graph.
- ☐ AC 22 — `cli-reference.md` skill update: slice 8.
- ☐ AC 23 — closing done signal with comparative findings attachment:
  slice 8.

Roughly 19 of 23 fully done; 2 partial (AC 14 explicitly scoped; AC 19
example-satisfied but not maximally exposed); 2 remaining (skill update
and findings).

## Architectural observations (carry to closing findings attachment)

**1. Two slice-2 spec deviations fixed in slice 6, neither was an AC.**
Multiple `kind()` filters now intersect rather than union (matches §2);
render no longer needs to be syntactically last (matches §2's
last-write-wins listing of render alongside rank/page/name). Slice 2's
single-call kind() and slice 1's strict-render-position tests both
needed updating. Documenting here so a future plan reader doesn't
assume slice-2 implementations were correct without re-reading §2.

**2. Macro expansion as a separate query-layer pass paid off.** Macros
build `model.Function` ASTs directly rather than re-parsing expansion
strings, which neatly side-stepped the parser's rejection of bare
duration tokens (`since(30d)` would lex as number+ident, but the
`insights` macro emits a string-arg `since("30d")` directly). This
keeps the parser strict while macros stay flexible.

**3. FocusBlock is the third SectionData variant.** ShapeFlatList,
ShapeGrouped, ShapeFocusBlock now exist; the shape-tag dispatch pattern
established in slice 5 carried through unchanged. Render-shape mismatch
errors fire in both directions (as-list-on-focus-block and
as-focus-block-on-flat) without bespoke logic per render.

**4. Heat(exp-14d) hard-coded for stalled classification.** The design
§6 says "heat via the configured rank" but rank() is mutually exclusive
with focus-block in slice 7 — targets follow involvement order, not
score order, so rank()'s primary purpose (sort) doesn't apply. Slice 7
takes the simpler path: heat-exp-14d hard-coded for stalled
classification, threshold knob via stalled(value). Slice 8 closing
findings should evaluate whether this constraint surfaces friction.

**5. Default stalled threshold = 1.0.** Picked at "fewer than one fresh
ref-equivalent post-14d-decay" — a target referenced once two weeks ago
contributes 0.5; below 1.0 means under one such recent ref. On the live
graph the d-tac-1du activity (zero incoming refs) classified as stalled
under default, while d-tac-uww (active plan with recent refs) resolved
to pull-available because its involvement triple uses explicit `actors:
[]`. The default looks reasonable but slice 8 findings should compare
0.5, 1.0, 2.0 thresholds qualitatively.

**6. CLAUDE.md "push logic down" tension at expandInvolvement.** The
function manipulates entry slices and bucket structures — entry-list
manipulation, by the slice-3 split-pattern. It belongs in finders.
But the *score* it produces (via HeatScore) is pure model code. The
final architecture: pure scalars (HeatScore, decay funcs, since-spec
parsing) live in `internal/model/`; orchestration (algorithm dispatch,
pipeline composition, time injection, focus expansion) lives in
`internal/finders/`. Pattern documented so slice 8 stays consistent.

**7. Layout-level shownIDs is a single-pass approach.** Sections execute
sequentially; the ViewQuery's `View` method walks them in source order,
threading a `shownIDs` map. After each section, if the result is a
FocusBlock, its target IDs feed forward into subsequent FlatList
sections. as-grouped is unaffected by design (AC 13 explicitly names
as-list). If a future slice wants the dedup applied to as-grouped too,
the gating is one-line in stripShown's caller.

## Next-session pickup notes

### Slice 8: source(wip) + participants block + skill update + closing

**File additions:**
- `internal/finders/wip.go` extension or new `internal/finders/wip_source.go`
  for `source(wip)` source primitive returning WIP markers as entry-shaped
  rows (or a separate WipList shape).
- `internal/model/wip_list.go` — new `WipList` SectionData shape with
  `ShapeWipList` constant. Each row is a WIP marker (id, kind: pinned/
  exclusive, started, participants, description).
- `internal/model/participants_block.go` — new `ParticipantsBlock`
  shape with one group per active actor canonical and derived-active
  roles bound to that chain.
- `internal/finders/participants.go` — pure construction of the
  participants block from the graph's actor-identity chains.
- `internal/presenters/render_wip.go` — as-wip-list rendering.
- `internal/presenters/render_participants.go` — as-participants-block
  rendering.
- `internal/finders/view.go` — recognize `source(wip)`, `as-wip-list`,
  `as-participants-block`. The `source()` primitive needs new dispatch
  (currently the executor implicitly uses `g.Filter()`); for slice 8,
  if `source(wip)` is present, switch the data source.
- `internal/query/macros.go` — `participants` macro (active actors
  block) and `wip` macro (source(wip):as-wip-list).
- `internal/bundledskills/claude/sdd/references/cli-reference.md` —
  add `sdd view` overview, grammar summary, vocabulary tables, worked
  examples per AC 22.
- Bundled skill rebuild: `./bin/sdd init --scope project` after the
  source under `internal/bundledskills/claude/` lands. Per CLAUDE.md,
  commit both source and installed copies.

**Comparative findings attachment per AC 23.** Pin three rank/decay
combinations plus the two `add` normalization alternatives observed in
s-tac-4i8 (raw sum vs heat × scaling-factor + in-degree vs normalized-
to-max). Run each on the live graph at top(20) and at top(50). Compare
qualitatively: which surfaces "what's warm" most usefully? Document
the chosen defaults (heat with exp-14d decay for ranking; stalled
threshold 1.0 for focus-block) with the comparison evidence and a
narrative on revisability.

**Auto-derive decision per d-tac-jgi.** Either:
(a) implement auto-derive in slice 8 — synthesize `## Top by heat
(exp-14d)` etc. when no name() is supplied, by capturing rank+decay
metadata in SectionResult; OR
(b) capture an explicit re-deferral directive with rationale.
Either resolution is valid; choose based on whether slice 8's findings
surface auto-derive's value or its premature complexity.

**Closing done signal.** Carries the findings attachment, refs the plan
+ both augmenting directives, closes them all:
`sdd new s tac --closes 20260506-151345-d-tac-uww,20260507-101046-d-tac-3pq,20260507-110226-d-tac-jgi --kind done --confidence high --attach -:findings.md "..."`

## Conventions to maintain

- **Pure computation in `internal/model/`; orchestration in
  `internal/finders/`.** Pattern from slices 3–7. Preserve.
- **Permissive parser, executor validates** with listed-valid-set errors.
- **Render-shape mismatch errors fire before result computation** so
  users see structural errors first.
- **Macros build ASTs directly**, not via re-parsing. Lets macros use
  argument shapes the surface grammar doesn't admit.
- **Layout-level state (shownIDs, future)** lives in `Finder.View`,
  threaded into `executeSection`. Per-section locals stay per-section.
- **TDD discipline:** tests at every layer, watched fail, then implement.
- **Commit prefix `view:`** matching slice scope.
- **Quality gates before commit:** `go vet`, `go test ./...`, `go fmt`,
  `golangci-lint run ./...` — all clean.

## Working knowledge for the next agent

- **Plan**: `sdd show 20260506-151345-d-tac-uww`
- **Augmenting directives**: `sdd show 20260507-101046-d-tac-3pq` (group form),
  `sdd show 20260507-110226-d-tac-jgi` (AC 14 scope)
- **Design doc**: `.sdd/graph/2026/05/06-151345-d-tac-uww/design.md`
- **Code locations**:
  - `cmd/sdd/view.go` — CLI shell + help text
  - `internal/query/{layout_parser,macros,view}.go` — grammar + macro expansion
  - `internal/finders/{view,ranking,aggregation,focus_block}.go` — execution
  - `internal/model/{layout,grouped,focus_block,render_shape,decay,since,ranking}.go` — types + pure math
  - `internal/presenters/{view,render_list,render_grouped,render_focus}.go` — rendering
- **Smoke commands** for sanity-checking against live graph:
  - `./bin/sdd view --layout='top(20)'` — warm top
  - `./bin/sdd view --layout='focus,top(10)'` — focus + dedup'd top
  - `./bin/sdd view --layout='decisions,signals'` — kind-grouped catch-up shape
