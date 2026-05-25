# Portable SDD skill + graph access — design sketch

## What doesn't port today

The current SDD skill structure has Claude-Code-specific features:

- **Slash commands** (`/sdd`, `/sdd-catchup`, `/sdd-explore`, `/sdd-bootstrap`, `/sdd-groom`) — user invocation surface specific to Claude Code's command system
- **Sub-skills** — skills invoking other skills via the Skill tool, including dynamic context injection (e.g. the `/sdd` skill injecting framing into `/sdd-catchup`)
- **On-demand reference loading** — the SKILL.md instructs the agent to load `references/foo.md` based on mode triggers, conserving context budget
- **CLI tool calls via Bash** — instructions like "run `sdd new ...`" assume shell access
- **Tool-availability assumptions** — Read, Edit, Bash, Skill tools

## What ports cleanly

- **Capture discipline** (dialogue before capture, playback contract)
- **Modes table** (capture / engage / reflect / decide / etc. as posture concepts)
- **Framework concepts** (the loop, entry types, layers, refs, kinds, retirement)
- **Tone-of-voice rules** (catch-up voice, register by surface)

## Proposed direction

### Graph access via adapter references

Abstract graph operations behind an adapter reference loaded based on environment. Today's `cli-reference.md` becomes "the Claude Code adapter." Alternates:

- **MCP adapter** — instructions describe operations in terms of MCP tool calls (e.g. `mcp__sdd__new` rather than `sdd new`)
- **API adapter** — for runtimes calling SDD via REST/streaming HTTP

The skill body refers to operations abstractly (e.g. "capture an entry" / "load entry chain"), and the adapter provides the concrete invocation form.

### Skills as templates, not Claude-Code-bundled

Instead of bundling skills in Claude Code's skill format, store the skill *content* as prompt templates and snippets in the SDD repo. A build step produces:

- A Claude Code skill bundle (current target)
- A Langflow system prompt + tool definitions
- (future) Other runtime-specific outputs

This way the source of truth is portable; runtime adapters render the final form.

## Open question: dynamic loading of references in non-CC runtimes

Claude Code skills load referenced files on-demand based on agent judgment (mode triggers). Langflow agents have one large system prompt fixed at flow design time. Candidates:

1. **Inline all** reference content into the system prompt (cost: context size always-on)
2. **References as MCP tools** the agent calls (cost: agent must remember to call)
3. **Hybrid** — small core inline, the rest behind MCP tools
4. **Sub-flow dispatch** — separate flows per mode, dispatched by an intent-classifier flow (cost: complex Langflow setup)
5. **Prompt-template-as-tool** — a Langflow Prompt Template component exposed as a tool that returns mode-specific instructions on demand (surfaced during PoC)

## Validation so far

A quick Langflow PoC tested the prompt-template-as-tool approach:

- **Setup**: Agent component with the SDD `SKILL.md` as system prompt; Read File tool wrapping a subset of references (cli-reference, meta-process, playbook-augment-plan, playbook-engage); a Catchup Skill Prompt Template component exposed as a tool (`BUILD_PROMPT_CATCHUP`). Model: GPT-5.4. No MCP yet — dynamic graph data was hard-coded in the template for the test.
- **Result**: The agent produced an on-format catchup (italic focus line, story-arc headers, numbered items with full IDs, drill section, final prompt). Threading was reasonable, if slightly different in arc-splitting (4 arcs vs the 3-arc shape Claude/Opus produced on the same input).
- **What it validated**: skill-content-as-system-prompt is viable; prompt-template-as-tool is a workable answer to dynamic loading of mode-specific instructions; the agent loop reaches far enough for SDD-shaped dialogue.
- **What it did not validate**: dynamic graph data injection (still hard-coded), capture flow (no `sdd new` invocation yet — needs MCP), multi-mode dispatch, and the full sub-skill set beyond catchup.
- **Friction observed**: Langflow's Read File tool concatenated all uploaded references into one output rather than letting the agent select one; the flow needed explicit chat input to trigger (no auto-greeting); model list in the Langflow UI was missing some current models (e.g. GPT-5.5 not present at PoC time).

## Next steps (deferred to follow-up dialogue)

- MCP server wrapping SDD CLI surface — closes the dynamic graph data gap and lets the agent invoke capture/query operations.
- Address reference-loading via one of: one Read File tool per reference, a custom Langflow component, or references-as-MCP-tools.
- Decide whether the adapter + templates architecture warrants a conceptual directive committing to that direction, after one more pass with MCP integrated.
