# Cross-repo references — implementation spec

**Status:** decided — ready for plan capture
**Realizes:** the cross-repo design as augmented in `d-cpt-spa` (⑤), which refines `d-cpt-v8t`; closes toward gap `s-cpt-r3l`.
**Grounded in:** `d-cpt-ba7` (① — every ref resolves at capture or blocks).
**Depends on:** `d-cpt-mvb` (② — terminal-experience architecture; this feature is feedback-heavy and must not improvise its output). New operational logging uses `slog` via `slogutils.FromContext`.

Two build layers — the foundation is independently shippable; discovery is the larger follow-on and depends on the terminal-experience groundwork landing.

---

## 1. Goals & Requirements

A mechanism for entries in one SDD graph to reference entries in another repo's graph, plus the infrastructure to resolve (read) and discover (search) those entries across a user's connected repos.

- **R1** Cross-repo ref id is `<repo-id>:<entry-id>`; the separator is the first `:`.
- **R2** `repo_id` is the canonical, URL-shaped identity (host/path), declared in the repo's own committed `.sdd/config.yaml`; identical for every user.
- **R3** Lifecycle never crosses graphs: a close or supersede has no effect across the boundary (a remote close doesn't change a local entry's status). The remote entry's own derived status may still be read from the cached graph for display.
- **R4** A cross-repo ref is stored verbatim and reads back unchanged; it doesn't break parsing or rendering of the entry or its other refs, and isn't mistaken for a local ref.
- **R5** Connected repos are registered in a global user-level config: per repo `{repo_id, clone_url}`. `clone_url` is distinct from `repo_id` (identity is not a live binding to the remote).
- **R6** Cross-graph search uses one global embedder; every connected repo's index is built with it.
- **R7** Participant identity stays per-graph (no cross-graph unification).
- **R8** Cross-repo *backlinks* (who-cites-me) are out of scope.
- **R9** A cross-repo ref must resolve at capture or capture blocks (high severity) — it targets an entry on the other repo's **default branch**; on cache miss the capture path fetches fresh and blocks only if genuinely absent. (Inherits the universal invariant ①.)
- **R10** `repo_id` is auto-derived from the git remote at init (Go-module convention), committed when absent, never user-chosen.

## 2. Architecture & Design Decisions (all resolved)

### A1. Ref id is a string with a `<repo-id>:` prefix; `Ref.ID` stays a bare string
`Ref.ID` is already a bare string and survives YAML round-trip unchanged (`internal/model/ref.go:99-196`); `ParseID` ignores colons (`entry.go:372-398`). Add `SplitCrossRepoID(id) (repoID, entryID string, isCrossRepo bool)` (first-`:` split) + a repo-id validator. Repo-ids are host/path and contain no colon.

### A2. `repo_id` field in `.sdd/config.yaml`, auto-derived at init
Add `RepoID string` to `Config` (`internal/model/config.go:53-75`). `sdd init` derives it from `git remote` (normalize ssh/https → `host/path`, strip protocol/user/`.git`), writes + commits via `SetYAMLField` when absent; backfills on later init. Not user-choosable. Local-only repos with no remote leave it unset (not referenceable until manually set). The committed value is canonical and survives hosting moves — derivation is a one-time bootstrap, reconciling d-cpt-v8t's "declared, not derived" intent.

### A3. Cross-repo refs show the remote's own status, from the cache
Render the full canonical `repo_id` prefix plus the remote entry's derived status, computed in the **cached** graph (so it can lag by the cache cooldown) — e.g. `{status: closed-by <remote-id>}`. `DerivedStatus` runs over the cached graph, not the local one, so this is a read, not a cross-graph lifecycle effect. The only constraint is R3: a close/supersede never crosses the boundary. No abbreviation/alias (a per-user alias would break R2). When the cache is absent, the ref shows `[unresolved: repo X]` instead of a status.

### A4. Cross-repo refs excluded from the local index; no dangling-ref warning
Guard `RefsTo` indexing (`graph.go:35-45`) and `validateIDRefs` (`graph.go:729-748`) with `SplitCrossRepoID` so cross-repo refs aren't indexed or flagged dangling. Entry-id and repo-id portions still syntactically validated.

### A5. Resolution + full cross-graph traversal via the cache
`sdd show <repo-id>:<entry-id>` resolves by loading the cached graph for `<repo-id>` and looking up the entry. `sdd show` follows cross-repo chains **fully**, bounded by max-depth, with a `(repo-id, entry-id)` dedup key (composite, since ids are unique only within a repo — this also makes cross-graph cycles safe). The cache→graph loader is reentrant + memoized per command. Uncached/unconfigured targets render `[unresolved: repo X]`.

### A6. Global user-level config: connected repos + single shared embedder
Introduce `~/.config/sdd/config.yaml` (XDG, honor `XDG_CONFIG_HOME`) holding `repos: [{repo_id, clone_url}]` and the shared `embedding:` config. New pattern (no homedir config exists today). **Embedder is global-only**: per-repo `embedding:` is migrated/deprecated for cross-graph purposes; the global embedder is the single vector space cross-graph search fuses over.

### A6b. Registration command derives the id from the clone
`sdd repo add <clone-url>` clones the URL, verifies it's an SDD repo, reads the declared `repo_id` from its config, and registers `{repo_id, clone_url}` — the id is **not** passed (the repo is the source of truth); errors if not an SDD repo or no `repo_id` declared. `sdd repo list` / `sdd repo remove` round it out. Hand-editable YAML underneath.

### A7. Managed read-only caches; lazy clone + cooldown pull
Connected repos cache at `~/.cache/sdd/<repo-id>/` (XDG, `XDG_CACHE_HOME`), cloned from `clone_url`. First use that needs a repo clones it; later uses auto-pull past a cooldown (reusing the ~15m sync cooldown); `sdd repo sync` forces. Separate from the current-repo background sync (`d-tac-hsu`) — read-only, no rebase. Auth via ambient git credentials. The current (local) repo is **not** cached: it keeps indexing itself at `.sdd/index/` (branch/worktree-sensitive, sees uncommitted entries); the cache holds only *other* repos.

### A8. Per-repo indexes merged by comparable score
Each cached repo gets its own index at `~/.cache/sdd/<repo-id>/.index/`, built with the global embedder. Search embeds the query once, scores it against each selected index, rolls up per entry (existing depth/summary/status scoring), tags each hit with its `repo_id`, and merges into one list sorted by **raw cosine score** — comparable across same-embedder indexes (chromem normalizes; scores in [-1,1]). Flags: `--repo <id>` (repeatable, **additive** — local always included) and `--all-repos` (current + every connected), on `sdd search` and `sdd view`. Local hits render bare; remote hits carry the `repo_id` prefix. A connected repo whose manifest fingerprint ≠ the global embedder's is excluded from cross-graph search and flagged by `sdd lint`.

### A9. Strict resolution at capture; pre-flight reads the cache
Every ref must resolve at capture or capture blocks (① d-cpt-ba7). For a cross-repo ref: mechanical pre-flight validates syntax; the capture path resolves against the cache, auto-fetching fresh on a miss, and blocks (high) only if the target is genuinely absent from the other repo's default branch. The LLM ref-meta consistency check reads the resolved cached remote entry (its content + within-repo derived status) — no silent fallback. (Mechanical ref checks today validate only kind; the assembly at `internal/llm/preflight.go:268-277` drops unresolved refs — this is where the cached remote entry is loaded in.)

### A10. `refines` cross-repo co-closure — forward constraint only
No v1 work: `refines` co-closure isn't implemented at all (`status.go` derives status only from explicit closes/supersedes), and cross-repo refs are excluded from the index (A4). When co-closure is later built, it must guard on repo-id so a remote close never cascades local status.

### A11. repo_id discoverability via the URL-path convention
Because `repo_id` is host/path (not an opaque short id), it's human-meaningful and the likely clone URL is recoverable from it — softening the indirection's cold-start cost (a ref to an unconnected repo hints its URL, so the user can `sdd repo add` it). The registry's `clone_url` remains authoritative for cloning and auth.

### A12. Code placement & coupling (CQRS)
- **Sync is a command; reads are pure.** Clone/pull has side effects (git, index writes), so a handler does it before any read; finders never clone. Same shape as `cmd/sdd/search.go` running `IndexHandler.LazyFill` before the search finder today.
- **Handlers stay in `internal/handlers`.** The connected-repos registry + cache machinery (clone/pull, locate a cached repo's graph + index) live in a new `internal/repos/` package as plain functions — no command structs. The handler in `internal/handlers` takes the command struct and calls into `internal/repos`. Global-config loading layers under the existing `loadConfig` / `meta.ReadConfig` chain.
- **One `model.MultiGraph` carries all cross-graph reads.** It holds the local graph plus member graphs keyed by repo-id and owns the cross-graph logic: resolve `<repo-id>:<entry-id>` to the owning graph + entry (a bare id → local), follow refs across graphs for `sdd show` traversal with `(repo-id, entry-id)` dedup, and derive a remote entry's status in its own cached graph. The handler loads the member graphs and builds the MultiGraph; finders just read it — no cross-graph logic in finders, no per-repo finder orchestration. A MultiGraph holding only the local graph behaves exactly like today, so the single-repo path is unchanged.
- **Search seam.** Retrieval comes from the per-repo **index set** (one store per selected repo); the MultiGraph is used only to resolve each hit's entry + status for rendering. `SearchFinder` widens from one index store to a set keyed by repo-id; merge-by-score stays pure.

## 3. Implementation Changes

### Layer A — reference foundation
- `internal/model/`: `SplitCrossRepoID` + repo-id validator (`ref.go` or new `crossrepo.go`); guard `RefsTo` indexing and `validateIDRefs` (`graph.go:35-45`, `729-748`); add `RepoID` to `Config` (`config.go:53-75`) + commented template line (`FormatConfig`).
- `cmd/sdd/main.go:2063-2105` (`parseRefFlags`): syntactic validation for cross-repo ids.
- `internal/presenters/`: cross-repo rendering — full prefix, no status segment (`show.go` ~58, `render_list.go:49-76`). Build on the **current** renderer (d-tac-z3k will re-do it).
- Pre-flight resolution precondition over all ref ids (`internal/finders/preflight_mechanical.go`).

### Layer B — discovery
- New global-config + cache package (extend `internal/meta/` or new `internal/global/`): load/write `~/.config/sdd/config.yaml`; cache manager (clone/pull, load cached graph, build index under the cache).
- `sdd repo add/list/remove`, `sdd repo sync`.
- `sdd init`: derive + commit `repo_id` from git remote when absent.
- `sdd show`: cache-resolution + full cross-graph traversal (`internal/finders/show.go`, `internal/model/graph.go:448-489`).
- `sdd search` / `sdd view`: `--repo`/`--all-repos`, multi-index merge by score, repo-id-prefixed results (`cmd/sdd/search.go`, `internal/finders/search.go:302-315`).
- Pre-flight reads cached remote entry for ref-meta (`internal/llm/preflight.go:255-346`).
- Skill/docs: `/sdd` capture (suggest cross-repo grounding) and discovery ("I think we had that in repo X" → `--repo` search); catch-up rendering; `cli-reference.md`, `search.md`.
- All new operational output via `slog`/reporter callbacks per the terminal-experience groundwork (d-cpt-mvb) — no ad-hoc stderr prints.

## 4. Test Cases

### Layer A
| Test | Setup | Action | Expected |
|---|---|---|---|
| Split local / cross-repo id | both id forms | `SplitCrossRepoID` | `("", id, false)` / `(repo, entry, true)` |
| Round-trip | entry w/ cross-repo ref | Marshal→Unmarshal | byte-stable, colon + kind preserved |
| Not dangling / not indexed | graph + cross-repo ref | load | no dangling warning; absent from `RefsTo` |
| Render | cross-repo ref | show / view expand(refs) | full `repo-id:entry-id` prefix, no `{status}` |
| Resolve-or-block | ref to missing local id / unresolvable cross-repo | `sdd new` | capture blocks (high) |

### Layer B
| Test | Setup | Action | Expected |
|---|---|---|---|
| Global config load | XDG config w/ repos + embedder | load | parsed; cross-graph embedder resolved |
| `repo add` derive+verify | clone-url of an SDD repo | `sdd repo add <url>` | repo_id read from clone, registered; non-SDD/no-id errors |
| `init` derives repo_id | repo w/ git remote, no repo_id | `sdd init` | host/path repo_id committed; ssh/https normalize equal |
| Cache lifecycle | connected repo | first search; later search | clone on first; cooldown pull; read-only |
| Cross-graph search | 2 repos, same embedder | `sdd search --all-repos` | one list by score; remote hits repo-id-prefixed |
| Embedder mismatch | repo indexed w/ other embedder | search + lint | excluded from cross-graph; `sdd lint` flags it |
| Show traversal | A→B→C cross-repo chain | `sdd show A:id --max-depth N` | full traversal, `(repo,entry)` dedup, max-depth bound |
| Pre-flight cross-repo | cached target | `sdd new` w/ cross-repo ref | ref-meta check runs on cached entry |

(Extend existing `internal/model/ref_test.go` and graph/search tests rather than new files where possible.)
