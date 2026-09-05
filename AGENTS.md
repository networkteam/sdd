# AGENTS.md

Canonical, agent-neutral guidance for working on the SDD (Signal-Dialogue-Decision) framework. Codex and other `AGENTS.md`-aware tools read this directly; Claude Code reads it via the `@AGENTS.md` import in [CLAUDE.md](CLAUDE.md). Anything Claude-Code-specific lives in `CLAUDE.md` after that import.

## Project overview

SDD is a CLI-driven framework for building and traversing decision graphs through human-agent dialogue. This repo contains the Go implementation and the agent skill definitions (rendered per agent from one neutral template source). See [docs/story.md](docs/story.md) for the conceptual narrative and [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) for the framework model.

## Setup

The project uses Devbox + direnv for the toolchain (Go 1.26, GNU sed, etc.). In a fresh clone:

```bash
direnv allow                  # loads devbox env; wires the git hooks (core.hooksPath)
devbox run build              # build fresh local sdd dev version
sdd init --scope project      # render skills for the supported agents from the embedded bundle
```

The `sdd` binary lives at `./bin/sdd` (gitignored — rebuild locally with `devbox run build`, never commit, added to PATH via devbox.json, so call it via `sdd`). Git hooks under `.githooks/` (wired via `core.hooksPath` on `direnv allow`) auto-rebuild it after every pull, rebase, and branch switch — but you still rebuild manually after editing source. `sdd init` is idempotent: on a fresh checkout it creates `.sdd/meta.json` and installs skills; on subsequent runs it refreshes whatever drifted.

## Commands

- `devbox run build` — build the local `bin/sdd` dev binary (git hooks also run this after pull/rebase/checkout)
- `go vet ./...` — compilation + correctness check (never use `go build` just to verify compilation — it produces no output on success)
- `devbox run test` — run all tests: the root module and the separate `examples/extendingsdd` module. Never `go test ./...` alone — `./...` prunes the nested example module (a `go.work` does not change that), so its drift surfaces only in CI.
- `go test -tags=eval -run TestPreflightEval ./internal/llmops/...` — pre-flight prompt calibration eval (live `claude` CLI, slow + paid; model via `SDD_EVAL_MODEL`, default `sonnet`). Capture full output to a file and grep the file — `… -v 2>&1 | tee /tmp/eval.log` — never filter the live stream, or a failure shows no findings and forces a costly re-run.
- `go fmt ./...` — format code
- `devbox run lint` — lint (must be clean; CI enforces). Convention findings print as warnings without failing the run — see `scripts/lint.sh` for the mechanism. Use the wrapper, not bare `golangci-lint run`, which fails on warnings too.
- `devbox run validate-skills` — validate the rendered Codex skills under `.agents/skills/` against the Agent Skills standard (`uvx skills-ref@0.1.1 agentskills validate`, managed via the devbox `uv` package). Requires a Codex render present (i.e. `codex` in `supported_agents`).
- `sdd view` — smoke-test the binary against the graph at `.sdd/graph/`
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

- **Type-system plan capture surfaces presentation impact.** When drafting a plan that changes entry types, kinds, or frontmatter, raise downstream presentation surfaces (`sdd view`, `sdd show`, catch-up narrative, skill rendering) in dialogue. The user decides whether to add ACs for each surface or explicitly carve them out as future work; the agent helps compose once the call is made. Raising this at plan time avoids rediscovery during unrelated execution.

- **Push logic down**: Finders and handlers are orchestration — they wire dependencies and delegate. Graph traversal, tree building, filtering, and any pure computation belongs in `internal/model/`. Always question whether code in a finder/handler could live in a lower package.

- **Single path**: I/O functions (file loading, etc.) should delegate to in-memory constructors. Don't duplicate indexing or initialization logic between production and test code paths.

- **Minimal comment hygiene**: A comment earns its place only by carrying a *why* the code cannot show. Default to none; when one is warranted, one line is perfectly fine.
  - **No history.** Don't narrate how the code used to be ("previously…", "with the old X…", "changed from Y"). Git carries that.
  - **No duplication — comments are subject to DRY.** Never restate what the code already says, and never re-explain a concept or decision that is stated once elsewhere (a doc comment on the type, a graph entry, `AGENTS.md`). Duplicated prose drifts out of sync with the code it shadows; that drift is debt. State it in one place and let the reader find it there — reference an entry ID or type name instead of paraphrasing it. This holds across a change set too: when one change touches several files, its why lands in one of them and the other sites point — writing each file's comment in isolation is how the same story gets told four times.
  - **Keep it small.** No section banners, no restating the signature, no step-by-step narration of the lines below. Prefer a shorter comment over a fuller one; prefer deleting it over shortening it if the code now speaks for itself.

- **Logging**: Use `log/slog`; retrieve the logger via `slogutils.FromContext(ctx)` (from `github.com/networkteam/slogutils`). Handler entry points take `ctx` and pull the logger from it — do not pass loggers as separate arguments, and do not use `fmt.Fprintf(h.stderr, ...)` for operational messages. Stderr writes are reserved for user-facing CLI output that isn't logging (prompts, structured CLI results).

- **Frame cost as maintenance surface and dependencies, not time**: When weighing a technical option (a library, an integration, an adapter), don't estimate effort in hours or days — that's speculation. State what ongoing maintenance surface it opens (code you own, protocols/APIs you must track) and what dependencies it pulls in — how heavy, and how reversible (vendorable? official vs community?). Those are the durable cost signals.

- **Fail loud — never swallow errors into silent fallbacks**: Catch errors early and propagate them to a caller that can act; never degrade a failure to a stderr `warning:` (or any other fallback) that lets execution continue as if nothing broke. This includes background side effects like the git auto-commit: a failed or timed-out commit returns an error — the entry file stays on disk for durability, but the failure surfaces — it is not printed and swallowed.

- **Test the exported surface**: Test files use the external test package (`package foo_test`) and exercise exported types, functions, and methods — that keeps tests honest about the API and free to refactor internals. An internal test is the exception, not the default: it needs an unexported seam that genuinely cannot be reached through the API, and it lives in a file named `*_internal_test.go` so the exception is visible. (Much existing code predates this rule — follow it for new test files, and prefer converting when touching old ones.)

- **Tests live at the layer that owns what they assert, and no test fakes another layer's contract** (graph entry 20260819-152950-d-prc-h1m carries the full reasoning): `internal/engine` tests engine semantics over synthetic specs its test files own; `internal/proctest` tests shipped-procedure behavior against the real application (real registry, real ops, scripted LLM); `application` keeps unit tests for its own surfaces; `mcpapp` tests transport concerns.

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
│   ├── llm/                # Provider adapters (claude, gollm, embed) and the config-driven factories that compose pkg/llm and pkg/llm/embed
│   ├── llmops/             # The LLM operations: pre-flight, summarize, writing guide, with their prompts
│   ├── llmstats/           # FileSink/Reader for .sdd/stats/llm.jsonl (owns the wire shape) and the debug-logging sink
│   ├── git/                # Git adapter: exec-based implementations of the consumer-defined git interfaces (handlers.Committer/…, finders.GitSyncer, repos.Git)
│   ├── repos/              # Connected repos for cross-repo refs: Locations (explicit paths), Registry (pure reads → finders), Manager (clone/pull/config writes → handlers)
│   ├── meta/               # Config resolution
│   ├── engine/             # Workflow engine: procedure specs, typed store, choosers, session event log
│   ├── proctest/           # Integration harness + per-procedure behavior suites over the real application
│   └── bundledskills/      # Skill source of truth (agent-neutral templates), embedded via //go:embed
│       └── templates/      # Neutral *.md.tmpl skill tree, rendered per agent (sdd, sdd-catchup, sdd-explore, sdd-groom)
├── pkg/                    # Exported packages — the only public Go surface (d-tac-zhc)
│   ├── llm/                # Public LLM contract: Runner over Request/Result/Identity/Usage (d-cpt-q6n)
│   ├── application/        # Protocol-neutral runtime + composition root (CLI and MCP are shells over it)
│   ├── local/              # Filesystem and in-memory adapters
│   ├── mcpapp/             # Shared MCP handler surface
│   └── sddtest/            # Reusable adapter conformance suites
├── .claude/skills/         # Installed Claude render for this repo (rebuilt from internal/bundledskills/templates)
├── .agents/skills/         # Installed Codex render for this repo (Agent Skills standard; same template source)
├── .sdd/
│   ├── config.yaml         # SDD config (graph_dir, supported_agents, etc.)
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
- **Finding entries: use `sdd search`, not grep.** To locate graph entries (including when delegating to subagents), use `sdd search` (vector or hybrid retrieval) — it matches semantically across summaries and bodies. Reserve `grep`/`ripgrep` for **source code**; `sdd search` only indexes graph entries, not Go source.
- **A delegate's report is a summary, not a source.** A subagent's research report has the same status as an entry summary: a pointer. Read the entries it names in full (`sdd show`) before reasoning, planning, or dialoguing on them — a report can misstate what an entry settles.

## Skill source of truth

Skills are **source-of-truth as agent-neutral templates in `internal/bundledskills/templates/`** (`*.md.tmpl`, Go `text/template` with default `{{ }}` delimiters) and compiled into the binary via `//go:embed`. `Load(target)` renders them per agent — each agent ("claude", "codex") is a render profile, not a source directory. The copies under `.claude/skills/` (Claude Code) and `.agents/skills/` (Codex) are the _installed_ per-agent renders for this repo — what each agent loads during a session.

- **Never edit the installed renders (`.claude/skills/`, `.agents/skills/`) directly.** Changes made there will be overwritten (or flagged as "modified") the next time `sdd init` runs.
- Edit the templates in `internal/bundledskills/templates/<skill>/` → rebuild (`devbox run build`) → reinstall (`sdd init --scope project`). The installed copies pick up the new render and refresh with fresh `sdd-version` + `sdd-content-hash` stamps. Per-agent deviations are `{{ if eq .Agent "claude" }}…{{ else }}…{{ end }}` conditionals and the `inject` helper — never duplicated files. Literal `{{ }}` in skill prose (e.g. the attachments placeholder) must be escaped as `{{"{{...}}"}}`.
- Codex skills must conform to the Agent Skills standard (stamps under `metadata:`, `compatibility:` set, no Claude-only frontmatter keys). After changing a SKILL.md template, run `devbox run validate-skills` to confirm the Codex render still validates.
- Skill changes include both template source and installed renders. Commit template-source changes before running `sdd init`, which auto-commits the installed renders.

## Git rules

- **Use Conventional Commits for every new non-merge commit not created by SDD tooling.** Use a lowercase type, an optional scope in parentheses, a colon and an imperative subject, for example `feat: add search synchronization` or `fix(search): verify returned candidates`. Types in use: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `build`. Reserve the `sdd` prefix exclusively for commits created by SDD tooling; never use it for manual or agent-authored commits. Keep the body short: when the why lives in the SDD graph, reference the entry by its full ID instead of repeating the reasoning. Merge commits may retain Git's generated merge message.
- **Never commit compiled binaries.** `bin/sdd` is in `.gitignore` and must stay there. Rebuild locally.
- **Stage and commit by explicit path.** Never `git add -A` or `git add .`. For non-merge manual commits, use `git commit -- path/to/file …` so unrelated staged changes from concurrent sessions or WIP markers are excluded.
- **Never rewrite history; recover only by adding commits.** Preserve every existing commit's hash and message, including SDD auto-commits. No reset, rebase, amend, force-push or squash, through Git or a forge. Concurrent sessions can advance `main` between commands. If a commit lands wrong, preserve it, report the mistake and obtain the user's direction before adding a corrective commit.
- **Land a branch with a plain local merge; the push is the user's.** In the main checkout, run `git merge <branch>`, then say it's ready. Never land through a forge.
- **Trust `sdd`'s auto-commit.** `sdd new`, `sdd summarize`, and `sdd init` make their own `sdd: …` commit. If the command exits without error, the entry and its commit are done — do not re-run `git status` / `git log` to confirm. The only thing worth reading back is the generated summary (for fidelity), not the commit.

## Writing

All dialogue with the user and every graph entry follows the `unslop` skill — load it before writing either. It carries the goal (the tersest text that carries the meaning exactly), how the dialogue and graph-entry registers differ, and the pattern catalog (Christopher, 2026-08-24).

Keep detailed Go API behavior and usage beside the code in package or symbol documentation. Use Go doc examples when they clarify usage. READMEs give an overview and point to those docs; do not duplicate API details there. Add only the documentation the change needs.

## Memory

**Do not keep a parallel memory store for this project.** This repo dogfoods SDD — we use SDD to develop SDD. A separate agent memory (auto-memory, a side notes file, a scratch store) creates a parallel record that shortcuts what the graph and skill dialogue are supposed to handle, and it contaminates the evaluation of whether SDD itself is guiding the work well. If memory quietly carries the context that SDD should be surfacing through signals, decisions, and skill guidance, we lose the signal about where the framework is falling short.

**No exceptions, and no separate store.** When you would reach for a memory note — a working preference, a recurring habit, a cross-session note — update `AGENTS.md`/`CLAUDE.md` instead (or capture it in the graph when it is SDD substance). This holds even for things that feel "general" or unrelated to SDD: if it surfaced while working here, it lands in these instruction files, not in a parallel memory store. Don't carve out a "this one's just a communication tic" exception — that loophole defeats the purpose. These instruction files and the graph are the only durable records.

## External material

This repo is public. Anything from another project — private, closed, or to be treated as such — is read-only input: session transcripts, handover files, feedback, graphs, code. Analyse it in the session, keep any working copy in the scratchpad, never inside the repo.

Nothing durable here may identify it. Graph entries, docs, code, comments, commit messages: no project, company, person, or path names, no internal specifics. Capture the pattern, not the case — generalised, or under a pseudonym when you need a handle to refer back to it.
