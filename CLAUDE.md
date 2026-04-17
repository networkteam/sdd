# CLAUDE.md

Guidance for Claude Code working on the SDD (Signal-Dialogue-Decision) framework.

## Project overview

SDD is a CLI-driven framework for building and traversing decision graphs through human-agent dialogue. This repo contains the Go implementation and the Claude Code skill definitions. See [docs/story.md](docs/story.md) for the conceptual narrative and [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) for the framework model.

## Setup

The project uses Devbox + direnv for the toolchain (Go 1.26, GNU sed, etc.). In a fresh clone:

```bash
direnv allow                      # loads devbox environment
go build -o bin/sdd ./cmd/sdd
```

The `sdd` binary lives at `./bin/sdd` (gitignored — rebuild locally, never commit).

## Commands

- `go vet ./...` — compilation + correctness check (never use `go build` just to verify compilation — it produces no output on success)
- `go test ./...` — run all tests
- `golangci-lint run ./...` — lint (must be clean; CI enforces)
- `./bin/sdd status` — smoke-test the binary against the graph at `.sdd/graph/`
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
│   └── meta/               # Config resolution
├── .claude/skills/         # SDD skills (sdd, sdd-catchup, sdd-explore, sdd-groom)
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

## Git rules

- **Never commit compiled binaries.** `bin/sdd` is in `.gitignore` and must stay there. Rebuild locally.
