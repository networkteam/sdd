---
type: signal
layer: process
kind: fact
confidence: high
topics:
    - engine/base-facts
    - type-system/kinds
refs:
    - id: 20260812-170000-s-prc-dnk
      kind: related
      desc: the done kind's authoring fact — the per-kind depth behind this overview
index:
    title: 'Understanding the type system: how types, kinds, and layers fit together, which entry to draft when kinds compete, and where each kind''s crafting guide lives'
    topic: type-system/kinds
summary: >-
    The type system introduced: every entry is a signal (something noticed) or
    a decision (something committed to), each carrying a kind picked by the
    question it answers and a layer naming the depth of thinking — with the
    tests that settle competing kinds, the retirement split, and a pointer to
    each kind's own authoring fact.
---

# The type system — entries, kinds, and layers

The graph records everything as entries of two types: a **signal** records something noticed; a **decision** records something committed to. That split is the first test, and the strongest. One loop drives the graph — a signal meets dialogue and becomes a decision; completed work lands as a done signal that closes the commitment and feeds the next loop — and entries are immutable: retiring one means adding a closing or superseding entry, never editing.

Each entry carries a **kind**; the question it answers picks it.

{{ .SignalKinds }}

{{ .DecisionKinds }}

Where two kinds compete, these tests settle it:

- **Aspiration vs directive — force, not completion.** Both may lack a completion criterion by design. An aspiration *pulls*: direction the work aligns with, never binding any single piece of it. A directive *pushes*: work is expected to conform, and a violation is observable.
- **Plan vs activity — WHAT vs THAT.** A plan defines what must be true when the work is done — verifiable outcomes, stated as acceptance criteria. An activity dispatches work whose shape is already known; its validation is a single "was it done?".
- **A directive states its posture** as intent:
  - `pending` — demands follow-up
  - `guiding` — standing context that never completes
  - `settled` — born terminal, with the why in its body
- **Standing constraints are guiding directives.** The contract kind takes no new entries — existing contracts stay valid, and a constraint that must always hold is captured as a directive with guiding intent.
- **Actor vs role — outside vs here.** What a participant brings from outside the project is their actor identity; what they do within it is a role bound to that actor. This week's task is neither.
- **A done records a past act** and points at what it completes.
- **An annotation carries structure, not narrative** — metadata laid over the entries it references.

Retirement follows the same split: a **done** closes what completed; a **directive** closes, with its reasoning, what will not be built or no longer holds; a **fact or insight** may close a question by answering it; same-kind supersession replaces.

Every entry also names its **layer**, the depth of the thinking:

- strategic — why, direction
- conceptual — approach, shape
- tactical — structures, trade-offs
- operational — individual steps
- process — how we work

This is the map, not the depth: each kind has its own authoring fact carrying its meaning, make-up, and craft — pull it before drafting that kind.
