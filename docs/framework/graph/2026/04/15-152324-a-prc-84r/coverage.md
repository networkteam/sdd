# Per-AC coverage for d-prc-g1h

- [x] **AC 1** (`PreflightResult`+`Severity`+`HasBlocking()`): `framework/sdd/query/preflight.go`, `framework/sdd/llm/preflight.go`.
- [~] **AC 2** (parser rejects unknown severities): **Deviation — JSON format**. Parser still rejects unknown severities; also rejects malformed JSON and empty category/observation. Reason: eval runs showed the bullet format was fragile (code fences, sentinel ambiguity, LLM commentary on identical inputs); dialogue confirmed JSON is cleaner.
- [~] **AC 3** (verdict.tmpl embedded in all 6 check templates): **Deviation on format** — describes JSON schema, not bullet format. Still semantic-only; no threshold/blocking language.
- [x] **AC 4** (verdict.tmpl contains calibration principles): contract calibration, immutability, unifying principle all present; also added dialogue-context-as-grounding as a fourth meta-principle.
- [x] **AC 5** (calibration blocks in all 6 templates): severity anchors drawn from the a-prc-dix corpus.
- [~] **AC 6** (closing_action.tmpl has conditional AC block): **Deviation** — rendered unconditionally with text "applies only when closed entry is kind: plan"; LLM applies contextually from `.ClosedEntries` kind. Reason: dropping the Go `{{ if }}` gate let us also drop the `ClosedIsPlan` context field.
- [~] **AC 7** (assembleContext extracts `## Acceptance criteria` section): **Deviation** — ACs moved inline into plan decision descriptions, no extraction. Reason: dialogue-driven simplification; embedding attachment content was token-heavy and the extraction plumbing was brittle.
- [~] **AC 8** (plan decisions include `## Acceptance criteria`): **Future plans** per updated SKILL.md (in description, not attachment). This plan (d-prc-g1h) is grandfathered — ACs in attachment under old spec.
- [x] **AC 9** (decision_refs.tmpl flags missing AC on kind:plan): template check #4 describes this; LLM applies contextually.
- [x] **AC 10** (closing_action.tmpl validates each AC with severity tiers): also refined to "reasoning presence, not quality" per eval-driven dialogue.
- [x] **AC 11** (handler_new_entry.go blocks only on HasBlocking(); displays all findings).
- [x] **AC 12** (/sdd SKILL.md guides plan AC capture): added to "Get the entry right" list.
- [x] **AC 13** (/sdd SKILL.md implementation mode reviews ACs): step 6 in "Transition to implementation".
- [x] **AC 14** (/sdd SKILL.md response to findings by severity): replaced binary PASS/FAIL rejection guidance.
- [~] **AC 15** (eval tests, one subtest per anti-pattern category) and **AC 16** (strict 13 FP / 16 TP criteria): **Deferred, dialogue-confirmed**. 8-test smoke suite shipped (4 closing-action variants: silent omission, named-without-reasoning, deviation-with-reasoning, full coverage; signal capture FP/TP; decision refs rendering; contract violation; real-graph history deferral). Decision to skip the full corpus expansion driven by: (a) the smoke suite validates core behavior across severities and all check types the plan identified; (b) pre-flight runs on every `sdd new`, so production usage surfaces new signals naturally — lazy expansion based on observed failures is preferable to speculative coverage; (c) the full corpus run is ~15–20 minutes of eval time + real API cost per invocation, a cost better absorbed incrementally. New regressions observed in use can motivate targeted subtests.

## Additional refinement (eval-driven, beyond the plan)

AC coverage calibration in closing_action.tmpl was clarified to **"reasoning presence, not quality"** — pre-flight checks that deviations carry reasoning, does not judge reasoning validity. Previously the silent-vs-ambiguous severity split had a gap where explicit-non-implementation-without-why fell through; LLM was inconsistent on identical input. Eval-driven dialogue confirmed the clarified rule stabilizes judgment across silent / named / deviated scenarios.
