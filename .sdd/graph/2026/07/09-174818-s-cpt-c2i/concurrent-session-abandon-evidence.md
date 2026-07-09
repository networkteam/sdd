# Evidence: a concurrent connection abandoned an in-flight move in a live session

Source: `.sdd/sessions/s_20260709-161700-ed59582d.jsonl` (this session's own event log), 2026-07-09. Two `sdd serve` connections (this Claude Code session + a concurrent Codex session) operated the one session concurrently.

## The foreign relabel + abandon
54 ms after this connection served its own capture's summary-verification step, the Codex connection bound the same live session, relabeled it, started its own move, and abandoned it:

```
seq 22  16:43:30.370  served      i_2   this connection: d-cpt-xbx verifySummary
seq 15  16:43:30.424  labeled     -     "Cross-repo references release plan"      (Codex)
seq 16  16:43:30.424  started     i_3                                             (Codex)
seq 17  16:43:30.424  served      i_3                                             (Codex)
seq 18  16:43:51.534  abandoned   i_3   reason: "User asked for a fresh session,
                                          not continuation of the existing
                                          cross-repo release readiness thread."   (Codex)
seq 19  16:43:51.536  served      i_1                                             (Codex)
```

The abandon reason comes from the Codex agent's dialogue, not this session's.

## Concurrent-writer log corruption
The append-only log carries duplicate seq numbers 13–19 (two independent seq counters) and two distinct `i_3` instances in one session:
- Codex's `i_3`: started 16:43:30, abandoned 16:43:51.
- This connection's `i_3` (the concurrency-gap capture): started 16:51:45, parked 16:53:23 — intact.

## Harm scope
Committed graph entries were unaffected (immutability; this connection's captures completed durably). What was trampled is session-level: the session label was overwritten by a foreign connection, a foreign in-flight move was started and abandoned inside the session, and the event log's seq ordering and instance-id identity guarantees broke.
