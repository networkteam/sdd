Based on the available sources from 2025–2026, here's what's known about how MCP clients handle the dual `content` + `structuredContent` pattern in tool results.

## 1. Client behavior: what reaches the model?

| Client                            | Preferred channel (2025–2026)                                                                  | Notes / sources                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| --------------------------------- | ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Claude Code CLI**               | Prefers `structuredContent` when present; falls back to `content`                              | Anthropic's own surfaces differ: Claude Desktop/claude.ai prefer `content` text, while Claude Code prefers `structuredContent` [glama](https://glama.ai/mcp/servers/cchadha2/mcp-response-types-lab).                                                                                                                                                                                                                                                                     |
| **Claude Desktop / claude.ai**    | Prefer `content` text; `structuredContent` shown in UI widgets but not always in model context | Mixed-content arrays (text + resource_link) are rejected in Claude Desktop with "unsupported format" despite being spec-compliant [github](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/1638). Widget data in `structuredContent` is explicitly described as "never enters the model's context" for large-result patterns [futuresearch](https://futuresearch.ai/blog/mcp-results-widget/).                                                        |
| **ChatGPT / Codex (desktop app)** | Prefers `structuredContent` when both present; passes both to model in some configurations     | ChatGPT Apps and "emerging MCP consumers" pass both `content` and `structuredContent` to the model, causing duplication if both carry the same JSON [github](https://github.com/blockscout/mcp-server/issues/319). A Codex App regression (build 6396+) dropped `content` entirely when `structuredContent` is present, keeping only `structuredContent` [ithub.global.ssl.fastly](https://ithub.global.ssl.fastly.net/openai/codex/issues/38287).                        |
| **Cursor**                        | Uses `content` only; `structuredContent` shown in UI but not sent to model                     | When `content` is empty and data is only in `structuredContent`, Cursor delivers an empty result to the model (silent data loss) [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346). Cursor staff explicitly recommend returning payload in both fields as a workaround [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346). |
| **Gemini CLI**                    | Expects `structuredContent`; auto-backfills from `content` if missing                          | A fix in gemini-cli adds automatic "structured content backfilling" when MCP bridges (e.g., Xcode's mcpbridge) return only a stringified JSON in `content` without `structuredContent` [github](https://github.com/google-gemini/gemini-cli/pull/18376).                                                                                                                                                                                                                  |
| **Windsurf**                      | Not explicitly documented in available sources                                                 | No direct evidence found in 2025–2026 sources.                                                                                                                                                                                                                                                                                                                                                                                                                            |
| **VS Code Copilot**               | Treats structured-only results as buggy; expects `content`                                     | VS Code hit the same edge case as Cursor: "MCP tool returning structuredContent only leads to error in output view" [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346).                                                                                                                                                                                                                            |
| **LangChain MCP adapter**         | `content` → model; `structuredContent` → `artifact` (not sent to model)                        | LangChain explicitly stores `structuredContent` in `ToolMessage.artifact`, separate from what the LLM sees; an interceptor is needed to append it to `content` if desired [forum.langchain](https://forum.langchain.com/t/why-can-the-model-see-the-structured-content-returned-by-the-mcp-tool/3076).                                                                                                                                                                    |

Key pattern: **no universal rule**. Some clients (Claude Code, ChatGPT/Codex in some modes) prefer `structuredContent`; others (Cursor, VS Code, LangChain) effectively use only `content`; still others (ChatGPT Apps, some Codex configurations) pass both.

## 2. Token doubling reports

Yes—there are explicit reports of token doubling:

- **Blockscout MCP server issue #319** (Feb 2026) describes the problem precisely: when `structured_output` is enabled, both `content` and `structuredContent` contain equivalent JSON, "doubling the context consumed by the AI model." They note that "ChatGPT Apps and other emerging MCP consumers pass both `content` and `structuredContent` to the model". [github](https://github.com/blockscout/mcp-server/issues/319)
- **FutureAGI "MCP Gateway Cuts Token Costs"** (Apr 2026) argues that MCP has "token-amplification baked into its data plane" due to serialized JSON round-trips, and highlights cross-agent duplication when the same server is used in both Claude Code and Codex CLI. [futureagi](https://futureagi.com/blog/how-mcp-gateway-cuts-token-costs-claude-code-codex-cli-2026/)
- **PacketNebula MCP roadmap analysis** (Aug 2026) calls the dual-field design a "defect" and advises server authors: "stop returning both `content` and `structuredContent` from `tools/call`" because "a server author has no way to know which one a given client will actually show the model". [packetnebula](https://www.packetnebula.com/articles/mcp-roadmap-after-2026-07-28-spec/)

**Workarounds people converged on:**

1. **Human-readable summary in `content`, full data in `structuredContent`** — recommended by Blockscout and the "MCP results widget" pattern. [futuresearch](https://futuresearch.ai/blog/mcp-results-widget/)
2. **Return payload in both fields** (as JSON text in `content` + object in `structuredContent`) to ensure compatibility with clients that read only one channel. [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346)
3. **Use `structuredContent` for widget data that never enters the model's context**, keeping `content` lean. [futuresearch](https://futuresearch.ai/blog/mcp-results-widget/)

## 3. Structured-only servers: what breaks?

Servers that ship `structuredContent` **without** the backwards-compat text block do exist, but they break several clients:

- **Cursor**: structured-only results are silently dropped; model receives empty result. [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346)
- **VS Code**: structured-only results lead to errors in the output view. [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346)
- **Claude Desktop**: rejects mixed content types (text + resource_link), forcing servers to detect the client and filter. [github](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/1638)
- **MCP Inspector**: historically complained when `structuredContent` and `content` are not the same, interpreting disparity as a spec violation. [github](https://github.com/modelcontextprotocol/modelcontextprotocol/discussions/1780)

**Clients known to ignore `structuredContent` entirely:**

- **Cursor** (model sees only `content`). [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346)
- **VS Code Copilot** (error on structured-only). [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346)
- **LangChain adapter** (`structuredContent` → `artifact`, not model-visible). [forum.langchain](https://forum.langchain.com/t/why-can-the-model-see-the-structured-content-returned-by-the-mcp-tool/3076)
- **Claude Code** in terminal mode (ignores `structuredContent` for widget purposes; no widget rendering). [futuresearch](https://futuresearch.ai/blog/mcp-results-widget/)

## 4. Spec direction: deprecation or negotiation?

The spec has **not** deprecated the duplicate-text recommendation as of July 2026, but there is active discussion:

- **SEP-1624** (Oct 2025) proposes clarifying `structuredContent` vs `content` usage, acknowledging the tension between "unstructured" vs "structured" purposes and the backwards-compat SHOULD. [github](https://github.com/modelcontextprotocol/modelcontextprotocol/discussions/1563)
- **Discussion #1715** (Oct 2025) requests a `responseFormat` parameter in `tools/call` to let clients choose `"text"`, `"structured"`, or `"both"`, explicitly to avoid duplicated payloads. [github](https://github.com/modelcontextprotocol/modelcontextprotocol/discussions/1715)
- **MCP roadmap (Aug 2026)** names the `tools/call` result shape as a priority area under "Improved primitives," with a Core Primitives Working Group (not yet formed) tasked with redesigning the contract. The advice is pragmatic: "Pick one field, put everything in it, and keep the other consistent rather than duplicated". [packetnebula](https://www.packetnebula.com/articles/mcp-roadmap-after-2026-07-28-spec/)
- **Content annotations** (audience/priority) may be deprecated because "most implementers have not adopted them". [packetnebula](https://www.packetnebula.com/articles/mcp-roadmap-after-2026-07-28-spec/)

No formal negotiation mechanism exists yet; the spec still says servers **SHOULD** return serialized JSON in `content` for backwards compatibility. [modelcontextprotocol](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)

## 5. Codex specifics

**How Codex CLI renders MCP tool results:**

- Codex's transcription layer has been observed to **keep only `structuredContent` and drop `content` entirely** when both are present (regression in builds 6396+ / app versions 26.803.61601+). [ithub.global.ssl.fastly](https://ithub.global.ssl.fastly.net/openai/codex/issues/38287)
- Earlier builds preserved `content`; the regression appeared when `node_repl` started attaching `structuredContent` to responses. [ithub.global.ssl.fastly](https://ithub.global.ssl.fastly.net/openai/codex/issues/38287)
- Codex extensions (v0.151.0+, Aug 2026) can now **inspect or replace MCP tool results before the model reads them**, giving servers a potential hook to influence what reaches the model. [ai-tldr](https://ai-tldr.dev/releases/openai-codex-cli-0-151/)

**Token budget and truncation:**

- Codex documentation recommends **~10K tokens per tool response**, with **half-head / half-tail truncation** ("middle-truncation") when the limit is hit: show first 5K tokens, then `…`, then last 5K tokens. [developers.openai](https://developers.openai.com/cookbook/examples/gpt-5/codex_prompting_guide)
- Codex CLI historically used **line-based truncation** (256 lines or 10 KiB, head+tail), extended to MCP tools in v0.56; a 2025 issue (#6426) requested switching to token-based limits like Claude Code's 25K default. [github](https://github.com/openai/codex/issues/6426?timeline_page=1)
- Codex supports **1M-token context windows** with auto-compaction around 900K, but per-tool truncation remains a separate concern. [community.openai](https://community.openai.com/t/1-million-context-to-enable-professional-workloads/1390013)

**Server influence:**

- No direct server-side control over Codex's truncation strategy is documented, but the new **extension API** (v0.151.0) allows rewriting tool results before they reach the model. [ai-tldr](https://ai-tldr.dev/releases/openai-codex-cli-0-151/)
- Codex's MCP extension profile is fixed at session creation; servers cannot dynamically negotiate representation. [github](https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md)

## Least-risk strategy for servers

Given the fragmented client landscape, the safest approach in 2025–2026 is:

1. **Always populate `content` with a standalone, human-readable text block** (even if it's just the JSON string). This ensures compatibility with Cursor, VS Code, LangChain, and any client that ignores `structuredContent`. [forum.cursor](https://forum.cursor.com/t/mcp-tool-results-containing-only-structuredcontent-are-silently-dropped/167346)
2. **Also populate `structuredContent` with the typed payload** for clients that prefer it (Claude Code, ChatGPT/Codex in some modes, Gemini CLI). [glama](https://glama.ai/mcp/servers/cchadha2/mcp-response-types-lab)
3. **Avoid exact duplication for large payloads**: put a concise summary or preview in `content`, and the full structured data in `structuredContent`. This reduces token costs for clients that pass both fields to the model. [futuresearch](https://futuresearch.ai/blog/mcp-results-widget/)
4. **Optionally detect the client** (via `clientInfo.name` or capabilities) and tailor the response:
   - For widget-capable clients (Claude.ai, Claude Desktop, ChatGPT Apps), use `structuredContent` for interactive data and `content` for a short summary. [futuresearch](https://futuresearch.ai/blog/mcp-results-widget/)
   - For terminal clients (Claude Code, Cursor), skip widget data and keep `content` as the primary channel. [futuresearch](https://futuresearch.ai/blog/mcp-results-widget/)
5. **Monitor for Codex-specific regressions**: if targeting Codex, test across app versions to ensure `content` isn't silently dropped when `structuredContent` is present. [ithub.global.ssl.fastly](https://ithub.global.ssl.fastly.net/openai/codex/issues/38287)

In short: **don't rely on either field alone**. Populate both, but make `content` a complete, standalone answer and avoid duplicating large JSON verbatim unless you know your target clients well.
