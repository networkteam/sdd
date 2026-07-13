# Direct connected-graph capture and capture-upstream

## Target-aware capture primitive

Capture accepts a target authority in addition to its semantic entry fields:

- target repository/project;
- explicit target branch, or the target repository's resolved default branch when omitted;
- current write authorization resolved at the write gate.

The agent session keeps its originating/home project binding. Selecting a capture target does not migrate the session or change the target of other moves; the capture submove alone receives the connected authority.

Before drafting and playback, target-aware capture loads the target graph's:

- repository identity and branch snapshot;
- configuration and language;
- actors/participants;
- declared dependencies and authorized dependency snapshots;
- graph rules, pre-flight context, and existing IDs.

Playback names the repository and branch. Confirmation applies only to that destination.

Write authorization and reference authorization are independent. A principal allowed to mutate A still cannot write an A entry that references B unless A declares B as a dependency. The source session's home project, current cwd, dialogue origin, or write credentials never expand A's reference permissions. No dependency is added automatically.

Connected writes use the explicit mutation-authority resolver: an ephemeral canonical-remote clone accelerated by but dissociated from the strictly read-only cache. Ordinary capture defaults to the target repository's actual default branch, never the literal name main; an explicit branch overrides it.

## Guided capture-upstream procedure

Purpose: while working in downstream repository B, create a native work item in dependency A without changing agent sessions or transporting the dialogue manually.

### Route

1. Establish the originating/home project B and the proposed target A.
2. Require B to declare A as a dependency.
3. Confirm through dialogue that the discovered work is owned by A rather than B.
4. Resolve current write authorization and a writable target branch for A.

### Draft in target context

5. Enter target-aware capture with A as the mutation target.
6. Compose a self-contained native A entry from the dialogue.
7. Apply A's actors, rules, dependencies, and pre-flight.
8. Do not include an A→B graph reference unless A independently declares B as a dependency.
9. Playback names A and the branch; the normal user confirmation gate remains.

### Apply and optional provenance

10. Capture and land the A entry first.
11. Return the A entry ID to capture-upstream.
12. Decide whether the originating dialogue contains meaningful B-side reasoning worth retaining.
13. If yes, dispatch a normal capture into B whose legal B→A reference points to the new A entry.
14. If no, finish without a bookkeeping entry.
15. Return to the originating implementation, exploration, or dialogue move in B.

A-first ordering prevents a dangling B→A reference if the upstream write fails. The optional B capture is a separate mutation and confirmation; there is no distributed transaction. If it fails, the A entry remains valid and the procedure retains a retryable provenance step.

## Rejected alternatives

- **Manual handoff to another A session:** rejected as the default because it forces users to restate or transport dialogue and coordinate session ownership.
- **Write through A's read cache:** rejected; caches remain derived read state.
- **Automatic A→B provenance:** rejected because it can violate A's declared dependency direction.
- **Automatic B bookkeeping entry:** rejected; source-side capture occurs only when reasoning is meaningful enough to retain.
- **Session project migration:** rejected; the home session remains stable and mutation authority is scoped to the capture submove.
