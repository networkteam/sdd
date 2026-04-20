# Draft plan: Branching and concurrent work in SDD

## Problem

SDD needs a model for concurrent and exploratory work — multiple sessions working in parallel, or a single session branching off to explore an uncertain direction without polluting the main graph until the exploration is resolved.

## Design

### Core mechanism: Git worktrees

Each concurrent work stream gets its own git worktree with a dedicated branch. Worktrees provide full working directory isolation — separate graph entries, separate code changes, separate build state.

### WIP markers on main for coordination

WIP markers are always committed to the **main branch**, not the feature branch. This ensures all participants can see what's in flight by running `sdd wip list` on main, regardless of which branches exist or which worktrees are active. No branch scanning needed for coordination.

### Conventions

- **Branch naming:** `sdd/<entry-suffix>-<title-slug>` (e.g. `sdd/s6w-preflight-validator`)
- **Worktree location:** `../[branch-name]` as sibling of the main repo (e.g. `../sdd-s6w-preflight-validator`)
- **Repo setup:** After creating the worktree, the skill checks CLAUDE.md for documented setup/init commands and runs them. If no setup documentation exists, the skill surfaces a warning: "This repo has no documented setup process — the worktree may need manual initialization."

### Lifecycle

**Start (on main):**
```
sdd wip start <entry-id> --branch --participant <name> "<description>"
```
- Creates WIP marker on main (committed)
- Creates branch with derived name
- Creates worktree at the sibling location
- Skill then runs repo setup from CLAUDE.md if documented

**Work (on branch, in worktree):**
- Graph entries (signals, sub-decisions, actions) created on the branch
- Code changes on the branch
- Normal SDD loop operates within the branch
- Entries are invisible to main until merge — that's the isolation property

**End — two playbook moves, agent recommends:**

The agent assesses the branch's content (number of entries, depth of reasoning, value of intermediate findings) and recommends one of:

#### Playbook move: "Conclude and keep"

For explorations that produced entries worth preserving — the reasoning chain has value for future traversal.

1. Merge branch to main
2. Revert all non-graph changes in a follow-up commit (only `docs/framework/graph/` entries survive on main)
3. Walk the entry chain — ensure all intermediate entries are properly closed or superseded (no open noise)
4. Capture closing action + forward-looking signal if insights emerged
5. `sdd wip done <marker-id>`
6. Clean up worktree and branch

#### Playbook move: "Discard"

For trivially shallow explorations where there's nothing worth preserving beyond a one-liner.

1. Capture summary signal on main with the key learning (if any)
2. `sdd wip done <marker-id>`
3. Delete branch and worktree without merging

### Ownership

- **Skill (playbook)** orchestrates: when to branch, repo setup conventions, end-of-branch decision, walking the closure chain. The skill holds context-dependent knowledge that varies per repo and situation.
- **Tool (CLI)** handles mechanics: creating branches/worktrees, WIP marker fields (branch name, worktree path), the `--branch` flag on `wip start`.

### Tool changes needed

- `sdd wip start` gains a `--branch` flag that creates branch + worktree and records both in the marker
- WIP marker format gains fields: `branch`, `worktree-path`
- `sdd wip done` optionally cleans up worktree (or skill handles this separately)

### Graph properties

- Entries on branches have unique IDs (timestamp + random suffix) — merge conflicts are structurally unlikely
- The only conflict risk: two branches closing/superseding the same entry. This is a real coordination concern, mitigated by WIP markers showing what each branch is working on
- Abandoned-but-kept entries follow normal graph semantics: closed entries don't appear in status/catch-up views

### Open questions

- Exact WIP marker fields: branch name only, or also worktree path? (Worktree path is derivable from branch name given the convention)
- Should `sdd wip start --branch` auto-derive the branch name from the entry, or should the skill provide it? (Skill providing it allows human-readable slugs)
- How does the skill detect missing setup docs? Check for a specific CLAUDE.md section? Any file matching a pattern?
