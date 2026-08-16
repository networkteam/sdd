# Research brief — the question signal kind

## A. Semantics
- A1: An acknowledged unknown — "not an observation, not a commitment — a marker of uncertainty" (d-cpt-omm two-type-design attachment).
- A2: "Acknowledged unknown requiring resolution", drives work by awaiting resolution (same).
- A3: Its question about itself: "What do we not know?" (overview.go kindQuestions — generated; do not restate).
- A4: A signal — noticed, not committed.
- A5: Open questions sit on the attention surface with gaps (OpenAttentionKinds = gap, question; OpenSignals doc: "gaps awaiting a decision/done and questions awaiting dissolution").
- A6: Facts/insights/dones deliberately excluded — the attention set is a deliberate two-kind allow-list.
- A7: Lifecycle — answered by knowledge (dissolution): "a fact or insight may close a question by answering it" (overview:57; validateCloses carve-out).
- A8: Lifecycle — resolved by a decision: "Questions are resolved by decisions (which answer them) or by facts/insights (which dissolve them)" (design attachment).
- A9: Superseded by a refined question (framework-concepts:136; live specimen s-prc-ggp → s-prc-9yt: "Refines the question of Git commit granularity … Supersedes the earlier formulation").
- A10: Won't-pursue close: "directive: 'answered as X' or 'won't pursue'" (framework-concepts:136; playbook-engage:41).
- A11: A question never closes anything itself (SignalCloseRule).
- A12/A13: Dissolution was designed in from the two-type redesign but mechanically blocked until 2026-06-17 (gap s-tac-7nz → done s-tac-4j7 relaxed validateCloses); "the only sanctioned signal-closes-signal path".
- A14: Captured precisely because the project is not ready to decide — "sits in the graph, waiting for more data or a conversation that resolves it" (docs/story.md, s-cpt-f6m scene).
- A15: A hedge against deciding by omission — "recorded so the resolution-ergonomics decision is made deliberately rather than by omission" (s-cpt-bae).
- A16: Can be captured deliberately instead of a commitment, so later design starts from it (s-cpt-m8n: "deliberately captured as a question rather than a commitment").
- Corpus: 16 questions; 12 open, 3 closed (2 directives, 1 plan), 1 superseded. ZERO live dissolutions — the designed path has never been exercised here.

## B. Make-up
- B1/B2: No per-kind field block, no per-kind structural requirement — universal rules only.
- B3: validateKind, validateIDRefs, validateInlineTopics apply.
- B4: closes on a question rejected via SignalCloseRule.
- B5: supersedes type-checked only — same-kind supersession is convention, not enforcement.
- B7: The capture procedure has no question lane (lanes exist for actor/role/focus/procedure only).
- B8: No question-specific pre-flight template — falls through to checkSignalCapture.
- B9: Mechanics render from: SignalKindValues(), OpenAttentionKinds() (question IS in it), SignalCloseRule.
- B10: Wiring: templates/question.md + question.go + QuestionFactID + authoringFactIDs + overview.md related ref.

## C. Craft claims
- C1: Open the body with the unknown itself, phrased as an interrogative that stands alone as the summary (all five read specimens do; first-sentence rule d-cpt-7zr).
- C2: Name the occasion that raised it and ref that entry — questions arrive out of concrete work (s-cpt-bae "Surfaced by the outer validation …", s-cpt-m8n "Raised by Christopher while accepting …").
- C3: State the tension that keeps it open — what pulls each way — so the unknown reads as genuinely unsettled rather than merely unasked (s-cpt-bae, s-cpt-sxn, s-cpt-li1).
- C4: Candidate answers, strands and criteria may be carried — explicitly marked as unsettled, never as the answer (s-cpt-sxn "None of these is yet a settled line"; signal_capture no-finding band; gap.md sibling discipline).
- C5: Say why it is being recorded rather than decided now (s-cpt-bae, s-cpt-m8n, story.md).
- C6: Point at what the answer must keep intact or would strain against, so a later conflicting decision is visible (s-cpt-m8n "any answer must keep serve-size discipline and host-neutral operation intact").
- C7: Name what would resolve it, when known — the move, spike, or evidence that would answer (s-cpt-li1, s-cpt-m6m "empirically answerable only through a real attempt"; playbook-engage:117).
- C8: Position it against sibling questions when it is one face of a larger unknown (s-cpt-sxn "mirrors the echo-chamber concern from the opposite angle").
- C9: Confidence is honest about the framing, not urgency (s-cpt-sxn in-body gloss — thin, one instance).
- C10: Layer names the depth of the unknown (corpus: 11/16 conceptual).
- C11: One entry, one thing — BUT see F5: the corpus routinely bundles strands.

## D. Reverse-side claims
- D1: Pose the question so a future answering entry can name it — by its framing or the unknown it resolves, not merely its topic. The dissolution check is levied on the CLOSER, making the question's framing the handle the closer must find (dissolution.tmpl:8,13).
- D2: Traceability, not correctness, is what the record owes — no surface adjudicates whether the answer is right (dissolution.tmpl:9).
- D3: A question may explore the solution space — possible answers, directions, trade-offs — and stays a signal until it commits (signal_capture:8; s-prc-sbc reframing).
- D4: Never an imperative with timeline and owner — that is a decision in a signal's clothes (signal_capture:16).
- D5: Frame the unknown sharply enough that a later decision can be checked against it as genuine resolution (closing_decision:8; parallel to gap.md).
- D6: Fact/insight closing a question is a standard pattern drawing no finding (unusual_close:9,22).

## E. Discriminators (act-general + non-software; NONE are in overview's competing-kind tests)
- E1 vs fact: does the entry assert something the project now knows, or mark something it does not? Roastery: "DHL ships a 500g package for €4.50–5.80" is a fact; "what should shipping cost the customer?" is a question.
- E2 vs gap: if the expectation itself is what the project lacks, question; expectation exists and is unmet, gap. NOTE: gap.md already ships "Could new knowledge alone resolve it? Then the expected side itself is unknown, and it is a question" — the question fact must carry the MIRROR in question-side words or point (duplication risk).
- E3 vs insight: an insight connects what is recorded leaving nothing owed; a question leaves an answer owed. Child care: "children settle faster when handover happens at the door" is an insight; "does handover work better at the door or in the room?" is a question.
- E4 vs decision: the moment it commits to a route it is a decision. A question declines to commit and says so.
- E5 vs gap, resolution side: a gap is normally answered by a CHOICE; a question can be answered by KNOWLEDGE alone — which is why knowledge may close a question but never a gap. Timber: learning the yard stocks only C16 dissolves "which grades can we source?"; it does not close "the spec calls for C24 and we got C16".

## F. Contradictions
- F1: framework-concepts claims "Pre-flight enforces that kind matches the narrative shape" vs signal_capture "prose-level type policing belongs to the kind system, not pre-flight" (legacy surface, frozen).
- F2: Which decision kinds may close a question? framework-concepts says directive; the design attachment says decisions generally; live counter-example: a PLAN closed question s-cpt-y33. validateCloses permits any decision.
- F3: A won't-pursue close is sanctioned but no check accommodates a close that answers nothing (retirement-rationale check excludes question).
- F4: graph.go says "questions awaiting dissolution"; live corpus: every closed question was closed by a decision; dissolution has zero instances.
- F5: One-unknown-per-question (d-cpt-7zr conflation axis) vs corpus practice bundling strands (s-cpt-m8n "Two directions are bundled here"; s-cpt-y33 "four investigation strands"; s-cpt-5jk; s-cpt-sxn). NEEDS RULING or careful wording.
- F6: Legacy kind-default framing must not migrate (d-cpt-1dk).

## H. Unsourced — flag, do not blend
1. "A well-posed question names who could answer it" — no source; routing narrative only (story.md); zero of 16 specimens do it.
2. "A question must be answerable/decidable in principle" — not stated anywhere.
3. Confidence semantics for a question — no analogous statement to gap's two-sides grading. Corpus: 14 medium, 1 low, 1 unset.
4. Sharper-vs-refined supersession distinction — only "refined question" is sourced.
5. Whether an answering decision should also ref a dissolved question — nothing states it.

## G. Overview coverage — point, don't restate
Signal/decision split; kind list incl. "question — What do we not know?"; the retirement split incl. dissolution line; layers; the per-kind fact pointer. Overview runs NO gap/question/fact/insight tests — those are this fact's own ground (mind the gap.md duplication risk in E2).
