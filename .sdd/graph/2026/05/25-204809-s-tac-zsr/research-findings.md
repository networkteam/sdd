# Framework research — findings

Research activity ran three Perplexity prompts (agent runtime backends, web UI candidates, Go ecosystem) plus follow-ups (AG-UI deepdive, LangChain vs LangGraph vs Pydantic AI, Google ADK Gemini coupling).

## Realistic shortlist

**Agent runtime:**

- **LangGraph (Python, Apache 2.0)** — production-mature stateful agent runtime. Native checkpointing (Postgres/Redis/SQLite), pause/resume, time-travel debugging, human-in-the-loop. The team positions classic LangChain `AgentExecutor` as legacy. Risks: blurry line between OSS LangGraph and the paid LangGraph Platform; Python deployment penalty.
- **Mastra (TypeScript, Apache 2.0)** — code-first agent/tool/workflow framework. First-class MCP support via `@mastra/mcp`. Apache 2.0. Single-language coherence with the React UI layer. Risk: youngest of the three runtimes researched.

**Web UI (React, AG-UI-compatible):**

- **assistant-ui** — React composition library; tool results as custom UI components.
- **CopilotKit** — first-class AG-UI client toolkit; production-ready stateful, streaming, tool-aware patterns. Currently uses a proxy hop in some setups.
- **TanStack AI** — newer neutral protocol/library layer, bring-your-own-everything.

LibreChat noted as the "extend an existing chat product" alternative; useful as reference rather than primary direction.

## Architecture seams

1. **Agent runtime ↔ UI**: AG-UI protocol. Real open spec (~16 typed event types, bi-directional, transport-agnostic over SSE/WebSocket/webhook), production deployments at AWS Bedrock AgentCore and via CopilotKit. Each framework still needs an AG-UI adapter; CopilotKit currently uses a proxy hop. Decouples wire-level behavior, not semantic alignment.
2. **Agent runtime ↔ Graph**: MCP. The Go sdd binary becomes an MCP server. `mark3labs/mcp-go` is the Go library; client integration exists in Mastra, LangGraph, Pydantic AI, AutoGen.
3. **MCP server implementation**: in-process within the existing Go sdd binary, reusing internal packages directly — no subprocess shelling, in-memory graph kept across tool calls.

## Candidates dropped

- **Letta**: specializes in long-term agent memory. SDD itself is the memory — graph is the source of truth, catch-up reconstructs context. Adding Letta's memory layer creates two sources of truth.
- **AutoGen**: specializes in multi-agent coordination. We have one agent talking to one user. Mastra and LangGraph can grow into multi-agent patterns later if needed.
- **CrewAI**: specializes in workflow-shaped task decomposition. Our agent is a general-purpose dialogue partner, not a workflow of specialists. SDD modes are postures the same agent enters.
- **Pydantic AI**: excellent for structured-output single-agent flows; explicitly weak for multi-step orchestration (SDD's actual shape).
- **OpenAI Agents SDK / Claude Agent SDK**: technically capable, but vendor-shadowed — re-raises the lock-in concern from the portability gap. Either could resurface as a tool-level integration later.
- **Langflow**: PoC confirmed it's UI-first, not backend-first. Agent loop constrained by available components. Useful as a baseline reference; out as runtime.
- **Mesop / Reflex / ChatGPT-Next-Web**: Python-first or chat-clone, wrong shape for our use case.

## Go ecosystem disposition

No Go-first front-runner exists for agent runtimes. The pragmatic Go path (official OpenAI/Anthropic SDKs + `mark3labs/mcp-go` + OpenTelemetry) is clean for **the MCP server side**, but Python and TypeScript dominate at the agent runtime layer. Google ADK Go (1.0 in March 2026) is the strongest Go framework but is Gemini-shaped — not strictly coupled, but the smooth path is Gemini and adapter work is required for other providers. That re-raises the lock-in concern the portability gap names.

Conclusion: Go stays at the graph-engine layer via MCP, not the agent runtime layer.

## Next foundation work

Before any agent runtime PoC, sdd needs an MCP server exposing the CLI surface. Captured as a separate tactical plan.
