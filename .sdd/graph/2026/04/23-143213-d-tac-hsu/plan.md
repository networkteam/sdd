# Background sync awareness — design plan

Supersedes `d-tac-n6y`. Same goal (detect graph divergence and guide sync), sharpened safety contract.

## Alternatives considered

**Commit-message pattern — broad vs narrow match.** Two options surfaced:

- **Broad `^sdd:`** (chosen): matches every commit emitted by SDD handlers. Includes init/skill-refresh commits that do not change graph entries, but in practice those are still commits worth rebasing onto.
- **Narrow verb match** (`^sdd: (signal|decision|wip|rewrite|summarize|lint) `): pedantically entry-changing only. Rejected because every new graph-writing handler becomes a regex maintenance point, and the broad rule ages better.

Rationale: `sdd:` is CLI-controlled — all handlers emit it via `fmt.Sprintf("sdd: ...")` (see `handler_new_entry.go`, `handler_summarize.go`, `handler_wip_start.go`, `handler_wip_done.go`, `handler_lint_fix.go`, `handler_rewrite.go`, `handler_init.go`). False positives from hand-written commits are impossible by construction — hand-written commits in this repo use `docs:`, `cli:`, `init:`, `multilingual:`, etc.

**Capture as refinement signal vs supersede the plan.** Initially leaned toward a refinement signal (less ceremony). Reversed on reflection: adding AC6 (conflict prediction) is a material change to the plan's contract with pre-flight, and plans are validated AC-by-AC at close time, so new scope belongs in the AC list. Plan was medium-confidence and never implemented — supersede has no sunk cost.

**Failure-mode reporting — silent skip vs warn.** Initially proposed silent skip for "no remote / no upstream / not a git repo" to avoid noise. User pushed back: those states are genuinely rare in a normal SDD setup and should be flagged ("let it be inconvenient before fixing it"). Same reasoning applies to transient fetch failure — warn once per cooldown window rather than degrade silently.

## Conflict prediction — mechanics

When local and remote have diverged (both `git log HEAD..@{u}` and `@{u}..HEAD` non-empty), run:

```
git merge-tree --write-tree --merge-base=$(git merge-base HEAD @{u}) HEAD @{u}
```

Output:

- Exit 0 + tree OID on stdout → clean rebase predicted.
- Exit non-zero + conflict info on stdout → conflict predicted; parse paths for user-facing message.

Requires git ≥ 2.38 (Oct 2022). Fine for current dev setups; can detect and warn if older git is present, but not a blocking concern.

Fast-forward case (local-ahead = 0, remote-ahead > 0) skips the check — no conflict possible.

## slog output states

The CLI emits one of these per command invocation (subject to cooldown):

- `sync: fast-forward available, N commits behind` (info)
- `sync: rebase is clean, N remote / M local` (info)
- `sync: rebase would conflict in <paths>, N remote / M local` (warn)
- `sync: local ahead by N, consider push` (info)
- `sync: up to date` (debug)
- `sync: not a git repo` / `no upstream for branch X` / `fetch failed: <err>` (warn, at most once per cooldown)

The SDD skill pattern-matches on these lines and acts per AC5/AC6.

## Implementation notes

- Cooldown + last-fetch timestamp live under `.sdd/tmp/last-fetch` (infrastructure from `d-tac-s2g`).
- Default cooldown `15m` hard-coded; `.sdd/config.yaml` override only (no init prompt).
- `sdd init` explicitly exempt — bootstrap phase, remote may not exist yet.
- Sync check runs from a shared entry point invoked by each command's dispatch, not sprinkled per handler.
