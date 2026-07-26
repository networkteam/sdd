# Procedure language and state design record

## Observed ceiling

The v1 engine deliberately shipped a small procedure language. Params and state may use arbitrary field names, but values come from a closed engine-owned base-type enumeration plus a single `list<T>` wrapper. There are no procedure-local records or enums, maps, numeric primitives, unions, or nested composite declarations. Adding a domain type means changing Go.

Guards parse only Boolean combinations of registered zero-argument predicate names. They cannot read a state field, compare it to a literal, call a helper with arguments, inspect a collection, or express a typed intermediate. As procedures become richer, simple routing such as `entryKind == "aspiration"` therefore requires a bespoke Go predicate, while validation logic risks being repeated across alternative transition expressions.

The capture evaluation exposed the consequence: a flat set of kind alternatives produced diagnostics from every branch, so an aspiration draft was told about actor canonical, role actor, focus involvement, and generic refs simultaneously. The immediate full-type-capture plan avoids this with explicit temporary kind predicates and one central `reportedDraftValid` helper; it does not solve the language ceiling.

## Collect semantics

A collect marker currently controls two concerns. A nonoptional collect appears in every report schema's required list, even when that value is already accumulated, and missing required collects appear as gate diagnostics. An optional collect is patchable without resending and does not gate cascade. Domain completeness is a third concern, currently expressed through predicates. These need separate concepts:

1. which fields a report at a step may patch;
2. what accumulated state must hold before the step may leave;
3. whether the accumulated domain object is structurally valid.

Separating them lets reports stay incremental while completion and validation remain central and reusable.

## State and type direction

The engine should own a generic, composable core type algebra. Procedures should be able to declare local enum and record types and use them for params and state. Applications should register namespaced framework types such as `sdd.entry-kind`, `sdd.entry-draft`, `sdd.ref`, and other opaque domain values. Project procedures may add local data shapes without requiring engine Go changes.

Executable shared type definitions stored as graph entries are an attractive extensibility layer but add a new meta-model: loading, versioning, supersession, trust, validation, and dependency semantics for code-like graph content. That possibility belongs in the investigation and is not selected by this gap.

## Expressions and helpers

Expressions should evaluate over the declared typed environment and cover at least field access, equality, Boolean composition, collection operations, and calls into a closed registered helper set. Expressions must remain side-effect-free, statically checked at procedure load, and terminating. Graph and session internals should not become freely traversable expression values; controlled helpers expose the permitted questions.

SDD model logic belongs in namespaced helpers such as `sdd.validEntry(draft)`. Helpers keep required-field and model validation DRY across CLI/application writes and procedures. Graph-dependent checks and session evidence remain separate helpers. Macros, if shipped, expand at load time as syntax sugar only; they must not become a second location for model rules.

Expr (expr-lang.org) is a candidate because it is Go-native, typed-environment aware, side-effect-free, terminating, and extensible with functions. It is not chosen. A later design must compare dependency surface, error/diagnostic quality, type integration, function exposure, deterministic evaluation, and whether a smaller owned evaluator better fits.

## Projection and visibility

The immediate resume asymmetry is fixed: resume serves carry a generic collected-state map with trust fields excluded and deterministic truncation/omission. The open problem is policy. Resume-only timing, visibility, size caps, deduplication, and exclusion are application rules rather than procedure declarations. State declarations should be able to express agent-visible projection policy at one seam, while the engine preserves the handover-fidelity invariant that material durable intent is shown or explicitly named as compressed/omitted. The later design must decide whether current state is enough or whether filtered trajectory/completed-ancestor context is also needed.

## Design constraints settled in dialogue

- The engine remains generic; SDD domain rules live in namespaced application types and helpers.
- Procedure-local types make project procedures extensible without minting Go fields.
- Expressions operate on typed declared state in a closed, pure environment.
- Graph/session objects stay opaque behind helpers.
- Required-field validation has one model-owned implementation.
- Macros are syntax sugar only.
- Report patchability, step completeness, and domain validity are distinct.
- Projection policy is declarative and preserves explicit compression/omission.
- Expr is evaluated, not assumed.
- Graph-defined executable shared types remain an explicit meta-model question.
- The redesign does not block full type capture, which uses current mechanics temporarily.