# Runtime spike evaluation — Mastra vs agent-sdk-go

## Goal
Decide the agent runtime for a hosted SDD webapp aimed at non-technical users.
Optimize for: shortest path to a genuinely usable web UI, a single integrated
stack (prefer staying in Go), faithful SDD-loop behavior, and no duplication of
framework instructions. Build two throw-away spikes, score them, decide from evidence.

## The two candidates
- **Mastra** (TypeScript) — current committed runtime. Mature, code-first,
  first-class MCP client, real React UI story via AG-UI (assistant-ui / CopilotKit).
  Cost: a second permanent stack (Node) plus an MCP/API bridge between agent and graph.
- **agent-sdk-go** (Go, Ingenimax, MIT) — Go-native, multi-provider (incl. Anthropic
  + Ollama), MCP client, tool calling, structured output into Go structs,
  plan-approval/HITL gates. Lets the agent call the existing finders/handlers
  in-process — no bridge, one stack. Risks: young/small project; no built-in UI story.

## Context
The May runtime research concluded no viable Go-native agent runtime existed and
parked Go at the graph layer, picking Mastra for TS/UI coherence. agent-sdk-go is
the data point that reopens that conclusion.

## Apples-to-apples bar (both spikes build the same flow)
A web UI where a non-technical user can, against a real sdd repo:
1. get a plain-language catch-up of the graph, and
2. capture one new entry through guided dialogue with an explicit confirmation step,
end to end.

## Spike shapes
- **agent-sdk-go:** integrate sdd in-process (finders + handlers directly), wire the
  agent loop, prove a UI path (HTTP/SSE -> React chat, or AG-UI-shaped events).
  The UI path is the make-or-break unknown.
- **Mastra:** rough throw-away MCP/API bridge to the sdd graph (unauthenticated,
  non-production, will not merge), Mastra agent loop, native UI stack.

## Scored dimensions (evidence for each, both runtimes)
1. **UI** — quality of result and effort to get there. Headline for the goal, not sole decider.
2. **Stack & ops** — one Go binary vs Go + Node; build, deploy, host, maintain.
   Staying in Go is a large net-positive, weighed directly against UI effort.
3. **Graph integration** — in-process directness vs bridge serialization and drift.
4. **SDD-loop fidelity** — catch-up, grounded capture, dialogue-before-capture +
   confirmation, pre-flight gate, full-ID refs.
5. **Single-source instructions** — agent's process renders from the one neutral
   template source, not a hand-ported copy.
6. **Maturity & ownership** — production-readiness, provider coverage, how much we maintain.

## Decision
Weigh UI effort against the single-Go-stack benefit explicitly — neither alone decides.
Produce a written per-dimension comparison and a runtime recommendation that confirms
or supersedes Mastra and unblocks the held runtime/architecture direction.
