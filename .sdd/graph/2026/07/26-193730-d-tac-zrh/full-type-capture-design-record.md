# Full type-capture design record

## Why this is its own plan

The bootstrap outer evaluation showed that bootstrap cannot reach parity while the shared capture procedure cannot assemble every current entry kind. Actor and role are the dominant live failures, annotation is required for safe topic founding, and the same structural problem applies to the remaining specialized kinds. Christopher therefore separated full type capture from bootstrap parity: the shared capture capability ships first; bootstrap then composes it.

## Knowledge model

Ship one indexed overview fact for the current SDD type system. It names the signal and decision kinds, their type mapping, and the common structural vocabulary. It references one unindexed detail fact per current kind. Detail facts carry the heavier composition guidance and examples; the overview is the retrieval doorway. Exact wording is implementation-owned and may be improved by later supersession. Only indexed facts appear in the compact shell index, and the index remains derived from active fact metadata.

The capture procedure serves concise lane guidance plus a pointer to the relevant detail fact. The agent pulls the detail fact when it needs it; guidance is not copied among facts, procedure units, and the frozen legacy skill.

## Capture flow

An agent reports `entryKind` and may batch every already-known draft field in the same report. The procedure classifies the kind and enters a concrete assembly lane. Kind determines signal or decision through the closed type-system mapping.

Each lane serves concise guidance and the detail-fact pointer. A generic carried-instruction mechanism accumulates ordered instruction units passed during a successful cascade and returns them with the final current serve. Connection-level content-hash memory deduplicates each block independently. Thus a complete draft can cross classify and assemble in one call while still exposing the lane guidance, landing directly at playback. If assembly stalls, its guidance is the current serve and is not duplicated as a carried block. The ideal knowledgeable-agent path remains two calls before playback: start capture, then report kind plus complete draft.

## Validation boundary

Procedure expressions do not restate required fields per kind. A single reusable new-draft construction and structural validation boundary owns common requirements and per-kind invariants; the engine exposes it temporarily as `reportedDraftValid`. Graph-dependent checks remain separate (`refsResolve`, current actor/canonical resolution, involvement targets), as does session evidence (`refsInspected`). Semantic preflight remains the final model judgment.

Reports are incremental patches. Under the current engine, `collect: field?` means the field is not mandatory in that individual report and does not participate in the generic missing list; it does not mean the completed entry may omit the field. Assembly lanes use optional collect markers and validate the accumulated draft centrally. Redesigning this overloaded engine semantic belongs to the separate engine-design gap.

Strict new-write validation does not change historical reads. The shipped GraphFinder continues loading every parseable document and exposes per-document load issues and structural warnings as data. The plan extracts and reuses the strict draft/write validation path; it does not add read modes or block legacy entries.

## Live-head read contract

Default `show` resolves a requested ID transitively to its live supersession head and returns the requested-origin to live-head breadcrumb. `exact` preserves the requested primary entry only. Every node in relationship trees resolves to its live head in both modes, so an exact historical primary does not reintroduce stale context. The model owns pure supersession and tree resolution, the finder orchestrates, the query carries `Exact`, and CLI plus MCP expose identical behavior.

## Presentation boundary

`show` is directly in scope, including structured rendering for every kind and the live-head contract. `view` is regression scope for counts, filters, topics, and participant behavior. Catch-up is regression scope, particularly ensuring annotation entries do not leak into narrative lanes. The `/sdd` skill and its rendered templates are frozen and unchanged; retirement is a later bootstrap-parity verdict.

## Settled trade-offs

- Full coverage is one plan rather than actor/role-only patches.
- One overview fact plus per-kind detail facts avoids both a monolithic prompt and duplicated procedural prose.
- Central validation wins over duplicated required-field predicates.
- Current collect mechanics are accepted temporarily to avoid blocking on the broader procedure-language redesign.
- A live-head default prevents stale references without transitive rewrites after supersession; breadcrumbs preserve historical inspection.
- Fact prose may iterate after implementation, but the retrieval and reference mechanism is fixed.