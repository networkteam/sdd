# Mastra runtime spike — findings (d-tac-f09)

Evidence for the runtime verdict (`d-tac-dcn`), scored against the shared
six-dimension rubric. Built throwaway in worktree `worktree-f09`; does not merge.

## Bar: HIT (both halves, against the real graph)

A non-technical user, through a web chat, end-to-end:

1. **Plain-language catch-up** — one request ("catch me up, I'm not technical")
   produced a genuine story-arc briefing synthesized from the live graph:
   `finishReason: stop`, 6 steps, tool sequence `sddInfo → sddView → sddView →
   sddView → sddList`. Output matched the `/sdd-catchup` shape (clustered arcs,
   numbered action-tight items, "drill A / survey" affordance) — because the
   rendered single-source instructions drove it, not bespoke prompt text.
2. **Dialogue-gated capture** — "note this for the project…" produced the full
   discipline: `sddInfo` + 4× `sddSearch` (widen) + `sddShow` + `sddView`
   (topic research), then a **play-back** of the proposed entry (kind, layer,
   refs *with kinds*, topics, confidence) and a request for confirmation —
   **no write**. On "yes", it ran `sddNew`, the **pre-flight gate fired**
   (a low-severity `grounded-in`-vs-`related` ref suggestion, surfaced
   non-blocking to the user), the entry was created
   (`20260614-191225-s-tac-vrh`, correct frontmatter, canonical participant
   `Christopher`, `surfaced-by` ref to `d-tac-f09`), and the agent **verified
   the generated summary**.

## Scored dimensions

### 1. UI — quality and effort  ·  STRONG
- **Mastra Studio** (built-in dev playground, `/`): a working chat UI with zero
  UI code — streams replies, shows tool calls/traces. Great for dev, but it's a
  developer surface, not a non-technical product.
- **Custom product chat** (`/chat`, `ui/chat-ui.ts`, ~150 lines, no framework):
  clean streaming chat that hides the SDD machinery (a "working…" pill stands in
  for tool calls), served same-origin by the Mastra server via `registerApiRoute`
  — no CORS, no separate front-end build. Mastra's `/stream` endpoint is a plain
  AI-SDK SSE stream (`data: {type:"text-delta", payload:{text}}`), trivial to
  render.
- Not built, but available and the rubric's named path: **assistant-ui /
  CopilotKit / AG-UI**. These add polished React components and generative UI on
  top of the same AI-SDK stream. Not needed to reach a good UI here, but they are
  why Mastra's UI story scores high: a mature, documented ecosystem exists.
- **Read:** a good web chat is cheap on Mastra — built-in for dev, ~150 lines for
  a clean product surface, a rich component ecosystem beyond that. This is
  Mastra's headline strength and it holds up.

### 2. Stack & ops  ·  WEAK (the core cost)
- Adds a **second permanent runtime**: Node 24 + pnpm (corepack) alongside the Go
  toolchain. Mastra pulls a large dependency surface (`@mastra/core` +
  loggers/memory/libsql/observability; the generator also wired DuckDB, dropped
  here as it fought the dev server's file lock).
- Two things to build, deploy, host, and keep updated instead of one.
- The bridge is a **subprocess** boundary (Node → `sdd` binary), so a deploy ships
  both the Node app and the Go binary, with env/key handling on both sides.
- **Read:** this is where staying in one Go stack (the `d-tac-vvo` premise) is a
  large net positive, and it must be weighed directly against the UI advantage.

### 3. Graph integration  ·  CLEAN but indirect
- Shell-out to the real `sdd` CLI = **zero drift** (the CLI *is* the behavior;
  the same finders/handlers, the same pre-flight, the same commit/auto-summary).
  No serialization layer to keep in sync, no schema duplication.
- But it *is* an out-of-process bridge: CLI args in, stdout/stderr out, one
  process spawn per tool call. Contrast the Go spike, which calls finders/handlers
  in-process with typed values and no spawn. Functionally identical results here;
  the difference is directness and latency, not correctness.
- The 1:1 mapping made this honest: catch-up and capture are agent behaviors
  composed from primitives, not bridge-baked shortcuts — so dimensions 1/4/5 test
  the agent+instructions, not the bridge.

### 4. SDD-loop fidelity  ·  HIGH
Observed, unprompted beyond the rendered instructions: catch-up clustering;
**widen-before-capture** (4 searches from different angles); **dialogue-before-
capture with explicit confirmation**; **pre-flight gate** honored and surfaced
(not auto-skipped); **summary verification**; full-ID refs; correct ref-kind
nuance (`surfaced-by` for the spike, `related` for the prior finding); canonical
participant resolution from `sdd info`. This is faithful SDD behavior from a
hosted agent, and it is the strongest evidence that the single-source render
carries the framework, not just trivia.
- Caveat: the model occasionally passed an array param (`topics`) as a CSV string
  — the Zod schema caught it and the model self-corrected on retry. Minor.

### 5. Single-source instructions  ·  WORKS, with a real cost finding
- A `mastra` render profile (`internal/bundledskills` + `render/main.go`) renders
  the one neutral template tree to a directory the agent reads — **no hand-copied
  prompt**. `inject` maps `sdd <x>` → the `sdd_<x>` tool. 16 files render clean.
- **Cost & caching:** the loaded substance is a **~26k-token system prompt**
  replayed on every step of the tool loop. Anthropic prompt caching is opt-in per
  request (a `cacheControl` breakpoint on the content block), so it was off by
  default — early runs paid full freight (~147k input tokens for one 6-step
  catch-up, `cache_read: 0`). Marking the system prompt cacheable fixed it,
  **measured**: cold call `cache_creation=26186`, warm call `cache_read=26186`
  with only 329 fresh input tokens. **This prompt-loading cost is runtime-neutral**
  — the Go spike must load the same rendered instructions and opt into caching the
  same way, so it is not a Mastra demerit. Remaining levers: a rolling cache
  breakpoint on the conversation tail (history + tool results, not just the static
  system prompt), and template conditionals (`{{ if ne .Agent "mastra" }}`) to
  trim CLI-only sections (heredocs, sync-check handling, attachment mechanics) from
  the hosted render.
- **Finding for the contract (`d-cpt-chi`):** the single-source render *works* for
  a new hosted surface with a tiny addition, but the templates are written for a
  CLI surface; truly serving a hosted agent needs hosted-aware conditionals, which
  is exactly the "hosted-agent prompts" surface the contract anticipates.

### 6. Maturity & ownership  ·  STRONG
- Mastra `@mastra/core` 1.42 / CLI 1.13: mature, batteries-included — Studio,
  agent memory, storage adapters, observability, streaming, multi-provider models
  via a gateway string (`anthropic/claude-sonnet-4-6`, aligned with sdd's config),
  first-class MCP client.
- Cost of that maturity: a big, fast-moving Node dependency tree we don't own and
  must track.

## Cross-spike note (comparison validity)
The 1:1 granularity is not a fidelity nicety — it is what makes the bake-off
valid. Both spikes expose the same operation set (the Go spike wraps finders/
handlers; this wraps the CLI over the same finders/handlers). Equal granularity
means the score reflects runtime + bridge + UI, not tool-surface design. The bar
("catch-up + capture") is *outcome*-framed; reading it as a *tool* spec would
hang coarse `catchup`/`capture` tools off the bridge and hollow out dimensions 4
and 5. Worth stating explicitly in the rubric for the Go spike too.

## Not tested / out of scope
- Auth (rough/unauthenticated by design).
- Production deploy (`mastra build` not exercised).
- Multi-turn memory via the raw HTTP API needed explicit thread wiring; Studio and
  the `/chat` page carry history client-side, which works.
- assistant-ui/CopilotKit not built — assessed, not measured.

## One-line read
Mastra clears the bar with high SDD-loop fidelity and a cheap, good UI path; its
real price is a second Node stack and an out-of-process bridge. The token-heavy
prompt is a shared hosted-approach cost (now mitigated by caching), not a Mastra
demerit. The verdict turns on how much the single-Go-stack benefit (`d-tac-vvo`)
outweighs Mastra's UI maturity — which is exactly the trade the plan said to weigh.
