# MCP tool results: `content` vs `structuredContent` — research record

Compiled 2026-08-15 from the MCP specification, SDK source, maintainer issue threads,
and direct measurement of `sdd serve`. Independent research pass run through Perplexity
at Christopher's request after a first pass reached an overconfident conclusion.

## 1. What the spec says

Current released revision: **2026-07-28** (an earlier pass in this session mistakenly
read the 2025-06-18 page — the guidance is materially unchanged, but check the revision).

> For backwards compatibility, a tool that returns structured content **SHOULD** also
> return the serialized JSON in a TextContent block.

- `SHOULD`, not `MUST` — unchanged in strength since 2025-06-18.
- If `outputSchema` is declared, structured results **MUST** conform; clients **SHOULD**
  validate.
- 2026-07-28 broadened `structuredContent` from object-only to any JSON value
  (SEP-2106-related).
- **The spec does not define which representation a host sends to the model.** It
  defines the wire result only; model-context projection is client behaviour.

## 2. Proposals in flight (none ratified as of 2026-08-15)

| Proposal | Status |
| --- | --- |
| SEP-1624 — clarify `structuredContent` vs `content` | Open proposal, Oct 2025. Would define `content` as model-oriented/token-efficient and `structuredContent` as machine-oriented, requiring both to be *semantically equivalent* |
| SEP-1576 — token bloat | Open, Sep 2025. Mostly schema redundancy and tool selection, not result-body duplication |
| Response-format negotiation (`text` / `structured` / `both`) | Discussion only, Oct 2025 |
| Field projection for tool output schema | Referenced, not established as accepted |

No ratified core mechanism lets a client negotiate "structured only" for ordinary tool
calls.

## 3. Client projection — the weakest evidence in this record

Which field actually reaches the model when both are present:

| Client | Behaviour | Evidence |
| --- | --- | --- |
| Claude Code | Prefers `structuredContent`; duplicated text blocks **not** forwarded | Strong — official docs |
| VS Code / Copilot | Prefers `structuredContent` for model context, shows both in UI | Moderate — SEP-1624 |
| Windsurf | Likely `content`-only | Weak |
| Cursor | Contradictory across sources — `content`-only, or stringified structured | Conflicting |
| Claude Desktop, ChatGPT, Codex CLI, Zed, Continue | No reliable primary evidence found | Unknown |

**No client is established to feed both copies to the model.** Wire duplication is not
the same as model-token duplication. Most clients publish nothing about this, and issue
reports describe particular versions rather than stable contracts.

## 4. Measurement of `sdd serve` (2026-08-15)

Raw JSON-RPC `tools/call` response for `start_session`, over stdio:

```
whole response          53,478 bytes   (~13,369 tokens)
  content[] text block  26,568 bytes
  structuredContent     27,003 bytes
  semantically identical: true
```

So the payload is serialized twice on the wire. Whether that costs model tokens depends
entirely on the client (see §3); on Claude Code it does not.

Door serve composition (single copy, 27,057 bytes ≈ 6,764 tokens):

```
framing              16,540   61.1%
  Guiding directives  3,849   23.8%   graph-derived
  Focus               3,796   23.4%   graph-derived
  Working principles  2,823   17.4%   static, added 2026-08-11
  Recent movement     2,441   15.1%   capped — only lane reporting truncation
  Aspirations         1,767   10.9%   graph-derived
  Participants        1,472    9.1%   graph-derived
instructions          9,600   35.5%
  Moves               3,775   40.3%   8 moves x ~420 bytes, near-uniform
  How dialogue feels  1,315   14.1%
  Ending is the arc   1,217   13.0%
  The graph, in short 1,072   11.5%
  six smaller blocks  1,964   21.0%
everything else         917    3.4%
```

For comparison: the door payload measured 19,096 chars in July 2025 against a 25KB
contract. The single copy is now 27,057 bytes — over that contract on its own.

## 5. Why the go-sdk emits both, and the escape hatch

`go-sdk@v1.6.1`, `mcp/server.go:355-392` — for a typed handler returning a nil
`CallToolResult`:

```go
res.StructuredContent = outJSON
// If the Content field isn't being used, return the serialized JSON in a
// TextContent block, as the spec suggests
if res.Content == nil {
    res.Content = []Content{&TextContent{Text: string(outJSON)}}
}
```

Every `sdd` serve handler returns `nil` for the result (`mcpapp/tools.go:471, 517, 535`),
so the compatibility block is always synthesized.

**The escape hatch exists in the pinned version**: return a non-nil `CallToolResult` with
`Content` set. `StructuredContent` is still populated from the typed output; the
duplicate is only synthesized when `Content == nil`. No SDK fork, no handler-signature
change.

The general research finding — that the Go typed path exposes no *flag* for this — is
correct; the point is that the path does not need one.

## 6. Client-side amplification observed

In a live Codex session, the agent reported forwarding both representations itself:

```js
for (const c of r.content ?? []) { if (c.type === "text") text(c.text); }
if (r.structuredContent) { text(JSON.stringify(r.structuredContent, null, 2)); }
```

— the second pretty-printed, so its context cost exceeded even the doubled wire. The
agent's own correction: *"the reported 16.1k tokens described the duplicated output I
attempted to forward, not necessarily the size of the original catch-up response."*

Caveat: this snippet is an agent's account of its own harness and could be
reconstruction. It is recorded as a report, not a verified mechanism. Two claims in this
session's earlier reasoning were agent self-reports taken as fact and both were wrong.

## 7. Documented size budgets

No MCP-wide budget or protocol limit exists. Claude Code is the notable documented case:

- warning above **10,000 tokens**
- default maximum **25,000 tokens**
- configurable via `MAX_MCP_OUTPUT_TOKENS`
- per-tool `_meta["anthropic/maxResultSizeChars"]`, ceiling **500,000 characters**

Codex CLI's ~10K-token per-tool-output budget is recorded separately (`s-tac-40d`), with
`tool_output_token_limit` in `~/.codex/config.toml`.

## 8. Position for sdd

1. **Keep emitting both.** Client projection is unstandardized and mostly undocumented;
   dropping `content` would silently blank the result on any content-preferring client
   (Cursor's reported failure mode), and structured-only is protocol-valid but not
   deployment-safe.
2. **The two need not be byte-identical** — only semantically equivalent. Compact
   markdown in `content` with the full object in `structuredContent` is the pattern
   SEP-1624 proposes and MCP Apps guidance already codifies. Our text block is currently
   JSON-escaped prose, which is the worst encoding for a payload that is mostly prose.
3. **Serve-side size discipline remains the only host-independent guarantee** — the
   conclusion `s-tac-40d` already reached, unchanged by any of the above.
