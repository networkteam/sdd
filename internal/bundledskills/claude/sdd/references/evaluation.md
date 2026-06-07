# Evaluation

How to judge whether work is sound and whether it met its target. This is the single definition — reference it wherever evaluation happens; don't restate the lenses elsewhere.

## Two lenses — always both

- **Inner** — is it sound on its own terms: code, architecture, concepts.
- **Outer** — does it work in use: an end-to-end / smoke test, and does it serve the user and product (product and brand fit live here).

A pass on one is not a pass — reason about both together. Each finding worth keeping becomes a signal.

## Three points — not once

A single check at the end is not enough in practice. Evaluate at each point that applies:

1. **During implementation** — sanity-check each slice as you go (inner; a smoke test once it runs), so problems surface early instead of all at the gate.
2. **At the landing gate** — before a branch or worktree merges (or an in-place change closes), present the briefing the user merges on: which acceptance criteria are met and any deviation from the plan, the implicit decisions you made during the run, and both the inner and outer checks. Give your read — merge-ready, or hold and why — and the user has the final say. This is the accountability that balances an autonomous run.
3. **After landing** — the learning loop: did it meet the target, what gaps or surprises remain. Capture findings as signals against what you evaluated.
