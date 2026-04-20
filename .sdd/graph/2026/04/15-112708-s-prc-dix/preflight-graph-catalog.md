# Pre-flight Graph Catalog

## Method

Enumerated all signals (16 entries) and actions (76 entries) via `sdd list --type s` and `sdd list --type a`. Triaged all 92 summaries for pre-flight relevance using semantic reading (not lexical grep). Pulled full `sdd show` details for 30 entries. Classified 28 as pre-flight-related across all four categories. Additionally examined 7 key decisions referenced by the signals/actions to understand the reasoning chains.

## Entries by classification

### friction-raised

**20260412-115018-s-prc-2nh** (2026-04-12, process) -- Over-correction on framing after human dialogue confirms closure

> "Attempt 1 rejection was useful (missing commit reference -- structural gap). Attempts 2-3 rejected on increasingly pedantic grounds: misinterpretation of d-cpt-vt1 (disagreed with how the contract was characterized), vague evidence (commit hash was present but validator wanted more), pattern unresolved (demanded the general pattern be addressed, not just deferred to s-prc-80l)."

> "The validator lacks dialogue context -- it sees only the entry text and graph, not the conversation that produced the decision to close."

Proposes distinguishing structural completeness (missing refs, uncovered requirements, no evidence) from argumentative re-evaluation of closure rationale. The latter is the human's job.

---

**20260412-115120-s-prc-as6** (2026-04-12, process) -- Pattern-reshaping entries get category-error rejections + reassembly cost

> "First rejection (missing related entry s-cpt-s43) was genuinely helpful. Second rejection produced four concerns: two were valuable [...] and two were category errors ('kinds introduced without grounding' -- a directive introducing new concepts inherently contains ungrounded material; 'alters contracts without superseding' -- the entry explicitly deferred superseding to post-validation, which the prompt couldn't assess)."

> "The re-assembly cost of a large heredoc command with attachment via stdin across three attempts was significant -- each retry required re-sending the full attachment content."

Identifies two distinct problems: (1) validator cannot distinguish pattern-building from pattern-reshaping entries, (2) operational cost of retries is punishing for entries with large attachments.

---

**20260414-145121-s-prc-okf** (2026-04-14, process) -- Regression test contract applied to template refinements

> "A template wording refinement (improving a definition, sharpening a distinction) is not a bug fix -- it's an improvement with no code path to regression-test. The contract needs to distinguish bug fixes (broken behavior that needs correction) from refinements (correct but imprecise behavior that gets sharper)."

Flags that d-prc-31v (regression test contract) lacks scope clarity, causing pre-flight to demand tests for non-testable prompt template improvements.

---

**20260414-164554-s-prc-6hw** (2026-04-14, process) -- Workflow disruption: cost-benefit of blocking validation is wrong

> "Pre-flight rejected entries 3-4 times before acceptance -- each rejection costs 60-120 seconds of wall-clock time plus context-switching overhead for rewording."

> "A tool that blocks the user 3 times per entry to catch 1 real issue is a net negative on velocity."

> "Haiku's 120-second timeout is too tight for closing actions with many refs -- the prompt grows with each referenced entry, and complex entries regularly hit the timeout."

Proposes three directions: make pre-flight advisory (warn, don't block), reduce prompt size by using summaries, add fast-path for entries where the proposing agent already dialogued the content.

---

**20260414-202842-s-prc-wi7** (2026-04-14, process) -- Closing action validates against full plan prose, not acceptance criteria

> "4 rejections for dedup algorithm, output format spec, and per-direction depth -- all present in the plan attachment and implementation."

> "Plans should carry explicit acceptance criteria that scope what pre-flight validates on closing, what the implementing agent uses as a work checklist, and what deviations must explain."

Identifies that pre-flight for closing actions validates against full plan prose rather than focused acceptance criteria, causing rejections for details that exist in code and attachments.

---

**20260414-233739-s-prc-316** (2026-04-14, process) -- Binary PASS/FAIL creates find-something bias

> "The binary PASS/FAIL format incentivizes the LLM to stretch for gaps when none exist, producing false rejections (e.g. demanding CQRS decomposition for a logging plan, demanding neutrality justification for a dialogued design choice)."

> "A confidence-scored output format (high/medium/low per finding, string not numeric) would let the LLM name observations without forcing a verdict, with tooling applying a threshold to decide what blocks."

> "Three levels are enough for the LLM to consistently assign; numeric scales invite clustering and calibration drift."

Proposes the confidence-scored verdict format that is the subject of the planned work. Names two specific false-positive cases (CQRS for logging, neutrality justification for dialogued choice). Prescribes calibration examples anchoring each level.

---

**20260414-144454-s-prc-q2m** (2026-04-14, process) -- Signals rejected for containing solution-space thinking

> "Pre-flight rejects signals containing solution-space thinking as 'crossing into decision territory.'"

> "Signals expand dialogue by bringing observations and potential directions as material for discussion. Decisions resolve dialogue into commitments."

Proposes testing whether entries prescribe commitments (decision) vs. explore material for dialogue (signal), rather than whether they mention solutions.

---

**20260414-002651-s-cpt-ix1** (2026-04-14, conceptual) -- Rigid validation on structural moves (contract retirement)

> "When the first attempt to capture d-prc-22i (superseding the d-prc-zch contract with a directive) was rejected, the validator demanded either a companion contract or a kind-preserving supersede -- structurally ceremonious when the substance of the decision was 'this rule shouldn't be a graph contract at all.'"

> "The risk of rigidity: SDD starts to feel bureaucratic. Every rare but legitimate move -- contract retirement, pattern reshaping, structural experimentation -- gets rejected and requires --skip-preflight, teaching users to bypass validation rather than engage with it."

Flags that pre-flight's dialogue-confirmed-context principle (from entry_quality template) does not extend to structural moves like kind changes and contract retirement.

---

**20260413-162853-s-prc-ql7** (2026-04-13, process) -- Knowledge-capture actions lack durability path

> "Pre-flight rejected three attempts to capture the action: first without any acknowledgment of the constraint, second with the dialogue context folded into the entry text, third explicitly noting the user's confirmation in dialogue. Pre-flight rejected each time, with the third rejection being explicit and correct in its reasoning: 'active contracts require formal override, not dialogue acknowledgment.'"

> "The gap is in the contract's framing, not the validator."

Identifies blind spot where d-prc-zch requires prior Git commits but knowledge-capture actions (research, design docs) create artifacts during entry creation via --attach.

---

**20260412-120433-s-prc-g7x** (2026-04-12, process) -- Self-description failure in entry first sentences

> "A plan entry started with 'Plan for d-tac-1g4' which is meaningless without following the ref."

Flags that pre-flight should enforce self-describing first sentences. Narrower than other friction signals but directly shapes entry_quality template content.

---

**20260413-085100-s-tac-7ze** (2026-04-13, tactical) -- Need for --dry-run to iterate on pre-flight

> "Current workflow forces a commit-to-test cycle for anyone wanting to check whether their wording would pass, which discourages experimentation."

Proposes --dry-run flag to preview pre-flight results without committing, reducing the cost of iteration.

---

**20260413-143351-s-prc-8na** (2026-04-13, process) -- Graph densification as pre-flight side-effect (positive friction)

> "Pre-flight doesn't just check structural completeness of individual entries -- it actively densifies the graph by forcing entries to cite the decisions they build on."

> "Pre-flight is doing graph-maintenance work as a side-effect of per-entry validation, not just per-entry quality control."

This is friction that is acknowledged as positive -- a second-order benefit where pre-flight forces explicit refs that improve graph structure.

---

**20260411-235938-s-prc-5ht** (2026-04-11, process) -- Signal depth enforcement proposal

> "Signals risk becoming shallow feature requests (the What) instead of capturing the underlying gap (the Why). 'Users want multiple emails' vs. 'users work around single-email with forwarding because they can't separate work/personal notifications.'"

> "The pre-flight validator could enforce it."

Proposes pre-flight enforcement of signal depth. Shapes what signal_capture.tmpl should validate.


### fix-recorded

**20260412-001439-a-tac-umu** (2026-04-12, tactical) -- Initial pre-flight validator implementation

> "Implemented pre-flight validator per plan d-tac-hjp. [...] Deviation: no PreflightFailedError type [...] Deviation from plan item 4 which specified graceful degradation with preflight:error annotation -- user feedback during implementation: silent annotation is not obvious degradation, fail hard and let the user decide."

> "6 golden cases -- missing plan items, full coverage different wording, smuggled decision, valid signal, real graph history a-tac-tsd scope-out, contract violation. All 6 pass."

Built the entire pre-flight infrastructure: templates, runner, CLI integration, hard-fail behavior.

---

**20260412-001854-a-tac-di2** (2026-04-12, tactical) -- Pre-flight self-evaluation found 5 real gaps

> "First run caught 5 legitimate gaps in plan coverage -- all real."

> "Three design adjustments: (1) hard failure on all errors including infra [...] (2) closing-action template needs to accept explicit deviations with reasoning [...] (3) timeout needs to be generous (120s default) and configurable for large prompts."

Applied pre-flight to its own implementation plan. All adjustments were implemented. Validated generation-validation separation in practice.

---

**20260413-084616-a-tac-eqc** (2026-04-13, tactical) -- Dialogue-context improvements to pre-flight templates

> "A shared entry_quality template block now included in all pre-flight templates enforces self-describing first sentences [...] and teaches the validator to treat dialogue-sourced reasoning in entry text as human-confirmed context -- focusing validation on structural completeness (missing refs, uncovered requirements, missing evidence) rather than re-evaluating rationale that was already dialogued."

Fixed the over-correction pattern (s-prc-2nh) and self-description failure (s-prc-g7x) by distinguishing structural checks from rationale re-evaluation.

---

**20260414-003043-a-prc-0ka** (2026-04-14, process) -- Action-durability enforcement moved to pre-flight template

> "Added 'Durability' as Check 5 with the three-paths wording: presence check accepting (a) a prior commit referenced by the action, (b) attachments on this entry via --attach, or (c) attachments on a referenced upstream entry already in the graph."

Fixed the knowledge-capture action blind spot (s-prc-ql7) by accepting three durability paths instead of requiring prior commits only.

---

**20260414-110339-a-tac-one** (2026-04-14, tactical) -- Template alignment: dialogue-driven validation

> "Supersedes.tmpl: replaced type-consistency check with dialogue-driven replacement -- the validator now checks that entry text explains what happens to the superseded entry's concerns and why the replacement takes the form it does, rather than demanding structural form matches."

> "Entry_quality.tmpl: extended dialogue-context-is-grounding principle to cover structural departures -- when entry text argues for a non-standard structural move, validator treats the reasoning as human-confirmed context."

> "Additional fix discovered during eval testing: formatEntryForPrompt did not include Attachments field, so the durability check couldn't see attachment metadata."

Fixed rigid structural checks (s-cpt-ix1) by replacing form-matching with reasoning-presence checks.

---

**20260414-144812-a-prc-sbc** (2026-04-14, process) -- Reframed signal_capture.tmpl type-correctness

> "Previous wording rejected signals containing solution-space thinking as 'smuggled decisions.' New wording uses the dialogued framing: signals expand dialogue by bringing observations and potential directions as material for discussion; decisions resolve dialogue into commitments."

> "No regression test: this is a prompt template wording change, not a code bug -- d-prc-31v applies to code fixes, not prompt refinements."

Fixed the signal/decision boundary check (s-prc-q2m). Also explicitly noted that prompt template changes are not subject to regression test contract -- directly relevant to s-prc-okf.

---

**20260414-145731-a-prc-68m** (2026-04-14, process) -- Grooming pass closing 4 pre-flight-related signals

> "s-prc-as6 (pre-flight over-fires on pattern-reshaping): Stdin persistence solved re-assembly friction (d-tac-q5p). Template alignment (a-tac-one) added structural-departures principle to entry_quality.tmpl. Signal/decision distinction fixed (a-prc-sbc). The original three-rejection scenario would go much better now."

> "s-prc-8na (graph densification as pre-flight side-effect): Observation recorded -- pre-flight actively densifies the graph by forcing entries to cite decisions they build on. No action needed."

Closed s-stg-3vr, s-prc-868, s-prc-as6, s-prc-8na based on cumulative pre-flight improvements.

---

**20260412-003919-a-prc-cw0** (2026-04-12, process) -- Resolved plan-required policy

> "The always-required policy was wrong -- start-side enforcement is instruction-only (unreliable per s-stg-3vr), and small fixes from signals don't need plans. The enforceable part (closing-action pre-flight validates plan items when present) already shipped."

> "Plan needed when the closing pre-flight needs plan items to validate against; skip for small obvious fixes."

Resolved tension between mandatory plans and practical workflow by making plans optional but pre-flight-validated when present.

---

**20260412-114432-a-prc-ifz** (2026-04-12, process) -- Scope-skipping signal closed via structural enforcement

> "The pre-flight validator (d-tac-s6w) now structurally enforces this -- the closing-action template validates that every requirement in the closed decision is accounted for by the action."

Demonstrated pre-flight catching real dropped requirements (attachment validation from d-tac-kfo).

---

**20260414-142957-a-ops-4zc** (2026-04-14, operational) -- Fixed kind-field inconsistency between writer and validator

> "Fixed kind-field disagreement between sdd new and pre-flight (commit 977769b). Added BuildEntry method to NewEntryCmd that defaults Kind to directive for decisions."

> "Regarding s-prc-17y: the original stdin persistence bug fix (pre-CQRS) shipped without a regression test, violating d-prc-31v."

Fixed a writer/validator inconsistency where sdd new didn't write kind to frontmatter but pre-flight rejected entries without it.

---

**20260413-100503-a-tac-e21** (2026-04-13, tactical) -- Stdin persistence to reduce retry friction

> "Adds stdin persistence and a --dry-run flag to sdd new so users can iterate on entry framing across pre-flight rejections without re-piping content."

Addressed the reassembly friction from s-prc-as6 where large attachments had to be re-sent on every retry.


### behavior-described

**20260410-211553-d-tac-s6w** (2026-04-10, tactical) -- Foundational decision: move pre-flight to tool layer

> "sdd new invokes claude -p as a pre-flight validator before creating graph entries. The CLI selects the appropriate pre-flight check based on operation type (closing action, decision, signal, supersedes). Pre-flight prompt templates are embedded via go:embed into the sdd binary."

> "Hard gate -- entry creation is rejected with reported gaps on failure."

The decision that created pre-flight as a tool-layer enforcement mechanism. Motivated by s-stg-3vr (instructions unreliable) and s-prc-868 (agents drop requirements).

---

**20260412-004028-d-cpt-vt1** (2026-04-12, conceptual, contract) -- Structural separation contract

> "The agent that acts cannot reliably certify its own compliance. All reliability-critical workflows must include a validation step performed by a separate agent or tool invocation."

The foundational contract that pre-flight implements. Pre-flight is the first (and primary) instance of generation-validation separation.

---

**20260413-120558-d-prc-31v** (2026-04-13, process, contract) -- Regression test contract

> "Every bug fix requires a regression test, regardless of how the bug was reported [...] the test must fail before the fix and pass after, and must land in the same change as the fix."

Contract that pre-flight enforces, which s-prc-okf identifies as lacking scope (doesn't distinguish bug fixes from refinements).

---

**20260412-004044-a-tac-7ca** (2026-04-12, tactical) -- Pre-flight completion action

> "Pre-flight validator is complete. [...] The validator is the first instance of the generation-validation separation contract."

Closes d-tac-s6w by bundling the implementation (a-tac-umu), self-evaluation (a-tac-di2), and plan-policy (a-prc-cw0).

---

**20260410-212631-a-cpt-1tt** (2026-04-10, conceptual) -- Process reliability research

> "External enforcement dramatically outperforms instructional guidance. Four control layers (prompt, tool, workflow, validation) -- SDD currently relies almost entirely on prompt layer. The generation-validation separation is the highest-leverage pattern."

Research that provided the theoretical foundation for pre-flight's design. Attachment contains full synthesis.

---

**20260413-084814-s-prc-awd** (2026-04-13, process) -- Positive validation of dialogue-context fix

> "Pre-flight passed on the first attempt -- no framing nitpicks, no demands to re-characterize referenced contracts, no hallucinated entry refs. Contrast: pre-change, closing s-prc-iwx took three attempts."

> "One data point isn't conclusive, but the self-test shows the shared entry_quality block shifted the validator's attention toward structural completeness."

Validates that the a-tac-eqc fix worked. Notes further evaluation needed on entries with genuine structural gaps.

---

**20260413-120456-s-prc-17y** (2026-04-13, process) -- Pre-flight catches real implementation bug

> "Pre-flight caught a real implementation bug during the closing action for d-tac-q5p -- dry-run + early validation failure didn't save stdin, contrary to the plan's 'save on every dry-run (pass or fail).'"

> "Every fix needs a test that captures the regression."

Positive pre-flight validation catch that led to the regression test contract (d-prc-31v).


### tangential

- **20260410-151529-s-stg-3vr** (2026-04-10) -- Instructions unreliable for agent behavior control. Pre-flight's raison d'etre but topic is broader (all enforcement, not just pre-flight).
- **20260410-151206-s-prc-868** (2026-04-10) -- Agent drops requirements during implementation. Motivated pre-flight but topic is agent behavioral consistency, not pre-flight itself.
- **20260410-150919-d-prc-4du** (2026-04-10) -- Mandatory plan-decision before implementation. The instructional predecessor to tool-layer pre-flight; closed by a-prc-cw0 as too rigid.
- **20260410-212636-s-cpt-91k** (2026-04-10) -- Generation-validation separation as highest-leverage pattern. Theoretical foundation; pre-flight is the application but signal is about the principle.
- **20260412-114647-d-cpt-omm** (2026-04-12) -- Type system restructuring. Was the entry being captured during s-prc-as6's three-rejection experience but topic is type system design.
- **20260414-000402-s-ops-dd8** (2026-04-14) -- Oversized sdd show output. Contributes to pre-flight timeout issues (prompt grows with refs) but topic is general output size.
- **20260412-111002-a-cpt-dwg** (2026-04-12) -- Explore mode validation against implementation pre-flight. Pre-flight is a test case for explore mode, not the primary topic.


## Reasoning chains

### Chain 1: Instructions fail -> structural enforcement -> pre-flight implementation -> self-evaluation

s-prc-868 (agent drops requirements, 2026-04-10) -> s-stg-3vr (instructions unreliable, 2026-04-10) -> d-prc-4du (mandatory plan gate, 2026-04-10) -> d-tac-s6w (move to tool layer, 2026-04-10) -> a-cpt-1tt (process reliability research, 2026-04-10) -> d-cpt-vt1 (structural separation contract, 2026-04-12) -> d-tac-hjp (implementation plan, 2026-04-11) -> a-tac-umu (implementation, 2026-04-12) -> a-tac-di2 (self-evaluation, 2026-04-12) -> a-tac-7ca (completion, 2026-04-12)

This is the foundational chain. Over two days, behavioral consistency failures drove research, which produced a contract, which produced a plan, which produced a working validator that then evaluated itself. The self-evaluation found 5 real gaps, proving the separation principle works.

### Chain 2: Over-correction discovered -> dialogue-context fix -> validated

s-prc-2nh (over-correction, 2026-04-12) -> d-tac-1g4 (improve accuracy directive, 2026-04-12) -> d-tac-73o (implementation plan, 2026-04-12) -> a-tac-eqc (entry_quality template, 2026-04-13) -> s-prc-awd (fix validated, 2026-04-13)

The first correction cycle. Pre-flight over-corrected on framing after dialogue, so the fix taught it to treat dialogue-sourced reasoning as human-confirmed. Validated within hours by passing first-attempt on its own closing action.

### Chain 3: Knowledge-capture blind spot -> contract superseded -> template aligned

s-prc-ql7 (knowledge-capture blind spot, 2026-04-13) -> d-prc-22i (supersede durability contract, 2026-04-14) -> s-cpt-ix1 (rigid validation on structural moves, 2026-04-14) -> d-tac-h3o (template alignment plan, 2026-04-14) -> a-prc-0ka (durability in template, 2026-04-14) + a-tac-one (template alignment, 2026-04-14)

Research action (a-cpt-xbv) triggered three pre-flight rejections for lacking prior Git commits. The contract itself was the problem, not the validator. Superseding the contract then triggered a new friction signal about rigid structural validation, leading to broader template alignment work.

### Chain 4: Reassembly friction -> stdin persistence -> dry-run

s-prc-as6 (reassembly cost, 2026-04-12) -> d-tac-m94 (persist stdin plan v1, 2026-04-12) -> d-tac-q5p (persist stdin plan v2, 2026-04-13) -> a-tac-e21 (implementation, 2026-04-13) -> s-prc-17y (pre-flight catches bug in implementation, 2026-04-13) -> d-prc-31v (regression test contract, 2026-04-13)

Operational friction (re-sending attachments on retry) drove tooling improvements. Pre-flight then caught a bug in the tooling meant to reduce pre-flight friction -- a recursive validation benefit. The bug catch led to the regression test contract.

### Chain 5: Signal/decision boundary -> type-correctness fix -> regression test scope

s-prc-as6 (category errors, 2026-04-12) -> s-prc-q2m (solution-space rejection, 2026-04-14) -> a-prc-sbc (signal_capture.tmpl reframe, 2026-04-14); s-prc-okf (regression test scope, 2026-04-14) -> noted explicitly by a-prc-sbc as non-applicable to template changes

The "smuggled decisions" criterion in signal_capture.tmpl was too strict -- it rejected signals that explored solutions as "crossing into decision territory." The fix reframed around commitments vs. dialogue material. a-prc-sbc then explicitly noted that prompt template changes don't need regression tests, directly addressing the s-prc-okf scope concern.

### Chain 6: Workflow disruption -> find-something bias -> confidence-scored verdict (open)

s-prc-okf (contract scope, 2026-04-14) + s-prc-6hw (workflow disruption, 2026-04-14) + s-prc-wi7 (plan prose validation, 2026-04-14) -> s-prc-316 (find-something bias / confidence-scored format, 2026-04-14)

The most recent chain. Three friction signals on the same day converge into the diagnosis: the binary PASS/FAIL format itself is the problem. This signal is the direct precursor to the planned confidence-scored verdict work.


## Patterns observed

### Which check types dominate friction

- **closing_action** is by far the most friction-prone check type. Signals s-prc-2nh, s-prc-as6, s-prc-6hw, s-prc-wi7, and s-prc-ql7 all involve closing actions. The check validates plan items, referenced entries, durability, and structural completeness -- the largest prompt surface area and most complex context. Timeouts (s-prc-6hw) specifically affect closing actions due to many refs inflating prompt size.
- **signal_capture** is the second source: s-prc-q2m (solution-space rejection), s-prc-5ht (depth enforcement proposal). The signal/decision boundary is inherently fuzzy -- signals that explore directions look like decisions to a strict type-check.
- **supersedes** had one specific friction event (s-cpt-ix1 -- contract retirement rejected for kind mismatch).
- **decision_refs** was refined (a-tac-one narrowed from "are related signals unreferenced?" to "does the entry duplicate or logically build upon unreferenced entries") but generated less user-visible friction.

### Recurring themes in false-positive complaints

- **Re-litigating human dialogue**: The validator re-evaluates rationale that was already confirmed through human-agent dialogue (s-prc-2nh, s-cpt-ix1). It sees only the entry text, not the conversation that produced the judgment.
- **Structural ceremony over substance**: Demanding formal structural moves (kind-preserving supersede, companion contract) when the substance of the decision is explicitly anti-ceremonial (s-cpt-ix1, s-prc-as6 "alters contracts without superseding").
- **Misapplied contracts**: Enforcing contracts in contexts where they don't semantically apply -- regression tests for prompt refinements (s-prc-okf), prior Git commits for knowledge-capture (s-prc-ql7).
- **Type boundary confusion**: Rejecting signals for containing solutions (s-prc-q2m) when the distinction should be commitment vs. exploration, not presence vs. absence of solution-space thinking.
- **Details-in-wrong-place**: Rejecting for missing details that exist in code, attachments, or plan files rather than in the entry text (s-prc-wi7).

### Recurring themes in design discussion

- **Generation-validation separation** as foundational principle (d-cpt-vt1, a-cpt-1tt): the acting agent cannot certify its own compliance, so independent validation is required. Pre-flight is the first and primary instance.
- **Dialogue-before-capture** as the authority for judgment calls: pre-flight should verify structural completeness, not re-evaluate rationale. The human dialogue is the ground truth for "why this entry, why this framing."
- **Hard gate vs. advisory**: The initial design chose hard gate (block on fail). Multiple friction signals argue for advisory mode or at least confidence thresholds. The confidence-scored verdict (s-prc-316) is the current proposed resolution.
- **Prompt size and model constraints**: Closing actions with many refs inflate the prompt, causing timeouts on Haiku's 120-second limit (s-prc-6hw). Summary-based rendering (d-tac-017, d-tac-h93) partially addresses this.
- **Positive side-effects acknowledged**: Graph densification (s-prc-8na) and real bug catches (s-prc-17y) are cited as pre-flight successes, creating tension with the friction signals. The system catches real problems but also creates false ones.

### Decisions that shape pre-flight scope

- **d-cpt-vt1** (structural separation contract): Mandates that all critical workflows include independent validation. This is the "why pre-flight exists" decision.
- **d-tac-s6w** (move to tool layer): Moves validation from prompt/instructional layer to sdd tool. This is the "how pre-flight works" decision.
- **d-prc-31v** (regression test contract): Every bug fix needs a test. Pre-flight enforces this, but s-prc-okf reveals it lacks scope for non-code changes.
- **d-prc-zch -> d-prc-22i** (action durability): Originally required prior Git commits for actions; superseded to accept three durability paths including --attach. Shows contracts evolving when pre-flight reveals their blind spots.
- **d-tac-h3o** (template alignment): Aligns templates with the principle that dialogue drives graph evolution. Broadest scope fix -- covers supersedes, decision_refs, entry_quality, and durability.


## Named false-positive cases

### "CQRS for logging plan" (named in s-prc-316)

> "producing false rejections (e.g. demanding CQRS decomposition for a logging plan)"

**Entry**: 20260414-233739-s-prc-316
**Context**: Pre-flight demanded that a logging plan follow CQRS architecture, which was irrelevant to the entry's purpose. The binary PASS/FAIL format forced the LLM to find something wrong, and it stretched to an inapplicable architectural constraint.

### "Neutrality justification for dialogued design choice" (named in s-prc-316)

> "demanding neutrality justification for a dialogued design choice"

**Entry**: 20260414-233739-s-prc-316
**Context**: Pre-flight rejected an entry for not justifying a design choice's neutrality, when the choice had already been confirmed through human-agent dialogue. The validator re-litigated a judgment call the human had already made.

### "Kinds introduced without grounding" (named in s-prc-as6)

> "'kinds introduced without grounding' -- a directive introducing new concepts inherently contains ungrounded material"

**Entry**: 20260412-115120-s-prc-as6
**Context**: During capture of the two-type system redesign (d-cpt-omm), pre-flight rejected because new type kinds were introduced without existing graph grounding. A directive that creates new concepts by definition introduces ungrounded material -- the validator applied a pattern-building check to a pattern-creating entry.

### "Alters contracts without superseding" (named in s-prc-as6)

> "'alters contracts without superseding' -- the entry explicitly deferred superseding to post-validation, which the prompt couldn't assess"

**Entry**: 20260412-115120-s-prc-as6
**Context**: The entry deferred contract supersession to a later step, but pre-flight couldn't evaluate multi-step intent. It saw a contract-altering entry without a supersedes field and rejected.

### "Contract retirement requires companion contract" (implied in s-cpt-ix1)

> "the validator demanded either a companion contract or a kind-preserving supersede -- structurally ceremonious when the substance of the decision was 'this rule shouldn't be a graph contract at all'"

**Entry**: 20260414-002651-s-cpt-ix1
**Context**: Retiring contract d-prc-zch with directive d-prc-22i was rejected because the superseding entry wasn't also a contract. The structural check (kind-preserving supersede) contradicted the intention to eliminate the contract entirely.

### Three rejections on knowledge-capture action (named in s-prc-ql7)

> "Pre-flight rejected three attempts to capture the action [...] with the third rejection being explicit and correct in its reasoning: 'active contracts require formal override, not dialogue acknowledgment.'"

**Entry**: 20260413-162853-s-prc-ql7
**Context**: Research action a-cpt-xbv (distribution tooling) was correctly rejected by the validator because the active contract (d-prc-zch) required prior Git commits. The rejections were technically correct but the contract itself was wrong for this class of action. Eventually captured with --skip-preflight.

### "Plan items present in attachment" (named in s-prc-wi7)

> "4 rejections for dedup algorithm, output format spec, and per-direction depth -- all present in the plan attachment and implementation"

**Entry**: 20260414-202842-s-prc-wi7
**Context**: Closing action for d-tac-017 (depth-limited show) was rejected 4 times for details the validator expected in the action text but that existed in the plan's code attachment and the implemented code itself.
