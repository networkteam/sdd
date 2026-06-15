# Go-native SDD runtime spike — findings (d-tac-vvo)

Evaluation of the Go-native candidate for the hosted-runtime decision (d-tac-dcn).
Throw-away spike on agent-sdk-go; code on branch `worktree-vvo` (not merged).
Self-contained — captures the substance independently of the code.

## What was built
A hosted SDD agent on agent-sdk-go (Ingenimax v0.2.57) integrating sdd in-process:
six tools (sdd_view/show/search/list/info/new) wrapping the existing finders +
handlers directly (no MCP bridge), a load_instructions tool, system prompt +
sub-skills rendered from bundledskills, and a hand-rolled HTTP/SSE chat UI.
~1,290 lines, 6 files. Verified two ways: a no-LLM self-test exercising every
tool against the real graph, and a live browser session (Sonnet 4.6) running a
catch-up and an attempted capture.

## Per-dimension

### Graph integration — strongest
Tools call internal/finders and internal/handlers directly, in-process, with the
same wiring as cmd/sdd. No bridge, no serialization, zero drift between CLI and
agent surface (it is the same code). CQRS (d-cpt-l3s) makes this natural:
reads -> Finder, the one write -> Handler.NewEntry. The dimension where
Go-native wins by construction.

### Single-source instructions — proven
System prompt (~34.7 KB) plus every sub-skill/reference rendered from the one
bundledskills template tree via Load(), not a copy. load_instructions is the
runtime analogue of the Skill tool. The only hand-written instruction text is a
short runtime-adaptation preamble (per-surface glue). Contract d-cpt-chi holds
on a third surface (hosted agent), alongside the Claude and Codex renders.

### UI — works; polished path is a contained adapter
Hand-rolled HTTP+SSE + one embedded HTML page (no Node, no build step) streams
the briefing, shows tool-activity cues, and renders tool output as expandable
blocks (pre-flight auto-opens). agent-sdk-go's RunStream maps cleanly to SSE;
the "make-or-break UI unknown" was not a blocker. For the intended
assistant-ui / CopilotKit direction (AG-UI protocol): reachable by owning one
isolated AG-UI adapter (decode RunAgentInput -> drive RunStream -> map events ->
SSEWriter, plus messages<->memory reconciliation) plus one dependency — the
community Go AG-UI SDK (github.com/ag-ui-protocol/ag-ui/sdks/community/go),
footprint uuid + logrus + testify only, and vendorable if it stalls. Core
runtime stays untouched. assistant-ui and CopilotKit share the same
@ag-ui/client backend contract, so one adapter serves both.

### SDD-loop fidelity — high, with fixable gaps
Faithful: skill-driven catch-up (refs -> /sdd-catchup -> prescribed layouts ->
good clustered briefing); capture dialogue held discipline (play-back, caught an
already-closed entry, corrected refs, dialogued kind); real pre-flight gate;
full-ID refs; confirm-before-capture. Gaps found live:
- skip_preflight escape hatch was missing from sdd_new (the CLI's
  --skip-preflight); the agent flailed when the user directed a bypass. Fixed.
- Redundant tool calls (catch-up re-ran the identical sdd_view trio; sdd_info
  twice) — inflates cost; appears tied to the SDK's tool-result handling.
- Plan-approval default (requirePlanApproval=true) initially hijacked the loop
  with an upfront "approve this plan" gate and silently disabled streaming;
  disabled it — SDD's confirmation is conversational, not an upfront plan.
- SDD-substance signal (not a runtime issue): the artifact-durability gate has
  no graceful path for human-attested manual verification (a fresh-session smoke
  test with no commit). Candidate for its own graph signal.

### Stack & ops — one binary, heavy deps
One Go binary, one process — the central upside (no Node, no second runtime or
deploy). But agent-sdk-go pulls a heavy transitive tree (AWS SDK v2 + Bedrock,
openai-go, google genai, redis, grpc, otel): go.mod grew ~43 require lines. That
dependency weight is the real stack cost, even in one language. Hosting
cost-model finding: subscription auth (claude-cli, ChatGPT/Codex OAuth) does not
generalize to a hosted multi-user webapp — hosting = per-token API billing
regardless of runtime, so it is runtime-neutral on the model bill.

### Maturity & ownership — pre-1.0, sharp edges
agent-sdk-go is pre-1.0 (Ingenimax v0.2.57, ~2 days old at spike time; active;
571 stars; MIT; broad provider coverage, API-key auth only). Rough edges hit in
one session: requirePlanApproval defaults on (disables streaming with tools),
constant "Failed to parse tool call arguments" warnings, lagging baked-in Claude
model constants (no Claude 4 — pass the model string), redundant tool calls.
Prompt caching supported but off by default (spike enabled it). Ownership
wrinkle: a second variant, agenticenv/agent-sdk-go, exists — a younger
(created 2026-03, 20 stars) re-published derivative with native AG-UI, the
official MCP Go SDK, newer LLM SDKs, and a leaner dep tree; not a registered
GitHub fork. Adopting it = betting the core runtime on a young single-org
re-publish; the community-AG-UI-SDK-on-an-established-core path is the
lighter-maintenance alternative.

## Bottom line
Go-native is viable: the loop runs, the in-process integration is clean and is
the architecture's strength, single-source holds, and the polished UI path is a
contained adapter rather than a blocker or a core-SDK gamble. The costs to weigh
against the single-Go-stack benefit are the heavy dependency footprint and a
pre-1.0 runtime with rough edges — i.e. ongoing maintenance surface. This
establishes the Go side's evidence; it does not decide the verdict (d-tac-dcn).
The Mastra spike and the head-to-head comparison are a separate session.

## Follow-ups
- -provider switch (anthropic|openai|ollama) — needed if Go proceeds (Ollama for
  free local runs; GPT-5.x on an OpenAI key).
- AG-UI adapter — decided path: community Go SDK + one owned adapter (not built).
- Artifact-durability vs human-attested manual verification — capture as a graph
  signal on the real graph.
