# Derived base-fact index, Markdown reference, and view filter — implementation design

## Resolved questions

1. **Which graph contributes entries?** The opening index uses the current project's graph after embedded/project merge and lifecycle resolution. Connected dependency graphs are excluded from the automatic opening payload. They remain reachable through ordinary search/show. This keeps session orientation bounded and avoids loading or advertising another repository's framework cues without an explicit read. Embedded facts appear once; an on-disk same-ID override wins through the existing merge.
2. **Does index metadata inherit through supersession?** No. Enrollment belongs to the active fact entry that carries `index`. A successor must repeat or revise the block to remain indexed; a successor without it deliberately removes the cue.
3. **How is the nested shape carried through writes?** Add one domain value object (`FactIndex` / `fact-index`) with `title` and `topic`. Persist it as nested `index:` YAML; expose it as one nested object in engine capture, application drafts, commands, CLI parsing, and `show`.
4. **How is the initial flat opening list ordered?** Sort by `topic`, then case-folded `title`, then full canonical ID.
5. **How is title quality enforced?** Mechanically require a trimmed, non-empty title and a valid topic that is also in `topics`. Capture guidance states that the title must stand alone as the retrieval cue. Do not add an LLM gate or arbitrary character threshold.
6. **How does the view tool expose enrollment?** Add a zero-argument `indexed` filter that tests presence of valid `index` metadata. It does not imply lifecycle state and takes no topic argument: `active:indexed:as-list` gives the live population, while `indexed:as-list` can inspect enrolled history. Existing `topic(...)` narrows by subject because `index.topic` is required to be an ordinary topic. No special render shape or index-title column is introduced.
7. **What does outer validation anchor on?** The evaluation move is started against the implementation's done signal. The indexed reference fact is evidence read during the scenario, not the evaluation anchor.

## Domain and serialization

- Add `model.FactIndex { Title string; Topic TopicPath }` and `Entry.Index *FactIndex`.
- Add the nested YAML field to frontmatter decode/encode and round-trip it without flattening.
- Extend `ValidateEntry`: fact-only; title and topic both required; topic must parse and occur in ordinary `topics`; invalid shapes surface through existing warnings/write rejection.
- Add a pure graph-derived read selecting active indexed facts and returning deterministic rows with full ID, title, and topic.
- Test superseded/closed omission, indexed successor inclusion, unindexed successor removal, and on-disk same-ID override behavior.

## Authoring and inspection surfaces

- Add a `fact-index` engine domain type whose JSON schema is an exact object with required `title` and `topic`.
- Add optional nested `index` state to capture; document fact-only opt-in semantics; render it verbatim in playback and allow adjustment/revision.
- Carry it through `newEntry`, `EntryDraft`, application construction, `NewEntryCmd`, and `BuildEntry`.
- Add `sdd new --index` accepting one JSON object such as `{"title":"…","topic":"cli/view"}`. Reject partial objects, non-fact use, and topic mismatch before write.
- Include nested `index:` in `sdd show` through shared show data.

## View pipeline

- Register `indexed` as a live zero-argument graph filter in the view executor and vocabulary.
- Apply it as a pure structural narrowing (`Entry.Index != nil`) in the canonical filter phase. Validation keeps indexed entries fact-only; the filter remains property-shaped so malformed historical entries are still inspectable rather than silently hidden.
- Compose with existing primitives: `active:indexed:as-list` selects the session index population; `active:indexed:topic("cli/view"):as-list` narrows by the index topic; ranking, paging, grouping, and ordinary render terminators continue to work.
- Reject arguments (`indexed(...)`) and reject the primitive for `source(wip)` with the same fail-loud pattern as other graph filters.
- Do not add `not(indexed)`, a macro, special title rendering, or a new render shape in this slice.

## Session-shell discovery

- Register a host-neutral `factIndex` workflow query that delegates selection/ordering to the model and returns rows as data; do not format or hard-code IDs.
- Inject it into the user-dialogue opening junction.
- Render a compact Markdown block only when rows exist. Each bullet is ``- `<full-id>` — <title>`` and tells the agent to pull the relevant fact before composing or guessing.
- Keep the opening list flat, while retaining topic in the data for possible later grouping.
- Do not add catch-up, skill, dependency-graph enumeration, or special view rendering.

## View-layout fact rendering

- Refactor `internal/viewlayout` around one semantic reference model: grammar, categorized live vocabulary, descriptions, and host-neutral example layout specifications.
- Preserve terminal `Reference` for `sdd view --help`; add a host-neutral Markdown renderer with headings, fenced grammar/examples, and tables or lists.
- Source both from the same live vocabulary and example data. The new `indexed` filter therefore appears automatically in both references with one shared description.
- Enroll the base fact with topic `cli/view` and the exact chosen title.
- Remove “building or debugging”; examples contain layout specifications only, including `active:indexed:as-list`, with no CLI or MCP host command.

## Verification matrix

- Model: YAML round-trip; malformed/partial/non-fact index; topic membership; active derivation; supersession/closure; deterministic ordering.
- Authoring: CLI JSON parsing; command/application propagation; engine schema/normalization; capture playback/write.
- View: `indexed` zero-argument validation; structural selection; composition with `active`, `topic`, rank/page/render; WIP rejection; live-vocabulary inclusion.
- Presentation: `show` exposes nested metadata; opening serve contains full ID and exact title; procedure has no fact ID.
- Reference: Markdown structure/fenced examples; host neutrality; all live vocabulary described; terminal help unchanged in shape.
- Binary: targeted tests, full tests/vet/lint, `sdd view --help`, `sdd view --layout='active:indexed:as-list'`, `sdd show` of the fact, and a fresh session opening.
- Outer evaluation: capture implementation done, evaluate anchored on that done signal with a fresh agent, and verify it notices the opening cue, reads the full-ID fact, and composes without source search, grep, or syntax guessing.