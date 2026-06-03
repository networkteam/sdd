# Grounding-skip-at-handoff — evaluation record (s-prc-u89)

Same scenario each run: a fresh `/sdd` session runs the catch-up briefing, then receives the two-point "project workflow" message (inbox processing + SDD-as-building-blocks) — a question that can only be answered well by grounding in graph facts the catch-up didn't name.

## Before the fix — failure
The agent asserted graph connections and a wrong "year-old" date from the catch-up summaries and recall, without opening any entries. It searched only after an explicit push ("do your homework and search"). Confident, unverified synthesis.

## Handback reset + grounding principle (show + search bundled together) — partial
The agent grounded before synthesizing — it opened entries first instead of asserting from summaries, a real improvement. But it read only the entries the catch-up had named (confirming what it already spotted) and skipped the proactive widening search until pushed ("why didn't you vector search?"). Verify-the-named fired; discover-the-unnamed did not. This motivated the restructure.

## Widen → Inspect restructure — pass (two independent sessions)
With grounding stated as two named moves (Widen first and mandatory, then Inspect) and the redundant CLI mentions cut:

- Session 1: widened unprompted with three vector searches from different angles (transport/intake, building-blocks/MCP, adoption-alongside-tracker), inspected a bundled `sdd show`, then synthesized accurately (idea 2 largely already decided; idea 1's new edge = the transport+inbox shape; the adoption-as-transport reframe). Closed on a question, not an overclaim.
- Session 2: widened unprompted from four angles, inspected bundled IDs, and surfaced entries and a tension the earlier (pushed) analysis had missed — s-tac-6on, and the orchestration boundary (the workflow agent "drives the loop").
- Follow-up turn: on a later turn within an eval session, the agent widened again before answering — so the move holds across turns, not only at first contact.

## Read
The text-level restructure moved the needle decisively and held across both sessions plus a follow-up turn. n is small and it remains a text fix; the durable answer is structural (the gated write surface, d-cpt-313) if regression appears.
