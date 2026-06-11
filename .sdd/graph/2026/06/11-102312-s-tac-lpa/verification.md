# Verification report — pre-flight ref-kind reliability (d-tac-tph)

## Per-AC evidence

1. **Pinned leak cases** — commit 8559bbf. Six cases: `RefMeta_AddressesOpenGap_NotHigh` (from the s-prc-l2d transcript), `RefMeta_BuildsOnTerminalDoneProvenance_NotHigh` (s-prc-2lm, covering the builds-on direction the prior pin missed), three advisory-precision cases from s-tac-4h7's findings (examples 1, 3, 4; example 2 skipped — its grounded-in argument is admissible under the current eight-kind vocabulary), and `Supersedes_PlanRestructure_NoHigh` from s-prc-vvd's per-run table. **Deviation:** pre-fix, the five ref-meta cases passed 3/3 on Sonnet — the captured leaks are model/context-dependent; the cases pin the contract. The supersede case reproduced the cross-run severity instability (high → absent → medium on identical input) on a true positive after a fixture layer/ID correction.
2. **N-run pass rates** — commit 8559bbf. Blocking tier 3/3, advisory tier 2/3, `SDD_EVAL_RUNS` rescales proportionally; infrastructure errors count as failed runs, not aborts.
3. **Reasoning-before-severity** — commits 398ea12 + cbb07b5. Final field order: observation, category, severity — after both Haiku (live) and Sonnet (eval) dropped a leading category field. Parser is field-order independent; both orders pinned in unit tests; schema order asserted for every check type.
4. **Applicability matrix** — commit 62ba68a. `model.RefKindApplicability`: ref kind × target class (live-decision / live-signal / terminal-done / retired, derived from kind + status). Inapplicable cells are only the documented-impossible ones: refines × terminal/retired, addresses × terminal-done. Soft cells stay applicable with notes — grounded empirically (the live graph holds accepted builds-on refs to 5 active/open targets). Mechanical check emits deterministic high with admissible alternatives; a refines stranded on a superseded target points at the live head.
5. **Prompt injection** — commits b25ab18 + 5dd5a80. Per-ref admissible-kind lines render from `ref_applicability.tmpl` into the user prompt; the template set parses once; system preamble byte-stable (unit-tested).
6. **Rubric narrowed** — commit b25ab18. LLM high = desc-vs-body contradiction only; kind mismatch = medium with quoted body anchor, low without; applicability never LLM-scored. Recalibrated `WrongKind` and `BuildsOnActiveSharpened` from high-expectation to evidence-medium.
7. **Successor directive** — d-prc-nfz captured through the new pipeline, superseding the never-medium rule while preserving body-derived kind selection.
8. **vvd pin + verdict** — supersede case 3/3 post-fix. Verdict: s-prc-vvd closed as resolved (oscillation mechanism structurally removed; misread-class highs did not reproduce; pinned case is the standing sentinel).
9. **Final eval + gate** — 29/29 (26 authoritative run + 3 on retry after eval-infra fixes: category matcher missed the model's `ref-desc-contradiction` tag on a correct high verdict, and two 120s timeouts on calls that pass alone — bumped to 240s). `go vet`, `go test -race` (19 packages), `golangci-lint` all clean.

## Regressions caught during verification

- **Empty-category findings**: after the initial reorder put category first, models skipped it — once on Haiku (live capture of d-prc-nfz, retry succeeded), once on Sonnet (eval, on an otherwise perfect evidence-backed medium). Fix: observation leads, all fields explicitly required (cbb07b5, 6276712).
- **Supersession displacement**: the builds-on/refines matrix notes offered only "next-step or refines" readings, so a replacement-shaped directive drew a well-formed evidence medium arguing refines instead of a supersession flag. Fix: both notes name overturned-commitments as supersession territory (cbb07b5); `GenuineSupersessionFlagged` passes.

## Observations for follow-up (not in scope)

- Dangling-mention check: body-typed entry IDs not among the entry's edges are mechanically detectable; live-graph scan found 6 of 31 ID-carrying entries with dangling mentions, one a hypothetical-example false positive — design direction (mechanical detection injected into the prompt, LLM verdict) agreed in dialogue, gap capture queued.
- `sdd view` cannot express ref-level queries (filter edges by ref kind and target status); raw grep was needed twice during this work. Gap capture queued.
- Live default model is Haiku while the eval measures Sonnet — the calibration eval does not cover the model most captures run on.
