# Pre-flight oscillation evidence — d-cpt-d34 capture session

This attachment documents the five pre-flight runs performed during capture of `d-cpt-d34` (superseding `d-cpt-thg`), showing how findings shifted non-monotonically across iterations.

## Session summary

| Run | Mode | High | Medium | Low | Result |
|-----|------|------|--------|-----|--------|
| 1 | dry-run | 0 | 2 | 2 | would pass |
| 2 | dry-run | 0 | 1 | 2 | would pass (different findings) |
| 3 | real | 2 | 3 | 1 | blocked |
| 4 | real | 2 | 3 | 1 | blocked (different highs) |
| 5 | real | — | — | — | JSON parse error (infra) |
| 6 | real + `--skip-preflight` | — | — | — | captured |

At no point did the validator produce a clean pass. The plan's substantive content (ACs, CQRS decomposition, cascade model, Participants block) was unchanged from the initial dry-run through the final skip — all wording adjustments across runs 2–5 were attempts to quiet findings.

## Per-run findings

### Run 1 — initial dry-run

- `[medium] supersede-scope-refinement`: opening should name all three material additions equally, not just two.
- `[medium] ac-expansion-vs-supersede`: AC count growth (stated as 12→18, actually 15→18) should be acknowledged in preamble.
- `[low] cqrs-decomposition-brevity`: new role-status finder should be called out explicitly as new.
- `[low] self-describing-opening`: no finding, stylistic note.

### Run 2 — after folding run-1 fixes

- `[medium] supersede-scope-refinement-underdocumented`: AC 7 refinement (current head vs historical canonical) should be named in opening summary.
- `[low] ac-granularity-expansion-pattern`: AC count growth deserves explicit naming of which ACs were split.
- `[low] future-work-scope-boundary`: informational, no finding.

**Observation 1**: run-1's findings were resolved; run-2 surfaced new ones. The `supersede-scope-refinement` finding in run 1 was about not naming all three material additions — fixed. Run 2's `supersede-scope-refinement-underdocumented` was about a different issue (AC 7 refinement naming). This is not convergence; it's new-issue generation.

### Run 3 — after folding run-2 fixes (first real submission)

- `[high] ac-coverage-regression`: claimed AC 7 dropped the refs-includes-entry-id requirement. **Misread** — AC 7 explicitly contains "AND `refs` includes that head actor's entry ID".
- `[high] contract-violation-d-cpt-ah1`: wanted explicit justification for placing cascade derivation in the finder layer (the superseded plan `d-cpt-thg` used the same framing without this finding).
- `[medium] supersede-silent-change`: asked about subtle wording shift ("free (typically shared...)" → "can be shared or changed"). Reverted to match d-cpt-thg's wording.
- `[medium] ac-scope-expansion`: wanted explicit unit-test-AC scope note (new scenarios).
- `[medium] reference-ambiguity`: wanted `d-cpt-kv1` either added to refs or reframed. It is downstream; refs are for upstream.
- `[low] prose-clarity`: stylistic.

**Observation 2**: the high on `ac-coverage-regression` was a direct misread — the AC contained the clause the validator claimed was missing. Confirmed by searching the AC text: "AND `refs` includes that head actor's entry ID" is present.

### Run 4 — after folding run-3 fixes

- `[high] supersede-scope-mismatch`: same claim as run-3's `ac-coverage-regression` under a different label — that cascade derivation is "incompatible with d-cpt-thg's literal requirement that refs include the current entry ID". **Misread** — cascade is derivation-time, the refs requirement is capture-time, and both are explicitly retained in AC 7.
- `[high] contract-violation`: same as run-3's `contract-violation-d-cpt-ah1` under a different framing — now about whether a plan can supersede a contract at all. `d-cpt-thg` used identical wording ("Supersedes the two-type kind contract") without triggering this finding.
- `[medium] ac-coverage-ambiguity`: wanted an explicit mapping table of old-AC→new-AC.
- `[medium] missing-rationale`: wanted explanation of how orphan-role check interacts with cascade (what scenario triggers it?). **Legitimate** — this was a genuine clarification gap and was folded in.
- `[medium] participant-introduction`: wanted `d-cpt-kv1` in refs again, despite run-3 addressing it.
- `[low] prose-clarity`: stylistic.

**Observation 3**: the same substantive concerns (AC 7 structure, contract supersession framing) returned under different labels. Addressing the labels doesn't make the concerns go away; the validator is pattern-matching on surface wording, not checking whether the claims are true.

### Run 5 — infrastructure error

`parsing pre-flight JSON: invalid character 'L' after object key:value pair`. LLM returned malformed JSON. Not content-related.

### Run 6 — `--skip-preflight`

Captured successfully as `20260423-195649-d-cpt-d34`.

## Pattern observations

1. **Non-monotonic finding sets.** Fixes at run N produce new findings at run N+1, not convergence. Six iterations produced six different finding sets with no reduction trend.

2. **Surface-wording matching.** Several "high" findings rested on surface claims about missing clauses that were present in the AC text — verifiable by substring search. The validator read the plan as prose without checking structural claims.

3. **Inherited-framing false positives.** Findings like `contract-violation` fired on framing identical to the superseded plan (`d-cpt-thg`), which had passed pre-flight at capture. The calibration treats the same words as blocking in one context and acceptable in another, with the difference being the supersede relationship — suggesting supersede transitions invite stricter scrutiny than either the source or target warranted on its own.

4. **Prose-clutter feedback loop.** Each iteration added defensive parentheticals, explicit restatements, and clarifying sections. Net effect: the plan accumulated prose that didn't improve substance, purely to shield against future misreads.

## Proposed calibration directions

Not a decision — candidate directions for dialogue:

1. **Convergence criterion**: if two consecutive runs produce disjoint finding sets on unchanged substance, the validator should down-weight or halt, treating oscillation as a signal the rubric is over-reaching rather than the plan being deficient.
2. **Structural verification**: findings that claim specific clauses are missing should be gated on substring/structural checks against the entry text, not purely LLM-prose-reading.
3. **Relaxed supersede rubric**: supersede transitions of plans should not hold the superseder to stricter standards than the original plan at its capture — if `d-cpt-thg` could say "supersedes contract d-cpt-ydf", the superseder should be able to say the same without contract-violation findings.
4. **Finding stability quota**: if a severity level (high, medium) is assigned and removed across iterations, treat its later reappearance under a different label as the same finding — one fix attempts one finding, not one label.

Any of these could be split into separate signals or combined; the core observation is that current calibration generates noise rather than shaping the plan.
