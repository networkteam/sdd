# Bounded serves: architecture record (2026-08-30)

Settled in dialogue between Christopher and Claude before slice 1 of the bounded-serves plan (20260829-173342-d-tac-rzi). Grounded in three delegated code explorations that verified the plan's serve-size audit against `main`; every file:line below was checked in that pass.

## Data flow, and the two seams

```
procedure entry (frontmatter: params/state/steps/framing; body: unit/lane templates)
  -> engine.ParseSpec / Spec.Validate(reg) / LoadSpec        spec.go:272/:772/:842  (all hard-fail; budget arithmetic never joins them)
  -> Session.serveWith / renderUnit                          instance.go:412/:583
       injects run once per step (:602-616), results enter the template context under EffectiveID (:615)
       store values via Store.TemplateContext                instance.go:671 -> store.go:343
       lanes render authored templates (:619-632)            (authored text, never cut)
  -> framing composed application-side                       application/workflow.go:877 via engine Session.Inject session.go:661
  -> WorkflowServe                                           application/workflow.go:80, publicServe :1190
  -> mcpapp serveResultBody                                  mcpapp/tools.go:1212 (per-lane dedup :1245, capCollected :1034, wire envelope)
```

Every scaling part passes one of two seams: renderUnit's inject loop, and the template context. `templateContext` splits into a unit context (bounded, feeds served text) and an arg context (raw, feeds inject args); it has exactly these two callers today (instance.go:593, :644).

## internal/truncate (new leaf package)

Pure mechanics, data separated from meta:

- `Cut{Dropped, Total, Bytes, Pull}`: the accounting record. `Pull` is a ready-to-run expression for the remainder, empty when none applies.
- `List[T]{Items, Cut}` and `Text{Text, Cut}`: carriers implementing `Payload() any` / `CutMeta() Cut`.
- `Items[T](items, size func(T) int, maxBytes, pull)`: accumulates whole rendered chunks while bytes fit. `Head[T](items, n, pull)`: first N. `Bytes(s, maxBytes, pull)`: line boundary, rune-safe fallback.

No notice wording in the package; each surface renders a `Cut` in its own register. Absorbs the three existing styles: `capOnLineBoundary` (application/workflow_registry.go:651, deleted), `guardViewSize` (mcpapp/tools.go:1001, rewritten, keeps its larger pull-path cap), `capCollected`/`capCollectedValue` (mcpapp/tools.go:1034/:1075, rewritten).

## internal/serveview (new)

The pure serve-construction layer, to serves what presenters are to finders. Owns `Budget{Total advisory; per-kind caps}`, `Cap{MaxBytes, MaxItems}`, `PartKind` (text, entry-list, line-list, item-list, store-value, draft, framing, produced), `Default()` with the calibrated numbers (single upgrade knob), and the bounded-part construction functions over `truncate`. Imports only stdlib and truncate.

## internal/engine changes

- `Query` gains `Bound{Part PartKind, Cap, Pull func(args, Cut) string}` (registry.go:73, beside the `ServeSafe` precedent). Declared at registration in application, where the result shape is known.
- `InjectCall` gains a typed `Cap` override; legacy `args.maxBytes` folds in at parse (shipped uses: d-prc-cat.md:20 at 28000, d-prc-dlg.md:35 at 2500, example_spec.go:23).
- Effective cap resolution at the seams: inject override, else `Query.Bound`, else `serveview.Default()` by part kind. Carrier unwrap: payload into the template context, `Cut` into `Serve.Cuts` plus an engine-owned `cuts` lane (precedent: the synthetic draft lane, instance.go:451). The cuts lane joins ordinary per-lane dedup; `fullReplay` restores it after context loss.
- `Serve` gains `Cuts []truncate.Cut` and per-part `Sizes` (what the harness measures).
- `draftserve.go`: bound the adjust-round diff and both list paths (:83, :110-150); first-serve head/tail (6+3) stays.
- `produced()` (instance.go:387) capped at the MCP layer symmetrically with `Collected`.
- `Spec.WorstCaseServeBytes(budget, resolver) []StepSize`: pure and advisory, never called by ParseSpec/Validate/LoadSpec. Worst case = lane skeleton bytes + effective inject caps + typed slot allowances per `VarDecl.Type` (engine-written values sized by shape, they carry no declaration).
- `procedureSpecSchema()` (schema.go:128-215) has `additionalProperties: false` on the spec block and the inject shape; the new cap and budget fields must be added there or agent-authored procedure captures get rejected by the served schema.
- Spec-level declared total (the finding silencer) touches six sites that move together: model/entry.go:446 (frontmatter struct), :573 (IsProcedure routing guard), :800 (FormatFrontmatter write-back), model/procedure.go:17/:30 (ProcedureSpecRaw; ProcedureSpecFromDocument rejects unknown keys), engine/spec.go (typed field + decodeStrict), engine/schema.go (served schema). The procedurespec base fact documents it for authors.

## View pipeline

- `skip(N)`: no parser change (ParseLayout is permissive on names). finders/section_spec.go:47 gains `skipN` (sentinel -1), view.go:523 gains the case (reusing parseIntegerArg :874), knownFunctions :111, applied before the take at :301 with scores kept parallel. Mirror every pageN rejection (section_spec.go:131, :171, :177, :181, :232). Vocabulary: viewlayout/reference.go:53 (propagates to --help and the view-grammar base fact) plus the bundled cli-reference template.
- The focus/participants/wip macros emit no `n()` because their render shapes reject it (section_spec.go:131/:181/:232). Define a unit per shape (focus groups, participant groups, markers), implement paging over it, lift the rejections, then macros bake a default `n(N)`.
- Pull expressions: the parser captures each section's raw source substring into `Section.Source` (expandSection preserves it); a section's pull is `Source + ":skip(K):n(M)"`. `Layout/Section String()` is the fallback for AST-built sections (no unparser exists today).
- `BuildShowTree` (model/show_tree.go:78) gains budget parameters (max nodes per direction, max children per node), default unbounded so explicit `show` is never cut; the fan-out cap reuses the `TruncatedRef` frontier idiom (:398, rendered presenters/show.go:191); direction-level drops land as counts plus IDs on `ShowTree`. Explore's primary-count bound lives in the `entryChains` inject, not in the tree builder.
- render_list.go:38: per-entry ref-expansion cap, cut at whole-RefExpansion boundaries. render_bodies.go: bounded by whole bodies via `n()`, never byte-cut (the principles lane is the case where a byte cut destroys the content's purpose).
- Section cut notices render inside the section data (RenderView drops empty sections; anything appended after concatenation orphans).
- The `viewLayout` inject stops self-capping; explicit pulls pass a zero budget (the view tool keeps its own guard).
- `factIndex` stays typed (`truncate.List[FactIndexRow]`); `sessionInfo` stays a map, only its `recovery` lines are bounded at source (application/recovery.go:190).

## MCP surfaces (mcpapp/tools.go)

- `capProduced` symmetric with `capCollected` on the same struct literal (:1217 vs :1218).
- Resume projection (:1338): instance-count cap plus an aggregate byte budget across `Open`, cut at whole-instance granularity only, omitted instances named by handle, procedure, and step (reachable via `next`, so the pointer is free). `fullReplay` is the worst case and the reason whole-instance is the only legitimate cut.
- `open_threads` (:1364), `discarded_threads` (:652, `HeldMarkers` too), `noHandleError` (:437): whole-line caps with named remainders; `noHandleError` points at `list_sessions`.
- Constants derive from the exported serveview budget; exported, so the external test package stops re-declaring 2000/8000/25000.

## Lint and pre-flight

- The application is the single composition root (companion conceptual directive). `app.Lint(ctx, query.LintQuery)` composes categorized providers; `query.LintResult` reshapes to `LintFinding{Category, Code, Severity, EntryID, Message}` grouped by category; advisory severity never flips the exit code; presenters render by category.
- The procedure-runtime provider validates spec load first (today lint never parses procedure specs; LoadSpec's "engine and lint" doc comment is stale), then runs the budget arithmetic with the registry as resolver.
- Pre-flight: the budget finding joins finders/preflight_mechanical.go's existing `kind: procedure` branch (:79) at severity medium; the "strictly binary severity" doc comment (:57) gets amended. `SkipPreflight` skips it; accepted, lint catches the spec on later sweeps.
- CLI `sdd new` routes through the application (Reader composed by the app); transitional until v0.18.0 removes skill-based CLI use.

## Measurement harness

- internal/proctest/fixture.go (production file, importable by mcpapp tests): `RealisticGraph(GraphShape{Entries, Facts, Actors, Roles, Focuses, WIPMarkers, HubFanOut, TopicLabels, ...})`, written through the production serializer.
- internal/proctest/servesize_test.go: walks every shipped procedure's steps against the fixture, measures serve + framing + schema + produced/collected per part (`Serve.Sizes`), asserts against `serveview.Default()`. Failure names procedure, step, measured bytes, budget, and the largest contributing lane. A step-coverage assertion diffs measured (procedure, step) pairs against every spec step, with a commented allow-list for deliberately unreachable steps.
- Calibration mode (env-gated) runs the same walk against this repository's live graph to ground `Default()`; the hermetic fixture stays the regression.
- mcpapp keeps one thin wire test replacing `TestDoorPayloadUnder25KB` (server_test.go:2613): door plus worst-case fullReplay resume, measured as JSON envelope bytes. (Fixture correction from exploration: base facts and base procedures merge into every graph load, so the old fixture did exercise those lanes; what it misses is project-scaling content: actors, focus, WIP, ref fan-out, topic breadth.)
- Engine unit tests over synthetic specs (engine_test conventions, serve_lanes_test.go): caps applied to inject results, authored lane text byte-identical after a cut, whole-item boundaries, budget overshoot never fails LoadSpec.

## Slice order

1. Measure: `Serve.Sizes` accounting, RealisticGraph fixture, proctest harness, calibration run. Defaults and fix order fall out of the data.
2. truncate + engine caps + collapse of the three cut styles.
3. View pipeline: skip, Section.Source, per-shape bounds, show-tree budgets, macro defaults.
4. MCP surfaces.
5. Authoring arithmetic: spec budget field (six sites), pre-flight finding, lint reshape on the application.
6. Live verification on a ~10K-token host: opening, catch-up, engage without a truncated serve.
