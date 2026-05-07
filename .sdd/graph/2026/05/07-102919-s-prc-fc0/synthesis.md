# SDD mode-table synthesis

This attachment records the design dialogue that produced the insight signal, capturing the matrix, move-flavor split, brief shapes, evaluation framing, and sequencing recommendation in one place.

## Trigger

The dialogue started from observing how `/sdd-explore` was invoked in a live session: the sub-skill ran before `playbook-explore.md` loaded, and its Step 4 used the pre-search LLM-as-semantic-matcher recipe (bare `sdd list` then read everything) instead of `sdd search --query` / `--term`. Captured as gap signal s-prc-kaw. While shaping the fix, three triggers exposed a deeper structural question:

- "Explore the topic of pre-flight validation oscillation" — topic, no anchor
- "Start implementation of d-tac-123" — concrete entry, implementation intent
- "Explore s-cpt-abc for implementation" — entry given, hybrid intent

These don't fit the current mode-table split (Explore vs Act/Implement) cleanly. The boundary is artificial — every implementation involves context gathering, every explore points at "ready to build?" as one outcome. A parallel session picking up d-tac-uww at slice 5 confirmed the universal-entry-move pattern naturally: the agent ran anchor → chain → brief → continue without a mode-switch, and produced an AC-status table brief on its own initiative.

## Universal entry move: Engage

Every session anchored on a graph entry (or a topic that resolves to one) follows the same opening:

1. **Anchor** — entry ID given, or `sdd search --query "<topic phrase>"` to surface candidates and dialogue toward picking the centre.
2. **Read chain** — `sdd show <id> --downstream`. Surfaces upstream rationale, augment-plan amendments, partial done signals against ACs, supersession.
3. **Brief** — what the entry is about, current status, what's around it. Shape varies by entry kind and chosen intent (see "Brief shapes by intent" below).
4. **Offer moves** — based on entry kind, current state, and dialogue intent, surface the candidate next moves. The user picks.

The sub-skill `/sdd-explore` is invoked from inside step 4 (or step 1 for heavy topic searches) when chain + neighbors would bloat the outer agent. It's a tool, not a mode-entry precondition. Crucially, the outer must pass a **goal** to the sub-skill — without a goal, compression has no axis.

## Post-engage moves: two flavors

| Flavor | Definition | Examples |
|---|---|---|
| **Acting on** the entry | Outcome is a graph change *against* the entry | implement, supersede, close, decompose, refine, short-loop, plan it |
| **Using** the entry as a lens | Outcome is signals/findings about *other* work or external state, framed by the entry | evaluate (against a commitment), compliance check (against a contract), alignment check (against an aspiration), focus-progress check (against a focus) |

Today's mode table only covers acting moves cleanly. Lens moves are undernourished.

## Per-kind move matrix (first pass)

| Kind | Type | Post-engage moves |
|---|---|---|
| gap | signal | capture decision · plan it · short-loop done · close as not relevant · refine |
| fact | signal | reference in decision · retire (no longer true) · correct (supersede) |
| question | signal | answer (fact/insight + close) · refine · won't-pursue close |
| insight | signal | capture decision based on it · refine · close (acted on) |
| done | signal | **evaluate / capture follow-up signals** · retrospective · no-action |
| actor | signal | identity correction · retire (directive) |
| annotation | signal | replace · retire |
| directive | decision | implement · supersede · close via done · **evaluate (post-closure)** · retire |
| activity | decision | implement (mechanical) · close via done · **evaluate (post-closure)** |
| plan | decision | implement (AC checklist) · augment · decompose · restructure · **evaluate (post-closure)** |
| contract | decision | **compliance check** · supersede · retire |
| aspiration | decision | **alignment check** · evolve · retire |
| role | decision | supersede · cascade-retire (via actor closure) |
| focus | decision | **progress check** · re-prioritize · close (cycle done) |

Bolded moves are where the lens-flavor framing lives. Closed commitments (directive/activity/plan) gain `evaluate (post-closure)` — today's matrix says "no further moves" once closed; that's wrong, closed entries are evaluable.

## Brief shapes by intent

The brief (step 3 of the universal entry move) takes its shape from the combination of entry kind and chosen intent. Examples:

- **Plan + implementation intent** — AC-status table with three columns: ✓ Done (covered by partial done signals or self-evident from chain), ◐ Partial (started but incomplete; includes augmenting commitments still open), ☐ Remaining (not yet touched). Surfaces the work checklist directly. Validated in the d-tac-uww slice-5 pickup session.
- **Signal + explore intent** — narrative: what is this about / status / what's happened since / what's around it. Closes with the orienting "what does this need?"
- **Contract + compliance-check intent** — divergence list: target work area (code, recent decisions, in-flight implementations) measured against the contract; alignments and divergences both surfaced.
- **Done signal + evaluate intent** — split brief: what landed (from the done body and what it closes) plus the lens choices available (outer / inner; see "Evaluation framing").
- **Aspiration + alignment-check intent** — alignment scoring: how a candidate decision pulls toward (or away from) the aspiration's direction.

The brief is not one shape — it's a family. The agent picks the shape based on what the user is engaging on and what they're trying to do.

**Mid-flight pickup** is a recognizable sub-pattern within plan + implementation intent: when prior implementation paused at a slice boundary, the most recent partial done signal in the chain typically carries pickup notes. The downstream traversal surfaces it; the brief reads the pickup notes as the resumption anchor and shapes the AC-status table around what's outstanding. No mode switch — same Engage flow, kind/intent-shaped brief handles continuity.

## Blind spots in today's mode table

| Blind spot | Today | What's missing |
|---|---|---|
| Evaluate (post-completion) | Mode-table row routes at `playbook-catchup.md` — wrong | Its own ref. `meta-process.md` already names the structure: "decision (what to evaluate against) → done signal (who reviewed) → signals (findings)" |
| Pre-completion review (QA) | Partly covered by `playbook-implementation.md` step 11 | Distinct concept: "is this ready to close?" against ACs + augmenting commitments. Could share lens machinery with post-completion evaluate |
| Compliance check | Pre-flight does it for new captures only | Move that takes a contract + a target work area, returns aligned/divergent findings |
| Alignment check | Implicit in dialogue, never named | Move that takes an aspiration + a candidate decision |
| Focus-progress check | Only via `sdd status` Participants block | Move that walks a focus's involvement triples for stale / completed / pull-available |

## Evaluation framing

Two anchor points lead into evaluation:

| Anchor | Framing question |
|---|---|
| Commitment (directive / activity / plan with done) | "Did we deliver what we said?" — AC contract, original intent |
| Done signal directly | "What do we make of this?" — work as delivered, on its own terms |

Two perspective axes within an evaluation, to be reasoned about together so neither is dropped:

| Perspective | Lenses | Mode |
|---|---|---|
| **Outer** (behavioral) | E2E (manual / automatic), user feedback, real-usage observation | "does it do what users / external systems experience it doing?" |
| **Inner** (structural) | Code review, architecture review, conceptual review (do captured concepts still hold under the implementation?) | "does it hold up under inspection?" |

Ceremony test for an evaluation:

- **Lightweight** (small diff, narrow scope) — review inline, capture findings as signals refed back
- **Heavy** (release, multi-day work, multi-perspective) — capture an `activity` decision for the eval ("review X via lenses Y, Z"), then run, then capture done signal + findings refed back

Pre-completion review (the "is this ready to close?" flavor) uses the same lens machinery; only the closing branch differs (informs whether to close / augment / keep working, rather than producing follow-up signals).

## Sequencing recommendation

1. **Engage refactor** (captured as a plan decision following this insight). Closes s-prc-kaw. Reshapes mode table, writes `playbook-engage.md`, repositions `/sdd-explore` with goal-tagged compression and search-primary related-entry surfacing, fixes load order.
2. **Lens-flavor moves** (a follow-on directive). Starts with `playbook-evaluate.md` to fix the misrouted Evaluate row and provide the eval framing as concrete instructions. Compliance / alignment / focus-progress checks queued behind, surfaced as need warrants.
