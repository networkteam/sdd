# Research brief — the activity decision kind

Corpus: 25 activities, exactly 1 active — captured and closed fast; almost no standing population.

## A. Semantics
- A1: Its question: "What's next to do?" (overview.go — generated; do not restate).
- A2: An activity dispatches THAT work happen, vs the plan's WHAT (overview owns the WHAT/THAT test verbatim — point).
- A3: Code doc: "Activities are THAT-shaped commitments — capturing that specific work happens, independent of the directive-style choice of what to do" (Graph.Activities()).
- A4: The work's shape is already known from context — three named carriers: parent plan, refs, or self-evident narrative (framework-concepts:72).
- A5: THE AXIS IS WORK-SHAPE, NOT OUTPUT-SHAPE — emergent outputs are compatible with activity when the work form is known (s-prc-01i, open gap, the failure evidence; carried from the directive side in directive.md).
- A6: An activity makes no choice against alternatives — what separates it from a directive (s-prc-01i).
- A7: A commitment (future-facing decision); done is the record (past signal).
- A8: Layer-flexible; live examples span ops/tac/cpt/prc.

Lifecycle:
- A9: Standard closure: a done signal ("commitment to fulfill; resolved by done signal" — d-cpt-e7i plan attachment).
- A10: Retirement without delivery: a directive closing it with the why (d-cpt-enb).
- A11: Supersession path: a replacement activity — documented but ZERO live instances (0 of 25).
- A12: An activity never closes a decision ("no use is named for a plan, activity or aspiration retiring a decision" — d-cpt-enb verbatim).
- A13: An activity may close signals (standard decision→gap pattern).
- A14: Partial work refs the activity; it does not close it (two-type-design; ref-kinds addresses row "incl. partial, without closing").
- A15: A valid implementation anchor (implementation procedure runs "a plan, directive, or activity").
- A16: Engage move menu for an activity: implement (mechanical) · close via done · evaluate — no supersede/refine/augment offered (playbook-engage; see F2 tension with retirement table).

## B. Make-up (Go names)
- POSITIVE FINDING: no per-kind block, no kind-specific structural requirement anywhere. Universal rules only. Absent from the 7+8 contract's per-kind requirements list.
- No activity counterpart to the declared rule constants (DoneAnchorRequirement etc.).
- KindActivity in decisionKindOrder; Graph.Activities() read accessor.
- Mechanics note: nothing to render beyond the decision-kind list; the only mechanically relevant rule (may close signals, never a decision) lives as inline strings in validateCloses — extract a constant or render the kind list only. FLAG for implementer.

## C. Craft claims
1. State the completion condition — the activity says when it is finished, and its closer is measured against exactly that (d-prc-iqw "Closes when each candidate has reached a disposition"; closer s-tac-64h "closes … on its exact stated completion condition").
2. Name the expected landing — what artifact or state the work leaves behind (d-cpt-eke "Expected landing: a v1 structure plan …"; closer confirms "fulfilling the exact expected landing").
3. Name the sequencing when the work waits on something — the prerequisite, not a calendar date (d-prc-iqw "Sequenced after the Engage refactor lands"; d-tac-9lv; depends-on/required-by vocabulary).
4. Let scope resolve at execution time when the world will have moved — and say so (d-tac-9lv verbatim: "The authoritative scope is computed at execution … any list frozen now would read as complete when it is not").
5. Say what the work is for when the purpose bounds it — especially a deliberately non-committing one (d-tac-tjw: "a throw-away spike … to learn whether it holds, not to commit to it").
6. The refs carry the context; the activity need not restate the why ("The task just says 'do this.' The done signal says 'did it.'" — two-type-design).
7. Research and evaluation are activities, not their own kinds — findings come back as their own entries (facts, insights, gaps, questions); the done closes the activity (two-type-design "Research as Activity, Not Kind").
8. Implementing an activity is mechanical — no design left to make while doing it (playbook-engage).
9. The closing done covers what the activity required — done or explicitly deviated with reasoning (closing_done checks 1-4; AC-coverage check is plan-only).
10. Grounded — every entry the body reasons from is a ref; no commitment beyond what refs support (decision_refs checks 1-3).
- Sizing: de facto unit is one working session / one delivery event (6 of 25 are "session N of the plan") — PATTERN, NOT A RULE. Flag.
- "WHO": NO SOURCE says an activity names who does the work — the who-slot belongs to focus. FLAG: likely wrong to state. "Roughly when" is sourced only as sequencing/trigger, never a date.

## D. Reverse side
- D1: Emergent outputs do not disqualify an activity — judge the form of the work, not of what comes out (s-prc-01i: the work "has a well-known shape; only the outputs are emergent").
- D2: When no choice is being made and one piece of work is dispatched, activity is the answer — directive is not the safe fallback (s-prc-01i fix shape).
- D3: An activity does not need acceptance criteria — being asked for them is the wrong-axis rejection (s-prc-01i; note the flagged finding category was LLM-invented, no such rubric exists).
- D4: An entry carrying a non-obvious design choice stays a directive — the WHAT-vs-THAT test is lossy at the boundary; ambiguous cases stay directives (d-cpt-e7i judgment-pass protocol, verbatim "the safe default"; applied live s-tac-23e).
- D5: Grounding supports it — no commitments from nowhere.
- D6: The closing done doesn't silently omit any part of the remit.

## E. Discriminators (acts in use: timber framing, roastery, child care)
- vs plan: OVERVIEW OWNS — point. Instantiation if needed: plan "the frame is done when all twelve bents are raised and every joint inspected"; activity "raise the four bents on the east wall this week".
- vs directive: is a choice among routes being made with a why? directive. One piece of work dispatched, no choice left in it? activity. Directive: "load-bearing joints are pegged, not screwed"; activity: "peg the joints on the east wall bents". Directive.md carries the mirror — carry this side without restating its sentence.
- Work-shape trap: "Run the cupping session with the new roaster on Thursday" is an activity — the cupping has a known form; which beans make the cut is not knowable in advance. (Directive.md uses the handover-redesign example — use a DIFFERENT act so the facts don't read as copies.)
- vs done: already happened → done; committed but not yet performed → activity. "Roasted and cupped the new lot" vs "roast and cup the new lot".
- vs focus: holds only for the present period naming who attends → focus. Focus: "through March, Jun carries the discovery line"; activity: "write the March discovery notes".
- vs procedure: run repeatedly step by step → procedure; one piece of work happening once → activity. Procedure: "the weekly cupping review"; activity: "this week's cupping review".

## F. Contradictions / rulings
- F1 Lean-vs-detailed bodies: design origin says lean ("says what, not how or why; the refs chain carries context") vs live practice (d-prc-aep ~400 words of scoping; d-tac-tjw design description). POSSIBLE RECONCILIATION (unsourced synthesis — needs ruling): the shape must be known SOMEWHERE — when a parent plan or ref carries it, the activity is lean; when nothing else holds it, the activity carries it. Consistent with A4's three carriers.
- F2 Supersession: retirement table offers replacement-activity; engage move table offers no supersede for activity; corpus has zero. Word the lifecycle accordingly (documented path, rarely exercised).
- F3 Kind default is legacy (d-cpt-1dk) — never frame activity as a non-default you opt into.
- F4 No rejection-side rubric exists for activity (the reported finding category was model-invented) — the audit surface is the kind-agnostic templates only.
- F5 s-prc-01i is OPEN; the directive fact shipped half its fix. The activity fact is the natural second half — RULING with user: does shipping it close s-prc-01i or only address it (the legacy skill text stays frozen under d-tac-9be)?

## G. Overview coverage — point, don't restate
Signal/decision split; the loop; immutability; kind lists incl. "What's next to do?"; the plan-vs-activity WHAT/THAT test IN FULL (the one to point at); other cross-kind tests; the retirement split (instantiate for activity, don't restate); layers; the per-kind fact promise.
