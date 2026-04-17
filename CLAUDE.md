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
- `./bin/sdd status` — smoke-test the binary against the graph at `.sdd/graph/`

## Architecture

- **Library first**: Domain logic (parsing, graph construction, querying) lives in `internal/`. `cmd/sdd/` is a thin shell that wires flags to library calls and formats output. Keep business logic out of command actions.
- **CQRS separation**: `internal/finders/` (data queries) and `internal/presenters/` (view rendering) are distinct packages; handlers and the CLI wire them together. Don't collapse reads and views.
- **Push logic down**: Finders and handlers are orchestration layers — they wire dependencies and delegate. Graph traversal, tree building, filtering, and any pure computation belongs in `internal/model/` (no I/O, no dependencies). Always question whether code in a finder/handler could live in a lower package.
- **Single path**: I/O functions (file loading, etc.) should delegate to in-memory constructors. Don't duplicate indexing or initialization logic between production and test code paths.

## Structure

```
sdd/
├── cmd/sdd/                # CLI entrypoint (main.go)
├── internal/
│   ├── command/            # Command-level types (Init, NewEntry, etc.)
│   ├── finders/            # Data queries (reads)
│   ├── handlers/           # Orchestration wiring
│   ├── llm/                # Pre-flight + summarization via LLM
│   ├── meta/               # Config resolution
│   ├── model/              # Pure graph/entry types (no I/O)
│   ├── presenters/         # View rendering (writes)
│   └── query/              # Read query types
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
