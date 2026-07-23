# Spec: Session Branch Binding & Machine-Global Session Store

Resolves the two sibling gaps in one design pass:

- **20260719-124038-s-tac-cwx** — dialogue sessions carry no branch context; ordinary captures from a worktree default to `default_branch` and fail resolve-or-block on worktree-resident refs.
- **20260719-133247-s-tac-c37** — the session store is bound to the server's launch checkout (`<checkout>/.sdd/sessions/`), so a server launched in a worktree sees an empty session list.

The two mechanisms are orthogonal and separable: **store locality** decides where sessions are found; **branch binding** decides which live checkout a session's graph operations resolve against. They compose into the target arc: user starts at repo root → implementation begins → agent enters a worktree and declares the branch → every capture and read follows the worktree branch → agent leaves and clears the binding → everything reverts to `default_branch`. Sessions stay visible throughout, from either checkout.

## Goals & Requirements

1. A session carries an explicit, durable **branch binding**: set and cleared by explicit agent declaration, persisted in the session log, and consumed as the default target for the session's ordinary captures **and** reads until cleared.
2. Sessions for the same logical project are **discoverable from any checkout** (base or worktree): `list_sessions` / `resume_session` behave identically regardless of the server's launch directory.
3. Governing constraints (all pre-existing, none revisited except where stated):
   - **Never ambient** (d-cpt-65i): no launch directory or cwd ever selects a mutation destination. The binding is an explicit, session-logged declaration — this spec revisits only the directive's *ordinary-capture default clause* (`defaultBranch` as fallback), inserting the binding between explicit selection and the configured default.
   - **Host-owned worktrees** (d-cpt-h4l): sdd creates no checkouts; an unresolvable branch fails loud.
   - **Engine capability** (d-cpt-476): the declaration is an MCP tool, never a served CLI instruction.
   - **Hosted deployments** (d-cpt-9of, D3): the binding is optional and inert where no branch/checkout concept exists.
4. Out of scope:
   - Automated enter/leave detection or procedure-driven auto-binding (explicitly rejected by Christopher: the agent does the real worktree lifting, so procedures *instruct* the agent to declare — coupling automation to procedure steps is more complex and less explicit).
   - Behavior of non-session CLI commands (`sdd new`, `sdd view`, …) — they have no session and keep today's resolution.
   - Re-keying existing identity-less **index** stores in the cache (accepted one-time re-embed, see Decision 7).
   - Sweeping other checkouts' session files from one `sdd init` run (each checkout migrates what it holds, see Decision 8).

## Architecture & Design Decisions

### 1. The binding lives in `SessionMetadata`

**Decision**: Add one optional field to `SessionMetadata` (`application/sessionstore.go:11-23`):

```go
// Branch is the session's declared branch binding: the branch that ordinary
// captures and session reads resolve against until cleared. Empty = unbound
// (the configured default_branch applies). Local-composition concern only —
// deployments without a branch concept never set it.
Branch string `json:"branch,omitempty"`
```

Set/cleared via the existing CAS `Append` (per-session flock + version check, `local/local_sessionstore.go:189-229`), carrying both the updated metadata and a typed event (`branchBound` / `branchCleared` with the branch in the payload) so the change records its specific cause (session-model invariant I7: status derivable from the store alone).

**Reasoning**: The gap itself fixes the scope — "session-level state … deliberately not a contained per-procedure state", outliving any procedure so ordinary captures after implementation finishes stay bound. `SessionMetadata` is the only durable session-level home; procedure `Store` fields (`captureBranch` et al., `internal/engine/store.go`) are per-instance and die with the move. Metadata mutability is compatible: only ID/Subject/Project are immutable across appends (`local/local_sessionstore.go:192-195`).

**Compatibility**: absent field = unbound. An older binary appending metadata to a newer session would drop the binding; accepted under the committed in-place format-evolution policy (d-cpt-vri) — same machine, transient mixed-generation window, and the failure mode is "reverts to default branch, next write fails loud on a dangling ref", not silent mis-targeting to a wrong branch.

**Projection into serves (required by d-cpt-0tm)**: the binding is durable, intent-bearing session state, so it must be agent-visible on every re-entry — persistence without projection is not continuity. The session info block carried by `start_session`/`resume_session` serves names the bound branch when set (e.g. `branch binding: worktree-x`), and `list_sessions` rows include it. Without this, an agent resuming a bound session (or recovering from context loss) would capture into a branch it does not know about — and the capture playback line (Decision 5) could not state the true target.

### 2. Declaration seam: a dedicated `bind_branch` MCP tool

**Decision**: One new session-handle-required tool in `mcpapp/tools.go`:

```go
type BindBranchArgs struct {
    Session string `json:"session"`          // required, attached handle
    Branch  string `json:"branch,omitempty"` // set the binding to this branch
    Clear   bool   `json:"clear,omitempty"`  // clear the binding
}
// exactly one of Branch / Clear must be provided; anything else is a tool error
type BindBranchResult struct {
    Branch string `json:"branch,omitempty"` // the now-effective binding ("" after clear)
    Status string `json:"status"`
}
```

Setting validates **at declare time** that the branch resolves under the same rule writes enforce — exactly one registered Git checkout has it checked out (a resolve-only variant of `GitWorktreeAcquirer.Acquire`, `local/git_target.go:46-77`) — and fails loud otherwise, so a typo or a not-yet-created worktree surfaces at declaration, not at the first capture. `Clear` needs no validation. In a deployment without local Git acquisition (hosted), the tool returns a clean typed error ("this deployment has no branch concept") and the field stays empty.

**Reasoning**: User decision (Christopher): dedicated tool, agent-invoked, no automated enter/leave coupling — "the agent has to do the real worktree / branch lifting", so the procedure instructs the agent to call the tool (Decision 5). Alternatives rejected: parameters on `start_session`/`resume_session` conflate transport attach with a host-fact declaration and offer no mid-session leave signal; a procedure transition cannot own state that must outlive the procedure. Precedent for the shape: session-handle-required work tools with attached-connection gating (`mcpapp/tools.go:380-389` `attachedSession`).

### 3. Ordinary captures target the session's effective branch; base/work roles stay explicit with no default at all

**Decision**: A session has exactly one *effective branch*: the binding when set, else the configured `default_branch`. This is not a fallback stack — it is the definition of where the session's ordinary work lives, with one explicit override:

- **Ordinary captures** (and `replaceSummary`) target the session's effective branch. A procedure that explicitly seeded `captureBranch` (e.g. implementation seeding `captureBranch: workBranch`) overrides it — an explicit instruction always beats the session default. Implemented in `WorkflowSession.mutationTarget(store, field)` (`application/workflow_registry.go:278-284`) for `captureBranch`-scoped targets only.
- **`baseBranch` / `workBranch` have no default and gain none.** The agent reports both explicitly in the implementation procedure (they may be equal at the start, for in-place work); the binding never feeds them, and neither does `default_branch`. **Tightening (new)**: the implementation procedure's WIP writes (`workflow_registry.go:171,190`), which target `baseBranch`, today silently fall through to `default_branch` when the field is empty — that fallthrough becomes a loud error (`WIP write requires an explicit baseBranch`). The procedure always collects `baseBranch` before the marker step, so a correct run never hits this; it exists so a broken run fails instead of quietly writing a marker to the configured default. This aligns the code with d-cpt-65i's own wording, which reserves the `default_branch` fallback for *ordinary captures* only. (Groom's stale-marker removal deliberately passes a zero target, `workflow_registry.go:209`, and is unchanged.)

**Example**: session bound to `worktree-session-model`. A capture from plain dialogue targets `worktree-session-model` — the live-friction case fixed. The same session running implementation with `baseBranch: main` writes its WIP marker to `main`; if `baseBranch` were somehow empty, the marker write now errors instead of landing on `main` by accident.

**Reasoning**: The gap: implementation roles "would consume it as their default rather than replace it" — realized as a prose suggestion at the report step (Decision 5), never engine-side adoption. Excluding `baseBranch` from the session default is required by d-prc-pop ("routing WIP marker creation to the base authority") — a session bound to the work branch must not pull markers onto it. Drift safety is live at every write: `GitWorktreeAcquirer.Acquire` fails loud when the effective branch no longer resolves to exactly one checkout (`local/git_target.go:58-68`), covering the forgotten-leave and server-outlives-switch scenarios (s-prc-akz). This is the single deliberate revision of d-cpt-65i's ordinary-capture default clause; the never-ambient rule is untouched (every binding change is an explicit, session-logged declaration).

### 4. Reads follow the binding

**Decision**: Two seams:

1. **Procedure-context reads**: `workflowGraphs.CurrentFor` (`application/workflow.go:941-994`) extends its `workflowReadTargetFields` scan (`captureBranch`, `workBranch`) with a final fallback to the session binding before resolving the default graph — serves, chains, and ref-validation snapshots inside a bound session see the bound branch's graph.
2. **Free reads on an attached connection**: `ViewRequest` / `ShowRequest` / `SearchRequest` gain an optional `Branch string` field. `Application.View/Show/Search` (`application/application.go:81,153,195`) resolve their local snapshot through the branch-target acquisition path (same seam as `snapshotMutationTarget`, `application/write_api.go:325-336`) when `Branch` is set, instead of `runtime.options.Graph.Current`. The MCP handlers (`mcpapp/tools.go:834,865,995`) populate it from the attached session's binding when present; unattached connections and unbound sessions are unchanged. Connected-dependency reads are unaffected (dependencies have no branch concept here).

**Reasoning**: User decision (recommended option): writes-only would leave the agent referencing entries in dialogue it cannot see — the exact asymmetry behind the live rejections. Vector search composes without new machinery: `runtime.searchSnapshot(ctx, snapshot, q)` already searches an arbitrary snapshot (`application/application.go:212-214`), so a branch snapshot slots in where the default snapshot did.

### 5. Procedure interplay is prose, not plumbing

**Decision**: In-place edits to two embedded base-procedure specs (established practice — `d-prc-imp.md` has in-place revision commits):

- `internal/baseprocedures/entries/20260706-170000-d-prc-imp.md`: the worktree run-mode instructions direct the agent to declare the work branch as the session binding (via the binding capability) right after the host enters the worktree, and to clear it at closeout after the host reports landing. The `workTarget` step names the current session binding as the natural candidate for `workBranch` — but the explicit report stays required (d-cpt-65i: "the agent must supply `baseBranch` and `workBranch` explicitly"); the engine never silently adopts the binding into these roles.
- `internal/baseprocedures/entries/20260703-094500-d-prc-cap.md`: the playback line (target branch = `captureBranch` else configured default, `:118`) adds the middle case — else the session's bound branch, else the default — so the user verifies the actual target during playback.

**Reasoning**: User decision — explicit over automated. Host-neutrality (d-cpt-476) holds: the served instruction points at an engine capability (the tool), not a host command. Automatic adoption into `baseBranch`/`workBranch` would weaken the explicit-supply clause of d-cpt-65i for the two fields where it matters most.

### 6. Sessions move to a machine-global store under the XDG **state** root

**Decision**: Session files and staged blobs relocate to a machine-global per-project store:

```
$XDG_STATE_HOME/sdd/                     (default ~/.local/state/sdd)
├── sessions/<repo-key>/<sessionID>.jsonl
└── staged-blobs/<repo-key>/...
```

`repos.Locations` (`internal/repos/config.go:42-61`) gains `StateRoot` resolved from `$XDG_STATE_HOME` (default `~/.local/state/sdd`), alongside the existing `CacheRoot`. Resolution follows the existing `DefaultLocations` style: the XDG convention directly, uniform across platforms (no macOS `~/Library` special-casing). Concretely, on a Mac with no XDG variables set, this repo's sessions live at:

```
/Users/<user>/.local/state/sdd/sessions/github.com/networkteam/sdd/<sessionID>.jsonl
```

`<repo-key>` comes from `index.RepoKey` (Decision 7). `cmd/sdd/serve.go:231-235`, `cmd/sdd/recover.go:127-151`, and the init migration path all resolve these two directories through **one shared helper** so they can never diverge.

**Local composition only**: everything in Decisions 6–8 — the state root, repo keys, relocation, and leftover notices — lives in the local adapters and the `cmd/sdd` composition root (`buildLocalApplication` wiring `FilesystemSessionStore`). A hosted deployment wires its own `SessionStore` implementation against `application/sessionstore.go` and is untouched by all of it; the only session-model change it sees is the optional `Branch` metadata field, which stays empty there.

**Deliberate divergence from the index precedent, flagged for review**: c37's endorsed direction says "keyed by repo identity the way the index store is keyed", and the index lives under the **cache** root (`~/.cache/sdd/index/...`). Sessions follow the keying but not the location: XDG defines cache as data the user may delete without losing anything — true for the index (re-embeddable), false for sessions (dialogue history, pending recovery, parked work are not derivable). Putting sessions in cache would make `rm -rf ~/.cache` destroy them silently. `XDG_STATE_HOME` is the spec-defined home for exactly this class ("state data that should persist between restarts, but is not important or portable enough for data"). Grounding: XDG Base Directory Specification; the placement directive's own principle (d-cpt-6cq, "placed by whose fact it is") classifies sessions as per-machine operational state, not derived cache.

Staged blobs move with sessions: they are session-scoped scratch keyed by `BlobOwner{Subject, Session}` (`local/local_blobstore.go`), consumed only during a live session, and the legacy migrator already couples the two stores (`local/local_sessionmigration.go:135-165`) — leaving them in-tree would strand a session's attachments when resumed from a sibling worktree.

### 7. `RepoKey` inputs resolve through the Git common dir — for sessions *and* the index

**Decision**: Keep `index.RepoKey(repoID, repoRoot)` (`internal/index/store.go:38-44`) unchanged in shape — declared `repo_id` when present, else `local/ + sha256(absRoot)[:12]` — but route **every call site** through a new helper before hashing:

```go
// StableRepoRoot resolves the root that is invariant across a repository's
// worktrees: the main working tree (parent of the Git common dir). Non-Git
// directories fall back to the directory itself.
func StableRepoRoot(dir string) string  // internal/git
```

implemented via `git -C <dir> rev-parse --path-format=absolute --git-common-dir` → `filepath.Dir` of the result; on any error, return `dir` unchanged. The one production call site today is `cmd/sdd/serve.go:271-273` (`index.RepoKey(repoIDOf(cfg), filepath.Dir(sddDir))`); it and the new session-store resolution both pass `StableRepoRoot(filepath.Dir(sddDir))`.

**Reasoning**: User decision ("fix both at once"). The index precedent hashes the *launch checkout's* root, which differs per worktree — replicating it verbatim would give identity-less repos per-worktree session silos, reproducing the exact c37 bug for that repo class. Fixing only sessions would leave two subtly different keys for the same repo in the same tool. Repos with a committed `repo_id` (the normal case, auto-derived at `sdd init`) are unaffected — their key never touches the root hash.

**Consequence, accepted — spelled out**: a repo *without* `repo_id` gets its store key by hashing a folder path. Today that input is the launch checkout's folder; after this change it is the main worktree's folder. Different input → different hash → different key. So for an identity-less repo that already has an embedded index in `~/.cache/sdd/index/`, the next `sdd` run looks under the *new* key, finds nothing, and embeds the graph again from scratch (one-time cost: embedding API calls and a slower first search). The old index directory is not renamed to the new key — no migration code is built — because the index already has this exact policy for a *moved* identity-less repo ("re-embeds — accepted", `internal/index/store.go` docs), and the abandoned directory is ordinary garbage for the existing index GC. Repos *with* a committed `repo_id` — including this one — never touch the path hash: their key is the `repo_id` string, so nothing re-embeds and no session store moves keys.

### 8. Migration runs on `sdd init` only; leftovers surface as a visible served notice, not a startup failure

**Decision**:

- `sdd init` (as part of its idempotent refresh, no new flag) relocates the launch checkout's session store: every `<checkout>/.sdd/sessions/*.jsonl` (current **and** legacy format — relocation is format-agnostic) and the `<checkout>/.sdd/staged-blobs/` tree move to the machine-global store, **move-if-absent** per file following the `index.MigrateDir` pattern (`internal/index/store.go:82-119`): rename, cross-device copy+swap fallback, never clobber. A name collision (same session ID already in the global store) skips the file and reports it loudly in the init summary — no silent drops. Emptied source dirs are removed.
- `--migrate-sessions` (format conversion, `cmd/sdd/main.go:1647-1730`) is rebuilt against the machine-global directory — relocation first, then format migration finds everything at the new path.
- `.sdd/sessions-legacy/` is a manual user backup, not a code artifact — ignored by migration.
- `sdd serve` performs **no migration** and **does not refuse to start**. A failed MCP server startup is invisible to the user — the host shows "server failed to connect" with no message surfaced — so blocking would hide the very information it means to deliver. Instead, serve detects leftover `<checkout>/.sdd/sessions/*.jsonl` at startup and (a) logs a `slog` warning, and (b) carries a **standing relocation notice** that is served visibly where the agent and user actually look: in the `start_session`/`resume_session` framing, in the `list_sessions` result, and in `info`. The notice names the leftover directory and says to run `sdd init`; it repeats on every serve until the directory is empty. Precedent: pending-recovery notices already render into served results this way (`renderRecoveryNotices`, surfaced in view/framing output). The engine keeps operating normally against the global store — nothing is blocked, nothing is silent.

**Reasoning**: User decision (`sdd init` only for the migration itself), with the surfacing shape corrected by Christopher's UX point: a startup failure has no visible error channel in MCP hosts, so the loud signal must ride the responses that do reach the user. This keeps the fail-loud rule's substance — the condition is impossible to miss — without the self-defeating delivery. Per-worktree fragments (each checkout that ever ran a server has its own `.sdd/sessions/`) merge as each checkout gets an `sdd init`; that is deliberately per-checkout best-effort, since enumerating sibling worktrees from one run would guess at directories the user didn't point sdd at — and each such checkout announces itself via its own served notice.

### 9. Drift protection: validate at declare time and write time, nothing speculative

**Decision**: Two validation points, both against live Git state; no cross-checks against other store state:

- **Declare time** (Decision 2): the branch must resolve to exactly one registered checkout.
- **Write/read time** (existing, kept): `GitWorktreeAcquirer.Acquire` re-validates on every acquisition (`local/git_target.go:53-68`). New requirement: when the failing target came from the session binding (not an explicit procedure field), the error message must say so — e.g. `session is bound to branch "X", which no longer resolves to a checkout — re-declare the binding or clear it` — so the agent's recovery move is obvious.

Rejected: cross-validating the binding against WIP-marker base branches or other session state. The marker's base branch is *legitimately* different from the binding (Decision 3), and any such check would couple session metadata to procedure internals for no added safety — Git resolution already catches every real drift scenario s-prc-akz enumerates (forgotten leave → worktree removed → acquisition fails; server outlives a branch switch → HEAD check fails).

## Implementation Changes

### `application/`

- `sessionstore.go` — add `Branch` to `SessionMetadata` (Decision 1).
- `session_runtime.go` / `workflow.go` — new `WorkflowSession.BindBranch(ctx, branch string, clear bool) error`: validates via the resolve-only seam, CAS-appends metadata + `branchBound`/`branchCleared` event, updates the in-memory session copy. Follows the `Append` usage pattern at `application/workflow.go:188-193`.
- `runtime.go` — add a resolve-only branch validation option alongside the existing acquire seam (wired from `local`).
- `workflow_registry.go` — `mutationTarget` fallback to session binding for `captureBranch`-scoped calls only (`:278-284`); `baseBranch` call sites (`:171,190`) instead gain the empty-field loud error (Decision 3 tightening); groom's zero-target `wipRemove` (`:209`) untouched.
- `workflow.go` — `readTarget`/`CurrentFor` fallback to the session binding after the `workflowReadTargetFields` scan (`:985-994`).
- `application.go` / `read_api.go` — optional `Branch` on `ViewRequest`/`ShowRequest`/`SearchRequest`; branch-snapshot resolution shared with `snapshotMutationTarget` (`write_api.go:325-336`).
- `write_api.go` — target provenance so acquisition errors can name the session binding (Decision 9).

### `mcpapp/`

- `tools.go` — register `bind_branch` (args/result structs above; `attachedSession` gate; exactly-one-of validation); populate the read requests' `Branch` from the attached session's binding in `search`/`view`/`show` handlers (`:834,865,995`).
- `server.go` / `tools.go` — accept an optional standing relocation notice from the composition root and render it into `start_session`/`resume_session` framing, `list_sessions`, and `info` results (Decision 8).
- `tools.go` — project the bound branch into the session info block of `start_session`/`resume_session` serves and into `list_sessions` rows (Decision 1 projection).

### `local/`

- `git_target.go` — extract the branch→checkout resolution from `Acquire` into a resolve-only method used by both `Acquire` and the new declare-time validation.
- new `local_sessionrelocate.go` — `RelocateSessionStore(srcDir, dstDir) (moved, skipped []string, err error)` for sessions and staged blobs, move-if-absent per file (pattern: `internal/index/store.go:82-119`).

### `internal/git/`

- new `StableRepoRoot(dir string) string` (Decision 7).

### `internal/repos/`

- `config.go` — `Locations.StateRoot` from `$XDG_STATE_HOME` (default `~/.local/state/sdd`), in `DefaultLocations`.

### `cmd/sdd/`

- shared helper (e.g. `cmd/sdd/serve.go` or a small `storepaths.go`) resolving `(sessionsDir, stagedBlobsDir)` from `StateRoot` + `RepoKey(repoIDOf(cfg), StableRepoRoot(repoRoot))` — used by `serve.go:231-235`, `recover.go:127-151`, and init.
- `serve.go` — detect leftover in-tree sessions at startup, log a `slog` warning, and pass the standing notice into the MCP server options (Decision 8); route `baseRepoKey` (`:271-273`) through `StableRepoRoot`; wire the resolve-only validation into the application options.
- `main.go` (init path) — relocation step before skill refresh; `--migrate-sessions` (`:1647-1730`) targets the global dir.

### `internal/baseprocedures/entries/`

- `20260706-170000-d-prc-imp.md` — worktree run mode: declare binding after enter, clear at closeout; `workTarget` prose names the binding as candidate default (explicit report still required).
- `20260703-094500-d-prc-cap.md` — playback target-branch line gains the session-binding middle case.

## Test Cases

### `internal/git` — `StableRepoRoot`

| Test | Setup | Action | Expected |
|---|---|---|---|
| main worktree | temp git repo | `StableRepoRoot(root)` | `root` |
| linked worktree | repo + `git worktree add` | `StableRepoRoot(worktreeDir)` | main repo root |
| non-git dir | plain temp dir | `StableRepoRoot(dir)` | `dir`, no error |

### `internal/index/store_test.go` — extend

| Test | Setup | Action | Expected |
|---|---|---|---|
| repo_id key unaffected | declared repo_id | `RepoKey(id, anyRoot)` | key = repo_id (existing behavior holds) |
| identity-less worktree invariance | repo + worktree, no repo_id | `RepoKey("", StableRepoRoot(base))` vs `RepoKey("", StableRepoRoot(wt))` | identical keys |

### `local/` — session store + relocation (extend `local_adapters_test.go`, new relocate tests)

| Test | Setup | Action | Expected |
|---|---|---|---|
| metadata Branch roundtrip | session with `Branch` set | Append, reload | binding persisted; older sessions load with `Branch == ""` |
| relocate move-if-absent | src with 2 sessions, dst empty | `RelocateSessionStore` | both moved, src emptied |
| relocate never clobbers | same session ID in src and dst | `RelocateSessionStore` | dst untouched, file reported in `skipped` |
| legacy format relocates as-is | legacy-format jsonl in src | `RelocateSessionStore` | moved unconverted; later format migration succeeds at new path |
| staged blobs move | blobs tree in src | relocate | tree present at dst, coupled with sessions |
| resolve-only validation | repo with branch in exactly one worktree / zero / two | resolve | ok / loud error / loud error (same rule as `Acquire`) |

### `application/` — binding behavior (extend `workflow_branch_target_integration_test.go`, `workflow_target_graph_test.go`, `session_runtime_test.go`)

| Test | Setup | Action | Expected |
|---|---|---|---|
| bind + clear | attached session | `BindBranch("wt", false)` then `BindBranch("", true)` | metadata Branch set then empty; `branchBound`/`branchCleared` events in log; CAS version advances |
| bind validation failure | branch resolves nowhere | `BindBranch` | error; metadata unchanged |
| capture falls back to binding | binding set, no `captureBranch` state | ordinary capture write | target branch = binding |
| explicit field wins | binding set, `captureBranch` seeded | capture write | target = `captureBranch` |
| WIP ignores binding | binding = "wt", `baseBranch` = "main" | WIP create | marker lands on "main" |
| WIP without baseBranch fails loud | binding set or unset, `baseBranch` empty | WIP create | loud error, no marker written anywhere |
| unbound falls to default | no binding, no field | capture write | target = `default_branch` |
| reads follow binding | binding set, entry exists only on bound branch | `CurrentFor` read / ref validation | branch snapshot used; worktree-resident ref resolves |
| binding drift fails loud | binding set, worktree removed | capture write | acquisition error naming the session binding |

### `mcpapp/` — tool surface (extend `server_test.go`)

| Test | Setup | Action | Expected |
|---|---|---|---|
| bind_branch happy path | attached session | call with `branch` | result echoes branch; session metadata updated |
| exactly-one-of | attached session | both/neither of `branch`,`clear` | tool error |
| requires attachment | no attached session | call | handle-required error |
| hosted inertness | runtime without git acquisition | call with `branch` | clean typed error, no metadata change |
| free reads see bound branch | attached bound session, branch-only entry | `show`/`search` | entry found; unattached connection does not see it |
| relocation notice served | server configured with standing notice | `start_session`, `list_sessions`, `info` | notice text present in each response |
| binding projected on resume | bound session, fresh connection | `resume_session`, `list_sessions` | bound branch named in the info block / row |

### `cmd/sdd` — startup + migration (test seams as functions where practical)

| Test | Setup | Action | Expected |
|---|---|---|---|
| leftover detection | `.sdd/sessions/x.jsonl` present in-tree | serve startup | server starts; slog warning; notice wired into server options |
| no leftovers, no notice | empty/absent in-tree sessions dir | serve startup | no notice configured |
| init relocates | in-tree sessions + blobs | init | files in `<stateRoot>/sessions/<key>/`, source dirs gone |
| sessions-legacy ignored | `.sdd/sessions-legacy/` present | init | untouched |
| worktree sees sessions | init at base, serve from worktree | `list_sessions` | base-created sessions listed |
