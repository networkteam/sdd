# Pattern A modularization plan

This document records the design dialogue and decisions behind the tactical plan to extract `/sdd`'s playbook moves into on-demand references with explicit load-gates. It does not duplicate the comparative findings (s-prc-gu6's attachment) — only the design that follows from them.

## Choice — Pattern A over Pattern B

The comparative examination of three Claude Code skill bundles (GSD, gstack, beads) produced two candidate directions for `/sdd` modularization (per s-prc-nxy):

- **Pattern A**: extract heavy playbook prose into on-demand `references/playbook-<name>.md` files with explicit load-gates in SKILL.md. Conditional content lives outside the always-loaded surface.
- **Pattern B**: create more sub-skills with independent triggers (e.g. `/sdd-augment`, `/sdd-implement`, `/sdd-decide`), each carrying its own playbook content.

Pattern A is selected for four reasons drawn from the comparative findings:

1. **Quantified token win precedent**. GSD's `<progressive_disclosure>` cut entry-point load from ~13K → ~0 for `discuss-phase` by gating Read() on flag/condition. `/sdd`'s playbooks (catch-up 55 lines, explore 37, groom 30, augment-plan 35, implementation 73) are conditionally relevant — direct analog.
2. **Trigger-collision avoidance**. gstack's experience (renaming `/debug` and `/checkpoint` after host built-ins shadowed them; collision-sentinel test as insurance) shows sub-skill proliferation increases collision risk and demands ongoing trigger-engineering investment. Pattern A keeps content inside `/sdd`, sidestepping the entire class.
3. **Reuse of existing infrastructure**. `references/framework-concepts.md`, `meta-process.md`, `cli-reference.md` already exist as the always-loaded supplement pattern. Extracting playbooks extends rather than reinvents.
4. **Reversibility**. If routing reliability drops below baseline, specific playbooks can be promoted to sub-skills later. The reverse direction (collapsing sub-skills back into references) is harder once trigger metadata is established and external callers depend on it.

## What gets extracted

| Section | Lines | Reference path |
|---|---|---|
| Catch-up Playbook | 214–268 (~55) | `references/playbook-catchup.md` |
| Explore Playbook | 270–306 (~37) | `references/playbook-explore.md` |
| Grooming Playbook | 308–337 (~30) | `references/playbook-groom.md` |
| Augment Plan Playbook | 339–373 (~35) | `references/playbook-augment-plan.md` |
| Transition to implementation (incl. branching + worktree) | 375–447 (~73) | `references/playbook-implementation.md` |

Total extracted: ~230 lines. SKILL.md projects from 447 → ~217 lines.

## What stays inline (always-loaded invariants)

The high-invariant constraints stay in SKILL.md because they apply to every interaction, not specific modes:

- Frontmatter (trigger surface)
- Intro + "First things first" (framework reference loading + initial check-in)
- Keep dialogue focused (response shape rule)
- Vocabulary register by surface (touched in every interaction — ~29 lines)
- Always dialogue before capturing (the playback-confirm-capture cycle + pre-flight severity guidance)
- Verify the captured summary
- Always suggest next steps
- Never jump to implementation
- After every completion (evaluation prompt)
- Use the right graph operations (full ID rule)
- Get the entry right (kind/layer/refs/short-loop smell test)
- Modes of working overview (refactored into the gate table — see below)

The "capture-detail" blocks (attachment assessment, infer participants from session context, write canonical names, language handling, reacting to sync-check output) total ~61 lines. They are loaded on every capture, which happens in most sessions; the load-savings ratio is meaningfully lower than for mode-specific playbooks. **Round 2** can evaluate moving them after Round 1 data is in. `Reacting to sync-check output` (10 lines, situational) is the easiest candidate within that group if a quick win is wanted.

## Gate format design

The Modes of working section (currently lines 186–212 of SKILL.md) presents each mode as a paragraph that points to "the Playbook below." The refactor turns it into an explicit gate table — one row per mode naming the trigger and the reference path:

```
| Mode | Trigger | Reference |
|---|---|---|
| Bootstrap | Empty graph; lacking actors or aspirations | invokes /sdd-bootstrap sub-skill |
| Check-in | Session start; "where are we?" | references/playbook-catchup.md |
| Capture | User shares observation, insight, finding | (inline — capture discipline already always-loaded) |
| Evaluate | A done signal landed; user asks about it | references/playbook-catchup.md (after-completion guidance) |
| Explore | "Dig into N", entry ID named, topic pointed at | invokes /sdd-explore then references/playbook-explore.md |
| Reflect/Dialogue | Open exploration, no specific entry | (inline — natural agent behavior) |
| Decide | Open signals or tensions need resolution | (inline — capture discipline + decision-kind rules) |
| Act/Implement | "Let's build this" | references/playbook-implementation.md |
| Augment plan | Refinement mid-implementation | references/playbook-augment-plan.md |
| Groom | "Let's groom"; user-suggested cleanup | invokes /sdd-groom then references/playbook-groom.md |
```

Followed by the instruction:

> Load the listed reference when entering the matching mode. Do not load references for modes you are not in. Reload (with fresh content) only if the file changed since the last read.

This is the explicit gate that GSD's `<progressive_disclosure>` block exemplifies, adapted to `/sdd`'s mode-driven dispatch shape.

## Catch-up structural demotion

The `/sdd-catchup` sub-skill fails the refined service-vs-mode test (per s-prc-gu6): its "output" is the agent's continuing context, not a discrete artifact the caller moves past. The current SKILL.md tells the agent to run `sdd status` and cluster directly anyway — the sub-skill is largely vestigial, used only by `/sdd-bootstrap`'s hand-back.

This plan removes `/sdd-catchup` and routes the full-catch-up content through `references/playbook-catchup.md` like any other playbook move. The content itself doesn't change — only its location and invocation path.

**Mode differentiation (quick / topic-drill) is explicitly deferred.** Quick-mode wants warmth-aware filtering of the graph (per dialogue notes); topic-drill wants semantic-search ranking. Both depend on capabilities not yet built — `sdd search` (d-tac-lqr) is the most direct dependency. A follow-up directive captures these enhancements as scoped follow-up work, activating after the underlying capabilities ship.

`/sdd-bootstrap` currently hands back to `/sdd` via `/sdd-catchup`. After this plan lands, the hand-back invokes the full-catch-up mode directly through `/sdd`'s gate table.

## Scenario rubric

10 scenarios, each specifying the expected reference loads (and implicitly: no other references should load). Tested in fresh agent sessions against the candidate branch.

| # | Scenario | Expected loads |
|---|---|---|
| 1 | Session start with `/sdd` invocation; user says "where are we?" | playbook-catchup |
| 2 | Capture flow — user shares an observation that becomes a signal | (none — capture discipline inline) |
| 3 | Decide flow — open signals need resolution; user asks for a directive | (none — decision-kind rules inline) |
| 4 | Evaluate flow — user asks about a recent done signal | playbook-catchup (after-completion section, if separate) |
| 5 | Transition to implementation — "let's build this" | playbook-implementation |
| 6 | Augment-plan trigger mid-implementation — refinement surfaces | playbook-augment-plan, possibly playbook-implementation |
| 7 | Grooming sweep — user says "let's groom" | invoke `/sdd-groom`, load playbook-groom |
| 8 | Explore drill — user points at entry ID or thread number | invoke `/sdd-explore`, load playbook-explore |
| 9 | Sub-skill hand-back — `/sdd-bootstrap` completes and hands back | playbook-catchup |
| 10 | Casual non-mode dialogue — user asks "what does X mean?" | (none — false-positive prevention) |

Each scenario gets a manual pass/fail judgment based on whether the expected loads matched what the agent actually loaded. The rubric is not exhaustive — it's representative coverage to detect regressions.

## Token measurement methodology

Two complementary instruments documented for the implementing agent:

1. **Headline numbers via Claude Code `/usage`** — replicate the s-tac-n8u methodology. Fresh session → `/sdd` invocation → read `Total Messages` from `/usage` panel. Run per scenario to capture per-mode load deltas. Authoritative because it uses the runtime that runs SDD.
2. **Per-file precision via Anthropic `count_tokens` API** — `POST /v1/messages/count_tokens` returns exact input-token count for given content. Used for the static-file budget (SKILL.md, each reference file) without running a session. Cheap, deterministic, useful for the per-file table in the closing done.

The closing done captures both: cold-start metadata + post-`/sdd` total Messages (vs s-tac-n8u baseline) for the headline; per-file token counts (count_tokens API) for the per-file budget.

## Decision criterion

The closing done articulates whether the change lands or reverts based on three sub-criteria:

1. **No scenario regressions vs baseline**: every scenario in the rubric that worked before still works after. New scenarios may pass or fail; existing ones must not regress.
2. **Measurable token reduction on cold start and per-mode loads**: cold-start metadata cost and per-mode load deltas show meaningful reduction. No fixed threshold (the magnitude of the change matters less than the structural improvement); the closing done articulates the observed magnitude and reads it qualitatively.
3. **Qualitative read of dialogue flow against existing experience**: does the agent feel as fluent as before? Are there observable hesitations or mis-loads? Subjective but load-bearing — the agent's ergonomics for the user matter as much as the token budget.

If 1 holds and 2–3 read positively, the change lands and the closing done captures the data. If 1 fails on any scenario or 3 reads negatively, the change reverts (via discard branch flow per existing implementation playbook); the closing done captures what was learned for a follow-up attempt.

## Open questions and risks

- **Reference-loading reliability**: the load-bearing risk per s-stg-3vr (text instructions don't reliably control agent behavior). The gate table is a textual instruction; if the agent ignores it and either skips loads (under-fetching) or loads everything (over-fetching), Pattern A regresses to gstack-shape. The scenario rubric is the test.
- **Mode boundary blur**: some sessions cross modes (catch-up → explore → capture). The gate table assumes the agent re-evaluates per mode entry. If the agent loads and then doesn't reload after a mode change, references go stale across modes. Handled by the "reload only if the file changed" instruction, but worth observing.
- **Implementation playbook scope**: 73 lines is the heaviest extraction; includes branching and worktree subsections. May warrant splitting into `playbook-implementation.md` and `playbook-branching.md` if the file feels unwieldy. Decided during implementation; not a plan-level commitment.
- **`/sdd-bootstrap` hand-back**: changing the invocation path from `/sdd-catchup` to inline-load-of-playbook-catchup requires verifying `/sdd-bootstrap` still completes cleanly. Tested in scenario #9.

## Out of scope

- **Mode differentiation for catch-up** (quick mode with warmth-aware filtering; topic-drill via semantic search) — deferred to a follow-up directive, scoped to activate after `sdd search` (d-tac-lqr) ships.
- **Capture-detail block extraction** (attachment assessment, infer participants, canonical names, language, sync-check) — Round 2 candidate, evaluated after Round 1 data lands.
- **Vocabulary register extraction** — stays inline; touched in every interaction.
- **Other sub-skill consolidation** (`/sdd-explore`, `/sdd-groom`, `/sdd-bootstrap`) — these hold under the refined service-vs-mode test (per s-prc-gu6); no demotion warranted.
- **Lint enforcement of gate-table consistency** (ensuring every named reference exists and every reference is named in the gate) — could be added as a follow-up, but not required for Round 1.
