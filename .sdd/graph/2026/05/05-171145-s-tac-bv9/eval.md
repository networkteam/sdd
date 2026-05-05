# Scenario eval — d-tac-ab1 Pattern A modularization

Purpose: validate that the gate-table dispatcher in the refactored `/sdd` SKILL.md routes each mode to the right reference load, with no over-fetching (loading references that aren't needed) or under-fetching (skipping references that are needed).

Each scenario is a **fresh agent session in this branch** (`sdd/ab1-implementing-pattern-a-modularization`), Claude Opus 4.7. Steps per scenario:

1. Start a new chat (clear context)
2. Type the **Trigger** prompt as the first user message
3. Watch which Read tool calls fire on `references/*.md` files — that's the **Actual loads** column
4. Mark Pass/Fail and capture any observations

**Pass criterion** per scenario: actual loads match expected loads (correct references loaded, no extras). Capture deviations even on Pass — they're signal for follow-up tuning.

When all 10 are walked, the table feeds into the closing done signal alongside the token measurements already captured (see commit `24d82e8` summary).

---

## Headline data already captured (no eval needed)

- Cold-load reduction on `/sdd`: 15,479 → 8,722 tokens (~44% reduction in always-loaded SKILL.md)
- Per-reference-file token counts: catchup 1,615 / explore 875 / groom 1,201 / augment-plan 1,200 / implementation 2,090
- Trigger metadata surface: −1 skill (sdd-catchup removed)

Outstanding for AC 5: post-`/sdd` total Messages via `/usage` panel from a fresh session — capture that during scenario #1 below and record it once.

---

## Scenarios

### #1 — Session start: "where are we?"
- **Trigger**: invoke `/sdd` with no arguments, or first message: "where are we?"
- **Expected loads**: `playbook-catchup.md` (plus the always-loaded `framework-concepts.md`, `meta-process.md`, `cli-reference.md` per "First things first")
- **Should NOT load**: any other playbook reference
- **Also capture**: total Messages from `/usage` immediately after `/sdd` returns its catch-up — fills AC 5 cold-start delta vs s-tac-n8u baseline (51.3k tokens)
- **Actual loads**: `framework-concepts.md`, `meta-process.md`, `cli-reference.md`, `playbook-catchup.md` — all expected refs loaded; no extras
- **`/usage` post-`/sdd`**: not captured this run
- **Pass / Fail**: Pass (correct refs loaded, none extra)
- **Notes**: Ordering observation — agent ran `sdd status` / `sdd wip list` *before* loading `playbook-catchup.md`, then loaded the playbook and produced the catch-up. User expected the reverse: load the playbook first, then gather data per its guidance, then render. Rationale for the user's expectation: the playbook is the rendering rule; loading it first means the agent gathers data already knowing how it'll be presented. Current order works but is mildly inverted from the gate-table intent. Candidate tightening: rephrase the "First things first" line in SKILL.md so the load-then-act order is unambiguous (e.g. "Load `playbook-catchup.md` for clustering rules, then run `sdd status` and `sdd wip list`, then present per the playbook"). Treat as a follow-up tuning signal, not a regression.

### #2 — Capture flow
- **Trigger**: after `/sdd` completes catch-up, share an observation that should become a signal: e.g. *"I noticed the `sdd lint` output is hard to read when there are dangling refs — the message format mixes severities."*
- **Expected loads**: none beyond what was loaded for #1 (capture discipline is inline in SKILL.md)
- **Should NOT load**: any playbook reference
- **Actual loads**: none (capture discipline served from inline SKILL.md content)
- **Pass / Fail**: Pass — directive decision captured without loading any additional reference
- **Notes**: Confirms that capture discipline (playback → confirm → run `sdd new`) works fully from the always-loaded surface. The decision-shaped capture (directive, not gap signal) shows the kind-distinction guidance was effective inline.

### #3 — Decide flow
- **Trigger**: in a session that has open signals visible from catch-up, ask the agent to help decide: *"There are a few open gaps around X — let's pick a direction."*
- **Expected loads**: none (decision-kind rules are inline in SKILL.md)
- **Should NOT load**: any playbook reference
- **Actual loads**: none — agent worked from inline content and inspected entries via `sdd show` / status data
- **Pass / Fail**: Pass
- **Notes**: Confirms the decision-kind distinguishing tests and confidence guidance work from the always-loaded surface. Entry inspection happened via the CLI rather than via reference loading, which is the right shape — the playbook references are about *how to render or transition*, not about reading entries.

### #4 — Evaluate flow
- **Trigger**: ask about a recent done signal from the catch-up: *"What landed in [recent done signal] — did it meet the target?"*
- **Expected loads**: `playbook-catchup.md` is acceptable (Evaluate mode points there per the gate table); none also acceptable if the agent answers from already-loaded context
- **Should NOT load**: explore / groom / augment-plan / implementation
- **Actual loads**: none new — `playbook-catchup.md` was already in context from earlier in the session; no other playbook reference loaded
- **Pass / Fail**: Pass
- **Notes**: The agent recognized that already-loaded context covered the evaluate move and didn't re-fetch. Aligns with the "Reload only if the file changed" instruction in the gate-table preamble. Caching across mode transitions within a single session works as intended.

### #5 — Transition to implementation
- **Trigger**: after a decision is in place, say *"Let's build this"* or *"Ready to implement [decision id]"*
- **Expected loads**: `playbook-implementation.md`
- **Should NOT load**: groom / augment-plan / catchup / explore
- **Actual loads**: `playbook-implementation.md` — exact match, no extras
- **Pass / Fail**: Pass
- **Notes**: Clean dispatch — gate table routed correctly on the "Let's build this" trigger. No over-fetching of adjacent playbooks (augment-plan, groom).

### #6 — Augment-plan trigger mid-implementation
- **Trigger**: in an implementation session (preferably continuing #5), surface a refinement that's narrower than a full supersede: *"Wait — AC 3 should also cover [edge case]. That's not in the plan yet."*
- **Expected loads**: `playbook-augment-plan.md`. May also re-reference `playbook-implementation.md` if not already loaded
- **Should NOT load**: catchup / groom / explore
- **Actual trigger used**: refinement on `d-tac-kud` AC 3 — the Bubble Tea selector's "default highlighted" doesn't name which scope; `project` is the obvious choice for SDD-instrumented repos since `.sdd/` is checked in
- **Actual loads**: `playbook-augment-plan.md` — exact match
- **Pass / Fail**: Pass
- **Notes**: User reports the dispatch was smooth — the agent recognized the augment shape (narrow refinement, single AC, no restructure) and loaded the right reference. No over-fetch onto implementation/groom/explore.

### #7 — Grooming sweep
- **Trigger**: in a fresh session, say *"Let's groom"* or *"There are some old open entries — can we sweep?"*
- **Expected loads**: invocation of `/sdd-groom` sub-skill, then `playbook-groom.md`
- **Should NOT load**: catchup / implementation / explore / augment-plan
- **Actual loads**: `/sdd-groom` sub-skill invoked first, then `playbook-groom.md` loaded once the sub-skill returned candidates
- **Pass / Fail**: Pass
- **Notes**: Exact match on the gate-table expectation. Ordering — sub-skill before playbook — is correct: the sub-skill produces the candidate data, then the playbook governs how to walk through them with the user. Mirrors the explore-mode pattern (#8) and confirms the "sub-skill produces evidence, playbook governs presentation" shape works for both.

  **Output-format observation (independent of Pattern A)**: User noted the rendered candidate list came out as one block per candidate (key-value lines, divider rules between entries) rather than the markdown table the playbook prescribes ("Build a summary table … with these columns"). Same instruction as pre-mod, so this is not a modularization regression — the agent's rendering interpretation. For 4 candidates with rich evidence the blocks are arguably more readable; for 10+ candidates a table would compress better. Candidate follow-up after the closing done lands: tighten the playbook wording to either insist on the table format or codify "table for ≥N candidates, block-per-candidate otherwise" if blocks are sometimes preferred.

### #8 — Explore drill (entry ID)
- **Trigger**: from catch-up, say *"Dig into #4"* or *"Tell me about [specific entry id from catch-up]"*
- **Expected loads**: invocation of `/sdd-explore` sub-skill, then `playbook-explore.md`
- **Should NOT load**: catchup (already loaded) / groom / implementation / augment-plan
- **Actual loads**: `playbook-explore.md` first, then `/sdd-explore` sub-skill ran (sub-skill internally loaded `framework-concepts.md` as part of its own startup)
- **Pass / Fail**: Pass on the load set (both expected refs loaded, no extras)
- **Notes**: **Ordering inconsistency observed across modes** — for catch-up (#1) and groom (#7) the agent fetched data first then loaded the playbook; for explore (#8) the agent loaded the playbook first then invoked the sub-skill. Both orderings produce correct behavior since the refs aren't sequence-dependent, but the inconsistency is itself a signal.

  **User-confirmed intent (post-#8 dialogue)**: load-first is the correct shape — *load the reference after the mode trigger fires (if not already in context) so the agent knows the exact playbook, **then** invoke sub-skills or gather data via CLI commands*. The playbook reference should guide the subsequent moves tightly. This makes the gate-table preamble's existing wording ("Load the listed reference when entering the matching mode") more specific by adding ordering.

  **Decision: roll the load-first refinement into AC 3's evidence in the closing done, no separate augmenting directive.** User flagged a separate directive as over-ceremonious for what's effectively a wording tightening on the preamble already adopted under AC 3. After scenario eval completes:
  - Edit SKILL.md preamble to make load-first explicit: "Load the listed reference first, then invoke any named sub-skill, then run any data-gathering commands. Sub-skills and CLI calls in a mode are guided by the just-loaded playbook, not the other way around."
  - Update the "First things first" intro line to follow the same order: load `playbook-catchup.md` first, then run `sdd status` and `sdd wip list`, then present.
  - The closing done's AC 3 evidence describes both the gate table and this preamble tightening as the implemented form of "load-gate format adopted."

### #9 — Sub-skill hand-back from `/sdd-bootstrap`
- **Trigger**: requires a bootstrap-state graph (sparse / no actors). Either set up a scratch graph manually, or simulate by invoking `/sdd-bootstrap` and observing the hand-back step at end of bootstrap
- **Expected loads**: `playbook-catchup.md` is loaded when bootstrap reaches its hand-back step (Move 5)
- **Should NOT load**: any other playbook reference; the old `/sdd-catchup` sub-skill must NOT be invoked (deleted)
- **Actual loads**: **deferred — needs a sparse-graph fixture to test cleanly**
- **Pass / Fail**: **Deferred (not blocking landing)**
- **Notes**: User chose to skip this round; capture as follow-up evaluation. The bootstrap hand-back path was updated in source (`internal/bundledskills/claude/sdd-bootstrap/SKILL.md` Move 5 now points at `references/playbook-catchup.md` instead of `../sdd/SKILL.md`), and the `/sdd-catchup` sub-skill is fully removed from the bundle and `.claude/skills/`. The structural change is verifiable via static inspection (commit `24d82e8`, AC 4); the runtime hand-back behavior in a fresh-bootstrap session is what's deferred. Run when next bootstrap-state graph appears.

### #10 — Casual non-mode dialogue (false-positive prevention)
- **Trigger**: in a session that already ran catch-up, ask a clarifying question that doesn't trigger any mode: *"What does `kind: directive` mean in this framework?"* or *"Why is layer separated from kind?"*
- **Expected loads**: none beyond what was already loaded; the agent answers from `framework-concepts.md` already-loaded context
- **Should NOT load**: any playbook reference
- **Actual trigger used**: *"What does `kind: directive` mean in this framework?"*
- **Actual loads**: none — agent answered from already-loaded `framework-concepts.md` context with a tight one-sentence reply
- **Pass / Fail**: Pass
- **Notes**: Critical false-positive check. The agent did not over-trigger into Decide / Capture / Explore mode despite the question containing "directive" — recognized it as a definitional question, not a mode trigger. Confirms the gate-table triggers are recognized at appropriate granularity (definitional vs. operational).

---

## Summary scoring

- **Pass count**: 9 / 9 walked + 1 deferred (#9). Effective scenario coverage 90%, all walked scenarios pass.
- **Regressions vs old behavior**: none. Every walked scenario produces correct routing; the agent answers and dispatches at least as well as the pre-mod monolith.
- **Over-fetching events**: none — no scenario showed loading of references outside the gate-table prescription.
- **Under-fetching events**: none — every expected reference loaded when its mode was active (or was correctly already-loaded from earlier in the session).
- **Qualitative read on dialogue flow**: positive overall. Dispatch feels crisp, capture/decide work fluently from the always-loaded surface, and false-positive prevention (#10) holds. One ergonomic friction surfaced: load order is inconsistent between modes — for catch-up and groom the agent gathers data first then loads the playbook; for explore it loads the playbook first then invokes the sub-skill. Root cause is structural (SKILL.md mode prose duplicates playbook content and pre-empts load-first dispatch), not behavioral. Resolution baked into the closing-done evidence: trim the mode prose for referenced modes so the playbook becomes the only "how to do this mode" surface.

## Decision call (AC 7)

Based on the three sub-criteria from the plan:

1. **No scenario regressions vs baseline**: ☑ holds (zero regressions across 9 walked scenarios)
2. **Measurable token reduction on cold start and per-mode loads**: ☑ confirmed — 15,479 → 8,722 tokens on `/sdd` cold load (~44% reduction), per-mode deltas 875–2,090 tokens
3. **Qualitative read of dialogue flow**: ☑ positive, with one structural refinement (mode prose trim) folded into AC 3 evidence

**Call**: ☑ **Land**

The closing done articulates the data, the call, and the load-first refinement (mode-prose trim + intro line update) as the implemented form of AC 3. Scenario #9 remains as a follow-up evaluation when a bootstrap-state graph is next available.
