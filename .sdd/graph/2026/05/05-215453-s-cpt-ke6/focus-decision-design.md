# `kind: focus` decision — design synthesis

## Problem with s-cpt-8tu's `kind: activity` proposal

s-cpt-8tu proposes planning entries as `kind: activity` decisions carrying involvement triples. But the proposal explicitly states: "closure is supersede-by-next-plan, not done-by-completion" — which directly contradicts activity semantics, where the expected retirement path is a done signal.

`kind: activity` means "work dispatched, completes via done signal." A planning entry is not work to be completed — it is a standing declaration of current focus that gets replaced when priorities change.

## Proposed: `kind: focus`

A 7th decision kind with dual lifecycle:

- **Supersedable**: when priorities shift, the next planning entry supersedes the current one
- **Completable**: when a planning cycle ends naturally, a done signal closes it

Both paths are valid and map to real scenarios. This matches how planning actually works.

## Distinguishing question

"What are we attending to in this period, and who is engaged?" — distinct from all existing kinds:

- directive: justifies a choice against alternatives
- plan: defines verifiable outcomes (ACs)
- activity: dispatches specific work via done signal
- contract: standing constraint that must always hold
- aspiration: perpetual direction with no completion criterion
- role: one actor's participation pattern

## Frontmatter structure

Carries involvement triples (same mechanism as s-cpt-8tu's proposal):

```yaml
involvement:
  - target: 20260504-100323-s-cpt-8tu
    actors: [Christopher]
    date_range: 2026-05-05/2026-05-12
  - target: 20260505-153647-d-tac-kv5
    actors: [Christopher, Claude]
```

The body carries narrative: what the focus period is about, why these entries were selected, what is in scope.

## Relationship to annotation signal

Focus entries use a specific edge type (involvement/assignment) with actor and date-range payload. The companion annotation signal (kind: annotation) generalizes this to arbitrary typed edge bundles. Both share the frontmatter edge-bundle format — focus is a specialised case.

## Type system impact

Extends decision kinds from 6 to 7, touching the type system contract (d-cpt-ygn). Requires a conceptual-level decision before implementation.
