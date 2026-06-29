# Runtime verdict — agent-sdk-go (Go) over Mastra (TypeScript)

Both spikes cleared the shared bar (Go: s-tac-f6k; Mastra: s-tac-i6m). This is a stack choice, not a capability gap.

## Per-dimension comparison

| Dimension | Go (agent-sdk-go) | Mastra (TypeScript) | Edge |
|---|---|---|---|
| Graph-integration cleanliness | In-process: tools call existing finders/handlers directly, zero CLI/agent drift | Out-of-process: shell-CLI tools or an MCP bridge | **Go** |
| Stack & operational cost | One Go stack end to end | Second Node stack + out-of-process bridge | **Go** |
| UI effort & quality | assistant-ui/CopilotKit reachable via one AG-UI adapter + a vendorable community Go AG-UI SDK | Mature UI story; a clean chat is cheap | **Mastra** (but reachable from Go) |
| SDD-loop fidelity | Cleared the bar — catch-up + dialogue-gated capture against real pre-flight | Cleared the bar — same | Tie |
| Single-source instruction rendering | Renders from the one bundledskills template source | Renders via a `mastra` profile from the same source | Tie |
| Maturity / ownership | pre-1.0, rough edges (plan-approval default disables streaming, noisy warnings, redundant tool calls); heavy dep tree | More mature runtime + UI ecosystem | **Mastra** |

## Why Go wins despite Mastra's maturity edge
- The core is already Go; in-process integration removes an entire class of CLI/agent drift and a bridge — a benefit that compounds across every future feature.
- The stateful loop's own LLM calls (pre-flight validator, semantic rule discovery) already live in the Go `llm` package; keeping them next to the graph and rules in one process beats straddling a TypeScript-to-Go bridge.
- UI maturity was Mastra's one decisive edge, and it is reachable from Go at the cost of owning a single adapter.

## The three LLM roles (all served by Go)
- **Deterministic rails** — mode state, goal sequencing, mechanical gate. No LLM.
- **In-loop validator LLM** — pre-flight judgment + semantic rule discovery; a *separate* validator (validation-separation contract), in-process.
- **Worker LLM** — external Claude Code (developer case, SDD embeds none) or embedded agent-sdk-go (hosted webapp only).

## Superseded contingencies (carried forward, not dropped)
- **LangGraph fallback** — the Mastra directive held LangGraph (Python) as the fallback "if Mastra proves insufficient." Moot now: choosing Go means no TypeScript/Mastra stack and no held cross-language fallback.
- **Vendor lock-in** — the Mastra directive rejected the OpenAI/Claude Agent SDKs on lock-in grounds tied to the agent-agnostic strategy (d-stg-6za). agent-sdk-go is Go-native and multi-provider, so it satisfies that concern rather than reopening it.

## Consequences
- Supersedes the Mastra directive (via the prior verdict d-cpt-n52).
- Downgrades the CLI-parity MCP server (d-tac-0v4) from prerequisite to optional — only for non-Go external agents.
- Accepted costs of agent-sdk-go: heavy transitive dependencies, pre-1.0 rough edges.
