# Exploration: engine graph authority during worktree implementation

## Observed failure

An engine-backed implementation can involve two independent filesystem contexts:

1. the checkout from which `sdd serve` was launched, which owns the engine's graph reads, graph mutations, session directory, and Git auto-commits;
2. the checkout or worktree in which the host agent edits and commits implementation artifacts.

The implementation procedure treats worktree creation and movement as host work, but creates the WIP marker, captures completion entries, and removes the marker through engine commands. It does not currently name, persist, display, or validate which checkout/branch is the graph authority for those commands.

The reported incident in another project had the WIP and done entries land on different branches. The current SDD repository shows a neighboring manifestation: the active marker and five slice-done entries are visible on `main`, while the cited implementation commits `941ec4a` through `62ddc92` are contained only by `worktree-x1m`, not by `main`. That run deliberately deferred the final merge, so this is not itself the same split, but it proves that engine graph state and artifact Git state can advance independently and that a done entry can claim “shipped” before its cited commit is reachable from the canonical branch.

## Code-path evidence

- `cmd/sdd/serve.go` resolves `GraphDir` and `SDDDir` once at process startup, constructs one handler around them, and gives the same fixed `GraphDir` to the MCP server.
- `internal/mcpserver/server.go` stores that directory on the server; registry command closures dispatch all entry and WIP writes through the fixed handler.
- `internal/handlers/handler_new_entry.go` writes entry files beneath the injected graph directory.
- `internal/git/git.go` runs `git add` and `git commit` from the server process working directory. The committer is not bound with `git -C` to the repository that owns the injected graph path, so an explicit graph path alone does not fully anchor the Git target.
- Project initialization generates Codex MCP configuration with `cwd = "."`. Current official Codex documentation says MCP `cwd` is the working directory used to start the server, and Codex-managed worktree tasks run in a distinct checkout. Therefore a project-local `sdd serve` may bind to the local checkout or to a managed worktree depending on how the task was started.
- The engine session records a logical project/session, but not a local checkout authority, symbolic branch, worktree identity, or graph-root fingerprint.

## Why logical project identity is insufficient

The active public project-aware runtime plan binds a session to a `ProjectRef`, using canonical `repo_id` when available. All worktrees of one repository intentionally share that logical identity. They do not share a working tree, index, HEAD, or branch. A project binding can therefore be correct while a filesystem/Git mutation lands in the wrong checkout.

The missing concept is the canonical mutation authority behind the logical project. For a hosted structured store this is naturally one store/revision. For the local filesystem adapter it must additionally identify the checkout/repository root and Git execution context that make the graph bytes canonical.

## Options considered

### A. Canonical graph authority, worktree only for implementation artifacts — recommended

Bind the session and every graph mutation to one explicit canonical project authority. A local adapter resolves or is configured with a canonical checkout/graph root; its Git finalizer is directory-bound to the same repository root. Host worktrees may change freely, but they never redirect engine reads or writes.

For an isolated implementation, WIP creation and removal both occur in that canonical graph. A completion entry should land only after the implementation artifact has reached the canonical landing state, or it must explicitly describe an unmerged intermediate state without claiming shipment.

Maintenance surface:
- one local canonical-authority resolver or machine-local mapping;
- a directory-bound Git adapter;
- authority metadata and validation in session start/resume/mutation gates;
- landing evidence for isolated implementations.

This aligns with the public `GraphStore` direction: the store is the canonical authority, while the host workspace is an external artifact surface. Remote/hosted adapters need no worktree knowledge.

### B. Follow the host into the implementation worktree

Restart or rebind the engine to the new worktree so all later graph writes follow the implementation branch. The initial WIP marker still needs base visibility, or coordination must move to a store outside the branch.

Maintenance surface:
- engine-session migration between server processes and session directories;
- explicit handoff of grounding, staged blobs, and mutation state;
- reconciliation of the marker created before entry with done/removal after entry;
- harness-specific worktree lifecycle integration.

This preserves atomic code-plus-graph branch merges, but conflicts with the current fixed server binding and opens the largest cross-harness protocol.

### C. Explicit dual-authority workflow

Treat base and worktree graphs as two intentional authorities: marker on base, done/removal on branch, followed by a graph-aware merge.

Maintenance surface:
- two revisions and two write paths per implementation;
- ancestry/content-equivalence rules;
- recovery for partially landed lifecycle mutations;
- branch-specific behavior in an otherwise host-neutral base procedure.

This makes the current skill recipe structural, but it is substantially heavier and does not translate naturally to non-Git hosted stores.

## Recommended invariant

One engine implementation instance has exactly one canonical graph mutation authority. Caller or host working-directory changes must never redirect it implicitly. Every graph mutation uses the same authority-bound store and finalizer, and resuming the session through a server bound to a different authority fails loud or requires an explicit, validated reorientation.

For local Git-backed projects:

- canonical graph root and Git repository root are resolved together;
- Git commands use an explicit directory, never ambient process cwd;
- the authority identity distinguishes logical repository identity from checkout/store identity;
- start/resume surfaces the binding for diagnostics;
- the mutation gate verifies that the authority is still the one the session bound to;
- isolated-run completion cannot claim canonical shipment until landing evidence says the cited artifact is reachable from the selected canonical branch/revision.

## Tests that would make the guarantee mechanical

1. Launch the server from the primary checkout, implement in a sibling worktree, and prove WIP, done, and marker removal all mutate the same authority.
2. Launch Codex/server from a managed worktree and prove it resolves the configured canonical authority rather than silently treating the detached checkout as canonical.
3. Resume a session through a server bound to another checkout of the same `repo_id`; require a typed authority-mismatch failure.
4. Pass an explicit graph directory while process cwd is another worktree; prove writes and Git commits target the graph-owning repository via directory-bound Git.
5. Switch the symbolic branch in the canonical checkout mid-session; require a fail-loud mutation gate rather than a write to the new branch.
6. Exercise two worktrees of the same repository and prove `ProjectRef` equality does not imply mutation-authority equality.
7. For isolated implementation closeout, reject or clearly classify a completion whose cited commit is not reachable from the declared canonical landing revision.
