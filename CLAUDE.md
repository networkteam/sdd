# CLAUDE.md

Guidance for Claude Code working on the SDD (Signal-Dialogue-Decision) framework.

## Project overview

SDD is a CLI-driven framework for building and traversing decision graphs through human-agent dialogue. This repo contains the Go implementation and the Claude Code skill definitions. See [docs/story.md](docs/story.md) for the conceptual narrative and [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) for the framework model.

## Setup

The project uses Devbox + direnv for the toolchain (Go 1.26, GNU sed, etc.). In a fresh clone:

```bash
direnv allow                  # loads devbox env; wires the git hooks (core.hooksPath)
devbox run build              # build fresh local sdd dev version
sdd init --scope project      # refresh .claude/skills from the embedded bundle
```

The `sdd` binary lives at `./bin/sdd` (gitignored — rebuild locally with `devbox run build`, never commit, added to PATH via devbox.json, so call it via `sdd`). Git hooks under `.githooks/` (wired via `core.hooksPath` on `direnv allow`) auto-rebuild it after every pull, rebase, and branch switch — but you still rebuild manually after editing source. `sdd init` is idempotent: on a fresh checkout it creates `.sdd/meta.json` and installs skills; on subsequent runs it refreshes whatever drifted.

## Commands

- `devbox run build` — build the local `bin/sdd` dev binary (git hooks also run this after pull/rebase/checkout)
- `go vet ./...` — compilation + correctness check (never use `go build` just to verify compilation — it produces no output on success)
- `go test ./...` — run all tests
- `go test -tags=eval -run TestPreflightEval ./internal/llm/...` — pre-flight prompt calibration eval (live `claude` CLI, slow + paid; model via `SDD_EVAL_MODEL`, default `sonnet`). Capture full output to a file and grep the file — `… -v 2>&1 | tee /tmp/eval.log` — never filter the live stream, or a failure shows no findings and forces a costly re-run.
- `go fmt ./...` — format code
- `golangci-lint run ./...` — lint (must be clean; CI enforces)
- `sdd status` — smoke-test the binary against the graph at `.sdd/graph/`
- `goreleaser check` — validate `.goreleaser.yaml`
- `devbox run gen-installer` — regenerate the curl installer (`install.sh`) from `.config/binstaller.yml`. Run this whenever `.goreleaser.yaml` changes its platform or asset surface, then commit the updated `install.sh`. Install binstaller separately (`go install github.com/binary-install/binstaller/cmd/binst@latest`) — it isn't in devbox's nixpkgs.

## Architecture

- **Library first**: Domain logic lives in `internal/`. `cmd/sdd/` is a thin shell that parses flags, dispatches commands/queries, and uses presenters to render results. Keep business logic out of CLI actions.

- **CQRS layering** (per d-cpt-l3s, enforced by the planning contract d-cpt-ah1): functionality decomposes across five packages —
  - `internal/command/` — write-intent structs (e.g. `NewEntryCmd`) with optional result callbacks carrying small identifiers (e.g. `OnNewEntry func(id string)`).
  - `internal/query/` — read-intent structs.
  - `internal/handlers/` — one `Handler` per area with methods per command. Holds injected dependencies (graph dir, git committer, pre-flight runner, clock, stdin reader). Returns errors only; richer results flow through callbacks on the command struct.
  - `internal/finders/` — process queries into results. Pure reads, no side effects. Used by handlers internally and called directly by the CLI.
  - `internal/model/` — pure domain types, no I/O.

  `internal/presenters/` sits on top of the read side for view rendering — kept distinct from finders so data and rendering stay separable (view-layer concern, not CQRS itself).

- **CQRS rules:**
  - **Side effects only in handlers.** Model, finders, presenters are pure.
  - **Handlers return errors only.** For richer data after a write, the caller issues a follow-up query via a finder — handlers are not a query back-door.
  - **Handlers may use finders internally**; finders never use handlers.
  - **Pre-flight is a query** (pure read intent at the domain level) despite the LLM runner's side effect — lives in `query/` + `finders/`.

- **Type-system plan capture surfaces presentation impact.** When drafting a plan that changes entry types, kinds, or frontmatter, raise downstream presentation surfaces (`sdd status`, `sdd list`, `sdd show`, catch-up narrative, skill rendering) in dialogue. The user decides whether to add ACs for each surface or explicitly carve them out as future work; the agent helps compose once the call is made. Raising this at plan time avoids rediscovery during unrelated execution.

- **Push logic down**: Finders and handlers are orchestration — they wire dependencies and delegate. Graph traversal, tree building, filtering, and any pure computation belongs in `internal/model/`. Always question whether code in a finder/handler could live in a lower package.

- **Single path**: I/O functions (file loading, etc.) should delegate to in-memory constructors. Don't duplicate indexing or initialization logic between production and test code paths.

- **Logging**: Use `log/slog`; retrieve the logger via `slogutils.FromContext(ctx)` (from `github.com/networkteam/slogutils`). Handler entry points take `ctx` and pull the logger from it — do not pass loggers as separate arguments, and do not use `fmt.Fprintf(h.stderr, ...)` for operational messages. Stderr writes are reserved for user-facing CLI output that isn't logging (prompts, structured CLI results).

## Structure

```
sdd/
├── cmd/sdd/                # CLI entrypoint (main.go)
├── internal/
│   ├── command/            # Write-intent structs (CQRS commands)
│   ├── query/              # Read-intent structs (CQRS queries)
│   ├── handlers/           # Command execution — side effects live here
│   ├── finders/            # Query execution — pure reads, no side effects
│   ├── model/              # Pure domain types (no I/O, no deps)
│   ├── presenters/         # View rendering of query results
│   ├── llm/                # Pre-flight + summarization via LLM
│   ├── meta/               # Config resolution
│   └── bundledskills/      # Skill source of truth, embedded via //go:embed
│       └── claude/         # Claude skill tree (sdd, sdd-catchup, sdd-explore, sdd-groom)
├── .claude/skills/         # Installed skill copy for this repo (rebuilt from internal/bundledskills)
├── .sdd/
│   ├── config.yaml         # SDD config (graph_dir, etc.)
│   └── graph/              # Entry files (markdown + frontmatter)
├── docs/                   # Narrative: story.md, signals.md, signal-dialogue-decision.md
├── bin/                    # Local build output (gitignored)
├── go.mod                  # module github.com/networkteam/sdd
├── devbox.json
└── .envrc
```

## Graph conventions

- Entries are immutable markdown files with YAML frontmatter under `.sdd/graph/` (matches `model.DefaultGraphDir`)
- Path layout: `.sdd/graph/YYYY/MM/DD-HHmmss-type-layer-suffix.md`
- Types: signal (`s`), decision (`d`), action (`a`)
- Layers: strategic (`stg`), conceptual (`cpt`), tactical (`tac`), operational (`ops`), process (`prc`)
- Full ID format: `{YYYYMMDD}-{HHmmss}-{type}-{layer}-{suffix}` — full ID used in code/CLI invocations, path derived from it
- WIP markers live at `.sdd/graph/wip/`

## Skill source of truth

Skills are **source-of-truth in `internal/bundledskills/claude/`** and compiled into the binary via `//go:embed`. The copy under `.claude/skills/` is the _installed_ output for this repo — it is what Claude Code loads during a session.

- **Never edit `.claude/skills/` directly.** Changes made there will be overwritten (or flagged as "modified") the next time `sdd init` runs.
- Edit in `internal/bundledskills/claude/<skill>/` → rebuild (`devbox run build`) → reinstall (`sdd init --scope project`). The installed copy picks up the new bundle and refreshes `.claude/skills/` with fresh `sdd-version` + `sdd-content-hash` stamps.
- Commits should include both locations when a skill changes: the source under `internal/bundledskills/claude/` and the re-stamped output under `.claude/skills/`.
- **`sdd init` auto-commits the installed refresh.** Running `sdd init` (e.g. `--scope project`) commits the regenerated `.claude/skills/` files on its own as `sdd: refresh installed skills and metadata`. So after a reinstall the installed copy already shows clean in `git status` (it's committed), and your bundled-source edit stays as the separate change you commit yourself — the two halves land in two commits, not one. This is expected; don't go hunting for "missing" installed-file changes or try to fold the auto-commit back in.

## Git rules

- **Never commit compiled binaries.** `bin/sdd` is in `.gitignore` and must stay there. Rebuild locally.
- **Never use `git add -A` or `git add .`.** Stage individual files by path (`git add path/to/file`). Wide-net staging sweeps in unrelated work from concurrent sessions or active WIP markers and binds it to the wrong commit narrative — the contamination is invisible until the commit lands and is hard to untangle without rewriting history.

## Auto-memory

**Do not write auto-memory in this project.** This repo dogfoods SDD — we use SDD to develop SDD. Auto-memory creates a parallel record that shortcuts what the graph and skill dialogue are supposed to handle, and it contaminates the evaluation of whether SDD itself is guiding the work well. If memory quietly carries the context that SDD should be surfacing through signals, decisions, and skill guidance, we lose the signal about where the framework is falling short.

**No exceptions, and no separate memory store.** When you would reach for auto-memory — a working preference, a recurring habit, a cross-session note — update this CLAUDE.md instead (or capture it in the graph when it is SDD substance). This holds even for things that feel "general" or unrelated to SDD: if it surfaced while working here, it lands here, not in a parallel memory file. Do not carve out a "this one's just a communication tic" exception — that loophole defeats the purpose. CLAUDE.md and the graph are the only durable records.
