# Research brief — the aspiration decision kind

## A. Semantics

**A1. What it is.** A decision recording a perpetual direction the work is pulled toward, with no completion gate — "something the team commits to working toward, referenced by downstream work, but never checked off" (s-cpt-7we proposal attachment). Model doc: "a perpetual direction" (entry.go IsAspiration); "durable" (graph.go Aspirations).

**A2. Its question.** "What are we pulling toward?" (overview.go kindQuestions).

**A3. The force — pull, not push.** "An aspiration exercises a pull — strategic direction that tells the work which way to go, with no single piece of work conforming to it" (20260810-144251-s-cpt-jz0 ¶1). It "orients on the way, pull, without ever binding", "a higher, unreachable strategic quality" (¶2). Original framing: contract = constraint push; aspiration = attractor pull — "A contract tells you what you can't do. An aspiration tells you what you're trying to become" (s-cpt-7we proposal).

**A4. Evaluation is gradient, never binary.** "Contract: binary. 'Does this comply?' … Aspiration: gradient. 'How well does this align?' — degree, not pass/fail" (s-cpt-wiv aspirations.md). Ruled in graph: aspirations "evaluated on gradient fit rather than binary compliance" (20260422-125438-s-cpt-xeo).

**A5. Constellation, not north star.** Aspirations are plural and coexist in tension; a decision is evaluated as "which aspirations does this move us toward? Which away from? Where's the tension?" — not yes/no (proposal). Tension is normal, resolved case by case (live: d-stg-7lu vs d-stg-qlt). Hierarchy among them is structural understanding only; operationally they coexist as peers.

**A6. Detours are tolerated by the gradient.** "A detour … is coherent if it's explicable against the constellation, even if it isn't the straightest line to any single star" (proposal).

**A7. Operational role.** "The active aspirations serve as the base constellation against which the graph's movement is checked for direction … and as preparation for dialogue … In a novel situation the graph does not back, the aspirations are the constellation to check it against" (s-cpt-jz0 ¶3). "A strategic guiding directive could not absorb this role without collapsing the direction check into conformance."

**A8. The active set is small and stable by kind semantics** (d-cpt-20r). Live count: exactly six active aspirations since 2026-04-22.

**A9. Decay.** "Aspirations shift when the underlying understanding of what we're doing shifts", slower than directives (proposal).

**A10. Not doctrine.** "Aspirations are not fixed doctrine — they can be extended over time, and change when a misalignment is observed" (s-cpt-jz0 ¶3).

**A11. Lifecycle.** Durable: "retire via supersede ('evolved aspiration') or close-by-directive with rationale" (graph.go doc; framework-concepts retirement table). No done-completion anywhere — but see F5: convention, not enforcement.

**A12. Whose slot it is.** "The aspiration layer belongs to each adopting project's own strategic pulls; a framework-level meta-posture parked as an aspiration would occupy the slot projects are meant to fill" (s-cpt-jz0 ¶2). FLAG: framework-vs-adopter argument; generalizes but needs user ruling for a shipped fact.

**A13. Where aspirations come from in a fresh project.** The Golden Circle WHY pass: "What's the pull behind this? What's it aspiring toward over time?" (bootstrap skill + procedure). "The why lands better once the what and how are grounded — let it emerge rather than opening cold on it" (d-prc-bst).

## B. Make-up

**B1. No kind-specific block, field, or validation — positive finding.** KindAspiration appears in no validation branch of construction.go.

**B2. Layer unconstrained in code.** Convention: all six live aspirations are strategic; bootstrap seeds `d stg`. Only actor/role/procedure are process-pinned.

**B3.** `model.KindAspiration`, member of decisionKindOrder, decision-type-only. `(*Entry).IsAspiration()`.

**B4.** `(*Graph).Aspirations()` = open aspiration decisions.

**B5. Closure edges (generic).** Only a directive decision may close another decision (validateCloses); supersede is same-type-checked. A done closing an aspiration produces NO warning (see F5).

**B6. Mechanics available:** decision-kind list (DecisionKindValues()); the directive-closes-decision rule has NO exported constant (inline fmt.Sprintf at graph.go:1174) — needs extraction if the mechanics should cite it.

**B7. Wiring:** AspirationFactID const + aspiration.go, Entries() append, authoringFactIDs registration, override: closed frontmatter, overview.md related ref.

## C. Craft claims

1. **Present-tense declarative statement of direction, first line.** All six specimens: "Dialogue shapes decisions." (d-stg-beb), "Non-developer participants engage with SDD directly." (d-stg-x0l), "Reasoning lives in one place — the graph, across one loop." (d-stg-3k0), etc.
2. **Name what it pulls away from.** Five of six specimens carry "What this aspiration pushes against:" with named counter-forces.
3. **Vision-shaped and transition-shaped framings equally valid** — toward-something or away-from-something; "sometimes the negative framing carries more energy" (aspirations.md).
4. **Give an operational test where one exists.** "Operational test for any new capability: how does it enable better dialogue?" (d-stg-beb) — pattern named in proposal.
5. **Position against sibling aspirations explicitly.** Five of six specimens carry "Distinct from …"/"Branches from …" naming the sibling and the difference.
6. **Point at the evidence rather than restating it** (three specimens end with attachment pointers).
7. **An aspiration may synthesize direction without every principle pre-established in a signal** — "demanding pre-grounding prevents strategic thinking" (decision_refs.tmpl no-finding calibration; d-cpt-20r: only contradiction is a finding, never missing alignment).
8. **Keep the active set small** — "small and stable by kind semantics" (d-cpt-20r); constellation of six since April. FLAG: no stated bound anywhere (F4) — state as demand ("few enough that a reader holds them all when checking direction") or flag.
9. **Confidence convention: medium, indefinitely** (framework-concepts:81; 6/6 live set; bootstrap seeds medium). Origin rationale: "not because conviction is low, but because it's not the kind of thing confidence eventually rises into 'done'" (proposal). FLAG for ruling: reads like a per-kind confidence rule; conflicts with confidence-grades-certainty; the origin rationale is the only text making it safe to ship (F6).
10. **A tension with an existing aspiration is stated, not avoided** (d-stg-7lu writes autonomy-vs-alignment into its body; aspiration_capture.tmpl requires acknowledgment).
11. **Scope the pull to this project's own direction** — a general working posture is a guiding directive, not an aspiration (s-cpt-jz0 ¶2; see A12 flag).

## D. Reverse side (aspiration_capture.tmpl)

- **D1.** An aspiration names its direction concretely enough that a later decision can be scored against it.
- **D2.** Tension with an active aspiration is acknowledged and argued (contradiction without acknowledgment = medium; acknowledged coexistence = no finding).
- **D3.** Reinforcement and orthogonal axes need no defence.
- **D4.** Partial tension on a related axis is dialogue material, not an error.
- **D5.** Direct contradiction reads as an unrelated pull dropped in.
- **D6.** (Serving mechanic — exclude) the check is capped: never high — "aspirations are gradient; incompatibility is a dialogue prompt, not a block." The semantics behind it (gradient ⇒ never a gate) is A4.
- **D7.** From decision_refs.tmpl mirror: "ongoing pull toward X", "we orient around Y", perpetual framing with no horizon — the RIGHT register for an aspiration, and the reason completion-absence can't discriminate against a guiding directive.

## E. Discriminators

**E1. vs directive — overview owns force-not-completion; point.** The fact carries only the pull side: no single piece of work conforms to it, and none can violate it. Non-software: "Every loaf we sell was baked that morning from grain whose farm we can name" pulls; "nothing leaves the shop without a second person's sign-off" (directive.md's example) pushes.

**E2. vs focus — perpetual direction vs this period's attention and who is on it.** A focus is bounded on period and people; an aspiration names neither. Non-software: "this quarter the bakery attends to the wholesale accounts, with Rosa on ordering" is a focus; the grain-provenance pull outlives every quarter.

**E3. vs plan — no enumerable done state.** "The new oven is in when it passes gas inspection and completes a 200-loaf trial bake" plans; the provenance pull never reaches a checked box.

**E4. vs contract — historical.** Carry the gradient/binary substance under E1; do not offer contract as a live neighbor test (contract takes no new entries — overview owns).

**E5.** Non-software canonical specimen exists: the Kōgen roastery aspiration (proposal); the proposal's own tension examples are non-software ("growth vs. brand purity, speed vs. depth, reach vs. authenticity").

## F. Contradictions — NEEDS ATTENTION

**F1. The discredited completion-criterion test still ships in three legacy places** (framework-concepts:74-79, bootstrap skill:234, aspiration_capture.tmpl opener) — against s-cpt-jz0 and overview: "force, not completion… absence of a completion criterion is shared by both kinds by design and cannot discriminate". Resolution: "no completion criterion" may be stated as a PROPERTY (true), never as the TEST.

**F2/F3. Stale contract-first ordering + kind: contract reclassification advice in legacy surfaces** — reconciliation items, not fact content.

**F4. Constellation size has no stated bound** — "small and stable" is a design premise; the display cap n(6) is rendering. FLAG: state as a demand or flag for ruling.

**F5. "Only a directive retires an aspiration" is convention, not enforcement** — a done closing an aspiration draws no warning. The fact can say nothing completes a pull (semantics) but must not imply write-path enforcement.

**F6. Confidence convention** — see C9 flag.

## G. Overview coverage — point, don't restate

Signal/decision split, loop, immutability; the kind question "What are we pulling toward?"; **the aspiration-vs-directive test in full (force, not completion) — point exactly as directive.md does**; plan vs activity; intent postures; contract closed; the retirement split (the fact adds only: nothing completes a pull → evolved-aspiration supersession or a directive with why); the layer list; the per-kind fact pointer convention.
