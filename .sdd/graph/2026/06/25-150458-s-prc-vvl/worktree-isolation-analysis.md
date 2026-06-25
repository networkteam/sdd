# Worktree isolation silently fails — session analysis

Consolidated from two concurrent implementation sessions that independently hit
the same failure, plus a third tooling instance found in the same work. The
entry body is the summary; this preserves the per-surface forensics.

## The core mechanism

Entering a worktree (`EnterWorktree`) moves only the **shell's working
directory** into the worktree. The worktree is a separate checkout — every file
has a distinct inode from its base-repo twin — but the switch does not rebase or
remap anything other than the shell CWD. So the same session ends up
simultaneously "in" the worktree (Bash) and "on" main (everything that resolves
paths independently of CWD), with nothing reconciling the two.

## Surface 1 — file-edit tools land on `main`

Read/Edit/Write take **absolute paths** and have no working directory of their
own. An agent that read base-repo files during grounding *before* entering the
worktree keeps handing those base absolute paths to Edit/Write:

- base: `…/sdd/cmd/sdd/main.go`
- worktree: `…/sdd/.claude/worktrees/oei/cmd/sdd/main.go`

The edits succeed — the base files exist — but they land in the **base checkout,
which is on `main`**, not in the worktree. Meanwhile shell commands (`git rm`,
grep, build) run in the worktree. The change tears in half: deletions/builds in
the worktree, edits on main, neither tree complete.

## Surface 2 — `devbox run` resolves to the base root

`devbox run` resolves to the base project root, not the active worktree. So
`devbox run build` / `devbox run validate-skills` operate on `main` even from a
worktree shell. The fix in-session was to bypass devbox and run the tools
directly in the worktree (`go build`, the validator by hand). Same root cause as
Surface 1: tooling resolves to base independently of the shell CWD.

## Why it's a structural trap, not a one-off slip

1. **No friction signals the mistake.** The wrong-tree path still resolves to a
   real file, so edits succeed silently — no error, no prompt. Paths read
   earlier are "sticky": they keep working while pointing at the wrong checkout.
2. **It defeats exactly what worktrees are for.** The failure mode is
   specifically "isolated work leaks onto the shared base/main" — the opposite
   of the isolation the worktree was created to provide.
3. **Verification gives false comfort.** `go test` / `go build` run from the
   worktree CWD compile the worktree's *unedited* source, so tests pass (they
   run old code, and any new tests aren't there to run). The only tell was a
   contradiction: green tests, but the freshly built binary reporting the
   feature missing. (LSP diagnostics were noise — the worktree isn't in gopls's
   workspace, so it emitted spurious "undefined" errors that masked real signal.)
4. **It collides with concurrent sessions.** The base/main working tree is
   shared. Two "isolated" sessions both leaking onto it pollute each other's
   `git status` and risk a wrong-narrative commit — the same hazard the existing
   "never `git add -A`" rule guards against.
5. **It reproduced in a concurrent session at the same time, on two distinct
   surfaces.** That is what moves this from "agent error" to "framework/harness
   gap" — any agent following the worktree playbook is exposed.

## How it surfaced

Caught by accident: a grep run in the worktree showed a file *unchanged* right
after a "successful" edit — i.e. the shell and the file-tool disagreed about
reality, and the disagreement happened to be visible. Without that cross-check
it stays invisible.

## Why current guidance didn't prevent it

The worktree note covers `direnv allow` and prerequisites; the implementation
playbook covers marker → enter → conclude. But nothing says *"after entering a
worktree, re-anchor file paths to the worktree; re-Read before editing; run
tooling from the worktree root."* Worse, the general "prefer absolute paths"
guidance actively points at the trap, because the absolute paths held from
before the switch are base paths.

## Candidate fix directions (to weigh later, not decided here)

- **Discipline / contract.** After `EnterWorktree`, treat any path read before
  the switch as stale — re-Read from the worktree path before editing, derive
  paths from the live CWD, and run tooling from the worktree root (`direnv
  allow` it).
- **Guard.** A hook that rejects `Edit`/`Write` whose target is outside the
  active worktree while a worktree session is live.
- **Harness-level.** `EnterWorktree` warns on, or rewrites, later edits and
  tooling invocations that resolve to the base checkout.

## The deeper read

This rhymes with the "structural enforcement transfers across model families,
instruction-carried discipline does not" finding (s-cpt-h5c). Trusting the agent
to remember the path/root swap is instruction-carried; a guard or harness remap
is structural. The gap sits inside the worktree division-of-labor established by
d-cpt-927 (harness owns enter/exit, git owns plumbing, the recipe lives in the
/sdd skill) — the missing re-anchor step belongs to that recipe, and the path
remap to the harness it relies on.
