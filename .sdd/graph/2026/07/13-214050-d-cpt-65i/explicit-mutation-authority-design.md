# Explicit branch-targeted mutation authorities

## Decision

Every graph mutation is addressed to an explicit repository-and-branch authority. The MCP server's launch directory, ambient process working directory, and the agent shell's current directory are locators at most; none may select where a mutation lands.

## Branch roles

- An implementation procedure requires the agent to supply `baseBranch` explicitly.
- After execution mode is chosen, it requires `workBranch` explicitly.
- In-place execution means `workBranch == baseBranch`.
- Neither role assumes the literal name `main`.
- `defaultBranch` is a repository property. It is used only as the default for an ordinary capture when no branch is explicitly selected.
- Session replay persists branch roles. Resume never reconstructs them from cwd.

A mutation target is therefore repository/project identity plus branch. The storage adapter may also use an expected graph revision for optimistic concurrency, but that revision protects mutation application only.

## Local-project resolver

For a branch in the session's local project:

1. Query Git's registered worktree topology for the explicitly named branch.
2. Require exactly one checkout.
3. Resolve that checkout's graph directory from its repository configuration.
4. Execute filesystem writes and Git finalization directory-bound to that checkout.
5. If no registered checkout resolves, fail and retain procedure state so the host runtime can create or enter the correct branch/worktree.

SDD does not create an ephemeral substitute for a missing local implementation checkout. Claude Code, Codex, manual Git, or another host retains ownership of local branch/worktree lifecycle.

A server launched from the base checkout can therefore mutate a worktree branch, and a server launched from a worktree can mutate the base branch, without changing process cwd.

## Connected-project resolver

For an authorized connected repository:

1. Resolve the canonical clone URL and selected branch.
2. Create an ephemeral writable clone from the canonical URL.
3. Reuse read-cache objects only as clone acceleration, then dissociate the writable clone.
4. Verify the cloned repository's declared identity.
5. Apply and finalize the mutation against the selected branch.
6. Refresh or invalidate the read cache and remove the ephemeral clone.

The managed connected-repository cache remains strictly read-only. It is never an origin, checkout, or push destination for a mutation. No persistent writable-checkout registry is introduced.

Local and connected targets deliberately differ: local project work follows an existing host-owned workspace; connected capture has no such workspace and acquires a transient write checkout.

## Concurrency and evidence

If a remote branch advances during a connected write, refresh the target, reapply the immutable mutation, rerun target-graph validation, and retry a normal non-force push. Unique entry paths make Git conflicts unlikely, but semantic conflicts—such as concurrent retirement of the same entry or newly invalid references—must still fail validation.

Commit hashes cited in done signals remain ordinary agent-supplied evidence. Target routing must not turn them into code-HEAD, cleanliness, or ancestry checks. GraphStore expected revisions are storage-concurrency tokens, not certification of implementation state.

Protected branches, PR landing, and provider-specific fallback are deferred. A policy rejection fails loudly and never silently selects a different branch.

## Rejected alternatives

- **One canonical authority for an entire implementation:** rejected because the established workflow intentionally places coordination on the base branch and implementation evidence on the work branch.
- **Ambient cwd selects the graph:** rejected because long-lived MCP servers and agent worktree transitions have independent directory state.
- **Write through the connected read cache:** rejected because it destroys cache derivation semantics and couples pulls, writes, and pushes.
- **Persistent managed writable clones:** rejected for now because it makes SDD own durable repository lifecycle, locking, cleanup, and recovery.
- **Ephemeral local fallback checkout:** rejected because it can separate a graph entry from the host's actual implementation workspace and crosses the host-owned worktree boundary.
