# Design context: hosted SDD sandbox agent

## Why this direction

The current path to engage with an SDD graph requires git, the sdd binary, Claude Code installed locally, and a checked-out repo on a laptop. That's a barrier even for technical colleagues already in the SDD circle, and a much bigger one for the less-technical participants the framework aspires to reach (d-stg-x0l). It also pins engagement to a single machine — the user wants to dialogue, capture, and read the graph from a phone, away from the laptop.

## What we want

A self-hostable web UI fronting an agent backend that runs in a sandbox with `sdd`, `git`, and the necessary skills. Use cases prioritized in this direction:

- Dialogue with the agent
- Capture new entries
- Read / report from the graph

In-repo development (writing code, running tests) is out of scope for this surface — it remains the local CLI / IDE workflow.

## User one: technical-team dogfooding

External newcomers as user one was considered and rejected: starting with people who don't already know SDD means designing against an imagined model. The technical team already uses SDD daily and can name what hurts — they are the dependable signal source while the shape is rough. Once it is usable for them, the path opens to less-technical colleagues, then external participants.

## Runtime fork — three shapes considered

**A. Claude Agent SDK + custom web app.** Anthropic's official SDK runs the agent loop; we build a thin chat UI. Skills load via the SDK. Per-user sandbox container with `sdd`, `git`, and the agent process. Full control over SDD-specific UX (entry preview, attachment flow, ref hover). Trade-off: ties the runtime to Anthropic.

**B. Headless Claude Code CLI + web wrapper.** Spawn `claude` processes per session, proxy I/O. Skills, sub-skills, slash commands work as-is. Trade-off: CC is terminal-first; headless web wrapping is awkward, and the runtime is doubly Claude-locked (CC + Anthropic).

**C. Existing chat UI (Open WebUI / LibreChat) + SDD-aware backend.** UI lift is near zero. Trade-off: generic chat surfaces; SDD-specific affordances must be bolted on and may fight the host UI.

## Why agent-agnostic reshapes the trade-off

Both A and B couple the agent runtime to Claude. SDD's principle is to remain open to other agents — both because Claude Code is programmatically restricted, and because we do not want SDD's reach gated on one provider. That pushes us toward stacks that are provider-agnostic at the runtime layer. Langflow is one candidate (open-source, self-hostable, supports many LLM providers, visual workflow + chat UI). LangChain or Letta-style agent platforms are adjacent alternatives.

## What's deferred

The specific framework choice. Langflow exploration is in progress; once we know what it does and does not give us, a follow-up tactical plan will commit to a stack and shape the actual build.
