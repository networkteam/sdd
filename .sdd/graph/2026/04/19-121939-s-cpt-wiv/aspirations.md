# SDD's aspirations + sharpening the aspiration concept

Companion to `s-cpt-7we` (aspiration kind proposal). Applies the kind to SDD's own strategic direction, and sharpens what "aspiration" means operationally.

## Validation-shape test: contract vs aspiration

A concrete operational distinction between the two "never closes" decision kinds, sharpening the push/pull and constraint/attractor framings from `s-cpt-7we`:

- **Contract**: binary. "Does this comply?" — yes / no. Easy to validate per-decision. Violations produce binary findings.
- **Aspiration**: gradient. "How well does this align?" — degree, not pass / fail. Multiple aspirations coexist in tension; a decision is evaluated against the constellation, not a single star.

Calibration example: `d-stg-574` (agent-platform neutrality) reads binary — "does this add agent-specific hardcoding? yes/no." Contract. Whereas "did dialogue shape this decision?" doesn't resolve to yes/no — only "more" or "less". Aspiration.

The gradient property also explains why aspirations tolerate detours: a detour can still be strongly pulled by some aspirations while departing from the straightest path to others. What matters is the constellation evaluation, not single-star compliance.

## Pre-flight implication

- **Contract violations**: binary, high severity — comply or violated, block capture until resolved.
- **Aspiration checks**: gradient, low / medium severity — "this drifts away from aspiration X, worth dialoguing." Observations to surface, not gates that block. Binary escalation would be the wrong shape for gradient signal.

## Four candidate aspirations for SDD

Applying the aspiration kind (proposed in `s-cpt-7we`) to SDD's own strategic direction, four candidates surfaced. They form a constellation with one root and three branches.

### #4 (root): Dialogue shapes decisions

> **Dialogue shapes decisions.** Decisions emerge from multi-party dialogue — between humans, between humans and agents, between agents — not from retrieval or solo generation. Retrieval, automation, and tooling all serve the dialogue; they don't replace it.

This is the load-bearing positioning. Retrieval-first memory tools (MemPalace, Zep, Mem0, Letta per `s-stg-gtu`) surface context; no dialogue required. Spec / ticket systems record commitments; dialogue happens disconnected. Solo reasoning / autofill generates decisions without multi-party thinking. SDD makes dialogue the process by which decisions come to be.

**Operational test** for any new retrieval / automation feature: *does it produce candidates that flow into dialogue, or does it bypass dialogue?*

- `s-cpt-bn1` (SDD session mining) — passes: low-confidence candidates for human review.
- `s-cpt-5ox` (external conversation mining) — passes: same pattern, externally sourced.
- `s-cpt-hhc` (post-session dialogue review) — passes: retroactive signal surfacing, humans choose what to capture.
- Hypothetical "auto-capture from dialogue without review" — fails.

Aspiration #4 doesn't block retrieval-adjacent capabilities; it shapes how they integrate.

"Reasoning-first" (the framing in `s-stg-gtu`) is a *consequence* of dialogue shaping decisions — reasoning happens *in* dialogue. Pre-flight-as-dialogue, the catch-up / explore skills, dialogue-before-capture — all downstream of this root claim.

**Lifecycle:** when the aspiration kind lands, #4 closes `s-stg-gtu` (the articulation the signal called for).

### #1: Keep humans and AI agents aligned across parallel work

> Humans and AI agents working in parallel stay coherent with each other. Decisions, reasoning, and context flow across participants so nobody rediscovers what's already been worked out.

Unclaimed territory. Retrieval-first memory tools address one-human-one-agent session continuity. Spec / ticket systems address developer-team coordination. Alignment across parallel *human and agent* work, preserved through graph-mediated dialogue, is what nobody else is tackling.

**Branches from #4:** alignment happens *through* dialogue. Without dialogue shaping decisions, parallel work fragments into silos.

**Evidence:** `s-cpt-5hn` (closed positioning signal) carried this as "outcome alignment"; subsequently expressed in the README tagline via `a-cpt-1aa`.

### #2: Accessible to non-technical participants

> Non-developer participants engage with SDD directly, not through a developer intermediary. Chat, voice, and GUI surfaces are first-class; CLI is one interface among several.

Not folded into #1. Alignment *could* happen dev-only (pair programming with an agent). Reaching non-developer participants — Mara the business owner, Jun the roaster in the Kōgen story — without developer mediation is a distinct product choice.

**Branches from #4:** non-technical access *requires* dialogue as the interface (chat, voice), not CLI or static documents. Without dialogue being the primary motion, non-technical participants have no entry point.

**Evidence:** `s-stg-qg0` (adoption paths — retrofitting + single-feature use), `s-tac-7kh` (CLI barrier for non-technical users), `s-tac-bta` (sdd mirror → Obsidian for GUI access), plus Kōgen's deliberate staging of Mara and Jun using SDD through chat interfaces.

### #3: Overcome ceremony, bureaucracy, and parallel artifacts

> Reasoning lives in one place — the graph. No standups / planning rituals, no wikis / tickets / docs / specs as separate systems of record, no dashboards to keep in sync. Whatever the work produces IS the artifact.

Echoes SDD's existing "each fact lives in exactly one place" principle — this aspiration is what that principle pulls toward, made explicit.

**Branches from #4:** overcoming parallel artifacts is possible *because* dialogue-in-the-work replaces parallel record-keeping. If decisions don't emerge from dialogue, they have to be documented separately. If they do, the dialogue record IS the artifact.

**Attracts decisions like:** generate status views from the graph rather than maintain a dashboard; don't mirror entries into Notion; don't introduce sprint markers; new "roadmaps" become graph views, not parallel files.

**Evidence:** `d-stg-0gh` (framework founding — "operating without legacy process overhead"), plus recurring session-level pressure to add parallel artifacts that we've repeatedly declined.

## Aspiration framing is agnostic to rhetorical flavor

Aspirations can be expressed toward-something (vision) or away-from-something (transition). Both are the same kind, just different rhetorical angles on the same perpetual pull. Sometimes the negative framing carries more energy because the counter-example is vivid — everyone knows what Scrum overhead feels like; fewer can articulate its positive opposite crisply. That's a rhetorical choice, not a categorical one.

#1, #2, #4 read vision-shaped; #3 reads transition-shaped; all four function identically as aspirations.

## Constellation, not hierarchy-as-gate

Noting #4 as "root" and #1-#3 as "branches" is structural understanding — it explains why SDD has the shape it has — but operationally the four coexist as peers. Each guides different decisions. A decision is evaluated against the constellation (alignment to which stars, tension with which), not filtered through the root first.

## Not aspirations (flagged for clarity)

- **`d-stg-574`** (agent-platform neutrality): contract. "Does this add agent-specific hardcoding?" resolves binary. Constraint on design choices, not a direction to travel.

## Open for future decisions

- Formalizing these four as `kind: aspiration` entries once `d-cpt-omm` is superseded with aspiration-kind support.
- #4 closes `s-stg-gtu` once the kind lands (the positioning articulation it called for).
- Retro-labeling any other strategic decisions that evaluation reveals are aspiration-shaped.
