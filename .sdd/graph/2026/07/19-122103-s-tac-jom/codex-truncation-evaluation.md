# Codex truncation of the catch-up compose serve — live evaluation record

Live outer-evaluation trace of the session-model realization (d-tac-e3x, Slice 5 shipped as s-tac-khq), run by Christopher in a Codex CLI session (version 0.144.5) against the SDD engine over MCP, 2026-07-19. Evidence verified against the raw Codex session rollout log by a subagent inspection.

## Timeline (Codex session s_20260719-092605-c958d852)

1. `start_session` — door serve returned 19,096 chars. **Not truncated.**
2. User asked what to build next; agent proposed catch-up, user agreed.
3. `start_procedure({canonical: "catch-up", …})` — response truncated by Codex's tool-output layer: `Warning: truncated output (original token count: 13749)`.
4. Agent called `resume_session({fullReplay: true})`, storing the response unprinted (see harness mechanics below), emitting only `{"blocks":1,"lengths":[74892]}`.
5. One later call loaded the stored replay and extracted lane entries line-by-line (per-entry 850-char cap).
6. Agent composed and delivered the briefing from the recovered full lanes.

## Log evidence

Session rollout: `~/.codex/sessions/2026/07/19/rollout-2026-07-19T09-25-52-019f7944-6029-7e43-9c1a-88b2e354b122.jsonl` (97 records).

- Catch-up serve truncation: `Warning: truncated output (original token count: 13749)`. Roughly 10,000 tokens survived; 3,749 were dropped from the **middle**, with an explicit inline gap marker left in the instructions string: `…sdd-managed homedir caches …3749 tokens truncated…e}: "the move-instance runtime and step-attached rule architecture…`. The cut landed inside the "Active and hot" lane; Focus and Recent done (head) plus Open and warm, WIP, and the composition rules (tail) survived. The JSON envelope still parsed cleanly.
- Door serve: 19,096 chars, no truncation warning — the 25KB door contract (verified at 18.6KB in tests, s-tac-khq) held on this host.
- Recovery calls verbatim:
  - `store("sdd_replay", r)` after `resume_session({fullReplay: true})`; printed output only the block sizes.
  - Later: `const r = load("sdd_replay"); … envelope.open_instances.find(x => x.instance === "i_2").instructions` with per-line `.slice(0, 850)` extraction (36,177 chars re-emitted in capped form).
- One additional truncation in the session (`original token count: 20226`) was a local file read + tools dump, not an MCP response.

## Codex CLI tool-output behavior (as of 0.144.5)

- Default per-tool-output budget is token-based, approximately 10K tokens (community reports also cite 10KiB/256-line variants in other versions; configurable via `tool_output_token_limit` in `~/.codex/config.toml` since ~0.59).
- Truncation is a **marked middle-cut**: head and tail retained, an explicit `…N tokens truncated…` marker inserted. Dropped content is not recoverable retroactively.
- In this harness, MCP tools are invoked from a JavaScript execution layer; the budget applies to **printed** output. A response held in a variable / `store()` is retained in full inside the exec runtime and can be re-emitted in chunks — the capability that made the fullReplay recovery work. A harness without such a layer receives only the truncated slice.

## Engine-side size measurements (worktree binary, this graph)

Full catch-up lane layout (d-prc-cat): **49,876 bytes total** (~13.7K tokens):

| Lane | Bytes |
|---|---|
| focus | 5,992 |
| recent done (n10) | 7,728 |
| active and hot (n8, expand refs(inactive)) | 9,893 |
| open loops (n8, expand refs) | 12,464 |
| open and warm (n15) | 13,763 |
| wip | 31 |

Contributing factors: no `brief` on any catch-up lane (each entry renders its full multi-sentence summary); two lanes expand refs; no `maxBytes` cap anywhere. Contrast: the user-dialogue shell (d-prc-dlg) renders all framing lanes with `brief` and caps its volatile recent-moves lane at 2,500 bytes — which is why the door stays at 18.6KB. `brief` alone drops the open-and-warm lane 13,763 → 5,375 bytes.

## Findings

1. **Validated:** the 25KB door contract held on a foreign host with default settings; the Slice 5 `fullReplay` escape was exercised in the wild and recovered a real context-loss situation without corrupting the running move.
2. **Gap:** serve-size discipline stops at the door. The uncapped catch-up compose serve exceeded the host budget and lost the middle of a lane — under instructions declaring the lanes "your entire input", making legitimate re-fetch impossible. Recovery depended on a harness-specific storage capability the engine cannot assume; the replay payload is as oversized as the serve it recovers.
3. This resolves the uncertainty the earlier continuity trace (s-tac-eqv) explicitly left open: model-visible truncation on Codex is real, token-budgeted, and marked.

## Fix directions discussed

- Interim: add `brief` and `maxBytes` caps to the catch-up lane spec — roughly halves the serve, under the ~10K-token default budget.
- Structural: restructure catch-up as a pushed brief lead sheet plus perspective-driven pulled expansion from composable view building blocks (recorded separately as its own direction entry).
