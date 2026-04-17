# Fork plan: resonance → github.com/networkteam/sdd

## Alternatives considered

### History preservation

- **Chosen:** `git filter-repo` on the SDD-relevant path set — preserves commits touching those paths
- Rejected: clean-slate init. Would lose the reasoning chain visible in prior commits (graph-entry commits, decisions that shaped the CLI, prior-iteration learnings).
- Rejected: `git subtree split`. Works for single subdirectory splits, but we're extracting multiple disjoint paths (`framework/`, four `.claude/skills/*`, `.sdd/config.yaml`, `docs/framework/`, devbox + envrc + gitignore).

### Go package layout

- **Chosen:** `internal/` for all non-main packages
- Rejected: top-level public packages. d-stg-574 commits to the CLI as the neutral interface, not a library API, so there's no external consumer to design for. Promoting `internal/model/` or `internal/finders/` to public packages is a small refactor once a concrete consumer appears (IDE extension, TUI, external tooling).

### Skills location in the new repo

- **Chosen:** keep at `.claude/skills/sdd*` (same as in resonance)
- Rejected: move to `skills/` + `//go:embed` now. Without the install command (d-cpt-uu1 scope), external users would have nowhere to get the skills from, and SDD's own development would break because Claude Code picks skills up from `.claude/skills/`. The embed + install mechanism lands together in d-cpt-uu1; until then, skills stay where they are.

### Graph location

- **Chosen:** `.sdd/graph/` — matches `model.DefaultGraphDir`, dogfoods SDD's own convention
- Rejected: keep `docs/framework/graph/`. The `framework/` prefix was meaningful in resonance's embedding context; in a standalone SDD repo the whole thing is the framework, so the prefix is noise.
- Rejected: `graph/` at repo root. Overweights the graph visually at the cost of diverging from the standard SDD layout that every downstream user will see.

### Initial version tag

- **Chosen:** no tag. Tagging is part of d-cpt-uu1 (first release via GoReleaser).
- Rationale: per Go conventions, pre-v1.0.0 minors do not carry compatibility guarantees, so v0.1.0 is the right first tag — but deferring until distribution prevents a tag-without-release inconsistency.

### License

- **Chosen:** MIT
- Rationale: user preference, standard permissive license for open-source Go tooling.

## Sequencing

1. **Extract** — in a fresh clone of resonance outside the resonance working directory, check out `future-framework`, run `git filter-repo` with the path set below. Rename the branch to `main`; delete any other branches.

    ```
    git filter-repo \
      --path framework/ \
      --path .claude/skills/sdd/ \
      --path .claude/skills/sdd-catchup/ \
      --path .claude/skills/sdd-explore/ \
      --path .claude/skills/sdd-groom/ \
      --path .sdd/config.yaml \
      --path docs/framework/ \
      --path .envrc \
      --path devbox.json \
      --path .gitignore
    ```

2. **Reorganize Go code** (single commit):
   - `framework/cmd/sdd/` → `cmd/sdd/`
   - `framework/sdd/{command,finders,handlers,llm,meta,model,presenters,query}/` → `internal/{...}/`
   - `framework/go.mod` → `./go.mod`, module path → `github.com/networkteam/sdd`
   - Update all imports across Go files
   - Remove empty `framework/` directory

3. **Reorganize graph + docs** (single commit):
   - `docs/framework/graph/` → `.sdd/graph/`
   - `docs/framework/{signal-dialogue-decision,signals,story}.md` → `docs/*.md`
   - Update `.sdd/config.yaml` to `graph_dir: .sdd/graph`
   - Remove empty `docs/framework/` directory

4. **Infrastructure** (single commit):
   - New `CLAUDE.md` — Go stack, build (`go build -o bin/sdd ./cmd/sdd`), test (`go test ./...`), `./bin/sdd` binary location, project conventions
   - New `README.md` — intro, install-from-source, basic usage
   - New `LICENSE` — MIT
   - `.gitignore` adds `bin/`
   - `devbox.json` drops `nodejs@24`
   - Regenerate `devbox.lock`

5. **Verify** — `go build ./cmd/sdd`, `go test ./...`, smoke test `./bin/sdd status` against the extracted graph

6. **Publish** — create `github.com/networkteam/sdd`, push `main`

## Out of scope

- **Resonance strip-out:** removing `framework/`, `.claude/skills/sdd*`, `.sdd/config.yaml`, `docs/framework/` from resonance is a separate operation. Can happen independently and on its own schedule.
- **Distribution (d-cpt-uu1):** GoReleaser, Homebrew tap, curl|bash installer, skills embed + install command, initial v0.1.0 tag. Separate plan.
- **Two-type redesign (d-cpt-omm):** semantic refactor of type system. Independent of fork mechanics.

## Open questions

None remaining at plan time. Implementation may surface specifics (exact filter-repo invocation variants, Go module version directive, README structure, CLAUDE.md depth).
