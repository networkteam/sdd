# Capped readiness view — layout (for review)

Two callers, one layout:

- **`orient` — push (engine inject).** First contact; the agent must ground before it
  opens, so the engine injects the view up front — guaranteed, not hoped-for.
- **`refresh` — pull (agent-run).** Not injected. The served unit points the agent to
  run this view over the graph *on demand* (via the free, never-gated `view` read
  capability — host-neutral, reachable on a remote/hosted graph, not the CLI), keyed
  against `producedIds` as the known delta. The agent pulls only when it earns the read
  (show the user growth, or re-ground after a cold resume) and skips it otherwise.

**Two callers of the same layout → make it a named `readiness` macro** (DECIDED — see
note below). Both `orient`'s inject arg (`{fn: viewLayout, args: {layout: 'readiness'}}`)
and the `refresh` unit's pull-instruction then reference `readiness` by name instead of
duplicating the raw string. Small Go change in `internal/query/macros.go`.

The macro expands to (composed entirely from existing `sdd view` filter/macro
vocabulary — no new predicate). On a fresh graph every lane is empty, which is the
intended "nothing yet" signal.

```
participants:brief,
aspirations:active:rank(heat(exp-14d)):n(6):name("Aspirations"):brief:as-list,
kind(directive):intent(guiding):layer(strategic):active:rank(heat(exp-14d)):n(6):name("Direction — strategic guiding"):brief:as-list,
kind(directive):intent(guiding):layer(conceptual):active:rank(heat(exp-14d)):n(6):name("Shape — conceptual guiding"):brief:as-list
```

Four lanes, matching AC 2 (actors/roles, conceptual guiding, strategic guiding,
aspirations):

- `participants:brief` — the actors and roles known so far (the `participants` macro
  renders the actor/role grouping).
- `aspirations` — the WHY pull, capped at 6, warmest first.
- strategic guiding directives — the project's chosen direction.
- conceptual guiding directives — the project's shape/approach.

## The view grammar reference (base fact s-prc-vwg)

The view layout language has a base fact — `s-prc-vwg`, topics `engine/base-facts` +
`cli/view` — that is the agent's pullable grammar reference: every filter, rank,
transform, render terminator, and macro the executor accepts. Two consequences:

- **The agent needs no grammar taught in the unit.** At `refresh` the served text just
  names the *readiness view*; if the agent needs to run or adjust a view, it pulls
  `s-prc-vwg` on demand. Keep the unit free of grammar (it already is).
- **Registering the `readiness` macro means updating `s-prc-vwg` too.** Its macro list
  today is aspirations, contracts, decisions, done, focus, insights, participants,
  signals, top(N), topic(L), wip — no `readiness`. Add `readiness` to the macro list in
  that fact (by superseding it, since entries are immutable) so macro-discovery stays
  complete. Rides slice 1 with the macro itself.

## Notes / things to verify in wiring

- The grammar in `s-prc-vwg` confirms the layout is valid vocabulary: `layer(L)`,
  `intent(I)`, `kind(K)`, `active`, `rank(heat(exp-14d))`, `n(N)`, `name("…")`, `brief`,
  `as-list`, and the `participants` macro are all listed. Remaining check: appending
  `:brief` to the `participants` macro — the fact says a macro's later modifiers
  override its defaults, so `participants:brief` should apply, but confirm the render
  (participants renders `as-participants-block`; verify `brief` composes with it).

- This splits guiding directives by layer (`layer(strategic)` vs `layer(conceptual)`)
  — the session framing layout does *not* do this today; it has one undifferentiated
  "Guiding directives" lane. `layer()` and `intent()` are confirmed in `s-prc-vwg`, so
  the split composes; confirm against the running executor in the wiring slice.

- Deliberately *not* the full session framing (no focus lane, tighter caps) — this is
  bootstrap's own slim read, sized so an empty or near-empty graph reads at a glance
  and a populated one doesn't flood context.

- Ships as a named `readiness` macro (decided above): `orient` injects it by name and
  the `refresh` unit names it in its pull-instruction, so the raw string lives in one
  place — `internal/query/macros.go` — not duplicated across the spec and the unit
  prose. Rides slice 1 (the bootstrap entry can't load without it).
