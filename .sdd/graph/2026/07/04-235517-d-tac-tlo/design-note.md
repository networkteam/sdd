# Design note: dispatch seeding, anchor contract, and the task class

Decided 2026-07-04 in dialogue (Christopher, Claude), resolving the evaluate→capture friction gap (20260704-201525-s-tac-g7f): ~11 minutes and ~10 tool calls to capture one already-articulated finding, driven by two redundant widen passes and two rejected starts from inconsistent anchor contracts.

## The abstraction

**One primitive — the dispatch edge.** Every procedure instance starts either at the session door (shell) or dispatched from a parent instance. The edge carries *seeds down* (params, declared evidence) and a *result up* (a briefing for explore, a written entry for capture). The evidence handoff is not a special mechanism — it is what flows down the edge; a task's briefing is the same thing flowing up.

**One axis — who the procedure talks to.** Everything else derives from it:

| class | talks to | start contract | surfacing | context |
|---|---|---|---|---|
| shell | user | auto-started at session door | is the session | session |
| move | user | may start unresolved (`anchorHint`) — a user is present to resolve with | enumerated, junction-offerable | session |
| task | its dispatcher | required resolved params — no user present | excluded from enumeration and junction offers | disposable fork preferred (hint), inline as paid fallback |

## Evidence handoff: options weighed

**A — engine-seeded inheritance (chosen, with explicit declaration).** The dispatching junction option declares what it seeds (e.g. evaluate's record option seeds `anchor` + `widenReport` into the spawned capture). Copy-at-start; child self-contained.

**B — gate relaxation (`hasWidenReport or parent-has-one`) (rejected).** Axes that decided it:

- **Resume fidelity**: A keeps the evidence in the child's event log — a parked capture resumed in a fresh context sees what was already grounded. B leaves the child's log empty; a resuming agent re-runs the widen, resurrecting the redundancy at resume time.
- **Engine machinery**: A is a one-time copy inside the existing params-seed-state shape. B needs cross-instance predicates — parent state loadable at every child transition evaluation.
- **Evidence trail**: under B, spawned captures systematically show no grounding record; every audit needs the "look at the parent" rule. Under A the record is where readers expect it, marked inherited.
- **Gate integrity**: seeding satisfies the gate, so for spawned captures it becomes advisory in effect (satisficing window per 20260702-172621-s-cpt-86s) — but B has the identical window with less of a record. A = hard gate + truthful provenance; the drift window is narrow because the finding was articulated from the parent's real widen.
- **Generalization**: groom/implementation/interview parents carry differently-named evidence fields; B needs per-parent predicate logic, A forces one declared convention.

**Anchor seed is default-with-override**: live evidence from this very gap — captured from evaluating the session-architecture done (s-tac-fyt) but correctly `surfaced-by` the plan (d-tac-ry0). Anchor is orientation context, never a forced ref.

## Anchor contract

Rejected: anchor as required param everywhere (kills resolve-in-dialogue; pushes resolution outside the recorded move) and recency fallbacks ("the last thing we shipped" — ambiguous the moment several participants ship concurrently). Chosen: dual seed — `anchor` (resolved) or `anchorHint` (user's words, preserved verbatim across the dispatch boundary); resolver step gates on resolved anchor only; hint feeds the resolution dialogue, never satisfies the gate.

## Class name

"mission" (unintuitive), "tool" (collides with MCP tool vocabulary — "the explore tool" implies a server tool named explore), "sub-move" (collides with the lineage sense: a capture dispatched from evaluate is a nested move yet fully user-facing; nesting is orthogonal to class) — all rejected. **task**: moves are what the user engages in; tasks are what moves delegate.

## Concrete runs played out

1. **Friction case replayed**: shell → evaluate with `anchorHint: "the session architecture work"` → resolver widens, user picks s-tac-fyt → assess (one widen, where it belongs) → junction → capture dispatched with seeded widenReport + anchor overridden to d-tac-ry0 → assemble gate satisfied on entry, draft from substance on the table, playback → write. One widen, zero rejected starts, capture ≈ 3–4 calls.
2. **Engage dispatching explore**: engage starts with seeded anchor + goal, resolver auto-advances; chains fan wide; agent forks a sub-agent that runs `start_procedure(explore, {targets, goal}, parent: engage)`; only the briefing returns; parent cites the task instance's evidence in its own widenReport (textual up-edge).
3. **Cold capture**: no parent, nothing seeded, fresh widen demanded — gate semantics unchanged.

## Deferred

- **Structured up-edge** (task results seeding parent state): textual citation suffices; failure modes (inline reading for small neighborhoods, thin widenReports) are either graceful or pre-existing under the split-enforcement posture (s-cpt-1dz). Watch in dogfooding: briefs citing no evidence; explore never reached for on huge chains.
- **Findings-as-draft seed**: the junction's `selectedFindings` seeding capture's draft material rides the same declared-handoff mechanism when wanted.
