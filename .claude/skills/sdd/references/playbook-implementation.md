---
sdd-content-hash: 3b9ae8d957bc7c3aaf6a9e664f1ba24047240003ac1daab6633de0bbd0394149
sdd-version: dev
---
# Transition to implementation

When the conversation reaches "let's build this":

1. Check: are there enough decisions to scope the work? **Prefer reducing scope over building into the unknown** — if a portion of the work depends on a decision not yet made, narrow the scope to what's decided rather than improvising the missing piece.
2. If gaps exist, surface them: "Before building, we should decide X"
3. **Decision-before-done-signal checkpoint**: if the upcoming work requires making a choice between alternatives — not just executing a known path — stop and capture the decision first. A done signal that closes a gap directly should describe *what was done*, not *why this approach*. Approach-shaped closures smuggle decisions past the graph; pre-flight will flag them and strictly block at higher layers (strategic / conceptual).
4. Assess whether a plan decision is needed. The test: **will the closing done signal have enough to validate against without a plan?** If the decision is specific enough on its own (small fix, single change, obvious path from signal to completion), skip the plan. If the decision describes a direction but implementation requires decomposition (multiple requirements, design choices, multi-step scope), capture a plan decision first — the pre-flight validates every plan item at closing time, which is where the rigor pays off.
5. If scope is clear, capture any needed operational sub-decisions
6. Create an exclusive WIP marker for the entry being implemented (`sdd wip start <entry-id> --exclusive --participant <name> <description>`)
7. **Before starting implementation**, run `sdd show <entry-id> --downstream` to surface any augmenting directives that ref the entry. Treat their commitments as extensions of the original acceptance contract — every AC the original entry carries plus every commitment in a downstream augmenting directive is part of what the closing done signal must address. This is required for plan decisions and recommended for any non-trivial decision; the augmentation pattern (per `d-prc-9ti`) lets refinements accumulate without supersession, so the implicit AC chain is the real spec. See the Augment Plan Playbook below.
8. **If implementing a plan decision**, read its `## Acceptance criteria` section alongside the downstream commitments and use the union as your work checklist. Each AC and each downstream commitment is a contract item: the closing done signal must either confirm it done with specific evidence or explain the deviation with dialogue reasoning.
9. Implementation happens in the same session — the meta-process stays active
10. If you hit a design choice not covered by existing decisions: **stop implementation**, capture a done signal recording what was done so far with the WIP marker still active, and capture a signal for the missing decision. Don't make the choice yourself. If the choice is a narrow refinement of the existing plan rather than a missing decision, capture it as an augmenting directive instead (see the Augment Plan Playbook).
11. After implementation, commit the code changes first, then capture the done signal addressing each original AC and each augmenting directive's commitment. Close the original entry and any augmenting directives in the same done signal via `--closes <entry-id>,<dir1-id>,...`. Then remove the WIP marker (`sdd wip done <marker-id>`).
12. Prompt for evaluation signals

## Branching for isolated work

**When to suggest branching:**
- The work is exploratory or uncertain — the direction might be discarded
- Multi-participant project — other participants are active on main, and in-progress entries would create noise
- The scope is large enough that intermediate entries would clutter main if the direction changes
- There's an active WIP marker from another participant on a related entry — branching avoids collision

**Don't branch for:** small confident changes, capturing signals/decisions from dialogue, solo work with no collaboration pressure.

**Starting a branch:**
```
sdd wip start <entry-id> --branch --exclusive --participant <name> "<description>"
```
The CLI creates a git branch (`sdd/<suffix>-<slug>`) and checks out to it. The WIP marker is committed on main before the checkout for coordination visibility. Same session, same directory.

**Working on a branch:**
- Normal SDD loop — entries, code changes, all on the branch
- `git merge main` regularly to stay synchronized with other participants' graph changes and WIP markers
- Entries on the branch are invisible to main until merge — that's the isolation property

**Ending a branch — assess and recommend one of two moves:**

### "Conclude and keep"
Recommend when: the reasoning chain has value for future traversal (even if the conclusion is "this direction is wrong"), code changes are worth keeping, or multiple entries connect to the broader graph.

1. Commit all work, `git merge main`, resolve conflicts on the branch
2. Walk the entry chain — close/supersede intermediate entries that shouldn't be open after merge
3. Selectively revert unwanted non-graph changes via new commits
4. `git checkout main` then `git merge <branch>`
5. Capture closing done signal + forward-looking signal on main
6. `sdd wip done <marker-id>` — removes marker, deletes branch

### "Discard"
Recommend when: the exploration was shallow, nothing emerged beyond "tried it, didn't work," and the key takeaway fits in a single signal on main.

1. `git checkout main`
2. Capture summary signal on main (key learning if any)
3. `sdd wip done <marker-id> --force` — removes marker, force-deletes branch

## Worktree mode (optional)

Worktrees isolate concurrent work in a separate directory so edits in one session never touch another. The sdd CLI owns none of this: the agent harness moves the session, git does the branch plumbing, and `sdd wip` only tracks the marker. Two gates bound the flow — the user confirms once to start and once to merge; everything between runs without check-ins.

**When to suggest a worktree:** the work is more than a short-loop change — multi-file, multi-commit, or worth keeping the base branch free to use meanwhile. Don't suggest one for small confident changes or pure capture.

**Repo prerequisites** (set once per repo): a `.worktreeinclude` at the repo root listing the gitignored local state to carry into each worktree — for an sdd repo that is `.sdd/config.local.yaml` and `.sdd/index/`, so search works without re-embedding — plus `.claude/worktrees/` in `.gitignore` and `worktree.baseRef: "head"` in `.claude/settings.json` so worktrees branch from local state.

**Start — Gate 1, user confirms:**
1. `EnterWorktree(name: "<entry-suffix>")` — the harness creates the worktree under `.claude/worktrees/`, switches the session into it, and `.worktreeinclude` carries local state across (so `sdd search` works with no re-embed).
2. `sdd wip start <entry-id> --exclusive --participant <name> "<description>"` — records the marker on the new branch.

**Work:** the normal SDD loop inside the worktree — entries, code, commits. Capture the closing done signal here, on the branch. No check-ins until the work is ready.

**Conclude — Gate 2, user confirms the merge:** stay in the worktree until the user confirms. Then:
1. `ExitWorktree(action: "keep")` — returns the session to the base directory. Use `keep`, never `remove`: teardown is the steps below, and `ExitWorktree` only removes worktrees it created itself.
2. `git pull --no-rebase` to bring the base current by merge (never rebase — a rebase rewrites base history and can orphan the branch), then `git merge <branch>` into the base. Use a real merge, not a squash, so the branch tip stays an ancestor.
3. `sdd wip done <marker-id>` — removes the marker.
4. `git worktree remove <path>` (re-derive `<path>` via `git worktree list`), then `git branch -d <branch>` — safe to delete now that it is merged.

**Notes:**
- `EnterWorktree(name: …)` always creates a new worktree; pass `path:` instead to re-enter an existing one.
- Conclude in a single pass: background sync runs on a cooldown, and a mid-conclude rebase could rewrite base history.
