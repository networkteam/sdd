# Engine branch-target read friction after a capture write

## Observation

The new branch-targeted mutation surface correctly writes a capture to the
declared `captureBranch`, but the reads and guards immediately following that
write do not consistently resolve against the same branch. A tracked
worktree implementation can therefore write its closing done signal
successfully and then appear unable to find it until the work branch is
merged into the base branch.

## What happened

- Implementation instance `i_3` ran with base branch `main`, work branch
  `codex/sdd-view-help-0o7`, and worktree
  `/private/tmp/sdd-view-help-0o7`.
- Its dispatched capture instance `i_4` was explicitly seeded with
  `captureBranch: codex/sdd-view-help-0o7`.
- Confirming playback successfully created and auto-committed done signal
  `20260717-111938-s-tac-its` as commit `3a5ca31` on the work branch.
- The transition into `verifySummary` then failed with:

  ```text
  generatedSummary: entry 20260717-111938-s-tac-its not found
  ```

- The entry and generated summary were both present and readable from the
  worktree via `./bin/sdd show 20260717-111938-s-tac-its` and directly in the
  branch's graph file.
- `application/workflow_registry.go` explains the mismatch: `newEntry` and
  `replaceSummary` use the mutation target derived from `captureBranch`, while
  the `generatedSummary` query loads `w.graphs.Current()`, which is the
  session/base graph rather than the capture target's graph.
- After manually reading the stored summary from the worktree, sending the
  `verifySummary: faithful` answer directly completed the capture. The normal
  serve/resume path could not render that step because its summary injection
  failed first.
- Reporting the resulting `doneEntry` to implementation instance `i_3` hit the
  same shape of failure: `doneEntry does not resolve in the graph`. The done
  signal existed on `workBranch`, but the record gate resolved it against the
  base/session graph.

## Likely invariant to restore

Once a workflow write names a concrete branch target, every read, injection,
validation predicate, and follow-up mutation concerning the written artifact
should resolve through that same target until the workflow deliberately lands
back on the base branch.

At minimum this applies to:

1. capture's `generatedSummary` injection;
2. capture's `replaceSummary` path (already target-aware, useful as the model);
3. implementation's `doneEntry` resolution at the record gate;
4. resume/re-serve of a capture paused after a branch-targeted write.

## Suggested regression shape

Use distinct base and work graphs where the new entry exists only on the work
branch. Exercise the real tracked flow:

1. seed `captureBranch` with the work branch;
2. confirm capture and reach `verifySummary` without merging;
3. verify or replace the summary successfully;
4. report the produced entry as implementation's `doneEntry`;
5. reach the landing junction while the entry still exists only on
   `workBranch` and the WIP marker still exists on `baseBranch`.

The important assertion is that no step relies on the base checkout merely
because it is the session's current graph.
