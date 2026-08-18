# Kind facts — review state, rulings, and what remains

Thirteen of the fourteen kinds ship an authoring fact, plus a discrimination fact holding
the tests that settle a competing kind. This file is what is left: the rulings still open
and the reconciliation items nobody has taken yet. It retires once those are resolved or
captured.

Two things that used to live beside it have moved:

- **The style contract** is now the guiding directive `20260818-194123-d-prc-qge` — standing
  authoring rules for every base fact, findable in the graph rather than in a working file.
- **The ten research briefs** moved to `.sdd/tmp/basefacts-research/`, out of the repository.
  They were the sourced material behind the drafts; the shipped facts carry the claims now.

## Reviewed and registered — 13 of 14 kinds ship

All kinds now ship an authoring fact except `contract`, which takes no new entries and
deliberately gets none.

| kind | fact ID | note |
|---|---|---|
| done | `20260812-170000-s-prc-dnk` | shipped earlier |
| procedure | `20260813-170000-s-prc-prd` | shipped earlier, with the spec reference beside it |
| gap | `20260815-100000-s-prc-gpk` | shipped earlier |
| directive | `20260815-110000-s-prc-drk` | shipped earlier |
| insight | `20260816-100000-s-prc-syn` | ruling 1 settled; compressed |
| fact | `20260816-110000-s-prc-kno` | rulings 2 and 10 settled; provenance over verification; compressed |
| question | `20260817-100000-s-prc-qry` | ruling 12 settled; rewritten from scratch |
| plan | `20260818-100000-s-prc-spc` | rewritten; retitled to *Specifying an outcome* |
| actor | `20260818-110000-s-prc-act` | rewritten |
| role | `20260818-110100-s-prc-rol` | rewritten; retitled to *Granting a part* |
| activity | `20260818-110200-s-prc-dsp` | rewritten |
| focus | `20260818-110300-s-prc-foc` | rewritten |
| aspiration | `20260818-110400-s-prc-asp` | rewritten; retitled to *Orienting the work* |
| annotation | `20260818-110500-s-prc-ann` | rewritten; retitled to *Making a thread findable* |

**Still to land:** the discrimination fact (`drafts/discrimination.md`, in compression),
its registration, one pointer sentence in the type-system introduction, and the done
signal that records all of it and closes `20260507-122000-s-prc-01i`. `NEIGHBOUR-TESTS.md`
retires once the discrimination fact ships.

## Rulings settled in this review round

- **Retirement is kind-open and rationale-bound** (rulings 1 and 2): any entry may retire
  another by saying why it stopped holding; being acted on is not a retirement. **Owes a
  code change** — `validateCloses` today sanctions only decision-closes-signal,
  done-closes-anything, and fact/insight dissolving a question, so a fact cannot retire an
  insight, which the shipped insight and fact facts now teach. The directive recording this
  is parked mid-capture with intent pending, closing `20260608-004727-s-prc-4kh`.
- **Kind discrimination leaves the per-kind facts** (`20260818-133146-d-cpt-fpm`).
- **Ruling 3 settled**: repeated examples stay, but every example names its domain, terse,
  from one set — timber framing, a village bakery, a coffee roastery, a child-care group.
- **Ruling 10 settled**: the actor-versus-fact discriminator is kept, ruled on merit.
- **Ruling 12 settled**: question strands travel together when one resolution answers them;
  the guard is against ceremony, not bundling.
- **Confidence keeps one meaning across kinds** — how much backs the claim — while each kind
  may say what backs it. Actor gets no sentence (no source beyond a repeated default), role
  keeps its evolving-commitment sentence, aspiration gets one: confidence rises as the work
  aligns with the pull and the alignment proves useful, and a high-confidence aspiration
  carries more force in dialogue.
- **Actor topics are optional, not forbidden** — the live entries are ratified; grouping
  earns its place once a project has more than a handful.
- **Focus layer is not pinned** — the layer sets reach and specificity, most land tactical,
  and foci nest. The wider question of the ordering and execution domain is unsettled and
  needs its own dialogue; nothing here pre-empts it.
- **Legacy surfaces are never named in shipped facts** — a discredited test may be warned
  about generically, but this repository's history stays out.
- **Summary and general topic craft stay out of kind facts** (capture-procedure territory);
  per-kind topic and confidence conventions stay.

## Open rulings still outstanding

- **Role references**: the validation flow demands a role reference the actor's entry; the
  capture flow says a role needs none. Recommendation stands — drop the check, since binding
  to the canonical already carries the relationship. Blocks nothing in the text; decides what
  role's mechanics block renders.
- **Activity bookkeeping**: `20260507-122000-s-prc-01i` closes on the done recording the
  discrimination fact, not on the activity fact, because the boundary test it complains about
  now lives in the discrimination fact.
- **Annotation reference kind**: required but unspecified — rule one the convention or have
  the write path fill it in.
- **Focus time range**: an elapsed end date does not by itself end a focus — true of the
  implementation, stated in no prose source. Write it into the focus fact, or leave it out?
- **Focus targets create no reference edge**, so a focus is invisible from its targets' side.
  Recommendation: involvement is the work channel and downstream visibility is a serving
  concern, not an authoring duty. Confirm rather than assume.
- **Annotation and focus confidence**: no sourced convention; confidence on an entry that
  asserts nothing is an odd fit and may deserve a design question rather than a sentence.

## Mechanics extraction — done and remaining

Exported and rendering: `DoneAnchorRequirement`, `SignalCloseRule`, `DirectiveIntentRequirement`,
`SettledCloseRule`, `PlanAcceptanceRequirement`, `FactIndexKindRule`, `FactIndexTopicRule`,
`ActorCanonicalRequirement`, `RoleActorRequirement`, `AnnotationRefsRequirement`,
`AnnotationTopicRequirement`, `FocusInvolvementRule`, `AliasHygieneRule`, `ProcessPinnedKinds()`.

Still inline, and only worth extracting if a fact needs to quote them: focus when-shape and
target-resolution details, annotation per-topic label parsing, the decision-close rule in
closure validation.

## Compression method (worth repeating per kind)

An Opus subagent, given the target draft plus the type-system introduction, the style contract, and the shipped sibling facts, instructed to analyze every paragraph for what it serves *before* cutting, to treat overview-owned content as the richest cut, and to flag judgment calls rather than make them. Insight 1167→1070, fact 1170→1089, nothing load-bearing lost. It also caught a reserved-word violation the human review had missed ("procedures" in fact's hygiene-plan example).

## Standing checks, still live

- **Every decision kind says where its why comes from.** All six rewrites now carry a
  dialogue paragraph with its own angle; actor and annotation carry none, deliberately.
  Any new kind fact must earn its zero or write the paragraph.
- **An unexercised path is not evidence against the design.** Where research reports a
  corpus count of zero for a sanctioned move, read it as the absence of capture guidance
  until now, not as a refutation. Ship the design.
- **Reserved-word check**: grep each draft for `procedure`, `contract`, `fact`, `plan`,
  `done`, `activity`, `focus` and `role` used in their ordinary senses. Caught twice so far.
- **Domain-marked examples**: every quoted example names its domain, from the one set.

## Reconciliation items surfaced by research (for the plan's reconciliation phase, not the facts)

- Three legacy surfaces still ship the discredited completion-criterion aspiration test (framework-concepts:74-79, bootstrap skill:234, aspiration_capture.tmpl opener) + stale contract-first ordering.
- framework-concepts "Default?" column and SKILL kind-default prose = CLI-legacy per d-cpt-1dk; frozen, must not migrate.
- Won't-pursue question close is sanctioned but no check accommodates it (candidate gap capture).
- graph.go doc "questions awaiting dissolution" vs practice (all closes were decisions) — align wording.
- sdd show renders no actors/when/involvement on a focus entry — candidate gap capture.
