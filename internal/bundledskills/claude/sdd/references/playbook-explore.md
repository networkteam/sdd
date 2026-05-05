# Explore Playbook

When the user points at a graph entry to explore, invoke `/sdd-explore` with the target entry ID. Use the returned context to brief the user, then drive a dialogue toward handling the entry. The goal is always a graph change — not just understanding.

## Briefing

Present the entry in context:
- What is this entry about? (one paragraph synthesis from the full chain)
- What's its status? (open signal, active decision, closed, stale?)
- What's happened since? (downstream entries, if any)
- What's related? (entries the sub-skill flagged as connected)

Then ask the orienting question: **"What does this need?"**

## Playbook moves

These are patterns to recognize, not steps to follow. Read the situation and apply the right one:

**Open signal, no decisions addressing it** — Is this still relevant? If yes, what would a decision look like? Explore the signal's implications, challenge assumptions, and work toward a decision or close it as no longer relevant.

**Active decision, no done signal yet** — What would it take to close this? Does the decision need decomposition into sub-decisions first, or is it actionable as-is? Work toward defining the concrete work (and its done signal) that would fulfill it.

**Active decision, needs decomposition** — The decision is too broad to act on directly. Help the user break it into sub-decisions at a lower layer. Each sub-decision should be independently closable.

**Active decision, partial progress** — Some downstream done signals exist but the decision isn't closed. What's left? Are the remaining parts still needed? Work toward completing or adjusting scope.

**Tension between entries** — Two or more entries pull in different directions. Lay out the tension explicitly, explore both sides, and work toward a decision that resolves it.

**Stale entry** — Old entry with no downstream activity. Is it still relevant? Has the context changed? Either close it or revive it with fresh context.

**Signal resolved through dialogue, no implementation needed** — The discussion itself was the work. Don't create a phantom decision that will sit "active" with no done signal to close it. Capture a done signal that directly closes the gap (short-loop), summarizing the conclusion — but apply the smell test: if the narrative reads like a choice, capture the decision first.

**Enough decisions exist, ready to build** — The exploration reveals that sufficient decisions are in place for a scope of work. Surface this: "We have enough to start building. Here's the scope: [decisions]. Want to transition to implementation?"

## After exploration

Always end with concrete next steps: what was produced (new signals, decisions, closures), and what remains open.
