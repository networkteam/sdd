# AC verification for d-tac-v4f

All 17 acceptance criteria verified against the post-push repo state.

- [x] **Repository `github.com/networkteam/sdd` exists with `main` branch pushed** — `git remote -v` shows origin; `git log origin/main` matches local
- [x] **New repo's default branch is `main` (no `future-framework` branch carried over)** — single-branch clone (`--branch future-framework`) followed by `git branch -m future-framework main`; `git branch -a` shows only `main` + `origin/main`
- [x] **Git history preserved** — 365 post-filter-repo commits + 3 reorg commits + this closing action; earlier SDD work (e.g. a-prc-fxn, a-tac-mca, d-tac-3yi) traceable via `git log`
- [x] **File layout: cmd/sdd/, internal/{command,finders,handlers,llm,meta,model,presenters,query}/, .claude/skills/sdd* (4 skills), .sdd/graph/ with entries + wip/, docs/*.md, bin/ (gitignored)** — verified via `ls -la` and `git ls-files`
- [x] **Root `go.mod` declares module `github.com/networkteam/sdd`** — `head -1 go.mod` confirms
- [x] **`go build -o bin/sdd ./cmd/sdd` produces a working binary** — 8.5MB binary at `bin/sdd`, gitignored
- [x] **`go test ./...` passes** — all 6 packages with tests pass (cmd/sdd, internal/command, internal/finders, internal/handlers, internal/llm, internal/model, internal/presenters); 3 packages have no tests (internal/llm/claude, internal/meta, internal/query) — consistent with pre-fork state
- [x] **`./bin/sdd status` runs against `.sdd/graph/` showing all extracted entries** — 268 entries (68 decisions, 108 signals, 92 actions) before this closing action
- [x] **CQRS separation preserved: internal/finders/ and internal/presenters/ remain distinct packages (d-cpt-ah1)** — directory listing confirms two packages; imports across Go files preserve the split
- [x] **Graph dogfoods SDD default: entries at .sdd/graph/ per model.DefaultGraphDir** — `.sdd/config.yaml` sets `graph_dir: .sdd/graph` explicitly (could also be omitted entirely since it's the default)
- [x] **`CLAUDE.md` covers Go stack, build/test commands, project conventions, binary location** — Devbox/direnv setup, `go vet` guidance, architecture principles (library first, CQRS, push logic down, single path), updated graph conventions, no-binaries rule
- [x] **`README.md` covers project intro, install-from-source, basic usage** — intro, Devbox-aware install-from-source, quick-start commands
- [x] **`LICENSE` file contains MIT license** — copyright 2026 networkteam GmbH
- [x] **`.gitignore` includes `bin/`** — as `/bin` (absolute match at repo root)
- [x] **`devbox.json` declares `go@1.26`, does not include `nodejs`** — verified; `DEVBOX_COREPACK_ENABLED` env and placeholder `test` script also dropped (resonance leftovers)
- [x] **No resonance-specific paths present (`skills/`, `docs/spec/`, `install.sh`, `package.json`, `.claude/settings.local.json`)** — all absent; filter-repo extraction set ensured this
- [x] **No git version tag present (v0.1.0 deferred to d-cpt-uu1)** — `git tag -l` empty

## Notes on execution

- **Accidental binary commit scrubbed.** Step 3 initially included `bin/sdd` because the root `.gitignore` hadn't been updated yet. Fixed via a second `git filter-repo --path bin --invert-paths --force`. The final step 2 and 3 hashes differ from the original composition.
- **Devbox activation gotcha.** GNU sed was not in PATH until `direnv allow` was run in the new clone — first `sed -i` attempt hit a BSD-vs-GNU flag mismatch. Saved as cross-project agent memory.
- **devbox.lock regenerated.** After removing `nodejs@24` from `devbox.json`, the lock was regenerated via `devbox install` to drop the unused package.

## Out of scope (plan deliberate)

- **Resonance strip-out.** Removing `framework/`, `.claude/skills/sdd*`, `.sdd/config.yaml`, `docs/framework/` from resonance. Separate operation on its own schedule.
- **Distribution (d-cpt-uu1).** GoReleaser, Homebrew tap, curl|bash installer, skills embed + install command, initial v0.1.0 tag. Independent plan.
- **Two-type redesign (d-cpt-omm).** Semantic refactor of signal/decision/action types with kinds. Independent of fork mechanics — next step in the agreed sequence (fork → two-type → distribution).
