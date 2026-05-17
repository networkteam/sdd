# expand(refs) — design

## The failure this resolves

During fresh-session evaluation of `/sdd-catchup` (recorded in `s-tac-7uy`), the agent rendered:

> Sequenced to land after the Engage refactor itself.

against `d-prc-iqw`. But the engage refactor had landed: `s-prc-st0` closed `d-prc-kyz` ten days earlier. The agent's data was `d-prc-iqw`'s summary text (captured before the refactor closed), with `(d-prc-kyz)` appearing as inline narrative. There was no rendered signal that `d-prc-kyz`'s state had changed. The agent threaded the entry as still-blocked.

`sdd show --downstream` already has the mechanism (walks each ref, emits `{status: ...}`). But that's chain-shaped — one anchor, depth-limited. The view pipeline that catch-up reads from never gained the equivalent.

## What the agent needs to understand a ref

Working back from the failure: for each outgoing ref, the agent needs four primitives.

| Primitive | Surfaced by | Status |
|---|---|---|
| Identity (ID) | Always present | ✓ |
| Relationship semantics (why this kind of ref) | `kind` per-ref field | `d-tac-cs0` (planned) |
| Per-ref narrative (why this specific ref) | `desc` per-ref field | `d-tac-cs0` (planned) |
| Current state of referenced entry | Derived status | This plan |
| Compact form of referenced entry | `short_summary` field | Low-confidence gap, separate |

This plan ships the third row. `d-tac-cs0` is a hard dependency — the verb-position rendering and `desc` clause both require the model-layer change. The short_summary primitive ships separately and `expand(refs)` adopts it via a small refinement directive once available.

## Naming rationale

The view pipeline already has `expand(involvement)` — a per-row expansion transform. `expand(refs)` extends that precedent for the refs case. Two consequences:

- **Distinct from `refs-of(<id>)`** in sibling plan `d-tac-jqq`. That's a filter (input: seed IDs; output: a set of entries that the seeds reference). `expand(refs)` is a render modifier (input: a result set; output: same set with per-entry nested sub-lines). Different positions in the pipeline; no name conflict.
- **Compose with `as-list`**, not focus-block. `expand(involvement)` stays scoped to focus-block; `expand(refs)` is its sibling for flat-list outputs.

Alternatives considered for the modifier name — `with-refs`, `nested-refs`, `show-refs`, bare `refs(...)`. Settled on `expand(refs)` for vocabulary symmetry with the existing transform.

## Verb-as-qualifier rendering

The catch-up failure surfaced an ambiguity concern: `{kind: <value>}` reads either way — entry-kind (gap, plan, contract) or per-ref kind (grounds, builds-on, addresses). Both vocabularies live in the project. Bracketing the per-ref kind risked confusion.

Resolution: the per-ref kind becomes the verb. `sdd show` today uses `refs`, `closes`, `supersedes` as verbs; with `d-tac-cs0`, the generic `refs` refines into seven specific verbs.

```
→ grounds <full-id> {status: ...}
→ builds-on <full-id> {status: ...}: "<desc>"
→ depends-on <full-id> {status: closed-by <id>}: "<desc>"
→ refs <full-id> {status: ...}                  # legacy ref, kind: unknown
```

Properties:
- No bracket ambiguity — the verb position carries the qualifier; no other `kind:` token appears on the sub-line.
- Reads as a sentence start: "grounds X", "addresses Y", "depends-on Z".
- Aligns with the existing `closes`/`supersedes` verb convention in `sdd show`.
- Forward-compatible: when `d-tac-cs0`'s `sdd show` adopts the same verb-position rendering (which the current AC doesn't preclude), the two surfaces share one convention.

## Composition with future primitives

When `short_summary` lands (via the low-confidence gap captured alongside), `expand(refs)` adopts it via a small refinement directive. Line shape extends:

```
→ depends-on <full-id> {status: ...}: "<desc>" — <short_summary>
```

No plan revision needed at that point — the AC is structured so the rendering composes additively as adjacent primitives become available. This plan ships against `d-tac-cs0`'s model layer and waits for nothing else.

## Sequencing

| Plan | Touches | Direction |
|---|---|---|
| `d-tac-cs0` | `internal/model/` (refs become objects with `kind` + optional `desc`) | Hard dependency for this plan |
| This plan (`s-cpt-kfl` resolution) | `internal/query/`, `internal/finders/`, `internal/presenters/` (new transform) | Ships after `d-tac-cs0` |
| `d-tac-jqq` | `internal/query/` (new filter `refs-of(<id>)`) | Parallel — different parser position |

Both view-pipeline changes (this plan's transform and `d-tac-jqq`'s filter) touch the parser but in non-overlapping positions. They can land in either order after `d-tac-cs0`.

## CQRS decomposition

- **`internal/query/`** — parser change adding `expand(refs)` and `expand(refs(state-changed))` to the layout grammar. The `state-changed` filter argument follows the existing nested-call pattern (`rank(heat(exp-14d))`, `group(by(<field>))`).
- **`internal/finders/`** — finder method walks each entry in the result set, resolves each outgoing ref's derived status from the loaded graph, and attaches the resolved sub-line data to the result row.
- **`internal/presenters/`** — render helper for the nested sub-lines, conditional on ref shape (object form vs bare-string). No state held in handlers.

No model changes — this plan reads object-form refs and bare-string refs both, as `d-tac-cs0` defines them.

## Smoke test approach

After both `d-tac-cs0` and this plan land, the catchup eval target case becomes mechanically verifiable:

```bash
sdd view --layout='kind(plan,activity):active:rank(heat(exp-7d)):n(8):expand(refs(state-changed)):as-list'
```

Expected: `d-prc-iqw`'s rendered row carries a sub-line showing `d-prc-kyz` with `{status: closed-by 20260507-172237-s-prc-st0}`. The agent reading this layout sees the closure unambiguously.

The catch-up sub-skill's "Active and hot" fetch adopts `expand(refs(state-changed))` via a small refinement directive after this plan ships — keeps the adoption coordination outside the plan's scope.

## Out of scope (carved out)

- **Downstream-direction expansion** (`expand(refd-by)` or similar). Would surface who references this entry. Separate transform, separate plan.
- **Other list-shape renderers** (`as-grouped`, etc.) gaining `expand(refs)`. Add later if a use case wants it.
- **`/sdd-catchup` fetch adoption.** Mechanical follow-up via a small directive.
- **Short-summary integration.** Captured as a low-confidence gap alongside; `expand(refs)` adopts via a small directive when short_summary lands.
- **`sdd show` verb-position rendering alignment.** This plan proposes verb-as-kind as the rendering convention; if `d-tac-cs0`'s `sdd show` implementation diverges, resolution happens through a small alignment directive at implementation time.

## ACs

See plan body. Seven acceptance criteria, no model changes, depends on `d-tac-cs0` landing first.
