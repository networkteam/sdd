# Hermes + Honcho vs SDD — comparative analysis

Participants: Christopher, Claude

## Sources

- Hermes Agent — https://hermes-agent.nousresearch.com/ and https://github.com/NousResearch/hermes-agent
- Honcho — https://github.com/plastic-labs/honcho (Plastic Labs)

Both were named in talks/presentations Christopher reviewed. Honcho also ships *inside* Hermes as its "dialectic user modeling" layer, so the two are coupled in practice.

## What Hermes is

A standalone, self-hosted autonomous agent ("the agent that grows with you", MIT). It *is* the agent, not a connector — run continuously on your own infra ($5 VPS to GPU cluster).

- **Modules:** `agent/` (core loop), `hermes_cli/`, `gateway/` (multi-platform messaging, separate process), `skills/` (procedural memory + autonomous skill creation), `tools/` (40+, RPC-called), `providers/` (LLM abstractions).
- **Sandbox / code execution:** six backends — local, Docker, SSH, Singularity, Modal, Daytona — with container hardening and namespace isolation. Modal/Daytona give serverless persistence (environment hibernates when idle).
- **Channels:** Telegram, Discord, Slack, WhatsApp, Signal, email, Home Assistant, webhooks/REST. Cross-channel conversation continuity; voice-memo transcription.
- **Tools:** RPC-called Python scripts ("zero-context-cost turns"); a Tool Gateway via Nous Portal routes web search (Firecrawl), image gen (FAL), TTS, browser automation.
- **Memory / learning:** FTS5 session search + LLM summarization for cross-session recall; `MEMORY.md` / `USER.md` / `SOUL.md`. A "closed learning loop": autonomous skill creation after complex tasks, skills self-improve, agent-curated memory with periodic nudges. Honcho provides persistent per-user modeling.

## What Honcho is

"Memory infrastructure for stateful agents that understand changing people, agents, groups… over time." Markets itself as reasoning-first: extracts conclusions rather than matching text chunks.

- **Loop:** Store messages → Reason (background deriver updates peer models) → Query (context / representations / insights / search) → Inject into any model.
- **Peer:** any participant — human or AI. Unified. Multi-participant sessions with mixed humans + AI; configurable observation (which peers see which others).
- **Data model:** Workspace → Peers, Sessions (many-to-many peers + Messages labeled by source peer).
- **Collections:** vector observations keyed by (observer, observed) peer pairs — the same mechanism powers self-representation and what one peer knows about another (theory of mind).
- **Outputs:** Conclusions (deductive/inductive), Representations (static snapshots), Peer Cards (compact identity summaries), Session summaries. Dialectic reasoning with configurable depth.
- **Stack:** Storage (sync API) + Insights (async deriver); pgvector / Turbopuffer / LanceDB; Python + TypeScript SDKs (`peer.chat()`, `session.context()`, `peer.representation()`, `peer.search()`).

## Connection 1 — building blocks & the road not taken

Hermes is a working instance of the standalone-agent path d-stg-6za weighed and s-cpt-r57 rejected. r57's bet: don't build an SDD agent — the user's own ChatGPT/Claude/Cursor is the executing agent, SDD is a workflow specialist behind MCP.

Consequences for "can we reuse its building blocks":

- **Sandbox:** if the executing agent is the user's own, SDD runs no arbitrary user code, so it needs no code-exec sandbox. Hermes' six-backend abstraction only matters on the *self-hostable* path (d-stg-6za, technical-team sandbox) — and there it's reference material, not a drop-in (Hermes core is Python; our runtime is Mastra/TypeScript per d-cpt-t3j).
- **Channels (`gateway/`):** the connector layer r57 explicitly decided *not* to build. Useful to study as "what it costs to own the channels yourself," relevant to non-developer access (d-stg-x0l, s-tac-7kh), not as a library to adopt.

Net: Hermes is most valuable as the alternative architecture made real — a foil to stress-test the connector bet — rather than as components for the stack we picked.

## Connection 2 — peer vs actor

- **Same unification:** peer = any participant, human or AI = our actor model (d-cpt-ni0; Christopher and Claude are both actors).
- **Inverted epistemics:** Honcho *infers* an evolving, observer-relative representation per peer (vector observations, background-derived conclusions, theory of mind). Our actor is a *declared, canonical, write-once identity*, authored through dialogue and superseded explicitly.
- Honcho's peer system is the implicit-learning mechanism applied to people. The (observer, observed) keying — what Alice believes about Bob — has no analog in our single-truth graph.

## Connection 3 — auto-learning and the candidate-vs-commitment line

Hermes' closed learning loop and Honcho's background derivation both commit knowledge without review. That cuts against dialogue-first (d-stg-beb), reasoning-not-recording positioning (s-stg-ob9), and the explicit-confirmation gate (s-prc-qdu).

We already drew the precise line:

- MemPalace analysis (s-cpt-g8r): retrieval-first complements reasoning-first.
- Mining (s-cpt-bn1): automatic capture is fine *if* it produces low-confidence candidates for human review, not committed entries.

So the discipline is not "no auto-learning" — it's **candidate vs commitment**. We rely on the human alignment step for now; the mining door stays candidates-for-later. Hermes + Honcho are a second working proof that the mining loop is viable when we choose to open it.

## New insight — perspectival memory as derivation

Honcho *stores* observer-relative conclusions. SDD could *derive* the same kind of view on read — another derived attribute, like status or the role-status cascade — so it never becomes a parallel record (stays inside d-stg-3k0). Derive-on-read can't drift from source (contrast the one derivation we persist, summaries, which does drift — s-cpt-tdp).

Scope matters: over a *reasoning* graph, a per-actor projection yields a **commitment/engagement footprint** (what X decided, questioned, grounded in, which topics X touches) — not a personality model. That inverts Honcho's theory-of-mind and keeps us clear of the implicit-learning line: we infer "what has X reasoned," never "what is X like."

First instance is already in reach: a **per-participant catch-up** — what moved in the areas you've engaged, where you'd re-enter at the edge — is a perspectival derivation. It gets real now that participants (Christopher, Claude, Jonathan) have touched different parts of the graph. It also plugs into the workflow agent's guide role (r57), which reads the graph directly.

## New insight — the "reasoning-first" collision

Honcho markets itself as "reasoning-first — extracts conclusions, not text chunks." That is the exact phrase SDD uses (s-stg-ob9, the lightning talk). But Honcho's reasoning is *automated background inference*; ours is *human-AI dialogue*. Same words, opposite mechanism.

This sharpens rather than blurs the differentiator: as more tools claim "reasoning," the distinction that matters is *who reasons and whether a human confirms*. A live positioning input alongside s-stg-ljq and the README rewrite.

## What we did NOT decide

- No adoption of Honcho or Hermes components — reference only.
- Perspectival derivation is an insight, not a commitment; if pursued it graduates to its own entry (and likely a plan).
- Whether SDD ever wants observer-relative (perspectival) state at all, vs strictly single-truth, stays open.
- The mining door (s-cpt-bn1) stays open and unbuilt.
