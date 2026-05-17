# Plan: drill expand-first behavior + `refs-of(...)` view filter

## What this plan delivers

A focused-catchup behavior for the drill verb in `/sdd-catchup`, plus the `sdd view` filter primitive that makes ref-graph expansion composable in the view pipeline.

The drill verb today is undefined — the catchup render names it ("Say drill A") but doesn't specify behavior, so the agent defaults to engage-mode on inferred entries. The eval surfaced this as good work in the wrong shape: the user said "show me C" and got synthesis, when they expected expansion.

This plan defines drill as a focused catch-up: same voice, same format, same register — just scoped to one cluster's neighborhood. After drilling, the user can drill another cluster, engage on a specific entry, or just keep talking. Drill never auto-transitions to engage.

## Mechanism composition

Three parallel expansion mechanisms, run on every drill invocation:

### 1. Refs of seeds (graph mechanism)

The seeds are the entries the agent inferred for the picked cluster (from loaded catchup data). Outgoing refs of those seeds — 1 hop — show the explicit chain: what the cluster depends on or builds on.

Today this has no CLI surface. `sdd show <id>` returns the full chain but with depth-tree rendering; we need a flat filter. This plan adds `refs-of(id1[,id2...])` to the view pipeline.

### 2. Semantic search on cluster phrase

`sdd search --query "<cluster label>"` — the cluster's agent-named label (e.g. "Heat scoring vs ref-type qualifiers") feeds the query. Returns ranked entries with citations; we take the top ~5.

This finds semantic neighbors the seeds don't ref but that share the concept. Vector mode requires the embedding provider to be configured (per `sdd info`'s Search line) — falls back gracefully to text mode otherwise.

### 3. Topic-filter on cluster label (silent today)

`sdd view --layout='topic("<label>"):active:rank(heat(exp-14d)):n(10):as-list'` — uses the topic primitive added in the 7+7 type system (`d-cpt-ni0`). Returns active entries whose effective topic set has the label as a prefix.

Today this returns ~nothing on this graph because no entries are annotated. Lights up as `s-cpt-sy4`'s annotation backfill lands. Wires it now so drill is ready when the backfill arrives.

### Merge and cap

Union by entry ID. Dedupe seeds back out (they're definitionally in the cluster). Cap total at ~10 entries. Order: seeds first (labeled), then merged neighbors ranked by whatever falls naturally from the source ordering — first appearance wins.

The agent renders the result in catchup register (numbered items, short trails, story-arc-style header naming the cluster). Re-applies the catch-up voice rules and format rules verbatim.

## Alternatives considered

### Refs expansion — three options

1. **Agent walks `sdd show <id>` for each seed** — works today, N+1 shell calls per drill. Faster to ship, slower at runtime. Couples drill's behavior to the agent's shell-call discipline.
2. **Add `refs-of(...)` to `sdd view`** — clean, reusable, composes with existing filters. Extends the pipeline grammar by one primitive. *Chosen.*
3. **Defer refs mechanism; ship topic + search only** — narrower plan, but refs is the most direct expansion path and would have to come anyway. Punting just defers the same work.

Choice rationale: option 2 is the structural fit. The view pipeline is the right home for graph-traversal filters. Bundling the filter into this plan makes drill's expansion cleanly composable from day one.

### Direction — outgoing only vs. bidirectional

Outgoing refs (`refs-of(id)` returns entries that `id` refs) is what drill primarily needs — show what the cluster's seeds depend on or build on. Downstream (`refd-by(id)` returns entries that ref `id`) is a different question — "who has acted on this gap?" — useful but out of scope here.

This plan ships outgoing only. A `refd-by(...)` companion filter could come later via a refinement directive if a use case surfaces.

### Composition semantics

`refs-of(id1,id2)` returns entries refed by ANY of the given IDs (union), not just those refed by ALL (intersection). Reasoning: drill seeds are alternates within a cluster, not a conjunction — a neighbor of any seed is in the cluster's neighborhood.

Composition with other filters intersects per existing pipeline semantics (`active:refs-of(...)` = entries that are both active AND refed by a seed).

## Slicing

### Slice 1 — `refs-of(...)` filter

- Model: nothing new (uses existing graph).
- Query: extend the layout parser to recognize `refs-of(...)` with comma-separated IDs.
- Finder: pure function over `*model.Graph` — given a seed-ID set, return entries that any seed's `Refs` field includes (resolves short IDs via existing mechanism).
- View executor: wire the new filter into the intersection chain.
- CLI: nothing new; the filter is available in `--layout=`.
- Tests: single seed, multi-seed union, composition with `active`/`kind`, empty-result behavior (unknown seed ID — error vs. empty? Probably error since unknown short IDs already error elsewhere).
- Docs: `cli-reference.md` documents the filter in the filter table.

### Slice 2 — `/sdd-catchup` drill behavior

- Edit `internal/bundledskills/claude/sdd-catchup/SKILL.md`:
  - Add a "Drill behavior" section after the existing fetch/render blocks.
  - Specify seed identification (recognize drill verb, match cluster letter to entries from the most recent catchup output in context).
  - Specify the three-mechanism expansion with verbatim layout strings the agent runs.
  - Specify merge/dedupe/cap.
  - Specify render rules (catchup register, seeds labeled).
- Drop the "or survey for the full picture" line from the render template.
- Rebuild + reinstall.

### Slice 3 — Evaluation

Fresh-session paste-test analogous to the catchup's design phase. Three scenarios:
1. Drill into a clearly labeled cluster (similar to "drill into C" in `s-tac-7uy`'s eval).
2. Drill into a phrase that doesn't match any labeled cluster — verify the search-fallback path.
3. Drill, then drill again — verify the sub-skill stays loaded and ready.

Closing done signal records voice fidelity, format adherence, expansion quality (do the surfaced neighbors actually belong?), and wall-clock cost.

## Implementation notes that may surface as augments

- **Layout-string composition in skill** — the agent runs three layout strings per drill. Embedding them verbatim in SKILL.md avoids inference burden but couples the skill to current CLI grammar. If the grammar shifts (e.g. layout-builder helpers in a future view rev), the skill needs follow-up.
- **Seed-ID resolution from rendered output** — the agent has to match a cluster letter back to entries in the rendered catchup. This works today (eval session 3 demonstrated) but is implicit. May benefit from a more explicit "drill index" line in the catchup output if reliability slips at scale.
- **Wall-clock cost** — three shell calls per drill (one per mechanism) plus an LLM pass. Target: under 60s, matching catchup's own envelope.

## Out of scope

- A `refd-by(...)` companion filter for the inverse direction. Could ship as a refinement directive if a use case surfaces.
- Topic-source labels in the "Other open" section of catchup (replacing agent inference with mechanical topic-aggregation). That's `s-cpt-sy4`'s domain.
- Mechanisms for users to *select* engage from drill output (e.g. "engage 3"). Engage stays a separate move invoked explicitly by the user; drill output's numbered items are informational.
- Deeper-than-1-hop refs traversal. A parameter could be added later as a refinement.
- Re-running catchup automatically when drill seeds look stale.
