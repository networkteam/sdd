# SDD

**signal · dialogue · decision**

**SDD is agent-native memory for your project's decisions and the why behind them.** Your agents get exactly the project context they need: what was noticed, discussed and decided. New knowledge is captured in dialogue with the people who create it, as the work happens.

SDD records your project's reasoning as an immutable decision graph of signals (what you noticed) and decisions (what you committed to). At any moment, anyone, human or agent, can see what's in flight, which decisions are active and what's still open. Built for developers and teams shipping with AI agents.

This repository is the open source core of SDD: the `sdd` binary, its MCP server and the Go packages to embed it.

**Documentation: [networkteam.github.io/sdd](https://networkteam.github.io/sdd/)**

## Why SDD

**Without SDD:** the reasoning from each agent session, what you noticed, what you decided and why, is lost once the session ends.

**Most other systems** either record everything automatically, a searchable pile of what was said rather than a decision you can trust, or scatter state across static artifacts (docs, plans, roadmaps, tickets) that duplicate across layers and drift.

**With SDD:** every entry is a confirmed record of what you decided and why: reasoning, not a recording. New insight becomes a new entry, deliberately captured, labeled and connected to the reasoning already there, so every capture makes the graph **richer, not just bigger**. State is _derived from the graph_, never maintained in parallel, and tracked in Git so humans and agents work against the same view.

## How you use it

You work in Claude Code or Codex through the `/sdd` skill. Your agent talks to the `sdd` server, which serves the work step by step: check-ins, capturing what you notice, decisions, implementation, evaluation and grooming, all through dialogue. Nothing lands in the graph before you confirm it. [Working modes](https://networkteam.github.io/sdd/working-modes/) describes each move.

## How it feels

The screencast walks through a recipe-app workflow: the importer's been making duplicate recipes on re-import, and the variants model that would settle it is undecided. Watch the gap get captured, the decision get made, and the implementation land, all through dialogue.

![SDD in action: variants, importer, end to end](docs/assets/screencast.gif)

## Philosophy

SDD lets each party do what they're good at. Agents work with information and run autonomously where they can: reading the graph, drafting entries, running operational steps. Humans make the calls, provide taste and raise observations data can't surface. Dialogue, not bureaucracy, moves the graph forward.

## What's here today

- An append-only graph of signals and decisions, with seven signal kinds and eight decision kinds
- First-class participants: actors and the roles they play on the project
- Topics that cluster entries across kinds and layers
- Focus decisions that commit the project's attention to a set of entries for a period
- Model checks on every capture: a writing check on the draft and a pre-flight check against the graph
- Search by keyword, by meaning, or both
- Connected repositories: reference, search and traverse entries across graphs
- Composable views that filter, rank and render the graph
- Multilingual graphs: dialogue in any language, entries in the graph's configured one
- Git-native and immutable, with background awareness of remote state
- Eight moves: bootstrap, catch-up, capture, interview, engage, implementation, evaluate, groom

## Install

With Homebrew:

```bash
brew install networkteam/tap/sdd
```

Or with the install script, which installs to `~/.local/bin/sdd` without `sudo`. Re-run it to upgrade; `-b <dir>` changes the location.

```bash
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh | sh
```

<details>
<summary><strong>Verify the installer</strong> for CI or security-conscious setups</summary>

Every release carries build-provenance attestations from GitHub Actions. Verify the installer with the `gh` CLI before running it:

```bash
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh -o install.sh
gh attestation verify install.sh --repo networkteam/sdd
sh install.sh
rm install.sh
```

</details>

Upgrading from v0.17.0 or earlier: run `sdd init` once in each project. It replaces the old skill files with the engine-driven `/sdd`, keeping and naming any copy you edited.

## Quickstart

Connect a language model, for example the Anthropic API, then set up SDD in your git repository:

```bash
sdd config set llm.provider anthropic
sdd config set llm.model claude-sonnet-4-6
sdd config set llm.api_keys.anthropic <your-api-key>

sdd init
```

Open Claude Code or Codex in the project and type `/sdd` (Codex: `$sdd`). On a new project your agent suggests bootstrap, the dialogue that founds the graph.

[Getting started](https://networkteam.github.io/sdd/getting-started/) walks through each step, including other model providers, search by meaning and your first capture.

## Documentation

- [Getting started](https://networkteam.github.io/sdd/getting-started/): install, set up a project, hold your first dialogue
- [Working modes](https://networkteam.github.io/sdd/working-modes/): the moves your agent offers and when to use each
- [Entry kinds](https://networkteam.github.io/sdd/reference/entry-kinds/): what each kind of entry is for
- [Configuration](https://networkteam.github.io/sdd/reference/configuration/): every setting, model and embedding providers
- [Connected repositories](https://networkteam.github.io/sdd/guides/connected-repositories/): work across several graphs
- [Browsing the graph](https://networkteam.github.io/sdd/guides/browsing-the-graph/): read the graph yourself from the terminal
- [Vision](https://networkteam.github.io/sdd/vision/landscape/): where SDD stands and where it is heading

## Develop, embed, contribute

### Build from source

Requires Go 1.26.

```bash
git clone https://github.com/networkteam/sdd.git
cd sdd
direnv allow                    # loads Go via Devbox (optional)
go build -o bin/sdd ./cmd/sdd
```

The binary ends up at `./bin/sdd`. Put it on your `PATH` as `sdd`, since your agent starts the server as `sdd serve`.

### Embed SDD in a Go application

`github.com/networkteam/sdd/pkg/application` exposes the protocol-neutral runtime and infrastructure ports. `github.com/networkteam/sdd/pkg/local` provides optional filesystem and in-memory adapters, while `github.com/networkteam/sdd/pkg/mcpapp` mounts the application as the shared MCP handler surface. An embedding application keeps ownership of authentication, project authorization, storage, model providers, HTTP server lifecycle, and deployment policy. The module root is documentation-only; public symbols have one canonical package owner.

See [`examples/extendingsdd`](examples/extendingsdd) for a nested external module that implements the public ports and mounts `mcpapp.Server.Handler()` behind bearer middleware. The reusable adapter conformance suites live in `github.com/networkteam/sdd/pkg/sddtest`.

### Contribute

SDD is developed with SDD: this repository's own graph lives in `.sdd/graph`. [AGENTS.md](AGENTS.md) carries the setup, commands and conventions for people and agents working on it.

## Star the repo

If SDD resonates, or if you're curious how the graph evolves, starring the repo helps others find it and lets you follow progress.

## License

MIT, see [LICENSE](LICENSE).
