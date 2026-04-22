# Pre-flight participant validation extension

## Context

The participant identity enforcement feature (s-tac-cbu) shipped 2026-04-22 and was immediately evaluated against its first real post-landing capture. Two done signals (s-tac-x9m, s-tac-d3u) slipped through with "Christopher Hlubek" instead of canonical "Christopher" — the feature designed to catch this exact class of drift missed it on first use.

Evaluation captured in gap signal s-prc-qom. Drifted entries mechanically corrected in place per d-cpt-e1i (commit 9384c30) — 11 entries normalized to canonical spelling without supersession.

## Failure modes

**Validator-side**: `participantDriftFindings` (`internal/finders/preflight.go:45`) does exact-string match against `graph.AllParticipants()`. That reference set includes same-session drift — today's morning aspirations had already introduced "Christopher Hlubek" before the feature shipped, so the variant registered as legitimate. The config canonical `Christopher` is ground truth for the local participant but never reaches the check.

**Agent-side**: When composing `--participants` with Claude as an additional voice, the agent pattern-matched on today's `sdd status` output (where the drift was prominently visible at the top) rather than consulting config or the longer-established spelling — despite explicit skill guidance to prefer established spellings. Another instance of the instruction-drift cluster (s-stg-3vr, s-prc-w5r, s-prc-cgk, s-prc-nck) where text rules alone can't control agent behavior.

## Design

Two-sided structural fix, both aligned to d-cpt-vt1 (structural separation):

**Validator-side** — move the check into the LLM layer with explicit inputs:

- `LocalCanonical` (from `.sdd/config.local.yaml`'s `participant`)
- `EstablishedParticipants` (from `graph.AllParticipants()`)
- Entry content (participants + description narrative)

The LLM judges each proposed participant against both reference sets and emits one `participant-drift` finding per unmatched name.

**Agent-side** — surface `LocalCanonical` in `sdd status` output as a header line above the graph summary. Agents read status at check-in; ground truth rides along automatically into context, eliminating the need to infer from recent entries.

## Severity model: binary, defaulting to high

Early drafts proposed a three-way split (high for known-name variants, medium for totally novel unmatched names, pass for narrative-introduced voices). Dialogue narrowed this: cases like "meeting transcript included Chris Hlubek" are ambiguous — the transcript could be drift or could be a legitimate new voice. Medium is the wrong middle ground because ambiguity should default to block, not advisory.

Final model:

- **Pass (no finding)**: exact match against `LocalCanonical` or `EstablishedParticipants`, OR narrative explicitly introduces the name as a new participant joining the graph
- **High (block)**: everything else — variant of known name, fully novel name not introduced, or name merely mentioned without explicit new-voice framing
- **Bootstrap exception**: empty `EstablishedParticipants` → no finding (can't drift from nothing; preserves first-entry capture behavior)

## Calibration rubric (template text)

> A participant not in the established set is acceptable only if the description **explicitly introduces them as a new voice joining the graph**.
>
> Qualifying phrases (pass): "A new colleague X joined the project…", "Introducing X as a new participant…", "Today X began contributing…"
>
> Non-qualifying (high): "X attended the meeting", "X was present", "Transcript included X", "X mentioned that…" — these describe observation, not graph participation, and are indistinguishable from misattributed drift.
>
> When in doubt, emit high.

Exemplar variant chain: Christopher / Christopher Hlubek / Christopher H. / Chris Hlubek / Mr. Hlubek — same person, different forms. All variants other than the canonical should emit high when appearing in `--participants` without explicit introduction. Same pattern applies to Claude variants (" Claude" with leading space, etc.).

## Alternatives considered

**Option 2: keep deterministic check + layer LLM on top.** Keep the existing exact-match Go check as a fast pre-filter; only invoke LLM for names that don't exact-match. Pros: cheaper, partially deterministic. Cons: two code paths doing overlapping work invite drift between them; the LLM check already runs for every capture anyway (other pre-flight categories), so the marginal cost of one more template section is small. **Rejected** in favor of single-path LLM check.

**Historical-distribution-based drift detection.** Cluster participant names across all entries, flag minority variants of the same person regardless of config. More general than config-canonical-based detection (catches drift for any participant, not just the local one), but heuristic — needs same-person clustering via edit distance or similar, with false-positive risk. **Deferred** as a possible refinement. The config-canonical approach covers the dominant case (solo-plus-AI graphs) and is deterministic.

**Additive flag semantics and CLI sugar.** `--participants` merging with config (rather than overriding verbatim), or a `--with-claude` flag. **Rejected**: changes the explicit-override semantic chosen in d-tac-ghr, and narrow sugar doesn't compose well with multi-participant cases.

## Open implementation questions

- **Template placement**: new dedicated template file (e.g. `participants.tmpl`) vs extension of an existing per-kind template. Leaning new file for separation, but existing templates may already consume participants implicitly.
- **Bootstrap handling**: pure Go pre-filter (skip LLM invocation entirely for the participant check when `EstablishedParticipants` is empty) vs letting the template handle it via the rubric. Go pre-filter saves a round-trip for first-entry captures.
- **Mocking strategy for regression tests**: LLM-layer behavior is non-deterministic in principle. Options: (a) mock the LLM runner with canned responses per template input, (b) run against a cheap local model (e.g. gemma) as an integration test, (c) run against the configured production provider as a slow integration test gated behind a build tag.

## References

- s-prc-qom — the gap signal this plan closes
- s-tac-cbu — the feature being extended (participant identity enforcement landing)
- s-tac-x9m, s-tac-d3u — observed drift entries (mechanically corrected in commit 9384c30)
- s-stg-3vr — instruction-drift cluster (structural-enforcement rationale)
- d-cpt-vt1 — structural-separation contract
- d-cpt-ah1 — CQRS planning contract
- d-cpt-e1i — immutability refinement (permits mechanical identity-preserving fixes)
