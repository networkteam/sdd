# Intake model — design

## Problem / context
- Adopting SDD shouldn't mean a hard cut away from existing issue tracking. Treat issue trackers, email, and chat as *transport*, not systems to rip out and replace (adoption gap s-stg-qg0).
- We do not want to auto-generate and auto-connect knowledge — capture stays a deliberate, dialogue-filtered act (dialogue-first d-stg-beb; the bulk-ingestion contrast s-stg-ob9).
- The mining thread already named the pieces: external-channel mining (s-cpt-5ox), candidates-not-commitments (s-cpt-bn1), the "mine" move (s-prc-p6q). This model unifies them.

## The model
- A connected, autonomous agent reacts to channel events and auto-captures each incoming item as an `intake` signal (new kind).
- Why agentic, not a raw dump: the intake entry arrives pre-connected to neighbours, topic-tagged, analyzed, and summarized — so it is searchable and usable in dialogue, not dead raw material. This is the core value over mechanical storage.
- Provenance frontmatter: a generic channel + source (email address, sender name, URL, document title), kept loose so new channels need no schema change.
- Primary payload = the agent's *reasoning*: why this shape might fit, why these connections, why these topics. That reasoning is the alignment-check the human reacts to when grooming — it is what converts a guess into a durable entry.
- Candidate structure reuses existing fields only: `related` refs (guessed connections) and inline `topics:` (guessed topics). The proposed kind/layer stays in prose — no machine-readable candidate-shape fields — so the kind keeps refusing premature classification. (Revisit if real grooming shows prose insufficient.)

## Promotion: intake → durable entries
- By dialogue, not edit (immutability d-cpt-e1i). Distilled entries `ref` the intake for provenance; the intake is closed deliberately.
- Close is **permissive** (any distilled entry may close it — a gap closing an intake is the same shape as a fact dissolving a question) but **deliberate** (never automatic on the first entry).
- The intake stays OPEN until deliberately closed → a visible open end in the triage worklist → a partial session never silently drops planned outputs. The moment of closing is the completeness checkpoint: re-read the intake's reasoning and confirm the distilled set covers what it proposed.

## Triage move ladder (the intake grooming playbook)
1. **Single entry** (gap, fact, or decision) closes the intake directly — the simplest case.
2. **Several entries** → a done signal is the deliberate closer + triage/completeness record.
3. **Complex** → a direction-setting directive/activity closes the intake and carries its own downstream work to a later done signal (its children, not the intake's). Can span sessions, days, collaborators.
4. **Dismiss** (noise/duplicate) → a closing done signal, terminal, no distilled output ("duplicate of the open reorder question").
- Invariant: the intake stays open until a deliberate close; the closer doubles as the completeness checkpoint. One done signal may close several intakes.

## Surfacing
- Intake entries live IN the graph (honors no-parallel-artifacts d-stg-3k0; a separate store would cause sync/coherence headaches and isn't needed — they are interconnected from the start).
- Held out of normal catch-up; surfaced as a triage count plus a dedicated grooming flow.
- Universal interaction across surfaces — dashboard, chat agent over MCP, or a local agent (Claude Code / Codex): select one or more intakes → start a session from them (the Cursor-files-as-context move). Tooling-independent.

## Alternatives considered & rejected
- **Drafts in an outside/parallel store** — rejected: drafts are interconnected from the start; a separate store fights no-parallel-artifacts (d-stg-3k0) and adds sync headaches.
- **Structured candidate-shape fields** (proposed kind/layer as metadata) — rejected: hardening a guess into machine-readable fields invites the system to trust it, defeating the kind's purpose. Prose keeps the guess legible *as* a guess.
- **Auto-close on the first distilled entry** (collective permissive auto-close) — rejected: silently drops planned-but-unmade outputs (the gap gets made, the decision is forgotten, the intake reads done). Deliberate close + open-until-closed fixes it.
- **Restricting the closer to decision/done only** — rejected: bought nothing for drop-safety (deliberateness does that) and blocked the simplest move (a gap closing an intake).

## Close-model evidence
- Multiple closers are supported by the model: `ClosedBy` is a list (`internal/model/graph.go:15`, appended at `:40`); status returns the first (`internal/model/status.go:39`, `ids[0]`); downstream shows all (`internal/model/show_tree.go:161`). Collective fan-out is mechanically possible, but we choose deliberate close for completeness visibility. (Related: s-tac-ohl flags that `ids[0]` can pick a superseded closer — relevant once intake closers get superseded.)

## Open items for the follow-up type-system plan
- Name the kind: `intake` / `inbound` / `inbox`.
- Frontmatter shape for channel + source.
- Close-path rules + pre-flight rubric for the new kind.
- Presentation surfaces: status, list, show, catch-up exclusion + count, skill rendering.
- The autonomous channel-reactive intake agent + its reusable prompt (connects to s-cpt-z7l, s-cpt-r57).
- Optional hardening: a light "expected-outputs" hint on the intake that `sdd lint` checks coverage against (structural completeness beyond the procedural check).
- Whether trusted contracted agents (analytics/review) bypass intake and capture directly, vs all external input routing through intake.
- Relationship to the born-closed / settled-at-capture decision gap (recurrence of s-prc-k6r) — dismiss-as-done-signal sidesteps it today.
