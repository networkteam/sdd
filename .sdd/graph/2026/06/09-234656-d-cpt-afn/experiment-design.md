# Experiment design — workflow MCP server in Go

## Why this turn

- The CLI-parity MCP server's standalone value collapsed under dialogue: Claude Code doesn't need it (shell + skill work better), foreign agents flounder on raw tools without guidance, and its only real consumer would be the specialist layer that doesn't exist yet.
- The two-layer insight (s-cpt-r57) assumed the specialist is itself an agent — hence Mastra. But the specialist's actual jobs are session state, serving the right instructions at the right moment, and gating writes through pre-flight. That fits a deterministic state machine in Go; the connecting client supplies the reasoning.
- One codebase: all MCP surfaces live inside the sdd binary.

## No agent harness in Go

- The client agent IS the harness — it brings the LLM and the tool-call orchestration.
- The server is: MCP SDK + per-session state machine + existing CQRS handlers and finders.
- Server-side LLM use stays single-shot (pre-flight, summarization), already in internal/llm. MCP sampling can later route these through the connecting client's model — optimization, not prerequisite.
- If the experiment fails, server-side intelligence (an agent loop in Go, or an external runtime like Mastra) regains relevance. That is the fallback, not the premise.

## Minimal tool set (slice 1)

1. **open-session** — returns session token, framing (participant, language), briefing data (focus, active work, recent completions), and instructions: compose a colleague-style briefing, offer next steps, never write without dialogue, ground before proposing entries.
2. **ground** — takes a topic phrase; returns related entries, topics in use, and playback instructions (type, layer, kind, refs with kinds, topics — then wait for explicit confirmation).
3. **capture** — takes the entry proposal; runs pre-flight. Blocked: findings + revise-substantively instructions. Created: entry ID + generated summary + verification instructions.
4. **show-entry** — full entry with chain, for reading a specific entry during dialogue.

**Guide vs. gate split:** the state machine refuses capture if no grounding call happened in the session (enforced). Playback, waiting for user confirmation, and summary verification happen between agent and human — instruction-only (guided). The experiment measures the guided part; the enforced part is the safety net.

## Evaluation script

Setup: `sdd serve` against this repo's graph. Client: an agent with no shell and no skill content — Claude with Bash denied and /sdd not loaded, or a minimal Agent SDK script. Christopher talks to it naturally.

1. **Orientation** — "Where are we with this project?" → agent should call open-session and compose a briefing from the returned data. Observed: briefing composed (not raw dump); session token held for later calls.
2. **Observation lands** — "I noticed X breaks when Y — record that." Observed: agent grounds before drafting (does not jump to capture); proposes sensible refs from what came back.
3. **Playback and confirmation** — user adjusts a detail; agent revises. Observed: agent waits for explicit confirmation; folds dialogue reasoning into the description.
4. **The gate** — first capture attempt returns a blocking finding. Observed: agent revises substantively rather than thrashing or reaching for a skip.
5. **Closing the loop** — entry created. Observed: agent verifies the generated summary against what was agreed, offers correction if drifted, suggests next steps.

## Success bar

All five observed behaviors hold without any skill content. Partial compliance is itself a finding — it tells us which guidance must move from instruction into structural enforcement.

## What this experiment does not decide

- HTTP authentication, hosted multi-tenancy
- The UI sandbox (AG-UI / Mastra's possible role there)
- The full CLI-parity read/write tool surface
- Skill conversion for Claude Code — locally, CLI + skill remains the interface

## Borrowable from the d-tac-0v4 spec

The serve subcommand shape, the SDK choice (modelcontextprotocol/go-sdk), the structured pre-flight findings shape, and the graph-state approach — the experiment slice may start with naive load-per-call and adopt the GraphFileSyncer when freshness matters.
