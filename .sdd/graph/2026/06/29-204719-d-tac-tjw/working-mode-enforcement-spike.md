# Working-mode enforcement — spike design

One candidate architecture for keeping an agent reliably in an explicit working mode, and re-assuring the mode (and its setup) before continuing. Low confidence — this is the spec to try, not a commitment. Applies the guest-surface enforcement principle (s-cpt-1dz).

## Problem
- The agent's current working mode is lost across turns (context is lossy).
- At a mode juncture nothing forces the agent to re-assure the mode + complete its setup (e.g. load the mode's reference) before proceeding; it pattern-matches to a quick action and continues. Concrete instance: s-prc-70o.
- SDD is a **guest** in the harness — it owns its own tools (the `sdd` CLI / an SDD MCP server), not the agent's native Read/Edit/Bash. So it cannot gate the agent's whole tool surface.

## The three layers (each at the surface it can control)

### 1. Goal-driving over a stateful server — neutral core
- A stateful server (MCP, or the `sdd` CLI sharing state) holds the authoritative session record: active mode, checkpoint state, pending transition, history.
- Each step the agent asks the server for its current goal; the server returns the goal + a report-back format. The agent executes with its OWN tools and reports back; the server checks the report and sets the next goal.
- Leans on reliable goal-following (an explicit current goal), not standing self-discipline. No tool ownership needed → host-neutral.
- Soft point: nothing hard-forces the agent to ask for / follow the goal (bootstrap). Reliable for cooperative agents, not guaranteed.

### 2. Hard gate on SDD's own write tool — the one owned surface
- `sdd new` / pre-flight already gates capture. Extend it: a recorded entry must meet the active mode's requirements (e.g. an evaluation finding carries the move it followed + the lenses covered).
- The only place SDD can hard-enforce — and it checks the ARTIFACT, not the process.

### 3. Claude-Code Stop-hook backstop — optional, agent-specific
- A `Stop` hook fires when the agent ends its turn. It can return `{"decision":"block","reason":...,"systemMessage":...}` to prevent the stop and re-feed a follow-up.
- The hook runs a shell command → calls the `sdd` CLI (which shares the server's state) → if a goal is open/unsatisfied, blocks + re-drives. Native `mcp_tool` hooks are documented but unproven; reach state via the CLI instead.
- Constraints: hooks do not hot-reload (restart on change); a slow CLI/server can hang the stop → needs a timeout + graceful fallback.
- Agent-agnostic (d-stg-dn6): the hook is Claude-Code-only → optional hardening; other harnesses run on layers 1+2 (softer bootstrap, still functional).

## Honest limits (the spike must probe)
- Layers 1 & 3 raise the floor (no silent skip) — they do NOT guarantee good execution. The re-drive re-asks; it cannot force quality.
- Enforcement only ever checks the recorded artifact / stored state, never the process. "Loaded the reference" is forceable; "absorbed it" is not.
- The bootstrap soft point is closed at the turn boundary by the Stop hook on Claude Code, but stays soft on harnesses without hooks.

## Corroboration
- An independent from-scratch design run (no access to our prior experiments) converged on server-held state + enforcement-at-the-tool — the same shape. Run: https://claude.ai/code/artifact/462a1c48-ef13-49a5-940b-2649ab8cffdf

## What the spike should answer
- Does goal-driving actually keep a cooperative agent in-mode better than standing instructions?
- Does the Stop-hook backstop reliably catch "stopped with an open goal" and re-drive usefully?
- How much friction does the loop add — tolerable for a fluid agent like Claude Code?
- Where does the floor-vs-ceiling gap bite in practice?
