# Evaluation model — design note

## The two lenses (generic)
- **Inner = verification** — is the work sound, judged against the project's own guidelines?
- **Outer = validation** — is it the *right thing*: works in use, serves the people it's for?

Verification ("built it right") vs validation ("built the right thing") is the classic split; "did we build the right thing?" is the heart of the outer lens.

## Why generic, not code-specific
SDD is a generic process — projects may be code, docs, design, user stories. What each lens concretely checks is a property of the work type, which the project defines in its own guidance (`AGENTS.md`/`CLAUDE.md`) and, later, rules deliver per-work-type checks. The framework bakes in no code assumption. This is why `evaluation.md`'s "code, architecture, concepts" / "smoke test" phrasing is wrong and gets corrected.

## Evaluation is real work
Between reading the chain and recording there is actual evaluation work: inspecting against the guidelines and running/using the thing (host work the agent does), plus human feedback where judgment is needed. Part-automated, part-human. A human attestation counts as evidence — the durability rubric already accepts attestations (s-prc-4pm).

## Per-lens, partial, multi-participant
An evaluation may cover one lens or both; different participants, roles, or runs cover the rest. So "a pass on one is not a pass" is a coverage property across the graph — computed over the evaluation-dones referencing a work entry — not a gate inside one run.

## Three points, not once
`evaluation.md`'s "three points — not once" (during implementation, at the landing gate, after landing) stands; this model refines *what* the lenses mean and *how* coverage accumulates, not *when* to evaluate.

## Realization
- Engine evaluate procedure: scope (identify what to check, per guidelines) then carry-out (do it, agent + human) then record (evaluation-as-done + findings); gate at least one lens.
- `evaluation.md` corrected to the generic verification/validation framing.

## Rejected
- **Forcing both lenses in one evaluation** — collapses the coverage axis (cannot tell inner-done-outer-pending from both-done) and blocks the partial/multi-participant case.
- **Baking code-specific lens meaning into the framework** — breaks genericity; that belongs in project guidance + rules.