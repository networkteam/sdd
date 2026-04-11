# Branching playbook

## When to suggest branching

- The work is exploratory or uncertain — the direction might be discarded
- Multi-participant project — other participants are active on main, and in-progress entries would create noise
- The scope is large enough that intermediate entries (sub-decisions, operational signals) would clutter main if the direction changes
- There's an active WIP marker from another participant on a related entry — branching avoids stepping on their work

**Don't branch for:** small confident changes, capturing signals/decisions from dialogue, solo work with no collaboration pressure.

## Branch mode (`--branch`)

For collaborative exploration where you don't need concurrent branches on the same machine.

**Start:** `sdd wip start <entry-id> --branch ...` — creates marker on main, creates branch and checks out to it. Same session, same directory.

**Work:** Normal SDD loop on the branch. `git merge main` regularly to stay synchronized with other participants' graph changes and WIP markers.

**Conclude and keep:**
1. Commit all work, `git merge main`, resolve conflicts on the branch
2. Walk entry chain — close/supersede intermediate entries that shouldn't be open after merge
3. Selectively revert unwanted non-graph changes via new commits
4. `git checkout main` then `git merge <branch>`
5. Capture closing action + forward-looking signal on main
6. `sdd wip done <marker-id>` — removes marker, deletes branch

**Discard:**
1. `git checkout main`
2. Capture summary signal on main (key learning if any)
3. `sdd wip done <marker-id> --force` — removes marker, force-deletes branch

## Worktree mode (optional, manual setup)

For multiple concurrent branches on the same machine — working on several branches and/or main simultaneously. Worktrees are a local environment concern, not managed by the CLI.

**Setup:** After creating a branch with `sdd wip start --branch`, manually create a worktree:
```bash
git worktree add ../<worktree-name> <branch-name>
```
Then start a new agent session in the worktree directory. Check CLAUDE.md for setup instructions (e.g., build binaries). A single worktree can be reused for different branches over time.

**Work:** New agent session in the worktree. Same SDD loop. `git merge main` regularly.

**Conclude/Discard:** Same playbook moves as branch mode, but coordinate across sessions. The main session handles the merge and `sdd wip done`. Clean up the worktree manually when no longer needed:
```bash
git worktree remove ../<worktree-name>
```

## End-of-branch assessment

The agent assesses the branch content and recommends one of:

- **Conclude and keep** when: the reasoning chain has value for future traversal (even if the conclusion is "this direction is wrong"), code changes are worth keeping, or multiple entries connect to the broader graph
- **Discard** when: the exploration was shallow (one or two low-value entries), nothing emerged beyond "tried it, didn't work," and the key takeaway fits in a single signal on main
