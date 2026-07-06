# Interview procedure — design research record

Design session 2026-07-06 (Christopher, Claude), Slice B of 20260705-010751-d-tac-dbk. Christopher supplied the framing; the graph supplied prior art; presented consolidated in free dialogue (no option prompts) per mid-slice meta-feedback.

## What the graph already held

- 20260523-114322-d-prc-nkw (live head: 20260628-154629-d-prc-1ob) — the alignment-first architecture; interview as the heavy tool of the alignment check. Its attachment carries: the two question triggers (misalignment detected; gap detected), the question craft (open, short, each building on what the prior answer surfaced — the user's articulation shapes the next question, not the agent's framing), the pushback definition (grounded in graph or a new perspective, sharpens rather than confronts, anti-parrot), push-pull (pressure-test framings via opposites), and the rejected alternatives: menu-shaped offers ("menus force the user to scan") and yes/no-as-essence ("yes/no traps the user into confirming the agent's guess; open questions let the user say the actual thing").
- 20260607-024146-d-prc-3wk (live head: 20260628-154633-d-prc-e2y) — the interactivity standard: the user produces rather than skim-confirms; the agent surfaces its own assumptions for correction; alignment runs both directions.
- 20260702-174013-s-cpt-qs2 — evidence by construction: one question at a time; each question, the actual answer, and the resulting shift accumulate as the alignment trajectory, a by-product of doing the alignment well. Scaling rule: interview when stakes are high, light playback otherwise.
- 20260703-142504-s-cpt-m6m — the open resume-fidelity half: interview-shaped procedures may lose the reasoning between turns on resume. The transcript design targets exactly this; dogfooding this procedure should answer it.

## Christopher's framing (new, previously uncaptured)

- Journalist posture: the agent as a human interviewer researching an article, the user as the expert; think about *the one* question to ask to advance the story, with research behind it.
- Balanced background per question — some context, not too much; the user must be able to answer freely. Option-prompt tools push toward "just hitting some option without thinking".
- Push back in a nice way; think about things from different perspectives (adversarial), leading to a better design or solution.
- One question at a time — strict; one answer leads to the next question.
- Added in dialogue: ground each answer back in the graph before the next question — search whether it is already captured (to connect it) or contradicts something (to push back); each cycle does its homework in a loop until enough information is gathered and alignment feels sufficient.

## Tension resolved

The May worked example batched three questions "to give room for thinking". Today's direction — and the assumption s-cpt-qs2 already carried — is strictly one at a time. Settled: one-at-a-time is the word.

## Evidence from this slice

The implementation-procedure design interview earlier in this same slice ran on structured option prompts and drew meta-feedback: background must come before detailed questions (the counterpart does not hold the plan and spec entries in view or memorized), the pace was too slow, and free dialogue with richer per-turn presentation beats option prompts. Recorded in the d-tac-fzn interview record's process note; realized here as the background-balance and free-answer rules.
