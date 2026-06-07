# Transition to implementation

When the conversation reaches "let's build this".

## Scope check first

1. Enough decisions to scope the work? **Prefer reducing scope over building into the unknown** — narrow to what's decided rather than improvising a missing piece. If gaps remain, surface them: "Before building, we should decide X."
2. **Choosing between alternatives, not just executing a known path?** Capture the decision first — a done signal closing a gap describes *what was done*, not *why this approach*. Pre-flight blocks approach-shaped closures, strictly at higher layers.
3. **Needs decomposition (multiple requirements, design choices)?** Capture a plan decision first. Specific enough on its own → skip the plan.
4. Scope clear but multi-step? Capture any needed operational sub-decisions before building.

## Before any code: mode + marker

Two moves, every time you implement an entry. Not judgment calls — an empty marker list or recent commits straight to `main` are not permission to skip them.

**Ask the user how to run it.** You can't infer this — whether they want a parallel session is theirs to say. Recommend one for the scope, then let them pick:

- **in place** on `main` — contained changes
- **branch** — isolation, same directory
- **worktree** — isolation + separate dir; you run autonomously, the user confirms the merge
- **quick** — the user judges it too small to track; the only mode without a marker

**Create the WIP marker** — mandatory for every mode except *quick*:
```
sdd wip start <entry-id> --exclusive --participant <name> "<description>"
```
Add `--branch` for branch mode; in worktree mode create the marker on the base first, then enter the worktree (below).

## Implement, then close

Implementation stays in this session — the meta-process stays active.

1. **First** run `sdd show <entry-id> --downstream` — augmenting directives that ref the entry extend the acceptance contract (the implicit AC chain is the real spec). Required for plans, recommended for any non-trivial decision.
2. For a plan, work the union of its `## Acceptance criteria` and those downstream commitments as the checklist — each confirmed with evidence or its deviation explained.
3. Hit a design choice no decision covers? **Stop**, capture a done signal for progress so far (marker still active), and capture a signal for the missing decision — don't decide yourself. A narrow refinement instead → an augmenting directive (see Augment Plan Playbook).
4. Commit code first, then the done signal addressing every AC and augmenting commitment, closing them via `--closes <entry-id>,<dir-id>,...`. Then `sdd wip done <marker-id>`.
5. Prompt for evaluation signals — apply the lenses (see [evaluation.md](evaluation.md)).

## Branch mode

The CLI creates `sdd/<suffix>-<slug>` and checks out; the marker is committed on `main` first for visibility. `git merge main` regularly. Branch entries are invisible to `main` until merge — that is the isolation.

- **Conclude and keep** (chain or code worth keeping): commit, `git merge main`, resolve on the branch, close/supersede intermediates, present the landing evaluation (see [evaluation.md](evaluation.md)), `git checkout main` then `git merge <branch>`, closing done signal + any forward-looking signal on `main`, `sdd wip done <marker-id>`.
- **Discard** (nothing emerged): `git checkout main`, a summary signal if any, `sdd wip done <marker-id> --force`.

## Worktree mode

Isolated directory; the harness moves the session, git does the plumbing, `sdd wip` tracks the marker. The CLI owns none of it.

**Prerequisites** (once per repo): a `.worktreeinclude` listing gitignored state to carry (`.sdd/config.local.yaml`, `.sdd/index/`), `.claude/worktrees/` in `.gitignore`, and `worktree.baseRef: "head"` in `.claude/settings.json`.

**Start:** create the marker on the base first, so it is visible there — `sdd wip start <entry-id> --exclusive --participant <name> "<description>"` — then `EnterWorktree(name: "<entry-suffix>")`. The harness creates the worktree under `.claude/worktrees/` and switches the session in; branching from the base (`baseRef: head`) carries the marker along, and `.worktreeinclude` brings the gitignored local state (config + index).

**Work** inside the worktree — commit and capture the closing done signal here, on the branch. You don't need the user's approval step by step; the next time you involve them is the merge confirmation below, unless you hit a design choice no decision covers (stop rule above).

**Conclude** — present the landing evaluation first (see [evaluation.md](evaluation.md)); the user confirms the merge on that basis, and you stay in the worktree until they do. Then:
1. `ExitWorktree(action: "keep")` — back to base. Use `keep`, never `remove`: teardown is below, and `ExitWorktree` only removes worktrees it created itself.
2. `git pull --no-rebase` (merge, never rebase — a rebase rewrites base history and orphans the branch), then `git merge <branch>`. A real merge, not a squash.
3. `sdd wip done <marker-id>`.
4. `git worktree remove --force <path>` (path via `git worktree list`; `--force` because the `.worktreeinclude`'d gitignored state leaves the worktree non-clean), then `git branch -d <branch>`.

`EnterWorktree(name:)` creates a new worktree; `path:` re-enters one. Conclude in a single pass — a mid-conclude background rebase could rewrite base history.
