# Design idea: expand-then-engage flow for topic drill

## The problem

The catch-up sub-skill ends every briefing with a drill section:

```
Other open:

- A. <area>
- B. <area>
- C. <area>

Say *drill A* or *survey* for the full picture.

What do you want to move forward?
```

The "Say drill A" prompt invites the user to dig into a clustered area. But the sub-skill doesn't say what should happen after the user types it. In fresh-session testing on "drill into C", the agent fell through to engage-mode behavior directly:

1. Recognized C named two paired entries (`s-cpt-k7z` + `s-cpt-zsd`) from the still-loaded catchup data.
2. Read their chains via `sdd show --downstream`.
3. Produced a narrative engage brief with surrounding context, decision dimensions, candidate moves, and an orienting question.

That output is good — high-quality engage behavior on a pair of entries — but it's the wrong response to "drill". The directive `d-tac-qom` asks for the catch-up to *expand* a topic cluster into more entries via ref-graph traversal, so the user sees the landscape before choosing where to engage. The current behavior collapses two distinct steps (expand area → choose engage target) into one.

## Why the fall-through happens

The sub-skill specifies the "Say drill A or survey" string as render output of the closing block but contains no instruction for what the verb should do when triggered. The agent has no anchored mode to enter, so it picks the most general next move available: anchor on entries it can resolve, run the engage flow.

This is identical in shape to the temporal-blur issue surfaced separately (refs without derived status) — when the skill doesn't specify enough, the agent fills the gap with reasonable defaults that don't always match the design intent.

## What expand-then-engage should look like

A drill response should be lightweight expansion, not synthesis. Something close to:

```
**C — Heat scoring vs ref-type qualifiers**

Three entries in this area:

1. Ref metadata gap — per-ref 'why' annotations (`s-cpt-k7z`, April 15).
2. Heat algorithm weighting consequence (`s-cpt-zsd`, May 7).
3. Filter negation sibling — shipped as `not()` (`s-tac-x1n` → `d-tac-e1s`).

Which one do you want to engage on, or do you want a wider survey of this thread?
```

Key differences from engage:

- No chain reads (`sdd show --downstream`) — the cluster surfaces from already-loaded catchup data plus any obvious siblings.
- No decision dimensions, no candidate moves, no synthesis — those belong in engage.
- One question: which entry (or whether to widen).
- Voice register stays closer to catch-up (short numbered items, plain), not engage (narrative brief).

The user then types "engage 1" or "engage `s-cpt-k7z`" or just the ID, and *then* the engage flow runs — anchored, chain read, brief, offer moves.

## Design dimensions (need dialogue before plan capture)

### Where the drill lives

- **In `/sdd-catchup`** — extend the sub-skill with explicit drill behavior. Pro: keeps the catch-up flow self-contained. Con: bloats the sub-skill with more decision logic.
- A new sibling sub-skill (`/sdd-drill` or similar) — main `/sdd` routes "drill X" to it. Pro: separation of concerns matches the catch-up/engage split. Con: more skill surface.
- A reference file (`playbook-drill.md`) — loaded on demand when the user invokes the verb. Pro: lightweight. Con: requires the agent to recognize the verb and load the reference, less reliable than a skill route.

Recommendation: extend `/sdd-catchup` initially since the data needed is already in its context; consider sibling sub-skill if the drill behavior grows substantially.

### How the cluster is computed

- **From already-loaded catchup data** — agent matches the drill letter back to the cluster it implied at briefing time. Cheap, but bounded to what catchup fetched (n=15 on open-and-warm).
- Via a `sdd view --topic` call — agent triggers a topic-scoped re-fetch using the cluster's topic label. Requires topics to be assigned to the entries (the topic-backfill concern `s-cpt-sy4`).
- Hybrid — start from loaded data, optionally widen via search if the cluster name suggests entries beyond what loaded.

Recommendation: start from loaded data; the wider fetch is a follow-up once `s-cpt-sy4` lands the topic backfill.

### What the drill renders

- **Numbered list of entries with one-line each** — matches catchup register.
- Numbered list with `{status: ...}` and refs (if the `refs(...)` transform from the sibling gap lands).
- Compact narrative trail per entry like catchup's main items.

Recommendation: numbered list with one-line each, matching catchup's existing entry-line shape so the register stays consistent.

### Engagement trigger

- "engage N" / "engage `<id>`" — explicit verb.
- "N" alone — minimal, assumes engage intent.
- Numeric-only triggers engage; entry ID triggers engage; "expand X" widens further.

Recommendation: bare number triggers engage on that entry, since the drilled list itself names the engage option. Keep "survey" as the verb for the full open-and-warm dump.

### Closing the drill loop

- Drill always ends with a single question — "which one do you want to engage on, or wider?"
- Drill ends with the same `What do you want to move forward?` as catchup.

Recommendation: drill-specific question since the user's intent narrowed.

## Adjacent work

- **`d-tac-qom`** — the topic-drill directive this gap leaves open. Still applies; this gap proposes the resolution shape.
- **`s-cpt-sy4`** — mechanical topic aggregation + topic backfill. Drill expansion will eventually want this as its data source; can ship the drill flow first and adopt topic-based expansion later.
- **`s-prc-fc0`** — mode-table synthesis insight that drove the engage refactor; the drill vs. engage distinction is a sub-pattern of that broader mode-shape work.
- **Closing done signal of `d-tac-k4l`** — the build context where this gap surfaced.

## Suggested next step

A small directive on placement (extend `/sdd-catchup` vs. sibling skill) plus a tactical plan with ACs covering the expansion render shape, engagement trigger, source-of-data (catchup buffer vs. topic re-fetch), and an evaluation against a fresh-session paste test analogous to the catchup's own design phase.
