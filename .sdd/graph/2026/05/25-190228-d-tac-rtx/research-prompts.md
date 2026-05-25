# Research prompts — hosted SDD sandbox framework comparison

Three Perplexity prompts, run separately to cut through clutter in different directions.

## Prompt 1 — Agent runtime backends

```
I'm evaluating open-source self-hostable frameworks for building a custom agentic AI application, as of late 2025 / early 2026.

Required:
- Open-source license (MIT, Apache, BSD — not closed-source, not SaaS-only)
- Self-hostable, Docker-deployable
- Multi-provider LLM support (OpenAI, Anthropic, local via Ollama/vLLM)
- Agentic loop with tool use (the agent decides which tools to call)
- Long-running session state across multi-turn dialogue
- Suitable as a backend for a custom web UI (not locked into a built-in chat surface)
- Tool extensibility — we will add custom tools
- MCP (Model Context Protocol) support, or extensible enough to add it
- Actively maintained (commits in the last 6 months)

Nice to have:
- Native streaming output (SSE/WebSocket)
- Traces / observability built-in
- Programmatic agent definition rather than visual flow builder (visual builder is not required)
- Implementation language: Python, TypeScript/Node, or Go acceptable; Go preferred for deployment simplicity

Compare specifically: Langflow, LangGraph, Letta (formerly MemGPT), Pydantic AI, Mastra, FastAgent, the Claude Agent SDK, OpenAI Agents SDK, Microsoft AutoGen, CrewAI, and any actively maintained Go-based options. For each, list:
- Implementation language
- License
- Architecture model (graph-based, code-based, visual, etc.)
- MCP support status
- Strengths and weaknesses for serving as a custom backend
- Production deployment story (Docker, scaling, single vs multi-tenant)
- Maturity and community size

Context: I'm building a hosted decision-graph collaboration tool. The agent serves multiple users via a custom web UI. It uses tools to call into a separate Go binary (likely via MCP) for graph operations. Sessions are long-running dialogue, mostly text in / text out with occasional tool invocations.
```

## Prompt 2 — Web UI candidates (chat + custom panels)

```
I'm building a web UI for an agentic AI application, as of late 2025 / early 2026. I need an open-source self-hostable frontend (or framework to build one) that supports:

Required:
- Chat with streaming responses
- Side panel(s) or layout slots where the agent can push custom content via tool calls (e.g. a viewer pane showing whatever the agent's "show X" tool targets)
- Tool/event-driven UI updates from the backend (agent invokes a tool → specific UI panel updates)
- Custom layout — not locked into "chat only" or a fixed sidebar of preset features
- Mobile-friendly (responsive) — users may engage from phones
- Open-source license, self-hostable
- Backend-agnostic — we have our own agent backend, not tied to OpenAI's API or LangChain specifically

Nice to have:
- TypeScript/React-based for easier customization
- Built-in conversation history / persistence
- Auth scaffolding (OAuth, sessions)

Compare specifically: Open WebUI, LibreChat, Chainlit, Vercel ai-chatbot template, assistant-ui, Mesop, Reflex, Lobe Chat, ChatGPT-Next-Web, and any newer entrants in the space. For each, list:
- License and implementation language
- How custom panels / side views are added (plugin system, custom routes, component composition?)
- Backend integration model — OpenAI-format only, or can we plug a custom backend cleanly?
- Tool-result rendering — can the agent return structured content that the UI renders as a specific panel?
- Mobile UX quality
- Active maintenance

Context: the UI shows a chat panel plus a graph-overview pane and an entry-detail pane. When the agent engages with a topic or entry, that entry should appear in the detail pane — driven by the agent's tool calls, not user clicks. We want a custom UI eventually; the question is whether to extend an existing project, build on top of a UI framework, or start fresh.
```

## Prompt 3 — Go agent ecosystem

```
I'm evaluating Go-based options for building an agentic AI application backend, as of late 2025 / early 2026. Specifically, what's the state of the Go ecosystem for:

- LLM client libraries with streaming + tool use support across providers (OpenAI, Anthropic, Ollama)
- Agent-loop implementations (multi-turn dialogue, tool calling, session state)
- MCP (Model Context Protocol) client and server libraries
- Tracing / observability for agent runs

For each library, list: maintenance status (last commit), completeness vs Python equivalents (LangChain, Pydantic AI), production users if known, and any rough edges to watch for.

Context: I have an existing Go CLI tool (decision graph manager) and want a Go-based agent backend so the runtime ships as one binary. Performance matters less than deployment simplicity and single-language consistency. Willing to do moderate plumbing if the libraries cover LLM client + tool calling + MCP basics cleanly. Not interested in calling Python from Go.
```
