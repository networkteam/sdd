# Engine session continuity and reorientation observation

Date observed: 2026-07-18
Project: github.com/networkteam/sdd

## Scope

This record preserves the observable trace and its immediate graph/source context. It deliberately does not select a solution. In particular, it does not assume why the agent invoked `resume_session`, why the holder was released, or whether lease expiry caused the later unbound state.

## Evidence sources

- User-provided Codex transcript beginning with a fresh `$sdd-engine` session.
- Original session log: `.sdd/sessions/s_20260718-085502-7a9d9515.jsonl`.
- Replacement planning-session log: `.sdd/sessions/s_20260718-090234-1aad51bc.jsonl`.
- MCP tool implementation in `mcpapp/tools.go`.
- Connection leave handling in `mcpapp/server.go`.
- Holder TTL and binding implementation in `application/session_runtime.go` and `application/workflow.go`.
- Current catch-up procedure entry `20260703-194500-d-prc-cat` and its exact view layout.

## Observable timeline

All log timestamps below are local Europe/Berlin time.

1. At 08:55:02, session `s_20260718-085502-7a9d9515` opened on MCP connection `connection-0x7f29cd8014a0`; shell instance `i_1` reached `junction`.
2. At 08:55:19, catch-up instance `i_2` started and served step `compose`.
3. At 08:55:31, the log recorded a new serve of shell `i_1/junction` followed by a new serve of catch-up `i_2/compose`.
4. At 08:55:41, the same pair of serves repeated.
5. No report or transition occurred between the initial compose serve and these two repeated pairs. The engine state did not advance. The transcript identifies both repetitions as calls to `resume_session({})`.
6. At 08:56:24, `next` reported the composed briefing. The catch-up then transitioned normally from `compose` to `junction`.
7. At 09:00:05, the durable holder metadata was refreshed and then released seven milliseconds later. Holder history records only `Reason: "released"`; the persisted record does not distinguish disconnect, server shutdown, session switch, or another leave path.
8. On the later request to plan the bootstrap procedure, `next(i_2)` returned “no session is open” and named the old session as parked.
9. `resume_session({"session":"s_20260718-085502-7a9d9515"})` returned the same unbound error.
10. `list_sessions({})` also returned the same unbound error.
11. `start_session` then created a different durable session, `s_20260718-090234-1aad51bc`, on MCP connection `connection-0x26db4aa72000`. This second file records the replacement planning session, not the earlier repeated-resume sequence.

## Catch-up payload reconstruction

The session log stores serve events but not the response payload bytes, so the historical response cannot be measured directly from the log. The following measurement reconstructs the same shipped catch-up layout against the current graph:

- Rendered lane payload returned by the view query: 39,375 UTF-8 bytes.
- The returned payload ends with a renderer truncation notice stating that the underlying lane output was 53,227 bytes before the view cap.
- Static `compose` instruction unit excluding `{{.viewLayout}}`: 3,711 bytes.
- Reconstructed `instructions` field: 43,086 bytes.
- Current framing view: 12,697 bytes.

The complete `start_procedure` response is therefore slightly larger than 43,086 bytes after its control fields and JSON representation. A same-session `resume_session` clears connection served-block memory before `ServeAll`, so a replay can include the catch-up instructions again plus framing. On the current graph, those two fields alone total at least 55,783 bytes, before shell instructions, schemas, and JSON overhead.

This establishes payload magnitude and replay amplification. It does not establish that Codex truncated the model-visible response or that payload size caused the agent to invoke resume.

## Resume/list interface observation

The public descriptions and implementation do not express the same reachable flow:

- The `resume_session` tool description says a parked session handle can be passed “to switch to and replay a different one.”
- `ResumeSessionArgs.session` says it is “a parked session handle (from list_sessions) to switch this connection to.”
- An unbound-call error names parked handles and says “resume_session picks one up.”
- The implementation calls `boundSession` before examining `args.Session`. Therefore an unbound connection cannot resume the named parked session directly.
- `list_sessions` also calls `boundSession` first, so it cannot be used as a free discovery read while unbound.
- On an unbound connection, `start_session` opens a new durable workflow rather than binding the named parked one.

## Lease observation

Configured holder TTL defaults are:

- gate: 2 minutes
- agent chooser: 5 minutes
- user chooser: 30 minutes

Immediately before the catch-up report, holder expiry was set to 08:58:24, two minutes after the 08:56:24 operation. The operation then served the catch-up's user junction. In code, `WorkflowSession.Advance` calls `begin` with `currentChooser(request.Instance)` before the transition is applied, so lease selection reflects the pre-transition step.

This is observable as a lease-classification anomaly. It does not by itself prove that the client binding was dropped: TTL expiry marks takeover eligibility in durable holder logic, while connection-local unbinding/release is a separate mechanism.

## Established facts versus open questions

Established:

- Two resume calls re-served the same shell and catch-up steps without advancing state.
- The catch-up compose instruction payload is tens of kilobytes and resume can replay it with framing.
- The old holder was explicitly released, but the persisted reason is nonspecific.
- Named resume and session listing both rejected the later unbound connection.
- Entering through the door created a new durable session.
- The lease applied before the user junction used the pre-transition gate duration.

Not established:

- Why the agent chose to call `resume_session` twice.
- Whether Codex truncated the model-visible start-procedure response.
- Whether MCP transport churn, server lifecycle, or another leave path caused the holder release.
- Whether the two-minute lease contributed to the later unbound response.
- Whether the observed symptoms should be resolved by one change or by separate changes.

## Connected graph context

- `s-tac-4hh`: measured oversized framing, catch-up compose, and drill serves; formally closed while compose remained deliberately unchanged.
- `d-tac-bfc`: established the lifecycle rule that the door opens fresh and other loop tools require a bound session.
- `d-tac-dbk`: retained the door as re-entry path, kept compose lanes unchanged, and moved served-once memory to the connection.
- `s-tac-w3v`: earlier reconnect observation with rejected calls and duplicate orientation payload.
- `s-cpt-h31` and `d-tac-zm2`: identified and addressed reorientation after context loss while the same connection remains bound.
- `d-tac-wjq` and `s-tac-paq`: specified and shipped durable exclusive holders with MCP logical session identity, chooser TTLs, generations, and fencing.
- `s-tac-2sp`: open adjacent gap where agent-conversation continuity diverges from MCP connection ownership after branching.
