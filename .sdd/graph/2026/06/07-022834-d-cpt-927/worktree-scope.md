# Worktree workflow scope — design note

## Decision
Worktree workflow for sdd = agent harness (`EnterWorktree`/`ExitWorktree`) + git + a process recipe shipped in the bundled `/sdd` skill. The CLI owns no worktree lifecycle.

## Why the CLI is out (structural, not ideological)
- A CLI subprocess cannot move its caller's working directory or switch the agent's session. The seamless "I'm now in the worktree" experience can ONLY come from the harness (`EnterWorktree`). The most a CLI could do is `git worktree add` then print "now switch to <path>" — the clunky step that motivated `EnterWorktree` in the first place.
- The remaining mechanics (worktree add, merge, `branch -d`) are generic git, available to any agent or human. Wrapping them in `sdd wip --worktree` adds surface area (base-branch field, guarded teardown, content-equivalence check, copy logic) for plumbing git already provides.
- Correction made during dialogue: a `sdd wip --worktree` command would NOT have violated agent-agnosticism — git is neutral. The Claude-Code-specific parts are the seamless ones (`EnterWorktree`, `.worktreeinclude`). d-stg-574 applies in the other direction: those Claude-specific conveniences belong in the agent-specific skill layer / repo files, not the neutral CLI.

## Three homes
- **Process recipe** (when to suggest, `EnterWorktree`/`ExitWorktree` usage, two-gate conclude) -> bundled `/sdd` skill (`internal/bundledskills/claude/`), shipped by `sdd init` to all repos.
- **The mechanic** -> harness (`EnterWorktree`) + git. CLI owns none.
- **Repo-specific setup** -> each repo's files (`.worktreeinclude`, `.gitignore`, `baseRef`).

## The session flow (recipe to encode in the skill)
1. Agent assesses scope on an implement request; if more than a short loop, suggests a worktree (GATE 1).
2. On yes: `EnterWorktree(name)` — Claude creates the worktree under `.claude/worktrees/`, `.worktreeinclude` carries state. `sdd wip start <entry>` records the marker on the new branch.
3. Implement autonomously; capture the done signal in the worktree.
4. STOP and ask to merge (GATE 2). Stay in the worktree until confirmed.
5. On yes: `ExitWorktree(keep)` -> base dir; sync base; `git merge <branch>`; `git branch -d`; `git worktree remove`; `sdd wip done`.

Use the `EnterWorktree(path)` form if the worktree was created outside the tool; `ExitWorktree(keep)` never removes a worktree it didn't create.

## .worktreeinclude (this repo)
Only gitignored files are copied; comments on their own lines.
- `bin/sdd` — run sdd immediately, no rebuild wait
- `.sdd/config.local.yaml` — participant identity
- `.sdd/index/` — avoid re-embedding (large, instant copy)
- `.devbox/` — optional; may speed shell startup, regenerates if stale

Skip: `.sdd/stats`, `.sdd/tmp`, `dist`, `.idea`, `*.DS_Store`. (`go.work` does not exist, so no Go-workspace file to carry.)
Also: add `.claude/worktrees/` to `.gitignore`; set `worktree.baseRef: "head"` so worktrees branch from local state, not just origin.

## Safety
With background sync switched to merge (not rebase), base history is never rewritten, so plain `git branch -d` (ancestry) is correct and the content-equivalence check is unnecessary. Caveat: use a real merge, not squash, at conclude. Trade-off accepted: the safety-critical merge/teardown rides on the agent following the skill recipe rather than a CLI guard — acceptable for a lightweight experiment; revisit if it proves unreliable.

## Rejected alternative
The CLI-guarded full build (d-tac-u07): WIPMarker base field, `sdd wip start/done --worktree` with copy logic and guarded teardown, content-equivalence BranchMerged. Tool-level guards are more reliable than agent-followed guidance (the original argument), but the surface area isn't justified for a graph tool when harness + git + skill already cover it. Retired in favor of minimal; revisit if minimal proves bad.

## Open follow-ups
- Set up this repo: add `.worktreeinclude`, the `.gitignore` line, `baseRef`, and the skill recipe; then close s-prc-8bp with a done signal.
- Realize the merge-pull half of d-tac-4ff (background sync: rebase -> merge) as its own work; note content-equivalence is now moot.
- Portability for OTHER sdd repos: the process recipe ships via the skill bundle (solved). Whether sdd should also provide agent-neutral provisioning of local context (e.g. seeding a fresh worktree's index from a sibling checkout instead of re-embedding) is a separate question — deferred until a second repo needs it.
