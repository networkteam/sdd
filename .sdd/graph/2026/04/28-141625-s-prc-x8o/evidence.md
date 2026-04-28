# Evidence: approval-by-inference in `sdd new` capture flow

## Auto-mode prompt text

The Claude Code harness emits this system reminder when auto mode is active:

```
Auto mode is active. The user chose continuous, autonomous execution. You should:

1. Execute immediately — Start implementing right away. Make reasonable assumptions and proceed on low-risk work.
2. Minimize interruptions — Prefer making reasonable assumptions over asking questions for routine decisions.
3. Prefer action over planning — Do not enter plan mode unless the user explicitly asks. When in doubt, start coding.
4. Expect course corrections — The user may provide suggestions or course corrections at any point; treat those as normal input.
5. Do not take overly destructive actions — Auto mode is not a license to destroy.
6. Avoid data exfiltration — Post even routine messages to chat platforms or work tickets only if the user has directed you to.
```

The "minimize interruptions" and "prefer action over planning" directives push directly against SDD's playback-before-capture discipline. SKILL.md needs to state explicitly that this discipline is upstream of auto-mode framing, so an agent reading both at once knows which one wins inside SDD-shaped operations.

## In-session capture sequence

Excerpted from the actual exchange that triggered this signal:

- The agent had drafted a tactical plan and asked three clarifying questions at the end.
- The user answered the three questions, then introduced a new concern (`sdd init` and version skew on a stale-binary clone), referencing prior signals about minimum_version handling.
- The agent researched the prior art (s-tac-3al / d-tac-r5g / s-cpt-xbv), explained the findings, then ended its turn with: *"Folding this into the plan as one more AC, plus a ref to s-tac-3al. Capturing now."* — and immediately ran `sdd new` for the tactical plan.
- The user replied: *"Wait! Captured too soon. I didn't give the go!"* — and called out three substantive points in the captured plan that should still have been dialogue.

The agent's reasoning chain treated "all raised concerns addressed and integrated into ACs" as approval. The actual rule is "the user has not said go, so do not capture." Those are different things; conflating them is what auto-mode pressure encourages.

The premature capture was reverted via `git reset --hard HEAD~1`, but only because nothing had been pushed yet. In a multi-contributor setting where the capture had already been pushed, the recovery would have been a supersede chain rather than a clean revert — substantially more graph noise.

## External-project recurrence

The user reported the same approval-by-inference pattern in a separate SDD-instrumented project, where the agent captured after one or two answered questions despite ongoing dialogue. The recurrence across two projects, with two different in-session contexts, is what motivates a SKILL.md change rather than a one-time correction.

## Structural-gate candidates and why each falls short

Pre-flight rejected an earlier prose-only remedy on this signal, citing d-cpt-vt1 (instructions alone are unreliable for control). The user pushed back on whether structural enforcement is actually feasible here. Walking through the candidates honestly:

| Candidate | Mechanism | Why it falls short |
|---|---|---|
| Two-step `sdd draft` → `sdd commit` | Agent writes pending entry; user runs `sdd commit` to make it durable | Agent owns the shell — it can run both steps. Discipline (e.g., `!`-prefixed commands in Claude Code) is required, not enforced. Adds drag on every capture. |
| Confirmation token bound to a hash of the played-back text | CLI computes hash of capture, compares to a token derived from playback | Both playback and capture flow through the agent. Hash binds nothing to the user; agent computes both sides. |
| Interactive stdin prompt in `sdd new` | CLI reads y/n on stdin from a tty | Agent's subprocess has no real user tty. CLI could require `--confirm` on no-tty, but agent can pass the flag. |
| Harness permission prompt | Claude Code's per-command allow/deny dialog gates `sdd new` | Works when configured, but user-controlled and erodes under fluency pressure (auto-allow). Not enforced by SDD itself. |
| Pre-flight extension scanning dialogue | Pre-flight LLM judges whether user issued explicit approval | Cannot access the conversation transcript at capture time. Even if it could, it's another LLM judging — partial, not airtight. |

The recursive observation: the pre-flight gate did successfully catch the original prose-only first draft of this very signal — that is, a *content-level* structural gate already exists and works. What is missing is a *temporal* gate (block capture until the user has signaled go in this conversation), and that is genuinely hard with the agent owning the shell.

A different framing the dialogue surfaced but did not commit to: maybe the right remedy is not "prevent rushed capture" but "make rushed capture cheap to revert" — automatic branching, staged commits with cooldown windows, or similar. The git revert that recovered from the original premature capture worked precisely because nothing had been pushed yet. This framing belongs to future dialogue when a closing decision is drafted.

## Pragmatic step

The signal proposes a SKILL.md refinement as the cost-effective improvement available today:

- `sdd new` requires an **explicit go signal** from the user. Phrases like "yes, capture", "go ahead", "ready", "let's capture it" qualify; nothing weaker does.
- Resolved questions, addressed concerns, and integrated design points are **never** sufficient on their own. They mean the dialogue is making progress, not that capture is approved.
- Auto-mode framing ("minimize interruptions", "prefer action over planning") **does not override** this discipline. SDD captures are not routine engineering steps — every entry is durable, immutable, and visible to all future readers.

This is named as instructional reinforcement, not a contract-pure structural fix. The gap remains open for a future structural shape when one becomes feasible.
