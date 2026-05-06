Shipped the type-system 7+7 expansion: `kind: annotation` (signal) for structural metadata layered onto referenced entries and `kind: focus` (decision) for involvement-driven planning with dual lifecycle, along with the topic data model, CLI capture surface, pre-flight rubrics, display rendering, and the new contract `d-cpt-ni0` superseding `d-cpt-ygn`. Closes the type-system plan (`d-tac-gvn`), the topic-filter ownership directive (`d-tac-9q1` — Plan 1 owns `internal/finders/topic_filter.go`), and the topics-shape unification directive (`d-tac-n0u` — annotation kind generalized; `topics:` items support string-or-mapping with refs as canonical edge field).

Live smoke test on the local graph captured an annotation (`s-cpt-vpc`, clustering three catch-up-scaling entries under one topic) and a focus (`d-tac-0qn`, three involvement triples exercising inherit-default, explicit-empty pull-available, and full per-involvement override). `sdd list --topic catch-up-scaling` surfaces the annotation members with the `<catch-up-scaling>` segment rendered on each entry line; `sdd show` on each member shows the derived `Topics:` line.

One implementation deviation worth naming: urfave/cli/v3's default StringSliceFlag CSV-splitting corrupts JSON-bearing flag values. Set `DisableSliceFlagSeparator: true` on the `sdd new` subcommand to fix it. The repeatable `--flag X --flag X` form remains canonical; the in-value comma form is no longer accepted on `sdd new`'s slice flags. Captured separately in commit `d5f80c7`.

See [AC coverage]({{attachments}}/ac-coverage.md) for the per-AC walk and the augmenting-directives' commitments.
# d-tac-gvn — closing done signal AC coverage

This document records how each acceptance criterion of the type-system 7+7 plan (`d-tac-gvn`) was met, including coverage of the augmenting directives `d-tac-9q1` (topic-filter ownership) and `d-tac-n0u` (topics-shape unification).

## Plan ACs

- [x] **Type system expanded to 7+7.** `internal/model/entry.go`: `KindAnnotation`, `KindFocus` added to the kind constants and to `signalKinds` / `decisionKinds`. `IsValidKindForType` accepts the new kinds.
- [x] **New `kind: contract` entry superseding d-cpt-ygn.** Captured as part of this delivery (see this done signal's `refs` to the new contract `d-cpt-ni0`).
- [x] **`kind: annotation` signal implemented.** `internal/model/annotation.go`: `AnnotationTopic` type with custom YAML un/marshaling that accepts string-or-mapping items. Frontmatter validation in `internal/model/graph.go` checks refs presence, topics presence, label parses as `TopicPath`, members ⊆ refs.
- [x] **Annotation entries excluded from `sdd status` kind-grouped narrative sections.** The kind-allow-list shape of `Graph.Directives()`, `Plans()`, `Activities()`, `Aspirations()`, `Contracts()`, `OpenSignals()`, `RecentInsights()`, `RecentDone()` already excludes annotation by virtue of not naming the kind. No code change needed — the existing pattern enforced this for actor/role and now does the same for annotation.
- [x] **Inline `topics: [<label>, ...]` valid on any entry kind.** `frontmatter.Topics []AnnotationTopic` field accepts both string and mapping items at parse time; non-annotation entries with object-form items emit a `Warning` and skip the offending item (load-permissive contract).
- [x] **Topic representation: []string components internally; /-joined I/O.** `internal/model/topic_path.go`: `TopicPath.Components` is `[]string`; `String()` joins with `/`.
- [x] **Topic component validation: [\p{L}\p{N}\-]+ Unicode-aware; reject empty.** `validateTopicComponent` in `topic_path.go` walks runes accepting Unicode letters, digits, and `-`. `ParseTopicPath` rejects empty input and empty components from leading/trailing/consecutive `/`.
- [x] **Topic comparison case-insensitive on path components; first-seen casing preserved.** `TopicPath.Equal`, `HasPrefix`, and `FoldKey` use `strings.EqualFold` per component. `CanonicalizeTopicPaths` deduplicates by fold-key while preserving the first-seen casing's `TopicPath` value.
- [x] **No alias mechanism for topic labels in v1.** Documented in the new contract's Topics section. No alias field on `TopicPath` or `AnnotationTopic`.
- [x] **`kind: focus` decision implemented with dual lifecycle.** `internal/model/focus.go`: `Involvement`, `FocusWhen` types. Validators do not enforce one lifecycle over the other; pre-flight template `focus_capture.tmpl` is explicit about the dual-lifecycle stance.
- [x] **Focus frontmatter validates `involvement: [{target, actors?, when?}]`.** `validateFocusFrontmatter` in `graph.go`: target presence, target resolution against the graph, actors non-empty when present.
- [x] **`when: {from?, to?}` validates ISO dates; at least one required when present.** `FocusWhen.Validate()` in `focus.go` parses each end with `time.Parse("2006-01-02", ...)` and rejects when both are empty.
- [x] **Top-level `actors:` and `when:` accepted as defaults; resolution rule enforced.** `Entry.ResolveActors` and `Entry.ResolveWhen` implement the per-involvement → top-level → empty/null fallback. The `Involvement.ActorsSet` flag preserves the explicit-empty distinction (pull-available vs inherit-default) through frontmatter roundtrip.
- [x] **`sdd new --actors NAME[,NAME...]`.** `cmd/sdd/main.go` adds `--actors` StringFlag, parsed via `splitCSV` into `command.NewEntryCmd.FocusActors`.
- [x] **`sdd new --when '{...}'`.** `parseWhenFlag` parses JSON into `*model.FocusWhen` and validates the shape.
- [x] **`sdd new --involvement '{...}'` (repeatable).** `parseInvolvementFlags` walks each spec, distinguishing nil vs empty actors via `*[]string` JSON unmarshaling.
- [x] **`sdd new --topic '{...}'` (repeatable, annotation only).** `parseAnnotationTopicFlags` accepts JSON object form, JSON-quoted scalar string form, or bare label string for shell ergonomics.
- [x] **`sdd new --topics LABEL[,LABEL...]` on any entry kind.** `--topics` StringFlag → `splitCSV` → `command.NewEntryCmd.TopicLabels` → `BuildEntry` parses each label through `model.ParseTopicPath` so errors surface at command time.
- [x] **CLI JSON parse errors and shape errors produce clear messages identifying field locations.** `parseWhenFlag`, `parseInvolvementFlags`, `parseAnnotationTopicFlags` cite flag index and field path in error messages.
- [x] **Pre-flight on focus captures validates frontmatter shape at high severity.** Mechanical validation lives in `validateFocusFrontmatter`, fired by `model.ValidateEntry` and surfaced as blocking errors by `handler_new_entry.go`.
- [x] **Pre-flight does not enforce supersession-vs-done-closure rubrics or commitment-completeness for focus entries.** `focus_capture.tmpl` explicitly states no high or medium findings; the rubric is observational only.
- [x] **Pre-flight emits low-severity informational findings for the three observation patterns.** `focus_capture.tmpl` enumerates closure-without-target-completions, supersession-with-zero-shared-targets, and all-pull-available with neutral wording.
- [x] **Topics rendered as `<label1, label2>` on entry lines.** `presenters.EntryLine` calls `g.EffectiveTopics(e)` and emits the angle-bracket segment between `{status:...}` and the summary; segment omitted when empty.
- [x] **`sdd show <id>` displays derived topic field.** `presenters.show.writeDerivedSection` adds a `Topics:` line when the effective set is non-empty, alongside the existing `Status:` line.
- [x] **`sdd list --kind annotation` and `sdd list --kind focus` work via existing `--kind` filter.** Verified during smoke test (see s-cpt-vpc and d-tac-0qn).
- [x] **`sdd list --topic <label>` filters via prefix-match.** `cmd/sdd/main.go` adds `--topic` StringFlag, parses through `model.ParseTopicPath`, and passes to `query.ListQuery.Topic`. `finders/list.go` applies `TopicFilter.FilterEntries` after the base kind/layer filter.
- [x] **CQRS decomposition per d-cpt-ah1.** Domain types in `internal/model/`; capture extension to `command.NewEntryCmd`; reads/finder for topics in `internal/finders/topic_filter.go`; presenters extended in `internal/presenters/`; pre-flight templates in `internal/llm/preflight_templates/`. Handlers unchanged (the existing `NewEntry` handler operates on the extended command transparently via `BuildEntry`).
- [x] **Migration is forward-only.** Documented in the new contract's Scope section. No migration code added; existing entries remain valid under the prior kind set per immutability.
- [x] **`cli-reference.md` updated.** Added `--topics`, `--topic`, `--actors`, `--when`, `--involvement` flag documentation, JSON shape examples, quoting convention, and capture examples for both annotation and focus.
- [x] **`framework-concepts.md` updated.** 7+7 type tables; new "Topics and annotations" and "Focus and involvement" sections; retirement primitives table extended; rendering conventions document the angle-bracket topic notation.
- [x] **Skill drafting guidance for actor/role-shaped sections extended to focus and annotation.** The new framework-concepts sections cover canonical-only-in-participants and topic canonicalization; the cli-reference quoting convention covers both annotation and focus JSON flags.
- [x] **Unit tests cover annotation/focus frontmatter validators, topic-path parsing and comparison, focus resolution rules.** `topic_path_test.go`, `annotation_test.go`, `focus_test.go` cover the core type system; `validate_*` tests in `focus_test.go` cover the frontmatter validators.
- [x] **Smoke test demonstrates end-to-end capture of focus with three involvement states and an annotation entry.** Captured `s-cpt-vpc` (annotation, three refs, single topic) and `d-tac-0qn` (focus, three involvement triples covering inherit-default, explicit-empty pull-available, and full per-involvement override). `sdd list --topic catch-up-scaling` correctly surfaces all three annotation members with the `<catch-up-scaling>` topic segment rendered between `{status:...}` and summary; `sdd show` on each member surfaces the derived `Topics:` line.

## Augmenting directives' commitments

### d-tac-9q1 — topic-filter ownership

- [x] Plan 1 ships `internal/finders/topic_filter.go` as the canonical `topic(L)` filter primitive. The file exists with `TopicFilter.MatchEntry` / `FilterEntries` and is exercised by `topic_filter_test.go` plus the live smoke test through `sdd list --topic`.
- [x] Plan 2 (d-tac-uww) consumes the primitive — does not own it. (Confirmed at the source level; Plan 2's implementation has not started, so this remains a forward commitment Plan 2 honors at its ramp-up.)

### d-tac-n0u — topics-shape unification

- [x] Annotation `topics:` items support either string form (applies to all refs) or `{label, members}` mapping form (sub-selection of refs). `AnnotationTopic.UnmarshalYAML` handles both shapes; `MarshalYAML` emits the most compact form.
- [x] Inline `topics:` on non-annotation entries is strings-only. Object-form items on non-annotation entries surface as `Warning`s rather than parsing into `AnnotationTopics`.
- [x] Annotation `members` (when given) is a subset of `refs`. Validated in `validateAnnotationFrontmatter`.
- [x] Annotation kind is general (forward-extensible). The new contract's annotation section frames the kind as "structural metadata layered onto referenced entries" with topics as one form, leaving room for future annotation forms without kind proliferation.

## Deviations and items not addressed

- **`--topic` flag accepts a bare-label shortcut** in addition to the JSON object form spec'd in the design. Documented in cli-reference.md; rationale is shell ergonomics — the bare form is unambiguous because it doesn't start with `{`.
- **CSV-splitting on slice flags disabled at the `sdd new` subcommand level.** Discovered during smoke testing — urfave/cli/v3's default splits StringSliceFlag values on commas, breaking JSON-bearing flags. Set `Command.DisableSliceFlagSeparator = true` on the `new` command. Behavior change: `--involvement`, `--topic`, `--attach` no longer accept the in-value CSV form (the repeatable `--flag X --flag X` form remains canonical). Other commands keep the v3 default until they need the same fix.
- **Pre-flight templates not picked up by sub-skill rendering routes.** Out of scope for this plan — the pre-flight LLM rubrics for annotation and focus are wired into `selectCheckType` and the templates exist, but the broader observation that pre-flight templates need additional augment-plan-pattern awareness (per `s-prc-vko` and the in-flight `d-tac-ghm` plan) is being addressed separately.
