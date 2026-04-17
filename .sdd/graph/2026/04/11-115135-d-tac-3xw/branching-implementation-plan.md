# Plan: Branching and concurrent work in SDD

## Problem

SDD needs a model for concurrent and exploratory work — multiple sessions working in parallel, or a single session branching off to explore an uncertain direction without polluting the main graph until the exploration is resolved.

## Design

### Core mechanism: Git worktrees

Each concurrent work stream gets its own git worktree with a dedicated branch. Worktrees provide full working directory isolation — separate graph entries, separate code changes, separate build state.

### WIP markers on main for coordination

WIP markers are always committed to the **main branch**, not the feature branch. This ensures all participants can see what's in flight by running `sdd wip list` on main, regardless of which branches exist or which worktrees are active. No branch scanning needed for coordination.

Branch workers stay synchronized by merging main into their branch regularly — this makes new WIP markers visible and prevents conflicts from diverging too far. This is the same git pull synchronization cadence needed for multi-participant collaboration generally (see s-prc-dk3).

### Conventions

- **Branch naming:** `sdd/<entry-suffix>-<description-slug>` — auto-derived by the CLI from the entry ID suffix and a slugified version of the WIP description
- **Worktree location:** `../<branch-name>` as sibling of the main repo (e.g. `../sdd-s6w-preflight-validator`)
- **Repo setup:** After creating the worktree, the skill checks CLAUDE.md for documented setup/init commands and runs them. If no setup documentation exists, that's fine — the worktree is vanilla and ready to use.

### Lifecycle

**Start (on main):**
```
sdd wip start <entry-id> --branch --exclusive --participant <name> "<description>"
```
- Creates WIP marker on main with `branch` field recording the derived branch name
- Creates git branch with derived name
- Creates worktree at the sibling location
- Commits marker on main
- Prints worktree path for the user
- Skill then checks CLAUDE.md for setup instructions and follows them

**Work (on branch, in worktree):**
- Graph entries (signals, sub-decisions, actions) created on the branch
- Code changes on the branch
- Normal SDD loop operates within the branch
- Entries are invisible to main until merge — that's the isolation property
- Regular merges from main into branch keep WIP markers visible and refs resolvable

**End — two playbook moves, agent recommends:**

The agent assesses the branch's content (number of entries, depth of reasoning, value of intermediate findings) and recommends one of:

#### Playbook move: "Conclude and keep"

For explorations that produced entries worth preserving — the reasoning chain has value for future traversal.

1. Merge main into branch (resolve any conflicts here, on the branch)
2. Walk the entry chain — ensure all intermediate entries are properly closed or superseded (no open noise on main after merge)
3. Selectively revert non-graph changes that shouldn't be kept via new commits. Code worth keeping stays — this is not binary "keep all graph, drop all code" but a deliberate per-commit assessment
4. Merge branch to main
5. Capture closing action + forward-looking signal on main
6. `sdd wip done <marker-id>` — removes marker, cleans up worktree and deletes branch (safe because it's merged)

#### Playbook move: "Discard"

For trivially shallow explorations where there's nothing worth preserving beyond a one-liner.

1. Capture summary signal on main with the key learning (if any)
2. `sdd wip done <marker-id> --force` — removes marker, cleans up worktree and force-deletes the unmerged branch

### Ownership

- **Skill (playbook)** orchestrates: when to branch, repo setup conventions, end-of-branch assessment, walking the closure chain, selective revert decisions. The skill holds context-dependent knowledge that varies per repo and situation.
- **Tool (CLI)** handles mechanics: creating branches/worktrees, WIP marker fields, branch cleanup on `wip done`.

### Graph properties

- Entries on branches have unique IDs (timestamp + random suffix) — merge conflicts are structurally unlikely
- The only conflict risk: two branches closing/superseding the same entry. This is a real coordination concern, mitigated by WIP markers showing what each branch is working on
- Abandoned-but-kept entries follow normal graph semantics: closed entries don't appear in status/catch-up views
- `sdd new` validation on branches resolves refs against the branch's graph state. Regular merges from main keep the branch's graph current enough for ref validation to work.

## Tool changes

### WIP marker struct

Add `Branch` field to `WIPMarker` struct and `wipFrontmatter`. Branch name only — worktree path is derivable from the naming convention.

```go
type wipFrontmatter struct {
    Entry       string `yaml:"entry"`
    Participant string `yaml:"participant"`
    Exclusive   bool   `yaml:"exclusive,omitempty"`
    Branch      string `yaml:"branch,omitempty"`
}
```

### `sdd wip start --branch`

New `--branch` boolean flag. When set:
1. Auto-derive branch name: `sdd/<suffix>-<slug>` from entry ID suffix and description
2. Create git branch from current HEAD (`git branch <name>`)
3. Create git worktree (`git worktree add <path> <name>`)
4. Record branch name in marker's `Branch` field
5. Commit marker on current branch
6. Print worktree path to stdout

### `sdd wip done` branch cleanup

When the marker has a `Branch` field:
- Check if branch is merged into current branch (`git branch --merged`)
- **Merged**: `git worktree remove <path>`, `git branch -d <name>` — safe cleanup
- **Not merged, no `--force`**: warn user that branch has unmerged changes, remove only the marker
- **Not merged, with `--force`**: `git worktree remove --force <path>`, `git branch -D <name>` — discard flow

### `sdd wip list`

When a marker has a `Branch` field, display it alongside the entry info.

## Verifiable implementation items

1. [ ] `WIPMarker` struct gains `Branch` field; `ParseWIPMarker` and `FormatWIPMarker` handle it; round-trip test passes
2. [ ] `sdd wip start --branch` creates branch, worktree, and marker with branch field; worktree exists at expected sibling path
3. [ ] `sdd wip list` shows branch name when present
4. [ ] `sdd wip done` on merged branch: removes marker, worktree, and branch
5. [ ] `sdd wip done` on unmerged branch without `--force`: removes marker only, warns about unmerged branch
6. [ ] `sdd wip done --force` on unmerged branch: removes marker, worktree, and branch
7. [ ] Skill playbook updated with branching lifecycle, conclude-and-keep, and discard playbook moves
8. [ ] End-to-end manual evaluation: start branch → create entries on branch → merge main in → conclude-and-keep → verify entries visible on main → verify cleanup
9. [ ] End-to-end manual evaluation: start branch → discard with --force → verify branch and worktree removed
