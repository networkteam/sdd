# Research brief — the plan decision kind

## A. Semantics
- A1-A3: A plan is a decision. Its question: "What must be true when done?" (overview.go — generated; do not restate). "A plan defines what must be true when the work is done — verifiable outcomes, stated as acceptance criteria" (overview:47).
- A4: A plan shapes the WHAT; ACs are the mechanism (framework-concepts:85).
- A5 (PRIME LESSON): **A plan must state an outcome, not deliverables, not an order, not a structure.** "None stated an outcome, so 'are we done?' resolved to 'do the fact entries exist?' — mechanism mistaken for result" (d-tac-9be design record §2; plan body ¶2: "None of the three said what is true about the world when the work is finished").
- A6: The plan's first paragraph is the stated end-state; the criteria enumerate it (9be exemplar).
- A7: Outcome-vs-mechanism "matters for content, not just bookkeeping" — a deliverables-only criterion set passes every mechanical check and misses the target (9be ¶2).
- A8: Layer-flexible.
- A9: Open plans are proposals, not facts — only a closing done (or retiring directive) turns proposal into fact (framework-concepts:158).
- A10: A plan may close a signal at capture (standard decision-closes-signal pattern).
- A11: When to reach for a plan at all: "plan only when the closing pre-flight needs plan items to validate" — small obvious fixes need no plan; always-required was ruled wrong (s-prc-cw0 closing d-prc-4du). Drafting side: "Needs decomposition (multiple requirements, design choices)? Capture a plan first. Specific enough on its own → skip the plan" (playbook-implementation:9). Word it so it doesn't endorse big-upfront planning (F4).

Lifecycle:
- A12: Closed by the done covering its criteria; or retired by a directive stating why nothing will be built.
- A13: **Partial delivery advances a plan without closing it** — "satisfies the first acceptance criterion … without closing it; deliberately deferred to slices 2 and 4" (s-tac-usb); repeated across many live dones; ref vocabulary encodes it: addresses = "supplying a plan's AC (incl. partial, without closing)".
- A14: **Refined in place by an augmenting directive** — refs the active plan, no supersedes/closes; sharpens or narrows an AC; refs one-way (directive→plan, plan never amended); both close together via the plan's done (decision_refs.tmpl:10,14-17; playbook-augment-plan; directive.md "Refining without replacing").
- A16: **Superseded when direction changes or commitments overlap** — 9be superseded its predecessor because "leaving it active would leave three overlapping commitments over one body of work"; everything carried forward is "restated in this plan's acceptance criteria, which become their home". Augment/supersede boundary: "augment when the plan's direction holds; supersede when the refinement changes direction, restructures multiple ACs, or invalidates the framing. When in doubt, augment first; if augmentations stack high enough that the plan's shape no longer reads cleanly, that's the signal to supersede" (playbook-augment-plan:11-15). "Augmentation refines; supersession replaces." A superseding plan covers the retired plan's ground and names what changed/narrowed (supersedes.tmpl).
- A17: A closed plan stays engageable — closure ends open-status, not dialogue; post-closure move is evaluate.

## B. Make-up (Go names)
- Required: a `## Acceptance criteria` section in the BODY with at least one `- [ ]` item. Constants: model.PlanAcceptanceHeading, model.PlanChecklistItem — "declared once for the validator and rendered kind facts alike" (construction.go:141-146).
- Rationale comment: "Plan: commits are checkable."
- Write-only rule (waived for entries on disk; grandfathered).
- No per-kind frontmatter block — the rule holds over the body.
- ACs live in the description, not the attachment (SKILL:189; F1 resolved in favour of body).
- An attachment carries the design record when dialogue produced trade-offs; 9be's attachment "keeps the reasoning, the alternatives rejected, and the source-code evidence out of the plan body".
- Mechanics render: DecisionKindValues() + PlanAcceptanceHeading/PlanChecklistItem.

## C. Craft claims
1. State the outcome first — what is true about the world when the work is finished — then enumerate it as criteria (9be).
2. Each criterion is a single verifiable outcome, not an implementation detail (d-prc-g1h attachment; SKILL:189).
3. Criteria state outcomes observable from outside the artifact — what a user or calling agent can see, read, do — "a criterion phrased about internals has no outside at all… satisfiable by the same understanding that produced it — which is precisely why review rounds certify it and first use breaks it. Internal shape may still be constrained where a project's guidelines demand it, but that is a design constraint, not an acceptance criterion" (d-prc-pck — CAUTION F2: pending intent, contradicted by live practice incl. 9be itself; needs ruling on binding vs preference).
4. A good plan names how each criterion will be checked — a test, or another verification method (s-prc-38f, open gap).
5. "It met reality once" is its own requirement — where an outcome's truth lives: inside the artifact or outside in something that gets a vote (s-prc-ogp, open insight).
6. Criteria are checked one by one at close: confirmed with evidence or deviation named with reasoning; silent omission is the failure (closing_done.tmpl; done.md "Cover the whole commitment").
7. Dialogue the design space before drafting criteria — the observed failure is "jump-to-ACs without surfacing the design dimensions that should shape them" (s-prc-5ms, open gap).
8. Ground the draft — propositions trace to existing decisions or the record; ungrounded propositions "codify into architecture" at plan capture (s-prc-nck, open gap).
9. Scope carve-outs stated explicitly ("Out of scope: …"), and what completion is checked by (9be).
10. Delivery order/slicing belongs in the body, separate from the criteria (9be).
11. Criteria cover the standing commitments the plan's scope implicates — narrowly scoped (s-prc-7i7, open gap, high).
12. First sentence stands alone — "'Plan for X' tells a reader nothing".
13. Partial completion records honestly: the partial done points at the plan, says what is covered and what remains, leaves closure to the finishing act.
14. The real contract at implementation time is the union of the plan's ACs and every open augmenting directive; the closing done addresses both and closes all of them (playbook-implementation:33-36).
15. A deferred criterion may be carved out at close with reasoning rather than blocking closure (s-tac-z2o; closing_done.tmpl:30).
16. Mid-flight design choice not covered by a decision → stop; capture the missing decision or a narrow augmenting directive (playbook-implementation:35).

## D. Reverse side
- D1: Missing AC section = blocking high finding at capture.
- D2: A refinement to an active plan is a directive that refs it — never a plan amendment or AC supersession.
- D3: Narrowing/sharpening AC scope via augmenting directive is the pattern's purpose — not scope smuggling.
- D4: The augmenting directive staying open across the window is not a dangling commitment.
- D6: refines = active plan sharpened in place; builds-on = closed target or forward step.
- D7: A gap the plan resolves is addresses, not grounded-in.
- D9: A plan closing a signal genuinely addresses it, not references it as context.
- D10/D11: A superseding plan states what happens to the predecessor's concerns; kind transitions argued in text.
- D12: A plan omitting a project ritual (decomposition table) is not a finding — "small tactical plans don't need decomposition tables".
- D13: An AC needn't be restated in the closing prose when the work lives in committed artifacts.

## E. Discriminators
- vs activity: OVERVIEW OWNS "WHAT vs THAT" — point, don't restate. Caveat carried in directive.md: work-shape not output-shape (open gap s-prc-01i).
- vs directive: route/rule to conform to vs enumerated end-state. The timber example ("the frame is done when all twelve bents are raised…" / "load-bearing joints are pegged, not screwed") IS ALREADY IN directive.md — run the plan side with different wording or a different act, never verbatim reuse.
- vs procedure: read-and-delivered-once (plan) vs a repeatable way of working that runs (procedure) — procedure.md carries it from its side.
- vs aspiration / contract: overview owns — point.
- vs focus: holds only for the present period, naming who attends → focus (one-liner, mirroring directive.md).

## F. Contradictions / rulings
- F1: AC location — resolved: body/description (attachment text is stale history).
- F2: Outside-observable criterion rule (d-prc-pck, intent pending, never landed on a shipped surface) vs live practice (9be's own internal-shape ACs). RULING NEEDED: binding, preference, or omit.
- F3: "Default? no" framing is legacy — kind is an explicit choice (d-cpt-1dk).
- F4: A plan is NOT always required — word "when to reach for a plan" so it doesn't endorse exhaustive upfront planning.
- F5: The plan-vs-activity test wording is under an open gap (s-prc-01i) — match directive.md's corrected wording, not framework-concepts.
- F6: No plan lane exists in the capture procedure — AC requirement served nowhere at draft time (open gap s-prc-38f context).

## G. Overview coverage — point, don't restate
Signal/decision split; kind question; plan-vs-activity WHAT/THAT verbatim; aspiration-vs-directive; intent triple; contract closure; actor/role; done; annotation; retirement split; layers; per-kind fact pointer. The plan fact's own ground: outcome-vs-mechanism, criteria make-up and craft, the partial/close/augment/supersede lifecycle from the plan's side, neighbor tests the overview doesn't run (directive, procedure, focus).
