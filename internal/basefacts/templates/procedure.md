---
type: signal
layer: process
kind: fact
confidence: high
topics:
    - engine/base-facts
    - type-system/kinds
refs:
    - id: 20260814-100000-s-prc-psr
      kind: related
      desc: the spec reference — the frontmatter fields, variable types, step shapes, and the live ability registry an author consults when actually writing the workflow
summary: >-
    The procedure kind commits a repeatable way of working as a runnable
    entry — every other entry is read, a procedure is also run: a canonical
    name everything binds to, a typed step workflow in frontmatter the
    engine executes with guaranteed structure, per-step guidance in the
    body — revised by supersession under the same canonical, with the
    workflow's structural validation running at engine load rather than
    capture.
---

# Defining a way of working — the procedure kind

A procedure records a committed way of working that runs: a repeatable dialogue the engine executes step by step — capturing an entry, a roastery's weekly cupping review, cutting a release. Every other entry is read; a procedure is also run. A directive, an activity, a plan record something — a direction held, work committed, an outcome to reach — and the graph holds them as records. A procedure instead defines a workflow the agent operates step by step while the engine holds the structure: a step cannot be skipped, a gate holds until what it checks is true, and a question declared for the user stops everything until the user answers. That is what the kind buys — dialogue with guaranteed structure, instead of guidance an agent might drift from. Reach for it only when you are defining something to run.

**The canonical is the identity.** A procedure is named by its `canonical` — a short, stable name (`capture`, `release`, `cupping`) everything binds to: sessions start the move by canonical, other procedures dispatch it by canonical, and the entry ID stays out of it. A canonical is written once and never reused for another procedure: revising means superseding the current head under the same canonical, and a closed chain still owns its name. Pick the word the team already uses for the routine.

**Class places how it enters.** Every procedure carries one execution role:

{{ .Classes }}

**The workflow is frontmatter; the guidance is body.** `params` declare what a start may pass in; `state` declares the working fields steps collect and read; `steps` is the walk itself — each step collects, serves, and advances. What a step wires in — the checks it gates on, the data it serves, the actions it performs — are abilities the framework provides, named from the engine's live registry; a step that asks the user covers whatever no ability does. The body carries one instruction unit per step (`## unit: <step>`) — the authoritative guidance served while that step runs, written to the person or agent in the situation, never documentation about the procedure. A pointer inside a unit says why and when to go read a fact — it never tries to replace it.

**Ask where the choice is real.** A gate advances on what must hold; an agent chooser records the agent's judgment with its evidence; a user chooser stops the run until the user answers. Gate what is mechanical, and reserve user choosers for what is genuinely the user's to decide: a confirmation that writes, a direction only they can set. A procedure that asks the user to confirm what a gate could check is ceremony; one that gates what only the user can judge is worse.

**Shipped and project procedures.** Some procedures ship with the framework and are always present; a project's own procedures live in its graph beside them, and may freely depend on the project's own environment — the tools, paths, and services it actually uses. Overriding a shipped procedure is ordinary supersession of its chain head; when a newer shipped version and a project override compete for the head, the project's wins for execution and the fork is flagged for deliberate, merge-style reconciliation — never automatic.

**Retire deliberately.** Supersede to revise — same canonical, new head. A practice that ended without replacement is retired by a directive closing the procedure head, stating why the move is no longer how work happens. Editing is never the path: the chain is the history of the practice.

**Validation waits for the engine.** Capture checks the entry's shape — canonical, class, layer. Whether the workflow itself is sound — step wiring, guards, the abilities it names — is judged when the engine loads the spec, and a broken workflow fails loudly there, not silently at capture.

{{ .Mechanics }}
