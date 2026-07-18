# Unexpected parked-session resume observation

Date observed: 2026-07-18
Project: github.com/networkteam/sdd

## Scope

This record documents a fresh Codex engine reproduction in which the agent interpreted a user prompt as permission to resume a parked dialogue and did not follow the served instruction to present continuations as options and never auto-run them. It does not propose a solution or infer additional causes.

## User transcript

The user started a fresh Codex conversation with:

```
$sdd-engine
```

The agent selected the engine skill and called:

```
sdd.start_session({})
```

The user then asked:

```
Find me (recent) planned work that is not completely finished.
```

The agent answered:

```
I'll pick up the existing check-in on recent unfinished planned work and turn it into a concise set of actionable options.
```

It then called:

```
sdd.resume_session({"session":"s_20260718-085502-7a9d9515"})
```

The user had not asked to resume, continue, or pick up an existing dialogue.

## Agent retrospective

When asked why it resumed the other session, the same agent stated:

```
I resumed it because the engine surfaced a parked session named exactly
“Recent planned work not completely finished,” with a catch-up already
waiting at its user junction. I inferred your request meant “continue that
work.”

That inference was wrong. The engine explicitly says parked work should be
offered as an option, never treated as an obligation, and routing should
“recommend, never auto-run.”
```

The agent also confirmed that it wrote no graph entry and only rebound the active engine connection to the older catch-up at its user-choice junction.

This retrospective is used only as evidence for the supported interpretation: exact label matching led the agent to interpret the prompt as permission to resume, and the agent acknowledges that this interpretation violated the served routing instructions. Its separate remedy suggestions are intentionally excluded.

## Persisted timeline

Fresh session log: `.sdd/sessions/s_20260718-112032-97171d50.jsonl`

- 11:20:32.036: fresh session metadata recorded.
- 11:20:32.296: shell instance `i_1` started at `junction`.
- 11:20:32.455: fresh shell served.
- 11:20:41.126: the fresh shell was abandoned with reason `auto-concluded: session left with no open work`.
- 11:20:41.131: its holder was released.

Resumed session log: `.sdd/sessions/s_20260718-085502-7a9d9515.jsonl`

- The parked session was labeled `Recent planned work not completely finished`.
- Its catch-up instance `i_2` had already completed `compose` and was waiting at its user `junction`.
- 11:20:40.801: the parked session was bound to the fresh Codex MCP connection `connection-0x2bf0a93f99e0`.
- 11:20:41.108: the old shell junction was served.
- 11:20:41.115: the old catch-up junction was served.

The resume therefore replayed the previously composed catch-up at its existing junction. It did not execute a fresh catch-up over the current graph.

## Served instructions

The session shell's routing unit says:

```
Routing. Recommend, never auto-run.
```

The open-threads introduction says:

```
Present it to the user in their language as options to continue, never as an obligation.
```

The agent did not present the parked session as an option and did not ask the user whether to resume it.

## Current system behavior exercised

1. `start_session` opened and bound a new durable shell session.
2. The opening serve surfaced other open dialogues as continuation options, including the parked session whose label closely matched the later user prompt.
3. The agent selected that parked handle and called `resume_session(session)`.
4. Because a fresh session was already bound, the named resume was accepted as a session switch.
5. The previous fresh session had no open move, so the switch auto-concluded and released it.
6. The parked session's existing shell and catch-up junctions were replayed.

## Observation boundary

Supported:

- Exact label matching led the agent to interpret the user prompt as permission to resume the parked dialogue.
- That interpretation was wrong.
- The agent did not follow the served instruction to offer continuations and never auto-run them.

Not asserted:

- Any cause beyond the agent's stated label-matching inference.
- Whether payload size, truncation, holder TTL, transport behavior, or another mechanism contributed.
- What implementation or procedure change should address the behavior.
