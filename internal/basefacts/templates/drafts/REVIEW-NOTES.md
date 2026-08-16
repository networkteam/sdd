# Kind-fact drafts — review state and open rulings

Ten drafted authoring facts for the remaining kinds (contract deliberately excluded — the kind takes no new entries and gets no fact). Drafted from per-kind research briefs (see `research/`) under `STYLE-CONTRACT.md`, calibrated on the shipped done/gap/directive/procedure facts. **Unregistered**: nothing here is embedded or rendered until each kind gets its `internal/basefacts/<kind>.go` wiring, `authoringFactIDs` entry, and overview ref — after Christopher's per-kind review.

## Review procedure (next session)

Walk the drafts with Christopher kind by kind (batches of 2–3 worked well for gap/directive), fold review edits in, then register each reviewed fact: Go wiring + mechanics renderer + tests + overview ref, and cut the corresponding lane teaser into the capture procedure.

## Cross-read fixes to apply mechanically before/with review

- Terminology: four drafts say "the overview"; shipped facts say "the type-system introduction" — harmonize to the latter everywhere (including one mixed use in the shipped directive fact).
- actor.md: "Yusuf, known in the graph as Yusuf" doubling — polish.
- "load-bearing joints are pegged, not screwed" now appears in directive (shipped), activity, and focus drafts — keep as deliberate canon or diversify (Christopher's call).

## Open rulings (one per line, writer recommendation in parentheses)

1. Insight retirement path: docs say directive-close, practice mostly done-close — open gap 20260608-004727-s-prc-4kh. Draft absorbs either; a ruling would let it name the path.
2. Fact-closes-fact: model warns, pre-flight teaches it as a valid unusual close (keep warned-but-permitted; draft avoids exclusivity claims).
3. Role head-ref: pre-flight demands role refs include the actor head's entry ID; capture lane says "no natural refs" (drop the pre-flight check — the canonical binding already carries the relationship). Recorded miss in 20260722-141659-s-cpt-rza either way.
4. Per-kind confidence conventions (actor high, role medium, aspiration medium-indefinitely): all kept out as CLI-legacy-shaped per 20260816-170529-d-cpt-1dk (stay out; aspiration's origin rationale could ship as one sentence if wanted).
5. Actor topics: live exemplars carry collaboration/identity; capture procedure says no natural topics (ratify the exemplars or fix the procedure — one line).
6. Annotation: {label, members} sub-selection form is unexercised in the live graph (teach it, don't name prose-only sub-grouping an anti-pattern — confirm); ref kind for member refs is unspecified while required (rule `related` as convention or auto-default).
7. Focus: layer line (framework-concepts: cadence-flexible; capture procedure: "typically tactical"; corpus 8/8 tactical) — pick one voice. Elapsed when.to does not end a focus (code-true, unstated) — adopt or leave out.
8. Plan: outside-observable-criteria rule (20260728-083647-d-prc-pck, intent pending, contradicted by live practice incl. d-tac-9be itself) — binding, preference, or omit (draft omits).
9. Activity: does shipping its fact close the work-shape gap 20260507-122000-s-prc-01i or only address it (legacy skill text stays frozen under d-tac-9be)? Writer leans close-with-carve-out.
10. Actor-vs-fact discriminator: unsourced — add by ruling or leave the choosing paragraph silent on it (draft silent).
11. Focus targets-in-refs: involvement creates no ref edge, so a focus is invisible in its targets' downstream (rule: involvement is the work channel, downstream visibility a serving concern — not an authoring duty).
12. Question strands: one-unknown-per-entry vs corpus bundling — draft's line "strands that stand or fall with one resolution travel together" (accept as the reconciliation).

## Mechanics extraction needed at registration (inline strings → exported model constants)

- plan: AC requirement string (construction.go:406) → e.g. PlanAcceptanceRequirement.
- actor: canonical-required, layer pin, alias hygiene (3 findings), write-once-across-chains rule text.
- role: actor-required, layer pin; resolution-gate strings (post ruling 3).
- annotation: refs-required, topics-required, members-subset, no-inline-topics.
- focus: involvement-required, target-resolution, when-shape rules.
- activity/aspiration: decision-close rule (graph.go validateCloses inline) → e.g. DecisionCloseRule, shared.
- fact: index rules (fact_index.go inline strings) if the mechanics should quote them; OverrideClosed line pending ruling on whether override renders at all.
- question/insight: nothing new — SignalKindValues, OpenAttentionKinds, SignalCloseRule all exist.

## Reconciliation items surfaced by research (for the plan's reconciliation phase, not the facts)

- Three legacy surfaces still ship the discredited completion-criterion aspiration test (framework-concepts:74-79, bootstrap skill:234, aspiration_capture.tmpl opener) + stale contract-first ordering.
- framework-concepts "Default?" column and SKILL kind-default prose = CLI-legacy per d-cpt-1dk; frozen, must not migrate.
- Won't-pursue question close is sanctioned but no check accommodates it (candidate gap capture).
- graph.go doc "questions awaiting dissolution" vs practice (all closes were decisions) — align wording.
- sdd show renders no actors/when/involvement on a focus entry — candidate gap capture.
