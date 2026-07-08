# Cross-repo references — outer (validation) evaluation

- **Date:** 2026-07-08
- **Evaluator:** Christopher (hands-on), with Claude (dialogue + capture)
- **Evaluated work:** `20260708-010505-s-tac-94a` — cross-repo references implemented end to end
- **Lens:** outer / validation only. Inner covered separately by `s-tac-vyq` (code review) + `s-tac-7lv` (remediation).

## Setup performed
- A real second graph was created and connected **one-way** to the sdd framework graph — the second graph referencing sdd, never the reverse.
- `sdd init` in `sdd`: derived + stamped `repo_id github.com/networkteam/sdd` from the git remote (upgrade path), then pushed. Worked cleanly.
- `sdd repo add https://github.com/networkteam/sdd.git`: connected, resolved the declared identity, cache at `~/.cache/sdd/github.com/networkteam/sdd`. Worked.
- Global config `~/.config/sdd/config.yaml` created by copying the full `.sdd/config.local.yaml` content (which turned out not to be honored — see finding 2).

## What worked (positives)
- `repo_id` derivation + upgrade-stamp via `sdd init`.
- `repo add` identity resolution + clone-URL cross-check.
- **Merged cross-repo search returned results across both graphs with reasonable relevance ranking across repositories** — the core feature validated in genuine first use.

## Findings
1. **No `sdd config` surface** — no CLI to create the global config, get/set keys, or print the effective merged config.
2. **Global config is a cross-repo-only schema** — `participant`/`llm`/local `embedding` placed there are silently dropped; only `repos` + cross-repo `embedding` are honored. Proven by `sdd lint` → "no local participant configured" after moving the project local config aside. Code: `repos.GlobalConfig{repos, embedding}`; `meta.ReadConfig` reads only project files; `crossRepoEmbedder` is the sole consumer of global embedding. Silent drop violates the fail-loud directive.
3. **No progress on `repo add`** — bare `fmt.Printf`, no color/styling, no clone progress.
4. **Connected cache = git clone of pushed state**, not the working tree → a fresh entry resolves cross-repo only after commit + push; refresh via `sdd repo sync`.
5. **No progress on cross-repo index build** — first `--repo` search silently lazy-fills the connected cache index; ran **~35 minutes** (Ollama `qwen3-embedding:8b`, ~400 entries) with zero output. Reads as a hang. Same root as (3); contradicts the done's claimed conformance to `d-cpt-dgk`.
6. **No eager connected-repo indexing** — `sdd index` lacks `--repo` / `--all-repos`; the only way to build a connected cache index is to trigger a search and endure the silent build.
7. **Index embeds twice per machine** — `sdd/.sdd/index` (129 MB) + a separate `~/.cache/sdd/<repo-id>/.index`; worktree sharing rides on a Claude-Code-only `.worktreeinclude` copy. `d-tac-lqr`'s local/per-participant/gitignored storage predates cross-repo's single-shared-embedder requirement.
8. **Cross-repo query latency** (low priority) — suspects: cold-load of the 129 MB flat index per CLI invocation (probably opened more than once on the cross-repo path) + cooldown-gated git freshness. The long-lived MCP server can keep indexes warm, so this is largely a CLI cold-start cost.
9. **Connections are per-user-global, not committed per-repo** — `sdd repo add` writes only to `~/.config/sdd/config.yaml`; the current repo's `.sdd/config.yaml` records nothing, and `model.Config` has no dependency field. A cloner of a dependent graph gets no list of what to connect, and the flat global registry makes the one-way guarantee a *discipline* (don't add the reverse) rather than a structural property.

### Timings observed
| query | total |
|---|---|
| second graph (local) `--query` | 0.176s |
| `sdd` local `--query` | 1.859s, then 0.637s, 0.661s |
| cross-repo `--repo sdd --query` | 1.152s, 2.352s, 3.169s |

## Directions (proposed — not yet committed decisions)
- **Config:** unified global-first overlay resolved `global → project-committed → project-local → flags`; add a `sdd config` surface; unknown/wrong-file keys fail loud. Project-only: `repo_id` (a global fallback would silently mis-stamp), `graph_dir`, `supported_agents`, `skill_scope`, likely `language`. Open: `language`/`skill_scope`/`sync.cooldown` homes.
- **Connections:** make a connection a committed, per-repo, **directed dependency** declaration (`depends_on: [repo_id…]` in `.sdd/config.yaml`) — like `go.mod` — with per-user `clone_url`/cache resolution staying global. Directed: resolve-or-block (`d-cpt-uh0`) permits a ref only if its `repo_id` is a declared dependency of the referring graph, so a graph that does not declare a repository as a dependency cannot reference into it — blocked *structurally*. Resolution rides on `sdd init` (the post-clone / post-update surface every user already runs) rather than a new command: init reads the current repo's declared dependencies, checks which the user hasn't connected, and prompts `sdd repo add <clone-url>` for each missing one — the prompt is required because `clone_url` is per-user (ssh vs https) and not committed, so init knows the `repo_id` to fetch but not how this user reaches it. `sdd repo add` writes both halves: the committed declaration in the current repo + the per-user resolution globally.
- **Index:** one machine-global content-addressed index per `(repo-id, embedder-fingerprint)`, shared across checkout, worktrees, and connected cache — safe because the index is additive/content-hashed and reads intersect current graph content; migrate an existing local index via `sdd init`. Open: keying identity-less repos; concurrent multi-process writers.
- **`sdd init` as the single post-clone / post-update contract:** the through-line under three of these directions — init already derives/stamps `repo_id`; it should also migrate the local index into the global store and resolve declared dependencies (prompt-to-add). One surface the user already runs, no new verbs.
- **Unifying:** config sits in the wrong layer in **both** directions — user/machine prefs (participant/llm/embedding) trapped per-repo-local when they belong global; repo *dependencies* trapped per-machine-global when they belong committed per-repo. Plus per-machine state (config.local, index) kept in the working tree forces harness copy hacks (`.worktreeinclude`). Correct home for each: user prefs → global; repo identity + dependencies → committed per-repo; index → machine-global cache.

## Not exercised — deferred to round 2
- Capture-time resolve-or-block on a real cross-repo ref.
- Cross-boundary `show` chain traversal.
- Remote / owner-derived status rendering.

## Verdict
The mechanism is right and works. Ship-usable for a motivated dogfooder; not yet for the non-developer / hosted audience the strategic aspirations target. Real-world usability is undercut by the config model, the connections model, the index-storage model, and no-progress on long operations.
