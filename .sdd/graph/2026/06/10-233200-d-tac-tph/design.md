# Pre-flight ref-kind reliability — design notes

## Failure-mode taxonomy (post-recalibration leaks of d-prc-v0h)

Three distinct axes, four instances in six days:

1. **Cross-run oscillation** (s-prc-pex, s-prc-l2d run 2): identical ref-meta input, `[high]` one run, `[low]` recommending the opposite kind the next.
2. **Intra-run self-retraction** (s-prc-lbv, s-prc-l2d run 1): a blocking `[high]` finding whose own observation prose concludes "Retracting — this is not a finding," yet still counts toward the blocking verdict.
3. **Confidently-wrong single-run highs** (s-prc-2lm): internally coherent prose that names the applicable kinds correctly while categorizing a documented defensible choice (`builds-on` on a terminal done) as a precondition violation. Not catchable by retraction detection; evades the existing eval pin, which guards only the `addresses` direction of the terminal-done shape.

The synthesis insight (s-prc-xr9) diagnoses the pattern as a non-converging patch loop: five calibration rounds since April each shipped a real fix, and each leaked within days. Rubric text is not the bottleneck — structure is.

## Root mechanism found in code

`verdict.tmpl` defines the finding schema as `{"severity", "category", "observation"}`. Under autoregressive generation the model commits the severity token before writing its reasoning — self-retraction is exactly what that field order produces. Fix: reorder so reasoning precedes severity; severity becomes a conclusion, not an opening bid. Applies to all check types, not just ref-meta.

## Applicability matrix

The applicable/inapplicable determination for a ref kind is a lookup, not a judgment: ref kind × target kind × target derived status → applicable / inapplicable, plus a per-cell note carrying nuance (e.g. "defensible alongside grounded-in on a terminal done — tie-break is author's call").

Declared once in Go, serving three consumers:

1. **Mechanical check**: a chosen kind that is inapplicable yields a deterministic high finding with no LLM call — same pattern as participant-drift, which already moved from LLM to mechanical. No variance possible.
2. **Per-ref user-prompt fragment**: "ref → target x (closed): admissible kinds are builds-on, grounded-in, surfaces; the chosen kind is admissible — applicability is settled, do not flag it." Per-capture content lands in the user block; the system prompt stays byte-stable (preserving the cache restructure from s-tac-sk1).
3. **Optionally** the canonical vocabulary table (build-time static, still cache-safe) — settles the applicability half of the definition-drift question (s-tac-uer).

Edge discipline: preconditions key on the target (kind + derived status). Soft cells ("builds-on on an active target as a forward next-step") are applicable-with-note, never inapplicable. Only genuinely impossible cells (refines on a closed target, addresses on a terminal done) go inapplicable.

## What remains for the LLM on ref kinds

- **Kind-vs-body fit among admissible kinds** — body says "gated on X landing first," ref says `related`; `depends-on` names it better. Semantic, stays LLM, advisory tier.
- **Desc-vs-body contradiction** — desc claims "extends the predecessor" while the body retires it. Stays LLM, may remain high (the body refuting its own metadata, not a vocabulary judgment).

## Advisory-tier calibration: evidence-gated medium

History: sharpness findings were medium, demoted to low by d-prc-2is after measurement showed zero real errors caught and vocabulary-contradicting suggestions (s-tac-4h7). Two things changed: the matrix constrains suggestions to admissible kinds (killing the contradiction class), and live use showed a low-only cap makes the advisory ignorable ("it's low, ignore"), removing all effect.

Revised rule: medium only when the finding cites the body phrase supporting the other admissible kind; no quotable anchor → low. Falsifiable (eval can check whether the cited phrase supports the suggested kind) and ratchetable in both directions — if post-matrix mediums regress to noise, demote again with the pinned corpus as proof.

## Alternatives rejected

- **Post-hoc mechanical cap** (code overrides LLM highs on applicable kinds after the fact): validates then retracts mechanically; inverted into the matrix design, where code answers applicability before the LLM sees the ref.
- **Post-parse withdrawal guard** (scan observation text for retraction language, demote): brittle text heuristic; the failure class it guards has no remaining path once severity follows reasoning and applicability is mechanical.
- **Chasing model determinism** (temperature 0, N-run agreement at capture time): already rejected by d-prc-v0h — variance is inherent to fuzzy judgment; the fix is to stop gating creation on it. N-run is used in the eval harness only, as measurement.

## Eval discipline

Regression-first order: pin the verbatim transcripts attached to s-prc-l2d and s-prc-2lm (both directions of the terminal-done shape), the debatable-medium findings attached to s-tac-4h7 (advisory precision), and a supersede-check oscillation case from s-prc-vvd's per-run findings table — all reproducing pre-fix before any behavior changes. Harness gains per-case N-run pass-rate assertions: blocking-tier cases at stricter rates than advisory-tier. Live captures stop being the discovery surface and become the regression surface (s-prc-xr9 proposal 3).

## Follow-up dispositions bound into ACs

- d-prc-2is: superseded at ship time by a successor directive carrying the evidence-gated medium while preserving body-derived kind selection.
- s-prc-vvd: pinned and measured; closed by the closing done signal if the schema reorder resolved the supersede-check oscillation, otherwise re-scoped in a fresh entry naming what remains.
