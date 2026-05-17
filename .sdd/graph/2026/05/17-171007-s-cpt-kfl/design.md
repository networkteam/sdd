# Design idea: refs transform for `sdd view`

## The problem

The catch-up sub-skill's data fetches (and any future `sdd view`-driven list consumer) show entries with their refs as inline text inside the entry summary, with no indication of those refs' current state. The catch-up's evaluation surfaced this concretely: the agent rendered `d-prc-iqw` ("depends on the Engage refactor `d-prc-kyz` landing first") as still waiting on the refactor, even though `d-prc-kyz` had closed via `s-prc-st0` days earlier. The agent had no way to know — the summary text was captured before the refactor closed, and `sdd view` rendered the summary verbatim.

`sdd show --downstream` already has the mechanism: it walks each ref, fetches the derived state from the graph, and emits `{status: closed-by <id>}` or `{status: superseded-by <id>}` on the rendered line. But `sdd show` is chain-shaped — one anchor entry, depth-limited expansion — not a transform that composes into list views.

The view layer needs a list-shaped equivalent: for each entry in a list, optionally surface its outgoing refs with derived status as nested sub-lines.

## Proposed shape

A `sdd view` transform that, when added to a layout, per-entry emits one indented line per outgoing ref:

```
  d-prc-iqw process activity decision [...] {status: active} <summary text>
    → refs d-prc-kyz {status: closed-by s-prc-st0}
    → refs s-prc-fc0 {status: open}
```

Compact two-component line: ID + derived-state marker. Optional summary text on the ref line — default likely off for list density.

## Design dimensions (need dialogue before plan capture)

### Default filter — which refs to include

- **`state-changed`** — only refs with `{status: closed-by}` or `{status: superseded-by}`. Lean default for catch-up; surfaces precisely the state changes that matter.
- `all` — every outgoing ref regardless of state. Verbose but general.
- `active` — only refs still open. Useful for "what's still load-bearing under this entry."
- `non-active` — inverse of `active`. Useful for "what already moved on."

Recommendation: `state-changed` as the default since it surfaces precisely what catch-up needs without bloating output. Other modes opt-in via the transform's argument.

### Depth

- **1 level (refs only)** — what's proposed.
- Multi-level (refs of refs) — creates trees, expensive to render and read.

Recommendation: 1 level. Use `sdd show <id>` if a deeper chain is wanted; the view pipeline shouldn't try to be a chain explorer.

### Direction

- **Outgoing (`refs`) only** — what this entry references. Solves the immediate temporal-blur problem.
- Downstream (`refd-by`, `closed-by`, `superseded-by`) — what references this entry. Useful for a different question ("who has acted on this gap?").
- Both — symmetrical.

Recommendation: outgoing first. Downstream could ship later as a separate transform or as a flag on the same transform.

### Line shape

- **Minimal** — `→ refs <id> {status: ...}`. Two-component.
- Full — `→ refs <id> <kind> {status: ...}: "<summary>"`. Matches `sdd show` shape, verbose.

Recommendation: minimal in `sdd view`; the agent can pivot to `sdd show` if a ref needs full chain expansion. Optionally allow a `:summary` modifier on the transform if a use case needs it.

### Grammar

- `refs(state-changed)` — transform with optional filter arg.
- `refs()` — bare, uses default filter.
- Composes with other transforms (`expand(involvement)` lives at the same position in the pipeline).

## Composition examples

```bash
# Catch-up's Active and hot, with state-changed ref state surfaced
sdd view --layout='kind(plan,activity):active:rank(heat(exp-7d)):n(8):refs(state-changed):as-list'

# Catch-up's Recent done, with all refs (what each done closed)
sdd view --layout='kind(done):rank(by(date)):n(10):refs(all):as-list'

# Open gaps with refs showing what they're waiting on
sdd view --layout='kind(gap):active:refs(state-changed):as-list'
```

## Why this is structural, not catch-up-specific

The catch-up sub-skill's stated value is connecting entries — surfacing what wants the user's next action by reading state across the graph. The current rendering shows each entry in isolation; the agent infers relationships from summary prose. That works while the prose is fresh, but degrades fast as the graph evolves and dependencies close.

This is a sibling failure mode to summary-body drift (`s-cpt-tdp`): there, the stored summary diverges from the body's truth at generation time; here, the stored summary diverges from the *referenced entries'* current state as the graph moves. Both lose visibility because rendering treats stored text as current truth.

A refs transform in `sdd view` doesn't fix stored-summary drift, but it does mean ref-state drift can be surfaced by rendering rather than waiting for re-summarization to catch up.

## Adjacent work

- **`s-cpt-tdp`** — summary-body drift sibling concern. Both are about rendered text not reflecting current state, but address different parts of the pipeline.
- **`s-cpt-k7z`** — per-ref "why" annotations stored at capture time. Different concern: this gap is about *computed* state at read time. The two are complementary — `s-cpt-k7z` adds stored metadata, this gap adds derived metadata.
- **`s-cpt-zsd`** — heat algorithm should differentiate refs by qualifier. Reads from stored ref qualifiers (`s-cpt-k7z`); doesn't help with state visibility on its own.
- **`d-tac-uww`** (closed) — the view-pipeline plan this proposal extends.

## Out of scope for this gap

- Backfilling existing summary text to remove ref mentions. Refs in narrative are valuable; the issue is augmenting them with state, not removing them.
- Auto-rewriting catch-up's fetches to use the new transform. The `/sdd-catchup` sub-skill adopts it via a refinement directive once the transform exists.
- Multi-level walking (out of scope per the depth dimension above).
- Stored-summary regeneration on dependency change (the `s-cpt-tdp` cluster owns that).

## Suggested next step

A small directive on format and optionality (inline parenthetical on the ref line, optional inclusion, no backfill of existing entries), followed by a tactical plan covering parser scope across `sdd view` + `sdd list` + `sdd show` consistency, default filter selection, and a smoke test against the catch-up fetches. Heat-weighting integration (`s-cpt-zsd`) is a separate decision once stored qualifiers (`s-cpt-k7z`) exist.
