# Framework

Go module for the SDD decision graph tooling.

## Setup

The `sdd` binary is not checked into git. In a fresh clone or worktree, build it first:

```bash
cd framework
go build -o bin/sdd ./cmd/sdd
```

## Build

Rebuild after changing Go source files:

```bash
cd framework
go build -o bin/sdd ./cmd/sdd
```

## Check

Use `go vet` to check compilation and correctness — never use `go build` just to check compilation (it produces no output on success):

```bash
cd framework
go vet ./...
```

## Test

```bash
cd framework
go test ./sdd/...
```

Unit tests live alongside the code in `sdd/`. Tests should use exported helpers and constructors (e.g. `NewGraph`, `ParseID`) from the library — never duplicate parsing or indexing logic in test code.

## Architecture

- **Library first**: Domain logic (parsing, graph construction, querying) lives in the `sdd/` package. The CLI (`cmd/sdd/`) is a thin shell that wires flags to library calls and formats output. Keep business logic out of command actions.
- **Single path**: I/O functions (file loading, etc.) should delegate to in-memory constructors. Don't duplicate indexing or initialization logic between production and test code paths.

## Structure

```
framework/
├── cmd/sdd/       # CLI entrypoint (main.go)
├── sdd/           # Core library: entry parsing, graph loading, querying
│   ├── entry.go   # Entry type, frontmatter parsing, ID generation
│   ├── graph.go   # Graph loading, indexing, query methods
│   └── graph_test.go
├── bin/sdd        # Pre-built binary (checked in)
├── go.mod
└── go.sum
```

## Key decisions

- Graph entries are immutable markdown files with YAML frontmatter in `docs/framework/graph/`
- Hierarchical directory layout: `graph/YYYY/MM/DD-HHmmss-type-layer-suffix.md`
- The CLI defaults to `--graph-dir docs/framework/graph` (relative to cwd, typically repo root)
- Three entry types: signal (`s`), decision (`d`), action (`a`)
- Five layers: strategic (`stg`), conceptual (`cpt`), tactical (`tac`), operational (`ops`), process (`prc`)
- ID format: `{YYYYMMDD}-{HHmmss}-{type}-{layer}-{suffix}` (full ID used everywhere, path derived from it)
