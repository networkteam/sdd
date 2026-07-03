# Slice 5 design record — engage + explore procedures

Decisions settled in dialogue (Christopher, Claude) before authoring, with the
reasoning that is not in the entry bodies.

## Engage ends at the junction (dispatch boundary)

Engage completes when the user picks a move; the picked move runs as its own
procedure (mostly capture entered with `anchor`/`supersedes`/`closes` params).
Alternative rejected: containing the moves as downstream steps. Rationale:
capture-with-params already models the acting-on catalog, engage stays small,
and procedure completion is exactly a junction — the open-threads block hands
the dialogue its continuation options at the moment a thread ends.

## Primary + companions, not a flat list

Engage takes one `anchor` (the subject the brief and move catalog key on) plus
optional `targets` engaged alongside; both are served by the entryChains
injection. A flat multi-anchor list was rejected: kind-shaped briefs and the
per-kind move catalog need to know which entry they key on.

## Inspection by construction (s-cpt-be9)

The brief step injects the anchor+targets chains — show-shaped, full bodies —
into the served unit. The agent cannot skip reading what is already in its
context; no fabricable "I inspected" claim exists for the anchor surface.
Widening stays evidence-carried (widenReport): search angles depend on the
dialogue, so the engine cannot derive them. The capture assemble step's
widen-depth hole remains open — candidate fix (inject entryChains for
candidate refs, or an inspected-IDs field) deferred until dogfooding shows the
shape.

## Who compresses, and who needs the goal

Explore's value depends on where its context burns. Injection serves chains
into whoever runs the loop — run inline, explore would dump the surface into
the outer agent's context, defeating its purpose. So the compressor is a
disposable context (sub-agent) the outer spawns; an engine-side LLM was
rejected (token cost on sdd's side, fixed model choice, breaks the
deterministic hot path; the harness already owns cheap disposable contexts).
The `goal` param exists because the compressor has no dialogue context: it is
the one slice of dialogue that crosses the boundary, and it returns verbatim
in the briefing so the caller verifies the axis. The outer never starts the
explore instance itself — the sub-agent calls start_procedure, so the
injection renders in the context that should pay for it.

## Why explore is a procedure at all

Not for gating (no writes, no choosers). Three reasons: (1) the sub-agent is
the least disciplined context in the system — no skill, no framing; the
procedure serves the mission (instructions, chains, report schema) without
lossy relay through the outer, honoring the single-source contract
(d-cpt-chi) across every caller (engage, catch-up, groom). (2) Trajectory
evidence: widenReport + briefing land in the append-only session log —
auditable after the worker's context is gone. (3) One home: projects revise
compression rules by superseding one entry; the report schema mechanically
requires the evidence.

## Iterative full-read discipline + inspectedIds

The worker reads the whole surface: every chain one-liner or search hit that
could bear on the goal is shown in full before judging; full bodies spawn the
next widen angle; stop when an iteration adds nothing. `inspectedIds` is the
evidence field (be9's second mechanism), graph-resolved by the
inspectedIdsResolve predicate so fabricated IDs stall the gate. Compression is
the exit step: full IDs kept, enough context per entry that the caller acts
without re-reading; drop for goal-irrelevance, never brevity.

## Junction-only open-threads (d-tac-nqo)

The shell attaches the block to session entry, terminal serves (completion and
procedure-exit abandonment), the abandon tool, and resume — never to running
serves; a test pins the property. Base instruction on first appearance,
one-line reminder after (served-instruction memory pattern, reset per agent
consumer). Ordering: current dialogue's other threads, then other open
dialogues. Resume lists other dialogues only — its own threads are already the
open_instances payload.

## Parent links exposed on the shell

The engine already validated, logged, and replayed Instance.Parent; only the
MCP surface lacked it. start_procedure now takes `parent`; validation stays in
the engine (single path). "Parent waits on child-completed" gate mechanics
stay deferred until a procedure nests mid-flight — v1 engage dispatches at its
end instead.

## Incidental

Unit parsing is now fence-aware: `## ` headings inside code fences are unit
content (explore's briefing template quotes them). Capture playback gained
re-serve guidance (s-tac-3ug): full first, delta after adjust with an offer of
the full entry, always the full body before a confirm that follows body edits.
