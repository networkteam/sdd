# `sdd view` plan progress — slices 1-4 shipped

**Plan**: 20260506-151345-d-tac-uww
**Status**: Active (interim hand-off, not closed)
**Captured**: After slice 4 completion

## Commits shipped this session

| Slice | Commit  | What landed                                              |
|-------|---------|----------------------------------------------------------|
| 1     | 36d815b | Pipeline skeleton, `active:as-list` end-to-end           |
| 2     | e83002c | Args/parens grammar, `kind()` + `n()` primitives         |
| 3     | c5c30e2 | Ranking + decay + score injection in `as-list`           |
| 4     | 77c3f7e | `layer()`, `since()`, `topic()` filter primitives        |

Total: ~3,500 net lines added across ~24 files (production + tests). All
four slices: TDD discipline, smoke-validated against the local 442-entry
graph, all quality gates clean (`go vet`, `go test ./...`, `go fmt`,
`golangci-lint`).

## Acceptance criteria status

Plan ACs are tracked from d-tac-uww's body. Current state:

- ✓ AC 1: New command `sdd view` accepts `--layout=<spec>`
- ✓ AC 2: Bare `sdd view` prints help
- ✓ AC 3: Empty `--layout=` errors with grammar message
- ✓ AC 4: `sdd status` is not modified
- ✓ AC 5: Layout grammar implemented (paren depth, nested calls)
- ✓ AC 6: Filters intersect cumulatively; modifiers last-write-wins per kind
- ◐ AC 7: Primitives partial — `source(...)`, `transform(expand)`, `aggregate(group)`, `output(name)` remain
- ✓ AC 8: Algorithm vocabulary {heat, in-degree, mult, add, log, by(date)}; in-degree ignores decay silently
- ✓ AC 9: Decay vocabulary all 7 names (exp/linear-{7,14,30}d, none)
- ✓ AC 10: `since()` accepts ISO date and duration; calendar arithmetic for m/y, exact 24h for d/w
- ☐ AC 11: Macros — slice 6
- ☐ AC 12: Focus-block state derivation — slice 7
- ☐ AC 13: `as-list` dedup against focus targets — slice 7 (depends on AC 12)
- ☐ AC 14: `name(string)` modifier — slice 6
- ✓ AC 15: Unknown function/algorithm/decay/render → clear error with valid set
- ☐ AC 16: Render-shape mismatch error — first exercised in slice 5
- ✓ AC 17: Invalid layout grammar → position-aware error
- ✓ AC 18: CQRS decomposition per d-cpt-ah1
- ◐ AC 19: Shared primitives — `TopicFilter` reused by `sdd list --topic`; other primitives not yet exposed
- ✓ AC 20: Unit tests cover parser, ranking, decay, since (render-shape validation lands in slice 5)
- ◐ AC 21: Smoke testing landed; "light/full/drill" compositions depend on macros (slice 6)
- ☐ AC 22: Skill `cli-reference.md` updated — slice 8
- ☐ AC 23: Closing done signal with comparative findings — slice 8

Roughly 13 of 23 fully done; 3 partial; 7 remaining. Slices 5-8 close the rest.

## Architectural observations (carry to closing findings attachment)

**1. Vertical thin slicing held up across 4 slices with zero rework.** No
backtracking, no architectural revision. The slice-1 seams (parser →
query → finder → presenter → CLI shell, with `model.SectionData` as the
render-shape contract) carried through every subsequent slice unchanged.
Concrete evidence the design's CQRS decomposition was right-sized.

**2. The "parser fully featured in slice 2" call paid off twice.** Slice
3's nested-call grammar (`rank(heat(exp-14d))`) and slice 4's quoted-string
args (`since("7d")`, `topic("infrastructure/cli")`) both worked first try
with no parser changes. Saved an estimated slice-worth of grammar churn
in slices 3 and 4.

**3. CLAUDE.md "push logic down" vs the plan's CQRS placement.** The plan
said "ranking/decay in `internal/finders/`" but pure formulas naturally
belong in `internal/model/` per CLAUDE.md. Resolved by putting math
(decay funcs, score formulas, since-spec parsing) in `internal/model/`
and orchestration (algorithm dispatch, pipeline composition,
`time.Now()` injection) in `internal/finders/`. Small but worth recording
for future plans that name specific package placements.

**4. `add` normalization remains an open finding.** On the live 442-entry
graph, in-degree dominates heat by ~10× (e.g. d-cpt-vt1: in-degree 16,
heat 6.5 with exp-14d). Raw sum makes `add` behave nearly identically to
in-degree. The closing findings attachment should test alternatives:
heat × scaling-factor + in-degree, normalized-to-max approach (each
dimension ÷ its max), or per-layout rebalancing (compute scales from the
result set).

**5. `heat(none)` collapses to weighted in-degree exactly.** Empirically
confirmed — d-cpt-vt1 ranks first in both with score 16.000. Validates
the design's claim and gives users a "structural centrality regardless
of recency" knob without needing a separate algorithm.

## Next-session pickup notes

### Slice 5: aggregate + grouped render

`group(by:field)` and `as-grouped`. First slice that genuinely exercises
`model.SectionData` — until now FlatList has been the only variant.
Render-shape mismatch error (AC 16) fires for the first time.

Files to add/extend:

- `internal/model/grouped.go` — `Grouped` shape implementing `SectionData`
  with `Shape() RenderShape` returning a new `ShapeGrouped`.
- `internal/finders/aggregation.go` — pure aggregation: given entries +
  field name, return per-group entry buckets.
- `internal/finders/view.go` — extend executor with `group(by:field)`
  recognition. Mutually exclusive with `rank()` for slice 5 (a grouped
  result has no global ordering to apply rank to without per-group
  sorting, which slice 5 doesn't add).
- `internal/presenters/render_grouped.go` — new render function for
  `as-grouped`.
- Tests at finder + presenter layers, plus smoke.

The parser already handles `group(by:field)` (parens, key:value-style
args) — verify by parsing it as a bare-arg call where `field` is an
identifier. May need a small parser extension if `by:field` is meant
literally with the colon — re-read d-tac-uww §4 grammar.

### Slice 6: macros

Pure sugar over what slice 5 lands. `top(N)` rewrites to
`active:n(N):rank(heat(exp-14d)):as-list` at parse time. Macro expansion
lives in the parser — once a macro name is recognized, replace its
function with the expanded section, then user-supplied modifiers append.
Last-write-wins resolves conflicts per modifier kind (already implemented
in the executor's intent-bucket pattern).

Macros to land: `top(N)`, `focus`, `topic(L)`, `decisions`, `signals`,
`insights`, `done`, `aspirations`, `contracts`, `participants`, `wip`.

### Slice 7: focus-block + state derivation

Most complex domain logic. `expand(involvement)` transforms a
focus-shaped result; `as-focus-block` renders pull-available / stalled /
driving state per the algorithm in design §6. The `as-list` deduplicator
(AC 13) requires seeing all focus-block targets in the same layout —
implies a layout-level pre-pass before per-section rendering, not
strictly per-section anymore. New shape `FocusBlock` implementing
`SectionData`.

### Slice 8: wip + participants + skill update + closing done signal

- `source(wip)` source primitive + `as-wip-list` render: WIP markers
  from `.sdd/graph/wip/`.
- `as-participants-block`: actor entries grouped by canonical with
  derived-active roles underneath.
- Update `internal/bundledskills/claude/sdd/references/cli-reference.md`
  with `sdd view` overview, grammar summary, vocabulary tables, worked
  examples. Then `./bin/sdd init --scope project` to refresh the
  installed skill copy. Commit both source and installed locations per
  CLAUDE.md.
- Closing done signal carrying the comparative findings attachment per
  AC 23: rank/decay combinations qualitatively compared on the local
  graph; chosen defaults documented with rationale; `add` normalization
  alternatives evaluated.

## Conventions to maintain

- **Pure computation in `internal/model/`; orchestration in
  `internal/finders/`.** Pattern established in slice 3 — preserve it.
- **Permissive parser, executor validates** with listed-valid-set errors.
- **Position-aware errors from parser; named-primitive errors from executor**
  (e.g. `"since: requires...", "kind: argument %d must be..."`).
- **`now` threaded as parameter** to ranking/since functions. Tests use
  fixed clocks; executor passes `time.Now()`.
- **TDD discipline:** tests at every layer, watched fail, then implement.
- **Commit prefix `view:`** matching slice scope.
- **Quality gates before commit:** `go vet`, `go test ./...`, `go fmt`,
  `golangci-lint run ./...` — all clean.

## Working knowledge for the next agent

- **Plan**: `sdd show 20260506-151345-d-tac-uww`
- **Design doc**: `.sdd/graph/2026/05/06-151345-d-tac-uww/design.md`
- **Current focus**: 20260506-190107-d-tac-0qn marks d-tac-uww as
  pull-available; will need a focus update once slice 5 starts.
- **Code locations**: `cmd/sdd/view.go`, `internal/{query,finders,model,presenters}/`
  with `view.go`/`view_test.go`/`ranking.go`/`decay.go`/`since.go`/`layout*.go`
  filenames.
- **Commit messages** document each slice's scope, observations, and
  smoke-test results.

