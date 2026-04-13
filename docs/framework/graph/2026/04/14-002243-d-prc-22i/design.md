# Durability enforcement: from contract to pre-flight template

## Context

The superseded contract `d-prc-zch` required actions to be captured after the work they describe is committed to Git. The rule had two blind spots and one meta-problem.

**Blind spot 1 — knowledge-capture actions.** Research, synthesis, and design artifacts drafted during the capture session have no prior commit. They live in scratch space (`tmp/`, `/tmp/`) and become durable only when `sdd new --attach` copies them into the graph's attachment directory and the auto-commit carries entry + attachments together. Observed during `a-cpt-xbv` (seven-angle distribution research), which needed `--skip-preflight` after three pre-flight rejections.

**Blind spot 2 — split knowledge capture.** When research is attached to one entry and a later action synthesizes or concludes from it, the artifacts are durable via a referenced upstream entry, not this entry's commit.

**Meta-problem.** The rule is mechanical, not judgment-requiring. Pre-flight checks it on every closing action; humans don't reason about it. Real contracts (`d-cpt-vt1`, `d-prc-31v`) ask for judgment; this one asks for a sequence. It belongs in the validator's prompt, not in the graph as a standing principle.

## Three paths to durability

An action's referenced artifacts can be durable via any of:

1. **Prior commit** referenced by the action — work-recording case (code/system/graph changes).
2. **Attachments on this entry** via `--attach` — single-session knowledge capture; the `sdd new` auto-commit carries both entry and attachments.
3. **Attachments on a referenced upstream entry** already durable in the graph — split or multi-session knowledge capture.

Enforceability is asymmetric: only code commits are truly verifiable (git log, hash, diff). Attachments and upstream refs can only be presence-checked. External artifacts (dialogue synthesis outside the system, shared docs, Linear tickets) remain trust-based and outside the check's scope. The contract will not pretend to enforce what it cannot verify.

## Plan items

### 1. Update `framework/sdd/finders/preflight_templates/closing_action.tmpl`

Add a durability section that recognizes the three paths. Presence-based — the action must reference at least one. Do not demand prior commits when attachments supply durability.

Proposed wording (final during implementation):

> **Durability**: The artifacts the action references must be durable in the Git-tracked repository by the commit that records the action. Durability can arrive via: (a) a prior commit referenced by the action — for code or system changes; (b) attachments on this entry via `--attach` — the `sdd new` commit carries entry and attachments together; (c) attachments on a referenced upstream entry already in the graph — for split or multi-session knowledge capture. Accept any of these as evidence. If none apply, flag as a durability gap.

### 2. Update SKILL.md line 60

Replace:

> Before capturing an action: Ensure the work it describes is committed to Git first. An action records a fact of execution — the code changes it references must be durable before the action entry is. Commit implementation changes, then capture the action.

With:

> Before capturing an action: Ensure the artifacts it references are durable — either via a prior commit (code or system changes) or via `--attach` on the entry itself (research, synthesis, design docs; the `sdd new` commit carries entry and attachments together).

### 3. Supersede `d-prc-zch`

Done by this decision's `--supersedes` flag.
