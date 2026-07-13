# Implementation procedure: explicit branch roles and landing lifecycle

## Required state

- `baseBranch`: explicitly supplied by the agent; the branch that owns coordination and receives landed work.
- `workBranch`: explicitly supplied by the agent after mode selection; the branch that owns implementation-scoped graph captures.
- `mode`: in-place, branch, worktree, quick, hold, or abort under the existing chooser semantics.
- `wipMarker`: engine-written marker ID, created and removed through the base-branch authority.
- `doneEntry`: closing done entry captured through the work-branch authority.
- `landing`: pending or landed; host-attested procedure state, not Git-derived certification.

In-place mode requires `workBranch == baseBranch`. Neither branch is inferred from cwd, named `main` by convention, or reconstructed from the repository's default branch.

## Revised flow

### Establish base

Before WIP creation, collect `baseBranch` from the agent. Resolve it to a local registered checkout. A missing or ambiguous checkout blocks setup and preserves the move.

### Choose mode and establish work

The user chooses the existing execution mode. Tracked modes create the WIP marker through the base authority.

Before host work begins:

- in-place: collect/confirm `workBranch`, which must equal `baseBranch`;
- branch/worktree: the host creates or enters its checkout, then the agent reports `workBranch`;
- resolve `workBranch` to exactly one registered checkout;
- retain both branch roles in procedure state for replay and resume.

Quick continues to skip only the WIP marker. It still has an explicit capture target.

### Work and captures

The working loop remains responsible for progress notes and dialogue-first blocked decisions. Every implementation-scoped capture dispatched from this run uses the work authority unless the user deliberately selects another target (for example, a connected upstream capture).

Commit hashes cited in progress notes and done entries remain evidence. The engine does not verify code HEAD, cleanliness, or ancestry.

### Record before landing

Conclude dispatches the closing done capture to `workBranch`. The done body remains responsible for addressing the contract and citing evidence.

A resolving `doneEntry` advances to a landing junction. It does not call `wipDone`.

### Landing

The agent presents the landing read from the work branch and the user decides whether to merge. Merge/landing remains host work.

- If landing is deferred, keep the move at the landing junction and leave the WIP marker on `baseBranch`. The procedure is resumable.
- After host landing succeeds, the agent reports the explicit landed outcome.
- The engine then removes the WIP marker through `baseBranch` and advances to evaluation offer/finish.

The landing report is procedural evidence. No ancestry, code-SHA, or tree-equivalence gate is added.

## Current procedure corrections

The current procedure must no longer:

- create and remove WIP through one launch-bound handler;
- omit base/work branch state;
- dispatch done without a work target;
- route `doneEntry` directly to marker removal;
- describe landing confirmation before the done entry exists on the work branch.

## Resume behavior

On resume, serve the stored `baseBranch`, `workBranch`, mode, marker, progress notes, done entry, and landing state. Re-resolve each branch to its registered checkout before the next mutation. If resolution fails, reorient the agent to restore/enter the host-owned checkout and retry; never silently select another branch.
