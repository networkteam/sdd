# Workflow-agent architecture — design synthesis

## Origin: three moves
1. A standalone, SDD-specialized agent is shallow for real work — it can be taught SDD but lacks the user's own tools (their code, their domain). Dead end where the user needs depth.
2. Flip it: keep the user on whatever generalist agent they already use, and reach them as a connector (plugin / MCP). Key unlock: a tool's *output* can carry instructions, not just data — so the connector can steer behavior.
3. Synthesis: SDD becomes a stateful thing behind MCP. It holds where the user is (engaging an entry, drafting a plan) and the mode drives what it tells the consuming agent to do next. The generalist talks and tool-uses; SDD drives the workflow from behind the connector.

## Vocabulary
- **Executing agent** — the generalist the participant already uses (ChatGPT / Claude / Gemini / Cursor). Holds the conversation and the user's tools; does the work; carries almost no standing SDD instructions, just enough to know to consult the workflow agent.
- **Workflow agent** — the SDD specialist behind MCP. Holds per-session mode state, applies framework judgment, emits the next instruction with callback info, gates transitions. No user tools; sees only what it is handed, plus the graph it can read.

The split is the #1 -> #2 -> #3 thesis: the user keeps their generalist; SDD shows up as the specialist behind the connector.

## The handshake loop
init (prime minimal) -> dialogue -> executing agent spots a scenario and calls start-mode -> workflow agent inits the state machine and emits instructions + callback -> executing agent acts -> calls back with evidence -> workflow agent guides / validates -> transitions.

## Workflow-agent internals: state machine + guide + gate
- **State machine (controller)** — deterministic; holds mode/position; decides which role runs when. Keeps cost sane for an ambient tool: guide runs often, gate only at transitions.
- **Guide** — dialogue-fed, frequent, close to the executing agent. Reads the graph directly (engaged entry, chain, focus, aspirations) and is passed only the *residue not yet in the graph* (current intent, fresh observations, decisions in flight). Pull, not push: pass a thin signal, the guide triages the mode, then asks for exactly what that mode needs. The executing agent — which doesn't know SDD's mode taxonomy — never has to guess what to compress.
- **Gate** — dialogue-blind, rare, independent. Validates artifacts against graph state from an evidence package, with no access to the dialogue. The pre-flight pattern generalized: pre-flight today is a tool-as-mechanical-gate-plus-LLM-judgment with no session context; the gate is a sequence of such gates, one per transition.

## Why guide and gate must be separate agents
d-cpt-vt1: acting agents can't self-certify; validation must be structurally independent. If the gate shared the guide's dialogue context it would rubber-stamp what the guide helped produce. The gate's trustworthiness comes precisely from not having the dialogue — separation is the contract made physical, not tidiness.

## The gate enforces "graph is the sole record"
Because the gate judges without the dialogue, anything that only made sense in conversation fails. So the gate is the structural guarantee that nothing important stays trapped in dialogue residue — it must be written into the artifact or it doesn't pass (d-stg-3k0).

## Evidence package
The executing agent assembles an evidence package the gate validates without access to the work itself — keeps the gate runtime-agnostic. Risk: the party being judged chooses the evidence, which reopens the self-certification hole. Two mitigations:
- Prefer mechanically-derived evidence (diff, test output, lint) over narration — hard to slant.
- Let the gate (or state machine) define a required evidence schema per mode — you can lie in a slot but can't silently omit a required one; gaps become visible.

## Rails: request/response first, deterministic upgrade later
- Starting mechanism: plain request/response. Each tool response carries the next step plus callback info; the executing agent must honor "call me back." Universal across MCP clients, but advisory (host-driven convention, not protocol-enforced) — the soft s-stg-3vr bet, minimized by one-step-at-a-time.
- Upgrade path: MCP sampling (server asks the client's model) and elicitation (server asks the user) give deterministic server-initiated interaction — but are negotiated client capabilities with uneven support, so feature-detect and degrade gracefully (see the feasibility fact).
- Deliberately deferred for the first cut: sampling / elicitation. Start at the advisory tier; add deterministic rails where a host supports them.

## Graph-resident process rules
Modes, gates, and per-mode evidence schemas become per-project configuration shaped through dialogue and stored in the graph (giving s-cpt-k8i a runtime-agnostic home — its Claude-Code-seam mechanism was bound to the runtime we're escaping). Guardrail: rules are **additive over a non-negotiable base** of core invariants (dialogue-presence, the independent gate, canonical-only, immutability) — they may strengthen and extend the workflow, never remove its guarantees, or a project could define a rubber-stamp gate and defeat d-cpt-vt1. Open: whether a process rule is a contract or a new kind (flagged in k8i).

## Strategic mapping
- **Define the architecture:** d-stg-x0l (non-dev direct access — origin of "usable without installation") and d-stg-qlt (coherence across participants — the workflow agent's session state is a projection over the durable graph, so a different person's executing agent resumes coherently).
- **Constrain it:** d-stg-2wb (ambient, no mode-switching — modes live inside the workflow agent, invisible to the user), d-stg-3k0 (no parallel artifacts — transient orchestration, durable graph; minimal init), d-stg-7lu (autonomy, sync as exception — gate only where alignment needs it; call back as rarely as possible).
- **The tension / guardrail:** d-stg-beb (tooling serves dialogue, doesn't replace it). The workflow agent orchestrates and protects dialogue; most instructions are "have this dialogue, bring back what emerged"; the gate validates dialogue happened. Decisions stay emergent from multi-party talk at the executing-agent surface.

## Hosting
Remote, OAuth 2.0/OIDC, session state keyed to authenticated user + MCP-Session-Id, Streamable HTTP, 404 -> re-init. "Without installation" = paste a connector URL; the burden shifts to a hosted service. Managed hosting reaches non-devs; self-hosting (d-stg-6za) stays the technical-team / enterprise path. One server reaches all five major MCP-client assistants at once.

## Open questions / next
- MCP client-capability detection and graceful-degradation specifics.
- Own-LLM vs sampling for guide/gate judgment (the gate likely keeps a separate independent model — the pre-flight pattern).
- The two mode vocabularies (skill task-modes vs the widening/narrowing/evaluation/action working modes in d-prc-nkw) — which granularity the state machine keys on.
- Process-rule type-system question (contract vs new kind).
- Calibration cost of generalizing the gate across artifact types (code, docs) — pre-flight already over-objects (s-prc-vvd).
