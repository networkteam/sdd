# Completion interview record — v1 workflow engine plan

Interview run 2026-07-05 (Christopher, Claude) over the open decisions blocking completion of 20260702-220449-d-tac-ry0. One decision per section, with rejected alternatives.

## 1. Widen-depth fabrication → session read log + mechanical gate

**Decided:** reads stay free and ungated; the server logs what it served per session — read events carrying tool, entry IDs, depth (full = body served via show or as injection primary; summary = chain bullet, search header, snippet), timestamp. IDs and metadata only, never payloads. Capture assemble gains `refsInspected`: every draft ref present in the session log at full depth. Failure = rejection naming exactly the un-inspected IDs (one extra turn only on indiscipline; tool use stays consistent — the agent reads via the normal tools and the gate passes on re-report). tlo-seeded captures pass automatically when the parent inspected the material (same session log).

**Rejected:**
- *Delta-injection at assemble* (engine serves chains for un-read refs): second content-serving path, machinery the design doesn't need; rejection + agent-driven reads is simpler and consistent.
- *Citation gating alone*: verifies fetching, not reading.
- *Verification reaching the forked worker's context*: impossible by construction; instead task workers' reads go through the same server under the parent session, making inspectedIds corroborable against logged serves.

**Honest boundary:** the log proves material entered the context, not comprehension. Composition (briefs/catch-up) stays trust-based in v1 — above skill parity (the skill records nothing) — with briefings-composed-from-zero-reads as the dogfooding watch metric.

## 2. Serve sizes (4hh) — three shares, three dispositions

- **Framing share:** done (21KB → 7.9KB, shipped in the session architecture work).
- **Drill share (fixed now):** the drill is the agent calling the free MCP search tool; 26.5KB was its response with snippet defaults. Fix: MCP defaults become header-only + hit cap (the CLI's `--max-citations 0 --limit 8` behavior); snippets stay available by explicit parameter. Depth = show (logged, full-depth).
- **Compose share (deliberately unchanged):** the 43KB six-lane injection at catch-up's compose step keeps its rich rendering — status segments, topics, participants, expand sub-lines were each added to fix measured catch-up fidelity failures (mis-attribution, dead deps treated as live). Measured today: 43KB as-shipped vs 18.4KB brief-render — the cut is available but not taken. **Reopening condition:** once the read log makes drilling observable, run rich-lanes vs lean-lanes-plus-logged-drills side by side and compare briefing fidelity; evidence decides, not taste.

## 3. Session operations (j25, w3v)

- `abandon(sessionHandle)` — teardown without resume or framing; response names the label and parked threads being discarded (nothing vanishes silently). Measured baseline: six calls + ~28KB framing per session to clear three stale sessions.
- Served-once memory keyed to **connection**, deduped by **content hash over the rendered block bytes** (post-template, post-injection). Identical bytes → one-line stub; changed render (graph moved mid-session, focus captured) → full serve. No semantic skip rules. The measured within-connection double-pay (~12.5KB + ~12KB in one minute) drops to once.
- Unbound-call rejections inline the parked-sessions list (handle + label). Checked: instance handles are per-session counters (`i_N`, engine/session.go) — no global resolution possible or desired; a fresh-context agent should re-orient through the door.
- **Park-a-seeded-capture affordance (new):** a move can be parked back to the shell junction, state kept, listed in open_threads at junctions and conclude. Converts mid-dialogue "note this for later" from conversation memory into a graph-visible thread. Small engine addition riding the tlo seeding work.

## 4. Registration (wfl) + gt3 + locale (fgy)

- Registration follows `sdd init --scope`: project default writes `.mcp.json` (Claude Code) and `.codex/config.toml` (Codex); user scope opt-in writes `~/.claude.json` / `~/.codex/config.toml`. Permissions pre-allowed per agent alongside.
- Graph resolution per call (walk up from cwd), never once at process start. Worktree edge is a named test case.
- No-graph behavior: server starts cleanly; door answers "no SDD graph in this workspace — run sdd init". Precondition for user scope.
- Writes are drift-respecting: stamped entries refresh, user-modified entries are left with a notice (same policy as skill renders). Writer mechanism decided in-slice: own surgical JSON writer; TOML via structure-preserving lib vs delegation to `codex mcp add` weighed there.
- Claude Code: `alwaysLoad: true` on the sdd entry — tools loaded at session start, no ToolSearch ceremony (~14 small schemas, constantly used; settles gt3). Codex does not defer schemas — nothing to do.
- Locale: shell framing carries the configured locale line (sdd info equivalent); non-English locale serves the vocabulary table as a hash-deduped data block per the single-source contract. Authoring-language enforcement already exists CLI-side.

## 5. Slice/task breakdown for handoff

Each slice = one dedicated agent session, anchored on graph entries (this directive + 20260704-235517-d-tac-tlo + the plan). Done signal per slice closes nothing prematurely — the plan closes when its ACs are evidenced.

**Slice A — seeding contract + spec revisions** (first; blocks B).
- Engine: params seed state, gates evaluate on entry, satisfied steps auto-advance; declared junction handoffs (seeds down); `task` class with fork hint; anchor/anchorHint resolver pattern.
- Spec revisions: capture, engage, evaluate, explore (reclass to task).
- Evidence: table tests; one live evaluate→capture run with one widen and ≤4 capture calls.

**Slice B — three missing procedures** (after A; parallel to C).
- groom (stale-marker walk, fork-lint candidates), implementation (WIP lifecycle: wipStart/wipDone, cross-session resume), interview (realizes the interactivity directives; its dogfooding answers the resume-fidelity interview-half: 20260704-201555-s-cpt-osk, 20260703-142504-s-cpt-m6m).
- All specced against the tlo template. Evidence: embedded entries + table tests + a live run each.

**Slice C — evidence + session hardening** (parallel to B).
- Read events in session log; refsInspected gate on assemble; MCP search defaults (header-only, hit cap); abandon(sessionHandle); connection-keyed hash-dedup serving; rejection inlining parked list; park-move affordance; locale line + vocabulary block.
- Evidence: table tests; a teardown run at one call per session; a reconnect run paying orientation once; a capture rejected for an un-inspected ref then passing after the show.

**Slice D — transition readiness + close-out** (last).
- init registration per scope + alwaysLoad + permissions; no-graph door; freeze governance confirmed (shipped specs authoritative, /sdd prose frozen); full side-by-side parity pass against the parity inventory.
- Evidence: fresh-clone engine session works out of the box; parity findings captured; plan's closing done signal cites AC5–AC8.

**Execution:** implementation sessions after A run engine mode themselves — implementation doubles as parity dogfooding. Sub-agents only for bounded read missions (explore) and review sweeps. This dialogue session remains the decision home.
