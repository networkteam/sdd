# Pre-flight Session Corpus

## Method

Processed 300 `sdd new` invocations from `/tmp/preflight-events.jsonl`. Of 52 `is_error: true` events, 37 are actual pre-flight validation failures (`pre-flight validation failed:`). The remaining 15 are non-preflight errors: binary not found (3), user-rejected tool use (4), flag/parse errors (3), graph parse errors (1), git errors (1), validation errors (1), preflight timeout (1), git-ignored file (1).

Each of the 37 pre-flight FAILs was classified by check type (inferred from entry type + `--closes`/`--refs`/`--supersedes` flags) and then classified as likely false-positive (FP), likely true-positive (TP), or borderline (BL) based on:
- **Gap language**: hedging ("could", "should clarify", "might") vs. definitive ("missing", "does not", "contradicts")
- **Retry pattern**: did the agent revise substantively, add `--skip-preflight`, or comply?
- **Gap substance**: does it identify a real missing element, or demand verification of something already covered in dialogue?

Of 12 sessions with pre-flight FAILs, 9 eventually resorted to `--skip-preflight` — a strong frustration signal. Only 3 sessions (1647b38f, 5cb2d479, 8395af10) consistently revised and passed without bypassing.

## Distribution

| Check type | FAILs | FP | TP | BL |
|---|---|---|---|---|
| signal_capture | 9 | 3 | 4 | 2 |
| closing_action | 8 | 3 | 3 | 2 |
| decision_refs | 6 | 2 | 3 | 1 |
| action_closes_signals | 5 | 2 | 2 | 1 |
| closing_decision | 4 | 2 | 1 | 1 |
| action_refs | 3 | 1 | 1 | 1 |
| supersedes | 2 | 0 | 2 | 0 |
| **Total** | **37** | **13** | **16** | **8** |

False-positive rate: ~35% (13/37). With borderlines included: ~57% (21/37) not clearly justified as blocking.

## Cases by check type

### signal_capture

9 FAILs across 7 sessions. Most overreach-prone check type — signals are inherently observational+directional, and the validator frequently enforces a purity standard that penalizes natural signal framing.

#### Likely false-positives

**1. Confidence level policing** — ffd95261, 2026-04-14T13:20:15.240Z
- `s-ops` signal about excessive token consumption in session entry points
- Verbatim gap:
  ```
  - Confidence level "high" does not match the evidence. The problem (excessive token consumption) is observable and high-confidence. However, the proposed solution (entry-embedded summaries with LLM generation, direct-relationship-only, stitched into chains) is analytical and unvalidated. Parallel signals in the same problem space (s-ops-dd8 proposing `--max-depth`, s-prc-6hk proposing narrative briefing with progressive disclosure) are both marked "medium" confidence for their proposed directions. This entry should be "medium" confidence to match.
  ```
- **Why FP**: The validator acknowledges the core observation is high-confidence, then argues the confidence should be downgraded because the *proposed solution direction* is unvalidated. But confidence applies to the signal's observation, not its suggested direction. The validator is also cross-referencing parallel signals' confidence levels to enforce consistency — a judgment call, not a structural gap. Agent revised to medium confidence on retry, but the original "high" was defensible for the observation.

**2. First-sentence rewrite demands** — ffd95261, 2026-04-14T14:24:13.715Z
- `s-ops` signal about CLI metadata being discarded by Runner
- Verbatim gap:
  ```
  - First sentence must describe the entry's substance (the gap and opportunity), not just one component. Currently it states "the CLI returns metadata" but doesn't convey "the Runner discards it" or "we're missing cost/token visibility that would enable optimization." A reader seeing only the first sentence wouldn't understand what this signal is signaling — they'd see it as a note about CLI capabilities. Rewrite the opening to: "The claude CLI exposes per-call cost and token metrics via --output-format json, but the Runner currently discards this data, losing visibility into expensive entries and optimization opportunities for prompt tuning."
  ```
- **Why FP**: The validator is dictating exact prose. The opening sentence was incomplete but not structurally wrong — this is editorial feedback, not a validation gap. The validator even writes the replacement sentence, acting as a copyeditor rather than a structural checker. Agent revised the first sentence and passed on retry.

**3. Opening-sentence restates contract** — 5b332d45, 2026-04-13T14:28:13.488Z
- `s-prc` signal about gap in d-prc-zch contract applicability
- Verbatim gap:
  ```
  - The opening sentence restates the d-prc-zch contract rather than describing the signal's substance. A reader seeing only "The d-prc-zch contract requires actions to be captured after the work they describe is committed to Git" would not understand that the signal identifies a gap in this contract's applicability. The first sentence must express the signal's actual observation: that knowledge-capture actions (which produce durability via --attach, not prior commits) expose a blind spot in the contract's framing.
  ```
- **Why FP**: Same pattern — editorial feedback on prose quality. The signal's substance is present in the entry, just not front-loaded in the first sentence. The agent had `--skip-preflight` on this call (was already bypassing) but still got rejected, suggesting even the bypass path was noisy. Agent used `--skip-preflight` on retry.

#### Likely true-positives

**1. Signal prescribes solution direction** — 6963615f, 2026-04-14T12:22:31.803Z
- `s-ops` signal about retroactive graph fixes needing a mechanical path
- Verbatim gap (2 gaps):
  ```
  - Entry prescribes a specific solution direction (lint --fix command with explicit implementation details: loads graph, applies fixers, rewrites files, commits). This crosses from signal (observation of need) into decision (commitment to direction). The core observation — "retroactive fixes need a mechanical path" — is signal-material, but the solution design should be a separate decision entry.
  - Proposed solution violates d-cpt-o8t (immutability contract): "Documents are never modified — superseded by new decisions." The lint --fix design that "rewrites files" directly modifies existing decision entries, breaking immutability. Retroactive fixes to missing fields must create new superseding entries, not rewrite existing ones.
  ```
- **Why TP**: Both gaps are substantive. The first correctly identifies type confusion (signal vs. decision). The second catches an active contract violation. The agent used `--skip-preflight` on retry — but the gaps were real and the entry should have been split.

**2. Signal mixes observation with prescription** — 8395af10, 2026-04-12T09:50:34.331Z
- `s-prc` signal about pre-flight not distinguishing between entry types
- Verbatim gap:
  ```
  - Entry mixes signal (observation of validator behavior) with decision-like prescriptive content ("The prompt should either..."). The observational part is valid signal: "Validator rejected 3 times during d-cpt-omm capture; 2nd rejection contained 2 valuable concerns and 2 category-error concerns; re-assembly cost was significant." The prescriptive recommendation for how the validator should change belongs in a separate decision entry, not smuggled into this observation.
  ```
- **Why TP**: Correctly separates observation from prescription. The agent revised the signal to remove prescriptive content and passed on retry. No `--skip-preflight` needed — the gap was actionable and the fix was clear.

**3. Prescriptive signal with poor opening** — d99ebb64, 2026-04-13T06:50:14.426Z
- `s-tac` signal starting with "Add to sdd new"
- Verbatim gap (2 gaps):
  ```
  - Type correctness: The opening "Add to sdd new" prescribes an action/direction—committing to add the feature—rather than presenting a pure observation. This should be either (a) a signal describing the workflow gap: "Current workflow requires commit-to-test cycles when evaluating entry framing, discouraging iteration," or (b) a decision if committing to implement --dry-run. The entry mixes prescriptive directive with observational reasoning.
  - Self-describing first sentence: "Add to sdd new" does not work as a standalone summary. It is imperative phrasing that conveys no substance—a reader seeing only this in `sdd status` output would not understand what is being proposed or why. The opening must describe the entry's substance (the workflow constraint or the proposed feature), not just assert an action.
  ```
- **Why TP**: "Add to sdd new" genuinely fails as a signal opening — it's imperative, not observational. Both gaps correctly identify a type-correctness issue. Agent revised on retry (with `--skip-preflight`, but the revision was substantive).

**4. Prescriptive language in signal** — dd52e257, 2026-04-13T22:25:13.786Z
- `s-cpt` signal about pre-flight rigidity contradicting framework principles
- Verbatim gap:
  ```
  - Entry contains prescriptive language ("they should explicitly accept", "should treat... as valid outcome") that prescribes direction rather than describing observation. The core signal — that pre-flight's rigidity contradicts the framework principle in a-tac-eqc and causes users to bypass validation — is solid and well-evidenced. However, the final paragraph moves from observation into decision territory. Signals capture facts and observations; decisions capture commitments to direction. Split this: keep the signal as the observation of the contradiction and its consequences; capture the proposed changes to pre-flight validation as a separate decision entry.
  ```
- **Why TP**: The validator correctly identifies the signal/decision boundary violation and even acknowledges the strong observational core. Agent revised on retry.

#### Borderline

**1. Missing ref demand** — 1647b38f, 2026-04-13T07:45:31.262Z
- `s-cpt` signal about SDD distribution options
- Verbatim gap:
  ```
  - Missing ref to d-cpt-dlw: The signal mentions "the fork decision (d-cpt-dlw)" in the description as affected downstream, but Refs only lists s-stg-qg0. Graph traversal requires explicit refs for all cross-linked entries — add d-cpt-dlw to Refs.
  ```
- **Why borderline**: The gap has a valid kernel — mentioning an entry by name without ref'ing it breaks graph traversal. But the threshold for "mentioned in narrative = must be ref'd" is aggressive. The agent added the ref and passed on retry hours later.

**2. Missing ref for evidence source** — c779a3cb, 2026-04-14T18:27:30.851Z
- `s-prc` signal about pre-flight verbosity for closing actions
- Verbatim gap:
  ```
  - d-tac-017 should be included in Refs to establish graph traceability — the signal's observation is explicitly sourced from "the d-tac-017 closing action," and this evidence source should be formally referenced, not just mentioned in narrative
  ```
- **Why borderline**: Same pattern — "mentioned in narrative = must be ref'd." The ref is arguably warranted here since it's the evidence source, but it could also be medium severity. Agent added the ref on retry and passed.

---

### closing_action

8 FAILs across 5 sessions. Actions closing decisions are heavily scrutinized — the validator checks every plan requirement against the action description and flags omissions. This produces both high-value catches (real gaps in implementation accounting) and overreach (demanding exhaustive prose restatement of every plan detail).

#### Likely false-positives

**1. Demanding exhaustive plan restatement** — c779a3cb, 2026-04-14T18:13:16.821Z
- `a-tac` closing d-tac-017 (sdd show tree traversal)
- Verbatim gap (4 gaps):
  ```
  - Per-direction dedup algorithm not mentioned. Decision specifies "BFS traversal per direction with per-direction dedup — each entry shown at shallowest occurrence" but action does not address these key algorithmic properties, only mentions "Truncation markers" generically.
  - Cross-reference marker for duplicate encounters not distinctly addressed. Decision requires both "Truncation marker at boundary when deeper entries exist" AND "cross-reference marker for later encounters" (when an entry recurs at shallower depth). Action only mentions "Truncation markers" without distinguishing these two marker types or explaining the dedup logic that prevents duplicate rendering.
  - Output format specification not included. Decision specifies exact format "{relations} {full-id} ({kind}): {summary}" with rules for comma-joined multi-relation edges and kind in parentheses. Action only vaguely mentions "relation labels, kind, and full IDs" without detailing format or relation-joining behavior.
  - Independent per-direction depths not clearly stated. Decision specifies "--max-depth flag with default 4, independent per direction (4 upstream, 4 downstream)" but action only says "--max-depth flag (default 4) bounds expansion" without explaining that depth limits apply independently to upstream and downstream traversals.
  ```
- **Why FP**: All four gaps demand the action description restate implementation details that are in the code (which was committed). The work was done — the validator is checking the *prose description*, not the actual implementation. "Action does not address" means "the action text doesn't restate this detail," not "the code is wrong." Agent used `--skip-preflight` on retry.

**2. Demanding explicit deviation markers for deferred work** — ffd95261, 2026-04-14T14:37:53.608Z
- `a-tac` closing d-tac-h93 and multiple signals
- Verbatim gap (4 gaps):
  ```
  - d-tac-h93 requires "Skills consume summaries for closed entries and full details for open entries only" — action defers this to d-tac-017 without explicit deviation marker or reasoning
  - s-ops-dd8 specifically proposes "`--max-depth` on `sdd show`" and "BFS traversal for dedup" — action acknowledges these are "scoped to a parallel session" but does not mark this as an intentional deviation with reasoning
  - s-prc-6hk specifically requires skill updates to "prioritize by grouping and detail level", "summarize each chain into narrative briefing", and "mark continuation points where detail was compressed" — action defers all three to d-tac-017 without explicit deviation marker or reasoning
  - The action mentions these items ("CLI output changes...and skill updates...are scoped to a parallel session implementing...d-tac-017") but treats the deferral as incidental rather than as a conscious deviation. Per the validation criteria, deferred requirements must be explicitly marked (e.g., "Deviation: skill consumption deferred to d-tac-017 — decided during implementation that [reasoning]"). "Scoped to a parallel session" is not sufficient. Either mark the deferrals as explicit deviations with reasoning, or do not close the entries whose requirements are deferred.
  ```
- **Why FP**: The action explicitly acknowledges the deferral and names the parallel session. The validator demands a specific format ("Deviation: ...") that isn't part of any contract — it's inventing a formatting requirement. The substance (deferral is acknowledged) is present; only the ritualistic marker is missing. Agent eventually passed on retry with increased timeout.

**3. Self-describing first sentence + skills updates** — ffd95261, 2026-04-14T14:30:43.620Z
- `a-tac` closing d-tac-h93 and s-ops-d1c
- Verbatim gap (4 gaps, representative 2):
  ```
  - Skills updates not mentioned: decision d-tac-h93 requires "Skills consume summaries for closed entries and full details for open entries only" — action does not confirm skills (/sdd-catchup, /sdd-explore) have been updated to use summaries
  - Self-describing first sentence fails: opening "Implemented sdd summarize command and llm package per d-tac-h93 plan" is reference-dependent; a reader seeing this truncated in sdd status would not understand what the entry is about or why it matters without looking up d-tac-h93
  ```
- **Why FP (partial)**: The skills-update gap is plausible (TP-adjacent for that sub-gap) but the self-describing first sentence complaint is editorial — "Implemented sdd summarize command" does convey what was done. The reference to d-tac-h93 is standard practice for closing actions. Agent revised and added more detail on retry.

#### Likely true-positives

**1. Contradicting own evidence** — d99ebb64, 2026-04-12T09:43:04.893Z
- `a-prc` closing s-prc-iwx (dropped attachment validation)
- Verbatim gap (2 gaps):
  ```
  - Evidence for closure contradicts a-tac-tsd: The action claims validateAttachmentLinks was "implemented and tested," but a-tac-tsd explicitly states "Does NOT yet cover broken or missing attachment references — that requirement from d-tac-kfo remains unimplemented." The attachment validation gap that triggered s-prc-iwx is still open.
  - Closure is incomplete: The signal documents that an agent requirement was silently dropped. Closing it requires evidence that the requirement was actually addressed. Instead, the action references an action (a-tac-tsd) that itself admits the requirement remains unimplemented.
  ```
- **Why TP**: The validator caught a genuine contradiction — the agent was trying to close a signal by citing evidence that explicitly says the requirement is still unimplemented. This is the best kind of pre-flight catch: hard, factual, grounded in graph content.

**2. Test coverage gaps in plan closure** — 5cb2d479, 2026-04-13T21:43:50.743Z
- `a-tac` closing d-tac-lce and d-tac-lwm (CQRS migration)
- Verbatim gap (2 gaps):
  ```
  - Tests for migrated commands and queries (lint, status, show, list, wip list, wip start, wip done) are not explicitly addressed. d-tac-lce requires "test moved or added in the correct package" for each migration; the action states "no new tests required" but does not clarify whether existing tests were moved to their correct package locations or confirm test coverage exists for each migrated command/query.
  - Existing sdd/ tests are not accounted for. d-tac-lwm step 7 requires "existing `sdd/` tests stay if they cover library primitives, move to `finders/` if they're pure-read tests," but the action only states "framework/sdd/ deleted entirely" without explaining what happened to the tests that were in the deleted package — were they moved, deleted, or preserved elsewhere?
  ```
- **Why TP**: The plan has specific test requirements, and the action says "no new tests required" without addressing what happened to existing tests. The agent revised on retry to explicitly account for test disposition and passed.

**3. Missing exit code coverage** — f4d48841, 2026-04-13T07:47:42.124Z
- `a-tac` closing d-tac-q5p (stdin persistence and --dry-run)
- Verbatim gap:
  ```
  - The action does not explicitly confirm implementation of the "non-zero on fail" exit code requirement. The decision specifies "Exit code 0 on pass, non-zero on fail" but the action only states "exit 0" (success case) in its implementation description of the --dry-run flag. The tests verify the success path (with --skip-preflight) but do not test or confirm exit code behavior when validation/pre-flight fails during a dry-run. This is a explicit requirement of the plan that lacks coverage in the action description.
  ```
- **Why TP**: A specific plan requirement (exit code on fail) genuinely lacks coverage in the action description. The agent tried to revise but hit additional gaps; eventually used `--skip-preflight`.

#### Borderline

**1. Dead code stripping not confirmed** — 5cb2d479, 2026-04-13T21:45:36.685Z
- `a-tac` closing d-tac-lce and d-tac-lwm (CQRS migration, 2nd attempt)
- Verbatim gap:
  ```
  - Action does not explicitly confirm that dead code in cmd/sdd/main.go was stripped. Both d-tac-lwm and d-tac-lce explicitly list "strip any dead code from cmd/sdd/main.go" as a required step before plan closure ("Once complete"). The action says migrations are done and framework/sdd/ is deleted, but does not address the disposition of cmd/sdd/main.go itself.
  ```
- **Why borderline**: The plan requirement is real, but "strip any dead code" is a cleanup step — asking for explicit confirmation in the action text is reasonable but borders on prose-completeness checking. The agent revised to mention it and passed on the next attempt.

**2. Stdin preservation on dry-run failure** — f4d48841, 2026-04-13T07:49:20.562Z
- `a-tac` closing d-tac-q5p (2nd attempt)
- Verbatim gap (2 gaps):
  ```
  - Stdin not preserved on `--dry-run` when pre-flight fails. Decision requires "Saves stdin to `.sdd-tmp/` on every dry-run (pass or fail)"; action describes stdin saved only after validation/pre-flight pass or are skipped: "After validation and pre-flight (or skip), if dry-run is set: save stdin". On pre-flight failure, error is returned before dry-run branch is reached. This deviation from the decision requirement is not explicitly called out or explained.
  - Cleanup handling not addressed. Decision explicitly specifies "No automatic cleanup — agent handles removal manually" as a plan item; action description does not mention cleanup (neither confirms nor deviates). Every decision requirement must be explicitly accounted for in the action.
  ```
- **Why borderline**: The first gap identifies a real implementation deviation (stdin not saved on fail path) — but the validator is reading the description text, not the code. The second gap ("cleanup handling not addressed") demands explicit mention of a negative ("we don't auto-cleanup") which is arguably prose completeness. Agent eventually tested the actual behavior and used `--skip-preflight`.

---

### decision_refs

6 FAILs across 3 sessions. Most concentrated: session 5b332d45 had 4 consecutive FAILs trying to create a single strategic decision about SDD distribution.

#### Likely false-positives

**1. Demanding grounding for strategic principles** — 5b332d45, 2026-04-13T14:00:19.149Z
- `d-stg` decision about agent-agnostic SDD distribution
- Verbatim gap (2 gaps):
  ```
  - Agent-agnostic design as a strategic principle is not grounded in referenced signals — none of s-stg-gtu, s-stg-qg0, s-cpt-4gj, or s-tac-7kh establish that SDD should prioritize agent-agnostic foundations or introduce the cheap-vs-expensive-to-change architectural principle used to justify it
  - Multi-agent support commitment (extending to Cursor, Codex, Gemini CLI) is introduced without grounding in any referenced signal — no signal establishes supporting multiple agents as a requirement or strategic direction
  ```
- **Why FP**: Strategic decisions can introduce new principles — that's what strategy *is*. The signals provide the problem context (distribution question), and the decision synthesizes a direction. Demanding that every strategic principle be pre-established in a signal prevents strategic thinking. This was the 4th consecutive FAIL for this entry. The agent eventually passed by simplifying the decision text to remove the broader principles.

**2. Missing Kind field** — 5b332d45, 2026-04-13T14:02:31.725Z
- `d-stg` decision (5th attempt for same entry)
- Verbatim gap:
  ```
  - Missing Kind field in header (decision text specifies "Starting as a directive (not kind: contract)" but Kind is not included in the formatted header)
  ```
- **Why FP**: This is a formatting/structural issue that should be caught by static validation, not LLM-based pre-flight. The description *mentions* the kind — the validator is checking header formatting, which is a CLI concern.

#### Likely true-positives

**1. Missing ref to related signal** — 5b332d45, 2026-04-13T13:54:50.025Z
- `d-stg` decision about SDD distribution (1st attempt)
- Verbatim gap:
  ```
  - Missing ref to 20260413-143855-s-cpt-4gj. Signal s-cpt-4gj directly raises the distribution question this decision prescribes an answer to (agent-neutral channels: Homebrew, curl|bash, GitHub Releases). s-cpt-4gj lists three options including "packaging as a Claude Code plugin," which the decision implicitly rejects. The decision should reference s-cpt-4gj and clarify how the prescribed direction relates to the options being evaluated.
  ```
- **Why TP**: The validator found a signal that directly raises the question being answered but wasn't referenced. The agent added the ref on the next attempt.

**2. Missing ref to tactical signal** — 5b332d45, 2026-04-13T13:55:46.806Z
- `d-stg` decision (2nd attempt, after adding s-cpt-4gj)
- Verbatim gap (representative):
  ```
  - Missing ref: s-tac-7kh (non-technical user access via chat interface) — the decision's entire argument about "extending to Cursor, Codex, Gemini CLI, or future agents should be a natural addition" is responsive to the tactical constraint in s-tac-7kh, but s-tac-7kh is not referenced
  ```
- **Why TP**: The decision's argument directly responds to a signal it doesn't reference. The agent added the ref.

**3. Missing ref in CQRS contract** — f4d48841, 2026-04-13T12:24:16.241Z
- `d-cpt` contract about CQRS decomposition enforcement
- Verbatim gap:
  ```
  - Missing reference to d-cpt-vt1. The proposed decision establishes that "Pre-flight should treat plans that omit the CQRS decomposition...as structural gaps" — applying structural validation as the enforcement mechanism. This principle is established in d-cpt-vt1 ("structural separation — tool-layer validation by independent agents") but is not cited. The grounding for pre-flight's role in enforcing architectural patterns should reference the contract that makes pre-flight a validation gate for reliability.
  ```
- **Why TP**: The decision extends a validation role established by an existing contract without citing it. The agent added the ref and passed on retry.

#### Borderline

**1. Missing ref to validator-specific signal** — d99ebb64, 2026-04-12T09:58:44.625Z
- `d-tac` decision about improving pre-flight accuracy
- Verbatim gap:
  ```
  - Missing reference to 20260412-115120-s-prc-as6 (validator doesn't distinguish between pattern-building and pattern-reshaping entries) — another validator over-correction instance observed in the same session that demonstrates the validator problem extends beyond dialogue-context gaps
  ```
- **Why borderline**: The ref is arguably relevant, but the validator is essentially saying "there's another related signal you should also reference." The decision already refs the primary signals motivating it. Agent used `--skip-preflight` on retry.

---

### action_closes_signals

5 FAILs across 2 sessions. Actions closing signals (not decisions) — the validator checks whether the signal's stated concern is actually addressed.

#### Likely false-positives

**1. Demanding same-commit proof** — 6963615f, 2026-04-14T12:28:18.470Z
- `a-ops` closing three signals about bug fixes
- Verbatim gap:
  ```
  - Signal s-prc-17y and contract d-prc-31v require regression test in the same commit as the bug fix ("must land in the same change as the fix"). Action confirms test exists (TestNewEntry_DryRun_SaveesStdinOnValidationFailure in commit 10aae02) but does not confirm it was in the same commit as the original fix. Verify the test and fix are in the same change, or if landed separately, the gap in d-prc-31v compliance must be explicitly acknowledged.
  ```
- **Why FP**: The action cites a specific test name and commit hash. The validator wants verification that test and fix are in the *same* commit — a level of Git forensics that goes beyond what the pre-flight can actually verify. The agent revised to clarify and passed on retry.

**2. Demanding regression test citation for template fix** — 6963615f, 2026-04-14T12:47:04.003Z
- `a-prc` closing s-prc-q2m (template fix for signal/decision distinction)
- Verbatim gap:
  ```
  - Action is a bug fix (correcting validation logic that incorrectly rejected valid signals) and must provide evidence of regression testing per d-prc-31v (contract d-prc-31v requires all bug fixes to include regression tests in the same commit). The action description should confirm tests were added in commit e80cc3e and describe what validation they provide for the new signal/decision distinction criterion.
  ```
- **Why FP**: The fix was to a *prompt template* (signal_capture.tmpl), not executable code. Demanding regression tests for a prompt template change is over-applying the d-prc-31v contract, which is about code bug fixes. Agent used `--skip-preflight` on retry.

#### Likely true-positives

**1. Closure contradicts cited evidence** — d99ebb64, 2026-04-12T09:43:04.893Z
- `a-prc` closing s-prc-iwx (dropped attachment validation)
- Verbatim gap (2 gaps):
  ```
  - Evidence for closure contradicts a-tac-tsd: The action claims validateAttachmentLinks was "implemented and tested," but a-tac-tsd explicitly states "Does NOT yet cover broken or missing attachment references — that requirement from d-tac-kfo remains unimplemented." The attachment validation gap that triggered s-prc-iwx is still open.
  - Closure is incomplete: The signal documents that an agent requirement was silently dropped. Closing it requires evidence that the requirement was actually addressed. Instead, the action references an action (a-tac-tsd) that itself admits the requirement remains unimplemented.
  ```
- **Why TP**: The validator caught the agent citing evidence that contradicts its own claim. This is pre-flight at its best — cross-referencing graph content to detect premature closure.

**2. Misinterpretation of reference** — d99ebb64, 2026-04-12T09:43:33.439Z
- `a-prc` closing s-prc-iwx (3rd attempt)
- Verbatim gap (representative):
  ```
  - Misinterpretation of d-cpt-vt1: The action claims d-cpt-vt1 acknowledges silent scoping as a "permanent design constraint," but d-cpt-vt1 is about reliability through independent validation, not about accepting silent scoping. It prescribes a solution (structural separation), not an acceptance of the pattern.
  ```
- **Why TP**: The agent was stretching the meaning of a reference to justify closure. The validator correctly identified the misinterpretation.

#### Borderline

**1. Scope of closure vs. broader pattern** — d99ebb64, 2026-04-12T09:42:16.808Z
- `a-prc` closing s-prc-iwx (1st attempt)
- Verbatim gap (representative):
  ```
  - Broader pattern (s-prc-80l) unresolved — the signal identifies "general bias toward doing over dialogue" with another instance in s-prc-80l; if s-prc-80l remains open, closing s-prc-iwx on the same pattern is premature without addressing the other manifestation
  ```
- **Why borderline**: Plausible — closing one manifestation of a pattern while another stays open is questionable. But signals can be closed individually even when a broader pattern exists. The validator is enforcing a "must solve all related signals" rule that isn't in any contract.

---

### closing_decision

4 FAILs across 3 sessions. Decisions that close signals — the validator checks whether the decision adequately addresses the signal's stated concerns.

#### Likely false-positives

**1. Demanding CQRS decomposition in plan text** — c71a18bb, 2026-04-14T21:23:05.257Z
- `d-tac` plan closing two ops signals about metadata surfacing
- Verbatim gap:
  ```
  - Plan omits explicit CQRS decomposition (action/command/query/handler/model types) required by d-cpt-ah1 contract. The summary must decompose across layers: CLI actions (verbosity flag parsing), commands (Preflight and Summarize), queries (Runner.Run), handlers (JSON parsing and metadata mapping), and model types (result struct, LLMMetadata). d-cpt-ah1 explicitly directs pre-flight to treat omitted CQRS decomposition as a structural gap.
  ```
- **Why FP**: The validator demands a CQRS-style decomposition table in every tactical plan. This was a 2-signal metadata surfacing task — forcing CQRS decomposition into the plan text is ritualistic architecture theater. The d-cpt-ah1 contract may say this, but enforcing it on every small plan creates friction without adding value. Agent used `--skip-preflight` on retry.

**2. Scope expansion demand** — c71a18bb, 2026-04-14T21:11:03.231Z
- `d-tac` plan closing two ops signals about metadata surfacing
- Verbatim gap:
  ```
  - Decision introduces "agent-neutral abstraction" and llm/claude subpackage structure without grounding in signals. Signals request metadata surfacing from Claude specifically; they do not call for provider-neutral design or multi-provider support. Decision must either justify why agent-neutrality is necessary for closing these signals or remove this scope expansion.
  ```
- **Why FP (partial)**: The "agent-neutral" design was a deliberate architectural choice, not scope creep. However, the gap has a valid kernel — the decision could explain *why* agent-neutrality serves these signals. Agent revised to justify the design choice, but then hit the CQRS decomposition demand.

#### Likely true-positive

**1. Signal target not met without explanation** — dc3b4a54, 2026-04-14T22:16:48.834Z
- `d-prc` decision closing s-prc-0z4 (catch-up performance target)
- Verbatim gap (4 gaps, representative 2):
  ```
  - Signal target not met: Signal requires ≤30s for default catch-up; decision delivers ~1m. Decision does not explain why this shortfall is acceptable or revisit the target requirement.
  - Tiered catch-up reasoning inconsistent: Decision defers tiered catch-up as not addressing "the output-bound bottleneck," but benchmarks show fewer entries directly reduces synthesis time (5 entries: 27s, 12: 47s, 24+: ~1m50s on Sonnet). Tiered catch-up would reduce entry count and thus synthesis time—the reasoning for deferral is backwards or missing clarification.
  ```
- **Why TP**: The decision claims to close a signal with a specific target (≤30s) but delivers ~1m without explaining the shortfall. The validator also catches an internal contradiction in the reasoning. Agent revised substantially and re-submitted.

#### Borderline

**1. Dialogue name not established in ref** — 1647b38f, 2026-04-13T07:39:02.769Z
- `d-stg` decision closing three naming signals
- Verbatim gap:
  ```
  - The decision states that SDD "directly names the core loop established by d-cpt-omm" but the provided excerpt of d-cpt-omm does not explicitly establish the "Signal-Dialogue-Decision" concept. The excerpt establishes Signal and Decision types and describes a gap-triggered loop, but does not mention "Dialogue" as a named component. Either d-cpt-omm's full content (including the referenced design.md) must be verified to contain this three-part naming, or the decision should clarify whether d-cpt-omm establishes the SDD name or merely confirms an already-understood SDD structure.
  ```
- **Why borderline**: The validator correctly notes the excerpt doesn't show "Dialogue" as a component, but it's working from a truncated excerpt. The full d-cpt-omm may well establish this. The validator's "either verify or clarify" framing is reasonable but may be asking the agent to verify something the validator itself can't access. Agent revised to clarify the relationship and passed.

---

### action_refs

3 FAILs, all from session 5b332d45 — the same session trying to capture a research action.

#### Likely false-positive

**1. Missing confidence field** — 5b332d45, 2026-04-13T14:12:08.301Z
- `a-cpt` action about distribution research
- Verbatim gap:
  ```
  - Missing Confidence field; confidence level is required by the structural checks (Check 3: Confidence honesty) and should be specified in the entry header, proportionate to evidence (work performed and confirmed in dialogue warrants high confidence)
  ```
- **Why FP**: This is a CLI flag omission (`--confidence` wasn't passed), not a content quality issue. Should be caught by structural validation, not LLM pre-flight. The agent added `--confidence high` on the next attempt.

#### Likely true-positive

**1. Contract violation — work not yet committed** — 5b332d45, 2026-04-13T14:13:43.628Z
- `a-cpt` action about distribution research (3rd attempt)
- Verbatim gap:
  ```
  - Contract violation of d-prc-zch: Actions must only be captured after work is committed to Git. This entry is being captured before the research artifacts are committed (entry acknowledges durability arrives "via the next Git commit"). Either d-prc-zch must be explicitly superseded (with a decision that closes it), or this entry's type must change until the artifacts are durable in Git. Dialogue confirming the framing is noted, but active contracts require formal override, not dialogue acknowledgment.
  ```
- **Why TP**: The entry literally says durability will come from a future commit. The contract says work must already be committed. The validator correctly enforces the active contract. The agent ultimately used `--skip-preflight` (this led to a new signal questioning the contract's applicability).

#### Borderline

**1. Missing git commit reference** — 5b332d45, 2026-04-13T14:06:09.053Z
- `a-cpt` action about distribution research (1st attempt)
- Verbatim gap:
  ```
  - Missing Git commit reference. Per d-prc-zch ("Actions must only be captured after the work they describes is committed to Git"), the action entry must cite the commit hash that includes the research documents, or otherwise confirm they are durable in Git before this action entry is created. Currently the entry references document paths but provides no proof of Git durability.
  ```
- **Why borderline**: The contract enforcement is real, but this was a *research* action with documents that would be committed along with the entry. The d-prc-zch contract was later superseded precisely because of this pattern — knowledge-capture actions don't fit the "code first, then action" model.

---

### supersedes

2 FAILs across 2 sessions. Both were genuine structural catches.

#### Likely true-positives

**1. Kind mismatch on supersede** — dd52e257, 2026-04-13T22:19:19.030Z
- `d-prc` plan superseding a contract
- Verbatim gap:
  ```
  - Kind mismatch: superseded entry is a `contract` (standing rule), proposed entry is a `plan` (implementation plan). Superseding a standing constraint with a plan is structurally inconsistent. Either (1) the proposed entry should be `Kind: contract` instead of `plan` to establish the refined durability rule as a new standing constraint, or (2) a companion `contract` entry should be created to document the new rule and the proposed `plan` entry should supersede only the old plan/implementation, not the standing contract itself.
  ```
- **Why TP**: Replacing a contract with a plan is a genuine type error. The agent changed the kind and re-submitted (with `--skip-preflight`, but the revision was substantive).

**2. Scope narrowing without acknowledgment** — 6963615f, 2026-04-14T12:41:29.035Z
- `d-cpt` contract superseding d-cpt-o8t (immutability contract)
- Verbatim gap:
  ```
  - The new entry silently narrows scope by dropping the definitions of the three types (signal, decision, action) and five layers (strategic, conceptual, tactical, operational, process) that were part of the superseded entry's contract. Since these are structural elements of the framework itself, not just contextual details, they must either be retained in the new entry, explicitly retired with explanation, or acknowledged as relocated (e.g., "types and layers are defined in s-stg-0gh; this entry focuses on immutability principles"). A reader consulting the current graph should not need to refer to a superseded entry to find core framework definitions.
  ```
- **Why TP**: Silently dropping core framework definitions from a contract during supersede is a real information loss issue. Agent used `--skip-preflight` but the gap was substantive.

---

## Patterns observed

### Characteristic language of false-positives

1. **"does not address"** / **"not mentioned"** / **"not explicitly stated"** — checking prose completeness rather than structural correctness. The action did the work; the description doesn't restate every detail.
2. **"should be"** / **"must describe"** / **"Rewrite the opening to:"** — editorial rewrites. The validator prescribes exact wording rather than flagging structural issues.
3. **"without explicit deviation marker or reasoning"** — inventing formatting rituals. The deviation is acknowledged in the text; the validator demands a specific marker format.
4. **"does not match the evidence"** (confidence policing) — the validator second-guesses confidence assignments using cross-referencing with parallel entries.
5. **"First sentence must..."** — a self-describing-summary check that triggers on any opening sentence the validator considers insufficiently standalone.

### Characteristic language of true-positives

1. **"contradicts"** / **"explicitly states"** — catching factual contradictions with graph content.
2. **"Missing ref to [ID]"** (when the entry *responds to* that signal) — real graph traversal gaps.
3. **"mixes signal with decision"** / **"prescribes direction"** — type-correctness violations where the entry genuinely crosses the signal/decision boundary.
4. **"contract violation"** — citing a specific active contract with a clear, falsifiable rule.
5. **Kind mismatch**, **scope narrowing** — structural type errors on supersede.

### Most overreach-prone check types

1. **signal_capture** (33% FP): The validator enforces an extremely pure signal model where even mild directional language triggers rejection. Natural signal framing ("The prompt should either...") gets flagged despite being standard observational language.

2. **closing_action** (38% FP): The validator demands exhaustive prose restatement of every plan detail in the action description. If the code was committed but the action text doesn't restate the BFS algorithm details, that's a "gap." This confuses documentation completeness with structural correctness.

3. **closing_decision** (50% FP): CQRS decomposition demands and scope-grounding requirements create friction for straightforward tactical plans.

### Anti-patterns in gap phrasing worth calling out in template calibration

1. **Prose-as-implementation checking**: "action does not address [algorithmic property]" when the code is committed — the validator should check whether the work was done, not whether the description restates it.
2. **Cross-referencing confidence levels**: comparing one entry's confidence to parallel entries' confidence to demand consistency is a judgment call, not a structural validation.
3. **Demanding negative confirmations**: "does not mention cleanup" when the cleanup policy is "no auto-cleanup" — demanding the entry state what it *doesn't* do.
4. **Over-applying contracts to edge cases**: d-prc-31v (regression tests for bug fixes) applied to prompt template changes; d-prc-zch (commit before capture) applied to knowledge-capture actions with `--attach`.

---

## Calibration anchor candidates

### signal_capture

**DO NOT flag as high (anti-examples):**

> "Confidence level 'high' does not match the evidence. The problem (excessive token consumption) is observable and high-confidence. However, the proposed solution [...] is analytical and unvalidated."
> *Caption: Confidence policing — the validator acknowledges the observation is high-confidence, then downgrades because the solution direction is unvalidated. Confidence applies to the signal's observation, not its proposed resolution.*

> "First sentence must describe the entry's substance [...] Rewrite the opening to: 'The claude CLI exposes per-call cost and token metrics...'"
> *Caption: Editorial rewrite — the validator dictates exact replacement prose. This is copyediting, not structural validation.*

> "The opening sentence restates the d-prc-zch contract rather than describing the signal's substance."
> *Caption: Opening-sentence critique — the signal's substance is present in the body; the validator objects to the narrative order. Medium at best.*

**Anchor as high severity (true-positive examples):**

> "Entry prescribes a specific solution direction (lint --fix command with explicit implementation details [...]) This crosses from signal (observation of need) into decision (commitment to direction)."
> *Caption: Genuine type-correctness violation — signal smuggles a full implementation design, crossing the signal/decision boundary.*

> "Entry mixes signal (observation of validator behavior) with decision-like prescriptive content ('The prompt should either...'). The observational part is valid signal [...] The prescriptive recommendation [...] belongs in a separate decision entry."
> *Caption: Clean type-correctness catch that separates observation from prescription.*

### closing_action

**DO NOT flag as high (anti-examples):**

> "Per-direction dedup algorithm not mentioned. Decision specifies 'BFS traversal per direction with per-direction dedup' but action does not address these key algorithmic properties."
> *Caption: Prose-completeness checking — the algorithm is in the committed code; the validator demands the action text restate implementation details.*

> "d-tac-h93 requires 'Skills consume summaries' — action defers this to d-tac-017 without explicit deviation marker or reasoning"
> *Caption: Ritual formatting demand — the deferral is explicitly acknowledged in text; the validator demands a specific "Deviation:" marker format that isn't in any contract.*

> "Self-describing first sentence fails: opening 'Implemented sdd summarize command and llm package per d-tac-h93 plan' is reference-dependent"
> *Caption: Editorial complaint about first-sentence self-sufficiency in a closing action, where referencing the closed plan is standard practice.*

**Anchor as high severity (true-positive examples):**

> "Evidence for closure contradicts a-tac-tsd: The action claims validateAttachmentLinks was 'implemented and tested,' but a-tac-tsd explicitly states 'Does NOT yet cover broken or missing attachment references.'"
> *Caption: Factual contradiction caught by cross-referencing graph content — the strongest pre-flight catch pattern.*

> "Tests for migrated commands and queries [...] are not explicitly addressed. d-tac-lce requires 'test moved or added in the correct package' for each migration; the action states 'no new tests required' but does not clarify whether existing tests were moved."
> *Caption: Real plan requirement gap — the action contradicts the plan's test requirement without explanation.*

### decision_refs

**DO NOT flag as high (anti-examples):**

> "Agent-agnostic design as a strategic principle is not grounded in referenced signals — none of [...] establish that SDD should prioritize agent-agnostic foundations"
> *Caption: Stifling strategic synthesis — decisions can introduce new principles; demanding every principle be pre-established in a signal prevents strategic thinking.*

> "Missing Kind field in header (decision text specifies 'Starting as a directive' but Kind is not included in the formatted header)"
> *Caption: Header formatting issue — should be caught by structural validation, not LLM pre-flight.*

**Anchor as high severity (true-positive examples):**

> "Missing ref to 20260413-143855-s-cpt-4gj. Signal s-cpt-4gj directly raises the distribution question this decision prescribes an answer to."
> *Caption: Real missing ref — the decision answers a signal's question without referencing it. Clear graph traversal gap.*

### closing_decision

**DO NOT flag as high (anti-examples):**

> "Plan omits explicit CQRS decomposition (action/command/query/handler/model types) required by d-cpt-ah1 contract."
> *Caption: Ritualistic architecture demand — forcing CQRS decomposition tables into every small tactical plan creates friction without value.*

**Anchor as high severity (true-positive examples):**

> "Signal target not met: Signal requires ≤30s for default catch-up; decision delivers ~1m. Decision does not explain why this shortfall is acceptable."
> *Caption: Concrete target miss without justification — the decision claims to close a signal with a specific requirement but doesn't meet it or explain why.*

### action_closes_signals

**DO NOT flag as high (anti-examples):**

> "Action confirms test exists (TestNewEntry_DryRun_SaveesStdinOnValidationFailure in commit 10aae02) but does not confirm it was in the same commit as the original fix."
> *Caption: Git forensics demand — the test and commit are cited; the validator wants same-commit proof, which it cannot actually verify.*

> "Action is a bug fix [...] and must provide evidence of regression testing per d-prc-31v [...] The action description should confirm tests were added in commit e80cc3e"
> *Caption: Over-applying bug-fix contract to a prompt template change — regression tests for .tmpl files are not meaningful in the same way as for executable code.*

**Anchor as high severity (true-positive examples):**

> "Evidence for closure contradicts a-tac-tsd: The action claims validateAttachmentLinks was 'implemented and tested,' but a-tac-tsd explicitly states 'Does NOT yet cover broken or missing attachment references.'"
> *Caption: Cross-reference contradiction — the gold standard for pre-flight catches.*

### supersedes

**Anchor as high severity (true-positive examples):**

> "Kind mismatch: superseded entry is a `contract` (standing rule), proposed entry is a `plan` (implementation plan). Superseding a standing constraint with a plan is structurally inconsistent."
> *Caption: Type-system violation on supersede — structural, clear, unambiguous.*

> "The new entry silently narrows scope by dropping the definitions of the three types [...] and five layers [...] that were part of the superseded entry's contract."
> *Caption: Silent information loss on supersede — core definitions disappear without acknowledgment.*
