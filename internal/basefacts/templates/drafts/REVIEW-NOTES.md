# Kind-fact drafts — review state and open rulings

Ten drafted authoring facts for the remaining kinds (contract deliberately excluded — the kind takes no new entries and gets no fact). Drafted from per-kind research briefs (see `research/`) under `STYLE-CONTRACT.md`, calibrated on the shipped done/gap/directive/procedure facts. **Unregistered**: nothing here is embedded or rendered until each kind gets its `internal/basefacts/<kind>.go` wiring, `authoringFactIDs` entry, and overview ref — after Christopher's per-kind review.

## Review procedure (next session)

Walk the drafts with Christopher kind by kind (batches of 2–3 worked well for gap/directive), fold review edits in, then register each reviewed fact: Go wiring + mechanics renderer + tests + overview ref, and cut the corresponding lane teaser into the capture procedure.

## Reviewed and registered

- **insight** — ruling 1 settled (see below); compressed; registered as `20260816-100000-s-prc-syn`.
- **fact** — rulings 2 and 10 settled (see below), plus provenance-over-verification; compressed; index rules extracted to `model.FactIndexKindRule` / `model.FactIndexTopicRule`; registered as `20260816-110000-s-prc-kno`.

## Compression method (worth repeating per kind)

An Opus subagent, given the target draft plus the type-system introduction, the style contract, and the shipped sibling facts, instructed to analyze every paragraph for what it serves *before* cutting, to treat overview-owned content as the richest cut, and to flag judgment calls rather than make them. Insight 1167→1070, fact 1170→1089, nothing load-bearing lost. It also caught a reserved-word violation the human review had missed ("procedures" in fact's hygiene-plan example).

## Rulings made in review (beyond the numbered list)

- **Summary and topic craft belong to the capture procedure, not the kind facts.** The copied claim "the first sentence stands alone, because overview surfaces lead with it" is wrong — generated summaries lead those surfaces, not entry bodies — and was cut from the shipped gap fact and the fact, plan, question and role drafts. Per-kind topic conventions stay: they state how much of an entry's meaning that kind already carries elsewhere (done's work-shape topics, fact's index topic, role's rarely-owed, focus's optional, annotation's topics-as-payload), which is entry semantics. Per-kind confidence conventions stay for the same reason.
- **Layer names where the thinking landed, not where its inputs sit.** Applied to insight ("a reading drawn from several operational deviations and one survey result is strategic if that is where the thinking arrived"). The gap and fact drafts still say "depth of the deviation" / "scope of the claim" — check those against this framing when their turn comes.

## Cross-read fixes to apply mechanically before/with review

- ~~Terminology: harmonize "the overview" to "the type-system introduction"~~ — done.
- ~~actor.md doubling~~ — done.
- "load-bearing joints are pegged, not screwed" now appears in directive (shipped), activity, and focus drafts — keep as deliberate canon or diversify (Christopher's call). **Still open.**

## Open rulings (one per line, writer recommendation in parentheses)

1. ~~Insight retirement path~~ — **SETTLED**: kind-open and rationale-bound. Any entry may retire a reading; what qualifies it is saying why the reading stopped holding (the material moved, or the reasoning had a flaw). Being acted on is not a retirement — a commitment taking the reading up points at it and leaves it standing. Closes 20260608-004727-s-prc-4kh; a settled directive recording this is still to be captured.
2. ~~Fact-closes-fact~~ — **SETTLED** by the same kind-open rationale-bound rule as ruling 1: whatever entry carries the news that a claim stopped holding may close it. Stops being an exception; folds into the `validateCloses` widening that ruling 1 owes.
3. Role head-ref: pre-flight demands role refs include the actor head's entry ID; capture lane says "no natural refs" (drop the pre-flight check — the canonical binding already carries the relationship). Recorded miss in 20260722-141659-s-cpt-rza either way.
4. Per-kind confidence conventions (actor high, role medium, aspiration medium-indefinitely): all kept out as CLI-legacy-shaped per 20260816-170529-d-cpt-1dk (stay out; aspiration's origin rationale could ship as one sentence if wanted).
5. Actor topics: live exemplars carry collaboration/identity; capture procedure says no natural topics (ratify the exemplars or fix the procedure — one line).
6. Annotation: {label, members} sub-selection form is unexercised in the live graph (teach it, don't name prose-only sub-grouping an anti-pattern — confirm); ref kind for member refs is unspecified while required (rule `related` as convention or auto-default).
7. Focus: layer line (framework-concepts: cadence-flexible; capture procedure: "typically tactical"; corpus 8/8 tactical) — pick one voice. Elapsed when.to does not end a focus (code-true, unstated) — adopt or leave out.
8. Plan: outside-observable-criteria rule (20260728-083647-d-prc-pck, intent pending, contradicted by live practice incl. d-tac-9be itself) — binding, preference, or omit (draft omits).
9. Activity: does shipping its fact close the work-shape gap 20260507-122000-s-prc-01i or only address it (legacy skill text stays frozen under d-tac-9be)? Writer leans close-with-carve-out.
10. ~~Actor-vs-fact discriminator~~ — **SETTLED**: kept and ruled here. Actor capture is the sharpest recorded failure point, and "this person is a trained X with twenty years at Y" is exactly the claim that reads like discovered knowledge but belongs in an actor.
11. Focus targets-in-refs: involvement creates no ref edge, so a focus is invisible in its targets' downstream (rule: involvement is the work channel, downstream visibility a serving concern — not an authoring duty).
12. ~~Question strands~~ — **SETTLED**: accepted as the reconciliation, with the reasoning written in — a question is temporary, whatever answers it can split what turned out to be two, so the guard is against multiplying entries into ceremony rather than against bundling.

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
