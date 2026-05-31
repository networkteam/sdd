# Feasibility research — workflow-agent via MCP (2026-05-31)

Two Perplexity research passes. Protocol facts are primary-sourced and firm; landscape, market-share, and client-support figures carry source caveats noted inline. Perplexity's self-ranking as the easiest onboarding is discounted as biased.

## Pass A — consumer assistant landscape & non-developer MCP onboarding

All five assistants can be extended in some way, but the easiest path for non-developers differs a lot across products. Broadest consumer-friendly setup is still ChatGPT/GPTs; the most straightforward "connect my own data" option is strongest in Perplexity and Microsoft Copilot Studio for business users; Gemini remains mostly tied to Google-account services and Workspace/admin settings.

### Reach today
- ChatGPT: reported above 400M weekly active users in early 2025; 2026 estimates still place it well ahead of the others in standalone assistant usage.
- Claude: much smaller consumer reach; 2026 referral-share estimates ~2.7%–6% depending on source/metric.
- Google Gemini: large mainstream reach via Google's ecosystem; ~9%–25% depending on measurement.
- Microsoft Copilot: broad reach via Windows, Edge, M365, enterprise; standalone consumer share modest, ~1%–4%.
- Perplexity: smaller but growing fast; ~2%–8% of assistant/referral share; one May 2026 estimate says >100M monthly active users.

### Extension mechanisms
- ChatGPT: custom GPTs, GPT Actions, newer ChatGPT Apps / MCP-style integrations.
- Claude: claude.ai MCP integrations/connectors; product-specific connectors and MCP Apps-style interactive experiences.
- Gemini: extensions to Google services, custom Gems; Workspace admins control Workspace extensions.
- Microsoft Copilot: Copilot Studio agents, connectors, plugins, custom connectors; M365 Copilot extended through agents rather than users adding tools to the base app.
- Perplexity: first-party connectors and custom remote connectors using MCP, with account- and enterprise-level connector management.

### MCP support
- ChatGPT: yes, as a client in the ChatGPT Apps / MCP setup; OpenAI documents remote MCP server support.
- Claude: yes, strongly positioned as an MCP client; most mature consumer-facing MCP story of the five.
- Gemini: yes in the broader ecosystem, but clearest public MCP activity is around Gemini CLI / developer tooling rather than consumer Gemini Apps.
- Microsoft Copilot: yes per 2026 coverage, but onboarding is usually via Copilot Studio / enterprise integration, not a simple consumer toggle.
- Perplexity: yes, via remote MCP custom connectors (documented for accounts and enterprises).

### Non-developer onboarding (and who hosts)
- ChatGPT: create/install a GPT, add an Action or supported app/integration; GPT editor handles most setup. Self-hosting only if the third party runs its own API/MCP server. Auth: OAuth for third-party services, or API key in the GPT/app config.
- Claude: in claude.ai, connect a supported MCP service/connector from the integrations flow. No self-host for most consumer connections; yes only if the third party exposes its own MCP server. Auth: OAuth/token typical.
- Gemini: enable extensions in Gemini Apps or create a Gem; Workspace-managed extensions toggled by admins. Auth: Google account + admin approval for Workspace.
- Microsoft Copilot: non-devs usually use an already-published Copilot Studio agent or admin-approved connector rather than wiring a service themselves. Auth: M365 sign-in + service auth; enterprise connectors often need admin approval.
- Perplexity: Settings -> Connectors -> add a custom remote connector, enter the MCP server URL, choose auth, authenticate, enable. The remote MCP server is provided by the third party; if you want custom connectivity you must host that server. Auth: OAuth, API key, or none; HTTPS required for the MCP server URL.

### Practical ranking (Perplexity's own, discounted for self-bias)
Easiest "add my own tool/data" for a non-dev in 2026: Perplexity (direct connector setup), ChatGPT (polished GPT experiences), Claude (MCP integrations), Gemini (Google ecosystem), Microsoft Copilot (governed workplace). Broadest reach: ChatGPT by far. Cleanest custom-connector story for ordinary users: Perplexity claims competitiveness.

Takeaway for SDD: all five are MCP clients, so a single remote MCP server reaches every one of them — the third party (us) hosts the server over HTTPS; the user just pastes a connector URL.

## Pass B — MCP protocol capabilities for a stateful workflow server

MCP is explicitly a stateful protocol; as of the 2025-11-25 spec it supports long-lived local connections and HTTP sessioning for remote servers. Mechanics differ between stdio and Streamable HTTP.

### Session lifecycle
- Base architecture describes stateful connections with capability negotiation during initialization; a server can keep per-session state after `initialize` and use it across later tool calls from the same client.
- stdio: client launches the server as a subprocess, JSON-RPC over that process's stdin/stdout; session state tied to the process/connection lifetime.
- Streamable HTTP: each client message is a new HTTP request, so explicit session management is added — on the response carrying `InitializeResult` the server MAY issue `MCP-Session-Id`; if it does, the client MUST send that header on later requests; the server may end the session with 404 semantics and optional DELETE termination.
- A Streamable HTTP server MAY (not MUST) assign a session id, so stateless HTTP is allowed for simple cases. For negotiated capabilities, durable per-user workflow state, or server-initiated requests, a real session is the practical pattern.

### Sampling and elicitation (server-initiated)
- Sampling: the server asks the client/host to run an LLM interaction ("server-initiated agentic behaviors and recursive LLM interactions"); requires user approval and control over whether it happens, the prompt, and what the server gets back.
- Elicitation: the server asks the client to obtain additional information from the user; added in 2025-06-18, expanded 2025-11-25 (URL-mode elicitation, defaults in schemas).
- Both are negotiated client capabilities — usable only if the client declared support during initialization.
- Streamable HTTP explicitly allows the server to send JSON-RPC requests to the client on an SSE stream (before the response to a POST, or on a GET-opened server-to-client stream). So protocol-wise the server CAN initiate a mid-operation request, assuming client support.
- Client support is uneven: VS Code referenced as supporting elicitation in v1.102 (June 2025); Claude Code did NOT support elicitation when an issue was filed 2025-06-30. No retrieved official release note confirming broad production support for both. Safe reading: protocol supports them, some clients expose them, feature-detect from negotiated capabilities rather than assume.

### Tool-output steering (advisory)
- Using a tool result to suggest the next tool call or ask a follow-up is an accepted application-level pattern, NOT a special MCP control primitive. MCP standardizes tool invocation and results; there is no built-in "call tool X next" orchestration opcode.
- A server can return "next, call confirm_plan with these args" or "ask the user which environment"; many hosts/agents may follow it because it is in the result the model sees — but this is host/agent-driven, not protocol-enforced. Treat as convention unless paired with sampling/elicitation.
- Good pattern: machine-readable fields like `next_step`, `recommended_tool`, `question_for_user`, `state_token`, assuming compliant execution still depends on the host's agent loop. For deterministic mid-operation interaction, sampling/elicitation is the stronger MCP-native mechanism.

### Prompts
- MCP prompts are a server capability ("templated messages and workflows for users"), discoverable features rather than ad hoc tool output; 2025-11-25 added icon metadata.
- Aimed at user-visible workflow templates rather than hidden agent control. Can deliver dynamic per-session instruction text if the server generates content from session state — but whether users see/invoke them depends on the client exposing prompt UIs. Client exposure is inconsistent; validate per host.

### Remote hosting and auth
- Model: authenticate the HTTP caller, then bind MCP session state to that principal plus `MCP-Session-Id` when using sessions. Spec evolution (2025-06-18 -> 2025-11-25) moved toward OAuth-based remote support: MCP servers as OAuth Resource Servers, protected resource metadata, Resource Indicators, OIDC discovery, OAuth Client ID Metadata Documents.
- Streamable HTTP security: validate `Origin`, recommend auth on all connections, secure handling of `MCP-Session-Id`. SDK material (ASP.NET Core) ties sessions to authenticated user identity and authorization policies.
- Practical architecture for many users over HTTP: OAuth 2.0 / OIDC bearer tokens per request; on `initialize` create a session record keyed to user + issued `MCP-Session-Id`; require both bearer token and session header on later requests; store workflow state server-side per session with expiry/revocation; return 404 when the session is gone so the client re-initializes.

### Bottom line for the design
Cleanest 2026-aligned approach: keep workflow state server-side per MCP session; use Streamable HTTP + `MCP-Session-Id` for remote; rely on negotiated elicitation/sampling for true mid-operation interaction; use tool-output follow-up instructions only as advisory orchestration, not a guaranteed protocol feature.
