# Cross-repo references — implementation spec (v2)

**Status:** decided — refreshed 2026-07-07 against the current codebase; supersedes the spec attached to `d-tac-rpm`.
**Realizes:** the augmented cross-repo design `d-cpt-s6q` (refining `d-cpt-v8t`); closes toward gap `s-cpt-r3l`.
**Grounded in:** `d-cpt-uh0` (every backward-class ref resolves at capture or capture blocks; forward-class kinds `surfaces`/`required-by` exempt), `d-cpt-dtv` (base facts merge into every graph), `d-cpt-dgk` (terminal-experience architecture).
**Depends on:** the GraphSource seam from `s-tac-jg8` landing first (finder-owned "current graph" provider, request/advance-scoped memoization, invalidate-on-write).

Two build layers — the foundation is independently shippable; discovery is the larger follow-on.

## What changed vs the superseded spec

1. **Placement corrected (was: "handler builds the MultiGraph").** Reads never pass through handlers — the CLI and the MCP read tools both call finders directly, and the engine's serves read through the same finder. `model.MultiGraph` is therefore assembled **inside the finder-owned GraphSource** (`s-tac-jg8`'s seam): one insertion point that CLI commands, MCP free reads, engine serves, and capture-time resolution all inherit. Side effects — cache clone/pull, remote index builds — stay in **handlers**, which invalidate the source on completion (same shape as `IndexHandler.LazyFill` running before the search finder today).
2. **MCP surface in scope.** The MCP `search` and `view` tools gain repo selection parameters (equivalent to `--repo`/`--all-repos`); `show` resolves `<repo-id>:<entry-id>` through its existing `ids` parameter. Repo management (`sdd repo add/list/remove/sync`) stays CLI-only — machine setup, not dialogue.
3. **Engine-session resolution semantics (new question, decided).** Cross-repo resolution always reads the **live cache path** — cooldown pull on reads, fetch-on-miss at capture — memoized per call/advance, never bound to session lifetime. Rationale: handlers already reload the local graph fresh at write time, so snapshotting only the remote side would invert freshness; a session snapshot would false-block captures referencing entries created remotely mid-session; parked sessions resume days later.
4. **Base-facts interaction (new since the old plan).** `LoadGraph` joins embedded framework entries (base procedures, base facts per `d-cpt-dtv`) into every graph it loads. Rule: member graphs load **with** the embedded set — chains stay complete and per-graph status derives correctly (a project may supersede a base fact for itself; that status is per-graph) — but remote **index builds exclude embedded entries** (base facts stay findable via the local index) and **traversal dedups base entries by bare entry-id** (they are binary-scoped and identical in every member graph; a base entry reached through repo X's chain renders with repo X's derivation). Exactly one copy ever surfaces in search results or a show tree.
5. **Invariant grounding moved** from the superseded `d-cpt-ba7` to `d-cpt-uh0`: same resolve-or-block, plus the forward-class exemption (`surfaces`, `required-by`).
6. **Renderer/flag reality.** The `sdd show` restructure (`d-tac-z3k`) has shipped: YAML envelope + markdown tree, `--up`/`--down` replacing `--max-depth`, `ResolvedRef` supersede head-walk. Cross-repo rendering builds on that renderer; head-walks resolve within the owning graph (a remote entry's supersede chain lives in the remote graph).
7. **Parsing is deeper than the old spec listed.** `ParseID` rejects the colon form today (malformed → hard block at write, before pre-flight); short-ID resolution (`ResolveID`/`ResolveRefIDs`) passes unknown strings through silently. `SplitCrossRepoID` guards are needed at: `validateIDRefs`, `RefsTo` indexing, `ResolveID`/`ResolveRefIDs`, `parseRefFlags`, `ResolvedRef` walks, and the engine capture path.
8. **Naming.** Avoid "registry" for the connected-repos configuration — it already names the engine function registry (`internal/engine/registry.go`), its MCP wiring, and the `registry` MCP tool. Use "connected repos" in code and docs.

## 1. Goals & requirements

R1–R10 carry over from the superseded spec unchanged in substance:

- **R1** Cross-repo ref id is `<repo-id>:<entry-id>`; the separator is the first `:`.
- **R2** `repo_id` is the canonical, URL-shaped identity (host/path), declared in the repo's committed `.sdd/config.yaml`; identical for every user.
- **R3** Lifecycle never crosses graphs: a close or supersede has no effect across the boundary. The remote entry's own derived status may be read from the cached graph for display.
- **R4** A cross-repo ref is stored verbatim and reads back unchanged; it doesn't break parsing or rendering, and isn't mistaken for a local ref.
- **R5** Connected repos are registered in a user-global config: per repo `{repo_id, clone_url}`; `clone_url` distinct from `repo_id`.
- **R6** Cross-graph search uses one global embedder; every connected repo's index is built with it.
- **R7** Participant identity stays per-graph.
- **R8** Cross-repo backlinks (who-cites-me) are out of scope.
- **R9** A cross-repo backward-class ref must resolve at capture or capture blocks (high severity) against the other repo's **default branch**; on cache miss the capture path fetches fresh. Forward-class kinds exempt (`d-cpt-uh0`).
- **R10** `repo_id` is auto-derived from the git remote at init (Go-module convention), committed when absent, never user-chosen.

New:

- **R11** The CLI and the MCP server are both first-class surfaces: every cross-repo read capability reachable via CLI flags is reachable via MCP tool parameters, served by the same finders.
- **R12** Embedded (binary-scoped) entries never duplicate across repos on any user-facing surface: excluded from remote indexes, deduped by bare id in traversal, while member graphs keep them loaded for chain/status completeness.
- **R13** All cross-graph read assembly happens inside the finder-owned GraphSource; no per-repo orchestration in finders, no graph assembly in handlers or shells.

## 2. Architecture & design decisions

A1–A11 carry over with amendments noted; A12 is rewritten.

- **A1** Ref id stays a bare string with `<repo-id>:` prefix; add `SplitCrossRepoID(id) (repoID, entryID string, isCrossRepo bool)` + repo-id validator. *Amended:* guards apply at every parse/resolve site listed in change 7 above, not only `validateIDRefs`/`RefsTo`.
- **A2** `repo_id` field in `.sdd/config.yaml`, auto-derived at `sdd init` (normalize ssh/https → `host/path`), committed when absent; local-only repos leave it unset.
- **A3** Cross-repo refs render the full canonical repo-id prefix plus the remote entry's derived status computed in the cached graph; `[unresolved: repo X]` when uncached/unconfigured. No abbreviation/alias.
- **A4** Cross-repo refs excluded from the local reverse index and dangling-ref validation; both id portions still syntactically validated.
- **A5** Resolution + full cross-graph traversal via the cache: `(repo-id, entry-id)` dedup key, bounded by `--up`/`--down`. *Amended:* base entries dedup by bare entry-id (R12); `ResolvedRef` head-walks run within the owning graph; the cache→graph loader is memoized per call/advance via the GraphSource.
- **A6** User-global `~/.config/sdd/config.yaml` (XDG) holding `repos: [{repo_id, clone_url}]` and the shared `embedding:` config. Global embedder is the single vector space; per-repo embedding config is migrated/deprecated for cross-graph purposes. Layering under the existing `loadConfig`/`meta.ReadConfig` chain (note `.sdd/config.local.yaml` sits in that chain too).
- **A6b** `sdd repo add <clone-url>` clones, verifies the target declares a `repo_id`, registers `{repo_id, clone_url}`; `sdd repo list`/`remove` round it out. Hand-editable YAML underneath.
- **A7** Managed read-only caches at `~/.cache/sdd/<repo-id>/` (XDG): lazy clone, cooldown pull (~15m), `sdd repo sync` forces. The local repo is not cached; it keeps indexing itself at `.sdd/index/`.
- **A8** Per-repo indexes at `~/.cache/sdd/<repo-id>/.index/`, built with the global embedder, **excluding embedded entries** (R12). Query embedded once, scored per selected index, merged by raw cosine score; remote hits repo-id-prefixed. `--repo` (repeatable, additive) and `--all-repos` on `sdd search`/`sdd view`; equivalent parameters on the MCP search/view tools. Mismatched-embedder repos excluded and flagged by `sdd lint`.
- **A9** Strict resolution at capture per `d-cpt-uh0` (forward-class exempt): mechanical pre-flight validates syntax; the capture path resolves against the cache, auto-fetching fresh on miss; the LLM ref-meta consistency check reads the resolved cached remote entry. Identical via CLI `sdd new` and engine capture transitions (live cache, A5 memoization — never the session snapshot).
- **A10** `refines` cross-repo co-closure: no v1 work; when co-closure is later built it must guard on repo-id.
- **A11** repo_id discoverability via the URL-path convention (a ref to an unconnected repo hints its clone URL).
- **A12 (rewritten) Code placement & coupling (CQRS).**
  - **GraphSource owns cross-graph assembly.** `model.MultiGraph` (local graph + member graphs keyed by repo-id; bare id → local; base entries resolved once) is built inside the finder-owned GraphSource's load path. Finders read the MultiGraph; a MultiGraph holding only the local graph behaves exactly like today, so the single-repo path is unchanged.
  - **Sync is a command; reads are pure.** Clone/pull and remote index builds run in a handler before reads need them, then invalidate the GraphSource. Registry-free naming: connected-repos + cache machinery in `internal/repos/` as plain functions; the handler in `internal/handlers` calls into it.
  - **Search seam.** Retrieval reads the per-repo index set (one store per selected repo); the MultiGraph resolves each hit's entry + status for rendering. `SearchFinder` widens from one index store to a set keyed by repo-id; merge-by-score stays pure.
  - **Surfaces.** CLI flags and MCP tool parameters map to the same query structs; the MCP server's per-call finder reads and the engine's serves inherit cross-repo through the GraphSource with no additional wiring.
  - **Output.** All new operational output via slog (`slogutils.FromContext`) and reporter callbacks per `d-cpt-dgk`; no ad-hoc stderr.

## 3. Implementation changes

### Layer A — reference foundation
- `internal/model/`: `SplitCrossRepoID` + repo-id validator (new `crossrepo.go`); guards in `RefsTo` indexing, `validateIDRefs`, `ResolveID`/`ResolveRefIDs`, `ResolvedRef`; `RepoID` in `Config` + commented template line.
- `cmd/sdd` ref parsing (`parseRefFlags`): syntactic validation for cross-repo ids.
- `internal/presenters/`: cross-repo rendering on the shipped YAML-envelope renderer — full prefix; status segment only when the cached graph is available, else `[unresolved: repo X]`.
- Pre-flight: resolution precondition over all backward-class ref ids (`internal/finders/preflight_mechanical.go`), forward-class exempt.

### Layer B — discovery
- `internal/repos/`: user-global config load/write; cache manager (clone/pull, locate cached graph + index); connected-repos listing.
- `internal/handlers/`: repo sync/clone handler; remote index build; GraphSource invalidation.
- GraphSource load path: assemble MultiGraph from local + selected member graphs (embedded entries joined per graph; excluded from remote index builds).
- `sdd repo add/list/remove/sync`; `sdd init` repo_id derivation.
- `sdd show`: cache resolution + full cross-graph traversal; `sdd search`/`sdd view`: `--repo`/`--all-repos` + multi-index merge.
- MCP server: repo parameters on search/view tools; show accepts prefixed ids via `ids`; engine capture transitions resolve through the same path.
- Pre-flight ref-meta against cached remote entries.
- Skills/docs: capture guidance (cross-repo grounding), discovery ("I think we had that in repo X" → repo-scoped search), `cli-reference.md`, `search.md`.

## 4. Test cases

Layer A: split/round-trip/not-dangling/not-indexed/render/resolve-or-block tests carry over from v1, plus: short-ID resolution does not misclassify colon forms; `ResolvedRef` walks stay within the owning graph; forward-class kinds skip resolve-or-block.

Layer B: global config load; `repo add` derive+verify; init repo_id derivation (ssh/https normalize equal); cache lifecycle (lazy clone, cooldown pull, read-only); cross-graph search merged by score with prefixed remote hits; embedder-mismatch exclusion + lint flag; show traversal A→B→C with `(repo,entry)` dedup and `--up`/`--down` bounds; base-entry single-copy: search returns one hit for an embedded entry with `--all-repos`, show tree dedups it by bare id, and a member repo's superseding fact renders that repo's derived status; MCP search/view repo parameters return the same merged results as the CLI flags; engine-session capture with a cross-repo ref resolves via live cache (fetch-on-miss) and does not consult the session snapshot; pre-flight ref-meta on cached remote entry.

(Extend existing `internal/model/ref_test.go` and graph/search tests rather than new files where possible.)