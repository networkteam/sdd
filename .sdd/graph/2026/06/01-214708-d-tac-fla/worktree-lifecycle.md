# Worktree lifecycle in `sdd wip`

## Design split
- **CLI owns the resource lifecycle**: worktree + branch creation, local-state
  copy, base-branch capture (start); merged-check, marker/branch/worktree
  teardown (done).
- **Agent + skill own the merge**: sync, build/test, conflict resolution,
  done-signal capture — where judgment and step-by-step visibility live.

## Assumption
Background sync pulls with merge, not `--rebase` (see the sync decision), so
pulls only add commits and never rewrite hashes. The content-based merged
check covers manual rebases regardless.

## End to end

### Start — `sdd wip start --worktree <plan-id>` (run from the base repo)
- record current branch as the **base branch** in the marker
- derive feature branch `sdd/<suffix>-<slug>`
- `git worktree add ../<repo>-<suffix>-<slug> -b sdd/<suffix>-<slug>`
- copy `.sdd/config.local.yaml` and `.sdd/index/` into the worktree
  (gitignored, so `git worktree add` does not carry them; without them the
  first search re-embeds everything)
- write the marker (feature branch + base branch)
- print the continuation choice

### Continue the session (user picks)
- `/add-dir ../<repo>-<suffix>-<slug>` — stay in this session (shell stays in
  base; less disruptive), or
- start a fresh session in the worktree dir

### Implement (agent, in the worktree)
Work and commits land on the feature branch. Background sync may pull base
updates via merge; ancestry stays intact.

### Conclude (agent-driven; user confirms the merge)
1. Finish work; build/test green in the worktree.
2. **Sync base in**: `git fetch && git merge <base>` — resolve conflicts here,
   re-test. Surfaces parallel work; gives the capture a complete graph.
3. **Capture the done signal in the worktree**:
   `sdd new s … --kind done --closes <plan-id>` — pre-flight + summary run
   against the synced graph; commits on the feature branch.
4. **`cd` back to the base worktree.**
5. **Merge into base**: `git merge <feature>` — usually a fast-forward,
   bringing work and closure together. If base advanced with conflicting
   writes, re-sync into the worktree and retry.
6. **`sdd wip done <marker-id>`** — cleanup only:
   - guard: current branch must equal the marker's base branch, else reject
   - content-based merged check (patch-equivalence)
   - remove marker, delete branch, re-derive worktree path via
     `git worktree list` and `git worktree remove`
