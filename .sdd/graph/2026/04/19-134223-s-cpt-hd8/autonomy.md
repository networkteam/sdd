# Aspiration #5: Autonomy

Extends the four-aspiration set captured in `s-cpt-wiv`.

## The aspiration

> Human and AI participants make better autonomous tactical decisions because the graph holds direction, alignment, and contracts. Most decisions are made locally — by whoever has the domain knowledge — with confidence they'll cohere with the rest. Coordination happens through dialogue when needed, not through standing meetings or blocking approvals.

## Why a separate aspiration from #1

Related but distinct pulls:

- **#1** (alignment across parallel work) — the *coherence* pull: parallel work doesn't fragment.
- **#5** (autonomy) — the *independence* pull: participants don't block on each other, and make better local decisions.

A decision can serve one while hurting the other. Example: "require human approval before any agent capture" passes #4 (dialogue shapes decisions, humans in the loop) and arguably #1 (keeps humans aligned with agents) but HURTS #5 (agents can't act locally in their domains). Only #5 catches this.

Autonomy is also the motivating *value* the other aspirations serve. Why do we make dialogue shape decisions (#4)? So decisions carry enough context that participants can act locally. Why overcome ceremony (#3)? Because ceremony trades autonomy for coordination. The others are mechanisms; autonomy is the value they produce.

## The layer relationship

Load-bearing structural insight: the graph's value is the compression of higher-layer agreement into shared context that enables many lower-layer autonomous decisions.

- **Direction, aspirations, contracts** — collectively-shaped higher-layer content. Strategic and conceptual entries that describe where we're going and what rules hold.
- **Tactical / operational decisions** — made autonomously against that context, by individual participants with domain knowledge.

Without clear higher-layer content, tactical decisions either stall (waiting for clarification) or drift (made without shared context).

**Kōgen story evidence:**

- Priya picks Stripe alone — the strategic direction and tactical shape are already clear
- Mara decides shipping pricing alone — within the subscription framing already decided
- Jun decides share-note behavior alone — within the "discovery experience" contract
- The implementation agent picks hosted vs embedded Stripe alone — within the tactical scope already agreed

None of these required synchronous coordination. Each participant had enough higher-layer context to decide locally with confidence.

## Autonomy is for humans AND agents

Agents are not tools within human autonomy. They are autonomous participants in their own domains.

When the implementation agent in Kōgen decides to use hosted Stripe checkout (after embedded fails for gift shipping), that's an autonomous tactical decision by the agent, exercising the same kind of autonomy Priya exercises when she picks magic-link auth. The graph's shared context enables both.

This matters for how SDD is designed. Agent-as-autonomous-participant implies:

- Agents capture signals from their work (as humans do)
- Agents make tactical decisions in their domain (as humans do)
- Agents flag when a decision exceeds their domain (escalation, not refusal)
- Agents participate in dialogue, not just execute instructions

Retrieval-first tools position agents as memory-augmented executors. SDD positions agents as participants.

## Aspiration test

- **Gradient**: "How much does this decision enable better local decision-making?" — yes, with degrees
- **Not check-off-able**: autonomy is a perpetual direction — there's always more higher-layer context the graph could carry, always more local decisions enabled by better higher-layer content
- **Catches unique failures**: "require human approval before any agent capture" passes #4 / #1 but fails #5

## Constellation position

With #5 added:

- **#4** (dialogue shapes decisions) — *mechanism*: the process by which decisions come to be
- **#5** (autonomy) — *value*: why the mechanism matters; what better local decisions produce
- **#1** (alignment across parallel work) — *outcome*: coherence despite autonomous motion
- **#2** (accessible to non-technical) — *reach*: who gets to participate
- **#3** (overcome ceremony / parallel artifacts) — *direction*: what we're moving away from

Structurally, #4 and #5 could both read as root. #4 is the process root; #5 is the value root. #1-#3 are outcomes and constraints on the system they produce together.

For evaluation purposes the distinction doesn't matter — they're five peers in the constellation. But naming the structural shape helps explain why SDD looks the way it does.

## Lifecycle

When the aspiration kind lands (via `d-cpt-omm` supersede, per `d-cpt-xqt`), #5 becomes a `kind: aspiration` entry alongside the other four. This signal is picked up as evidence for that plan.
