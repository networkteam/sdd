# Failed newEntry write wipes the session binding — debugging record

Live incident during this dialogue session (s_20260719-092904-b584ae0f, Claude Code, 2026-07-19), debugged to root cause with the store, MCP logs, source, and a probe client.

## Timeline (times local, from Claude Code's MCP transport log)

1. **10:04** — capture confirm → `next` fails: `command "newEntry" at step write: newEntry: validation failed: dangling ref in refs: 20260719-015148-s-tac-khq (entry not found)` (legitimate rejection — the ref existed only on the worktree branch while the write targeted the default branch).
2. **11:50** — `next` (same instance, same handle) fails: `this connection is not attached to s_20260719-092904-b584ae0f — attach with resume_session`.
3. **11:50–11:56** — `resume_session` with the handle + userWords fails `sdd: invalid session ID ""`; retry identical; retry with `takeover:true` identical.
4. `list_sessions` works throughout and shows the session idle with the capture at step write.
5. `start_session` **reopens** the session — open threads correctly list `i_2: capture at write` — but the response carries `"session": ""`.
6. Recovery: user reconnects the sdd MCP server (`/mcp`); fresh process attaches normally; the capture resumes at its write gate and completes after seeding `captureBranch`.

## Root cause

`application/workflow_registry.go` (`runWorkflowNewEntry`):

```go
result, err := w.app.CreateEntry(w.ctx, w.identity, w.project, w.binding, draft)
w.binding = result.Binding        // unconditional — runs before the error check
...
if err != nil {
    return fmt.Errorf("newEntry: %w", err)
}
```

On failure `result` is the zero value, so the workflow's `SessionBinding` is wiped (SessionID "", Subject "", Version 0). All four sibling mutation commands guard this assignment — `replaceSummary` and `wipRemove` with `if err == nil`, `wipStart` and `wipDone` with an early `return err` — `newEntry` is the single unguarded site.

## Why each symptom follows

- `next`: `attachedSession` compares the bound root's ID (now `""`) with the handle → mismatch → "not attached", pointing at `resume_session`.
- `resume_session` (any variant): the connection still counts as attached, so the `StillHeld` pre-check runs first and calls `Sessions.Load("")` → `FilesystemSessionStore.filename` rejects the empty ID → `sdd: invalid session ID ""`. The named-attach path is never reached.
- `start_session`: the Reopen path works purely in memory (serves fine, threads listed) but stamps the response with the wiped ID — `"session": ""`.

## Evidence that isolated it

- Session file `.sdd/sessions/s_20260719-092904-b584ae0f.jsonl`: fully healthy — 15 lines, latest metadata carries correct ID, label, attachment stamp; all draft state and the staged attachment intact. The store was never corrupted.
- Probe: a scripted MCP stdio client against a **fresh** `sdd serve` process (same binary, same store) attached to the same session successfully (74,892-char resume payload). Only the live process's memory was poisoned.
- Running server verified as the worktree binary (built 01:52, post-Slice-5) via lsof.

## Severity notes

- The trigger is a *normal* event: any rejected capture write — dangling ref, validation failure — poisons the connection for all subsequent work tools.
- The failure mode is misleading twice over: the "not attached" error directs the agent to exactly the tool that then fails with a cryptic internal error, and nothing names the actual state.
- No tool-reachable recovery exists; only a transport reconnect (fresh process + re-attach) heals it.

## Fix direction

Guard the assignment like the siblings (`if err == nil { w.binding = result.Binding }`). Note: `CreateEntry` may have advanced the session store version before failing (durable intent bookkeeping) — a stale in-memory version is already handled by the append path's resync-and-retry, so keeping the old binding on error is safe. Add a regression test: failed `newEntry` (validation rejection) → subsequent `next`/`resume_session` on the same connection still work.
