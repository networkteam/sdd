# Session architecture — evaluation record (dogfooding runs 3 and 3b)

Implementation: commits 7edcd86 (architecture), a2a80cb (first-sentence refactor).
Runs: session s_20260704-004038 (fresh Claude Code session via /sdd-engine, ~20 min, deliberate derailment attempts) and its post-restart continuation (resume test). Transcript bbb17284 analyzed forensically; engine JSONL logs correlated.

## Inner lens — sound on its own terms

- **Cold start**: 2 calls to first useful serve (one schema load, then start_session) vs ~7 guessing calls on the prior run. The door was found and used directly; /sdd-engine's pointer sufficed.
- **Tier structure**: tier-0 handshake 868 bytes; opening serve 12,754 bytes (framing 7,970 + orientation 4,581) — framing trim landed as designed (was 21KB). Framing delivered exactly once per consumer within a session binding (verified across all 29 serves).
- **Landing**: both completed moves and the abandon returned the shell junction nested in the response; landing serves compressed to reminders (858/707 bytes). The agent visibly stayed oriented and never went goalless.
- **Open-threads placement**: exactly on shell serves, never mid-procedure (verified across all serves).
- **Gates under pressure**: user choosers always relayed, never self-answered; chooser-sequence validation caught two malformed attempts; the playback gate held; the summary-fidelity step caught the generated summary smuggling a causal claim into an observation-only entry (s-prc-1qr) — concrete proof of verifySummary's value.
- **Lifecycle**: quiescent sessions auto-ended on leave (verified in two logs: the pre-run false start and the reconnect-stranded session); a session with open moves parked; the parked dialogue appeared on the door by its label; abandon dropped a draft cleanly and landed back on the dialogue.

## Outer lens — works in use

- **Resume fidelity**: a parked capture at playback survived a full client restart — draft re-served verbatim, confirm landed, entry written, shell landing. Captured as s-cpt-osk; partially answers s-cpt-m6m (capture-shaped half).
- **Friction, real**: an already-articulated finding took ~11 min / ~10 calls to reach playback — double widen ceremony (evaluate assess + spawned capture assemble) plus two rejected starts including the engage/evaluate anchor-contract asymmetry. Captured as s-tac-g7f.
- **Reconnect cost**: two rejected calls before the door, then orientation double-paid (~24.5KB in a minute) because served-once memory keys to the session binding, not the connection. Captured as s-tac-w3v.
- **Conduct vs gates**: the agent ignored the served plain-language rule twice ("lay of the land") — served text raises the floor but does not bind conduct; live instance recorded observation-only as s-prc-1qr against the open register gap s-prc-42x.
- **Serve sizes beyond framing**: compose serve 46KB → 71s model turn; drill 39KB → ~66s; evaluate serve 25KB → ~53s. Confirms the remaining (non-framing) share of s-tac-4hh.

## Deliberate scope notes

- The MCP tool surface now deviates from the plan's §5 "exact" table by design: start_session added, loop tools gated, open-threads relocated from junction payload fields to shell serves, list_sessions narrowed to parked sessions. The governing augments are d-cpt-h99 and d-tac-bfc — the plan's AC4 reads through them.
- Not exercised live: the conclude walk over open threads (covered by table tests and shell integration tests only); interview-shaped resume (open half of s-cpt-m6m).
