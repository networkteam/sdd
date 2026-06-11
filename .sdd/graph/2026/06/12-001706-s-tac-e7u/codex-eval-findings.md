# Codex eval — first run findings

## Setup
- Throwaway branch `sdd/4ln-codex-smoke` off the slice branch (discarded after the run).
- Agent: OpenAI Codex (gpt-5.5); `$sdd` invokes the rendered skill (no native `/sdd` command).
- Codex skills rendered to `.agents/skills/` via `sdd init --agents claude,codex`.
- Target graph: the real SDD graph (~696 entries).

## What transferred (procedural discipline)
- **Catch-up**: ran the `sdd info` / `sdd view` injections in their instructed-run form, read the references, routed to a normal check-in (not bootstrap) on the populated graph, and produced the story-arc clustered briefing with numbered actions and a drill offer. Read and interpreted the active WIP marker.
- **Grounding**: topic research (`active:as-counts`) before proposing; located the baseline gap via search plus a bundled `sdd show` inspection of the chain; gracefully degraded vector→text search when the embedding endpoint was blocked.
- **Capture discipline**: dialogue-before-capture — played back type/layer/kind/refs/topics and waited; re-played-back and re-confirmed after a mid-flight correction (the MCP-driven test had failed exactly this).
- **Mechanics**: full-ID refs, canonical-only participants (reasoned that no Codex actor exists yet), English entry body under `Language: en`, pre-flight execution, and post-capture summary self-verification for drift.
- Notable correct subtlety: `builds-on` on a terminal `done` signal rather than `addresses`.

## Judgment ceiling (where a review pass adds value)
- **Ref-kind over-reach**: proposed `refines` for a gap→gap "narrows the area" relationship where `related` is correct.
- **One-idea-per-entry slip**: bundled two unrelated gaps (Codex-harness friction + the gollm bug) into a single entry when prompted with both together.
- Read: the procedural loop transfers; finer semantic judgment is good-but-imperfect.

## Operational caveats (captured as separate gaps)
- **Codex sandbox approvals** (s-tac-gtq): the first network/graph-touching commands need explicit approval; `sdd search --query` degrades to text if denied.
- **gollm `max_tokens`** (s-tac-t96): OpenAI gpt-5.x pre-flight returns 400 on `max_tokens`; needs `max_completion_tokens`. The eval completed on the Anthropic pre-flight path instead.
- **Shell-quoting**: apostrophes in a `--refs` `desc` broke single-quoted JSON; the agent recovered via a heredoc. General `sdd new` ergonomics edge.

## Scope / status
- Single smoke-grade run with semi-arbitrary content — not a rigorous baseline.
- Partially addresses s-tac-qy8 and AC 8 of d-tac-4ln; closes neither.
- Suggested next: a second independent capture, and probing instructed sub-skill invocation across other flows.
