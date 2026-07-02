# SDD Workflow Engine — architecture direction

Design note for the conceptual decision adopting a move-instance engine as the portable carrier of the SDD experience. Distilled from the design dialogue of 2026-07-02, grounded in the enforcement spike (s-tac-du6), the reconstructed session plays (see `session-plays.md`), and the prior experiment line (s-cpt-22d, s-cpt-h5c).

## Why

- The SDD experience is bound to one harness: skill prose + Claude Code. Discipline is instruction-carried, and agents satisfice — with a summary visible in context, grounding or engaging reads as unnecessary work toward the implicit goal (s-cpt-86s). Prompt tuning shifts the odds; it cannot remove the judgment call.
- The strategic pull: any agent runtime (d-stg-dn6), non-developer access through a hosted surface (d-stg-x0l, d-stg-6za), coherent parallel participation (d-stg-qlt) — all need the experience to live somewhere portable and structural, not in per-harness prose.
- The stack is decided: Go core, in-process (d-cpt-8k5). The engine is the missing middle layer between the graph core and the surfaces.

## Core model

### 1. Moves are declarative specs — data, not code

A playbook move is a small state graph. Each step declares:

- **goal template** — the instruction served to the agent (template-rendered; LLM-adaptive phrasing is a later upgrade, not v1)
- **evidence schema** — the structured fields a report-back must carry
- **checks** — tiered (see §6), each marked blocking or advisory
- **transitions** — named exits, each typed by **chooser** (see §3)

From this one spec derive: the agent's per-step instructions, the gates, and the visible junction map. Guide and gate cannot drift because they are projections of the same source. This extends the single-source instruction contract (d-cpt-chi): core move specs ship in the binary and are the same source today's skill renders come from; project-specific moves and rules extend from the graph (rules attach binding/advisory constraints at move steps, per the rule system d-tac-eho).

Base-process correctness stays independent of any particular graph (d-cpt-dym): core moves bundled, extensions graph-resident.

### 2. The engine is a move-instance runtime — not an agent, not a session controller

API surface (approximate):

- `start(move, context) → handle` — open an instance, receive the first goal
- `report(handle, evidence) → verdict + next goal` — the deterministic tick
- `amend(handle, context)` — update the instance's working context without a state change (mid-move re-scoping is first-class; the session plays show user corrections inside open moves are the *normal* case)
- `status(handle)` / `abandon(handle)`

Properties:

- **Parallel instances.** A session is a workspace of small machines, not one machine. Mid-groom, a capture opens; the groom handle suspends and resumes. Which handle the conversation touches next is deliberately undetermined.
- **Seeding.** A handle's evidence can seed a new handle (evaluate spawns captures; implement spawns partial-done captures). First-class mechanism, observed in every reconstructed session.
- **Suspension/resume across sessions.** Instance state is structured and persistent — pickup after a break or context loss is structural, replacing WIP-marker + task-list + context-window continuity.
- **Delegation.** The engine does not care who reports to a handle. A research handle can be worked by a sub-agent; the handle is the coordination point. First hook toward parallel autonomous participants (d-stg-7lu) at no extra design cost.

The engine holds **no transcript**. Per-instance structured state plus a session working set (anchor, candidate refs, confirmations, findings). Dialogue enters as evidence — including trajectory digests — never as raw history. This keeps the gate dialogue-blind (d-cpt-vt1), sessions portable across surfaces, and forces artifacts to stand on their own (d-stg-3k0).

### 3. Chooser-typed transitions

Every transition names who may take it:

- **gate** — taken automatically on evidence-check outcome (advance and re-drive are a pass-transition and a self-loop)
- **agent** — a genuine junction: the agent picks among declared exits, with rationale
- **user** — a handoff state; only the human's reply selects the exit

The available transitions are visible in each goal payload — the agent is never walking a corridor blind, and steering is legitimate exactly where the spec declares it. Real junctions from the session plays: capture-after-grounding may exit to `already-covered` (propose a ref instead of a new entry); engage-after-brief fans out to the per-kind move catalog as a user junction; confirm is a user state with confirmed / revise / abandoned exits.

Dialogue-presence (s-cpt-p6f, d-stg-beb) is not a rule the loop might drop — it is chooser semantics: no one can take a user exit but the user.

### 4. Moves vs queries

A handle earns its existence through one of: **multi-turn state, an evidence gate, or a write effect.** Everything else is a **query**: one response carrying `{data, guidance}` — served data plus just-in-time procedure. Catch-up is a query (the engine serves the lanes pre-fetched plus composition guidance; nothing to gate). Ad-hoc reads are free. Structured research (explore) IS a move — procedure with an evidence schema — but with advisory checks only.

Instructions ride with every response, move or query; the agent needs no prior knowledge (delivery proven by s-cpt-22d).

### 5. Shells — the session root is itself a spec, chosen by posture

- **Interactive shell** (Claude Code, Desktop, hosted chat): `open → workspace → close`.
  - *Open*: orientation, move/query menu with activation hints, active WIP, ultralight delta since the participant's last briefing (engine holds per-participant state), and residue of previous sessions (suspended handles). Full catch-up is opt-in from here.
  - *Workspace*: goal-free by design. Hosts the parallel instances; the root only tracks the registry and working set. **This state must never accrete sequencing goals** — every reconstructed session shows the weave does not follow a fixed order.
  - *Close*: checklist derived from held state — open handles to resolve/carry/abandon explicitly, findings without spawned captures ("have I captured everything?" made structural), active WIP markers, session ledger. Advisory; abandonment is tolerated — the next session's open state serves the leftovers.
- **Autonomous shells** (intake per d-cpt-fbi, scheduled sweeps): corridors. No workspace, no user-chooser states; the move menu is shell-filtered (moves requiring user exits are unavailable). Dialogue-presence is realized *deferred*: auto-captured entries are open triage ends that become settled substance only through later grooming dialogue. The spike's corridor architecture is this shell — right shape, headless posture.

### 6. Enforcement follows surface ownership and error cost

- **Writes only through moves.** No externally callable graph-write tool; a write is the terminal effect of a completed capture-family move. This is the one surface a guest framework fully owns (s-cpt-1dz), and the backpressure that defeats satisficing: the implicit goal (entry captured) is unreachable without the work's products.
- **Evidence tiers:**
  1. *shape* — fields present, kinds valid (sub-ms, spike-proven)
  2. *state* — deterministic lookups against graph and session: reported IDs resolve, quoted snippets exist, **read-log membership** (the engine records what the agent actually fetched this session — grounding claims stop being self-report, no LLM needed), commits exist
  3. *LLM judgment* — the one judge, at marked transitions; this is pre-flight, in-process (d-cpt-8k5), calibrated the way pre-flight already is (rubric templates + eval fixtures + severity ladder). Advisory unless graph-anchored (the oscillation lesson, d-tac-tph). Re-drive reasons never contain values that satisfy the check (spike lesson).
- **Everything outside the write path is economics, not policing** (s-cpt-86s, s-cpt-icg): pre-fetched chains make the disciplined path the cheapest path; interview-style moves generate dialogue evidence by construction (s-cpt-qs2); rules *suggest* the heavy alignment form when stakes are high (activation conditions as the scaling rule d-prc-e2y left open).

### 7. LLM roles — judgment only, never control flow

The engine needs no LLM to run: transitions are declared, junctions are agent/user-chosen, goals are template-rendered. The single LLM role is the gate/judge. Move and rule *selection* is: user explicit (slash-command reflex), engine suggestion via activation matching (`when`/`matches`, the d-tac-eho machinery), or agent proposal — all converging on the same handle, all cheap to correct (a wrong move entry costs one conversational turn; hence no hard enforcement there).

### 8. What remains host-side

A thin bootstrap ("you are in an SDD session; call session_start; follow goals; etiquette") plus conversational judgment — the weave, the thinking-partner quality. This is the accepted ceiling: on strong agents the experience is today's fluid dialogue; on weak agents the conversation stiffens but the floor holds — grounded, confirmed, validated writes. The floor is structural; the ceiling is the agent.

## Sequencing

1. **v1: MCP-stdio into Claude Code** — the dogfooding driver and the *strictest* guest-surface test (the host owns tools the engine cannot gate; if discipline holds here, every other surface inherits a stronger position). First moves: capture, engage, explore, interview; interactive shell; read log; write path.
2. **Claude Desktop** as near-free second client (same server; behavioral precedent s-cpt-22d).
3. **Hosted webapp** (d-stg-6za) as a second adapter over the identical engine — in-process, embedded worker LLM in the executing-agent role. Web UI, auth, multi-user are ordinary hosted-product work, not discipline design.
4. **Transition discipline:** skills remain as renders of the same source until engine parity; no forked prose (d-cpt-chi).

## Deliberately open

- **The generic evidence-field form per surface (s-cpt-li1)** — the first question the v1 build answers empirically, not an implementation detail to discover mid-build.
- **Goal granularity feel** in human-driven sessions — the spike tested a cruder loop; whether goal-serving feels right when the human drives is untested.
- **Weave quality on weaker agents** — accepted as ceiling, monitored, not enforced.
- **Move-spec format details** (frontmatter shape, versioning, when moves become a graph-resident kind vs bundled specs) — tactical, for the v1 plan.
