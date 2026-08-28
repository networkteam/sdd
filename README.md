# SDD

**signal · dialogue · decision**

**SDD is agent-native memory for your project's decisions and the why behind them.** Your agents get exactly the project context they need — what was noticed, discussed, and decided — and new knowledge is captured in dialogue with the people who create it, as the work happens.

SDD records your project's reasoning as an immutable decision graph — signals (what you noticed) and decisions (what you committed to). At any moment, anyone (human or agent) can see what's in flight, which decisions are active, and what's still open. Built for developers and teams shipping with AI agents.

## Why SDD

**Without SDD:** the reasoning from each agent session — what you noticed, what you decided, and why — is lost once the session ends.

**Most other systems** either record everything automatically — a searchable pile of what was said, not a decision you can trust — or scatter state across static artifacts (docs, plans, roadmaps, tickets) that duplicate across layers and drift.

**With SDD:** every entry is a confirmed record of what you decided and why — reasoning, not a recording. New insight becomes a new entry — deliberately captured, labeled, and connected to the reasoning already there, so every capture makes the graph **richer, not just bigger**. State is _derived from the graph_, never maintained in parallel, and tracked in Git so humans and agents work against the same view.

## How you use it

You work in Claude Code or Codex through the `/sdd-engine` skill. `sdd init` registers the `sdd` MCP server for the configured agents, and the engine serves the session step by step — check-in, capture, decisions, evaluation, grooming — all through dialogue. The `sdd` CLI underneath stores entries and derives state from the graph; you rarely invoke it directly. The older skill-driven `/sdd` flow still ships but is deprecated: v0.18.0 will remove it and rename `/sdd-engine` to `/sdd`.

## How it feels

The screencast walks through a recipe-app workflow: the importer's been making duplicate recipes on re-import, and the variants model that would settle it is undecided. Watch the gap get captured, the decision get made, and the implementation land — all through dialogue.

![SDD in action — variants, importer, end to end](docs/assets/screencast.gif)

## Philosophy

SDD lets each party do what they're good at. Agents work with information and run autonomously where they can — reading the graph, drafting entries, running operational steps. Humans make the calls, provide taste, and raise observations data can't surface. Dialogue, not bureaucracy, moves the graph forward.

## Install

### Homebrew (recommended)

```bash
brew install networkteam/tap/sdd
```

Works on macOS and Linux (Homebrew on Linux). Updates via `brew upgrade sdd`.

### Curl installer

For environments without Homebrew:

```bash
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh | sh
```

Installs to `~/.local/bin/sdd` by default (XDG-compliant, user-scoped — no `sudo`). Re-run to upgrade. Pass `-b <dir>` to change install location.

<details>
<summary><strong>Verify the installer</strong> — for CI or security-conscious setups</summary>

Build-provenance attestations are produced by GitHub Actions on every release. Verify before execution with the `gh` CLI:

```bash
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh -o install.sh
gh attestation verify install.sh --repo networkteam/sdd
sh install.sh
rm install.sh
```

This confirms the installer was signed by the `networkteam/sdd` release workflow via GitHub's native artifact attestations (Sigstore-backed).

</details>

<details>
<summary><strong>Build from source</strong> — requires Go 1.26</summary>

```bash
git clone https://github.com/networkteam/sdd.git
cd sdd
direnv allow                    # loads Go via Devbox (optional)
go build -o bin/sdd ./cmd/sdd
```

The binary ends up at `./bin/sdd`. Add it to your `$PATH`.

</details>

## Quickstart

### 1. Run `sdd init`

Whether you cloned an SDD-instrumented repo or you're adding SDD to a project from scratch, run `sdd init` in the project root:

```bash
sdd init
```

For a cloned repo it reads the recorded config and installs the skills for the configured agents at the same place every contributor uses. For a fresh project it prompts for the basics — graph directory, authoring language, which agents to render skills for, where to install them, your participant name — and writes the config.

See [Configuration](#configuration) for re-running, version bumps, and non-interactive flags.

### 2. Start a session

In **Claude Code**, run:

```
/sdd-engine
```

In **OpenAI Codex**, invoke the skill:

```
$sdd-engine
```

The engine opens the session with an orientation — where the project stands and the moves available. Everything after that is dialogue. See [Multiple agents](#multiple-agents) for how the same skill source renders to each.

<details>
<summary><strong>Deprecated:</strong> the skill-driven <code>/sdd</code> flow</summary>

The original `/sdd` skill (`$sdd` on Codex) still works in v0.17.0 and opens with the same graph state. It is deprecated: v0.18.0 will remove the legacy skills and rename `/sdd-engine` to `/sdd`.

</details>

## What's here today

- An append-only graph of signals and decisions, with seven kinds each
- First-class participants — actors and the roles they play on the project
- Topics that cluster entries across kinds and layers (nested tags)
- Focus decisions that commit the project's attention to a set of entries for a period, with actors assigned per target
- A pre-flight validator (LLM based) — reviews a capture before it lands
- Three-mode search: keyword, semantic (vector search), or a hybrid that fuses both
- Cross-repo references — connect another graph and reference, search, and traverse across repository boundaries
- Composable views — filter, rank, and render the graph to make it accessible for the agent
- Mining external material — transcripts, articles, meeting notes — into the graph through dialogue
- Multilingual graph authoring, with translated SDD vocabulary in the skill
- Git-native and immutable, with background awareness of remote state
- A skill covering the common moves: bootstrap, catch up, capture, decide, engage, groom, augment, explore

## Embed SDD in a Go application

`github.com/networkteam/sdd/application` exposes the protocol-neutral runtime and infrastructure ports. `github.com/networkteam/sdd/local` provides optional filesystem and in-memory adapters, while `github.com/networkteam/sdd/mcpapp` mounts the application as the shared MCP handler surface. An embedding application keeps ownership of authentication, project authorization, storage, model providers, HTTP server lifecycle, and deployment policy. The module root is documentation-only; public symbols have one canonical package owner.

See [`examples/extendingsdd`](examples/extendingsdd) for a nested external module that implements the public ports and mounts `mcpapp.Server.Handler()` behind bearer middleware. The reusable adapter conformance suites live in `github.com/networkteam/sdd/sddtest`.

## Where this is heading

Today SDD lives in the terminal and assumes a developer at the keyboard. We want anyone on the project — a designer reviewing a flow, an operator triaging an incident, a business owner weighing a trade-off — to engage the graph directly, through chat, voice, or whatever interface fits their work. Capture should happen inside that work — a finding while testing, a question raised in a meeting — without stepping into a separate mode for it. And AI agents should work autonomously, guided by the decisions and guidelines already in the graph, while humans focus on reviewing what comes back and steering what comes next.

SDD also shouldn't be tied to one agent harness. It runs in Claude Code and Codex today; the direction is to let any agent or runtime drive the graph over an open interface (MCP), so the choice of tool never locks you in.

## What a session looks like

A session covers a handful of common moves. You don't pick a mode — you just talk, and the dialogue routes itself.

### Bootstrap

```
> /sdd-engine

Claude: This graph is empty. Want me to walk through bootstrap?
        We'll capture who's on the project and what it's pulling
        toward — gives the rest of the work something to anchor on.
```

Bootstrap walks you through capturing your project's actors (participants) and strategic frame. After that, every later session has identity and direction to thread against.

### Catch up

Every session starts here. The check-in renders a colleague-style briefing — what's in flight, what changed recently, what wants your next move.

```
> /sdd-engine

Claude: *Current focus: get the importer unblocked.*

        **Variants picked, importer un-parked**

        Last session closed d-cpt-vr4 — copy-on-edit variants with
        derivedFrom. Importer can now ask "duplicate or new copy".

        1. Wire the importer prompt against the new schema (d-ops-...).
        2. Backfill the sync log for existing recipes.

        **Shopping list still warm**

        d-cpt-8hr is ready to pick up.

        3. Implement shopping-list aggregation.

        What do you want to move forward?
```

### Capture

Signals usually come up while you're in the work — testing what was just built, noticing a gap, hitting a trade-off. Capturing keeps the insight durable.

```
> The importer chokes on recipes with no servings field.

Claude: [plays back an operational gap signal — "importer rejects
         recipes without 'servings'." Refs d-cpt-vr4. Confidence high.]

> Looks right.

Claude: [records the gap in the graph]
```

Before writing, Claude plays back the proposed entry. A pre-flight validator (LLM-based, invoked by the CLI) reviews the draft against its refs and flags contradictions, missing acceptance criteria on plans, unrelated references, and similar calibration gaps. `high` findings block the write; `medium` and `low` are advisory. It works as a required gate to make sure entries are consistent and follow contracts and SDD rules.

Capture also works on material from elsewhere — a transcript, an article, a meeting note. The agent reads it against the graph, ties passages to existing entries, and synthesizes with you what's actually new — facts the source adds, insights the connection produces, gaps it raises, decisions the source records. Those become entries.

### Plan

Plan decisions (`--kind plan`) carry an `## Acceptance criteria` section — each AC is a verifiable outcome that becomes the contract between plan author, implementing agent, and the pre-flight validator that checks the closing done signal.

```
> Let's plan copy-on-edit variants.

Claude: [drafts d-cpt-vr4 — kind plan, six ACs covering the
         derivedFrom FK, the importer prompt, orphan-on-delete,
         the flat sync log, migration backfill, and tests.
         Closes s-cpt-v3n. Confidence high.]

> Looks right.

Claude: [records the plan in the graph]
```

Claude can attach a longer design doc for plans that need more than the ACs to carry.

### Engage

Engage anchors on an entry — implementation, research, or evaluation, depending on what the entry calls for.

```
> Engage d-cpt-vr4.

Claude: [reads the entry and its chain, surfaces the design
         questions still open in the ACs, suggests a start point]

         AC 3 — orphan-on-delete — has a UI question buried in it.
         Capture that first, or proceed and address it inline?
```

For implementation, a WIP marker signals the work is in flight. When the work finishes, Claude plays back the closing done signal, captures it, and takes the marker down. The entry chain can ride a git branch when multiple participants work in parallel.

### Groom

```
> Let's groom.

Claude: [sweeps the graph — returns a table of candidates: stale
         entries, missing `closes` links, superseded-in-practice
         relationships — then walks through them one at a time]
```

The goal is to keep the graph's derived state reflecting reality.

## Concepts in a minute

**The loop**: Signal → Dialogue → Decision → Done signal → Signal...

Dialogue is the work that turns signals into decisions — it isn't recorded directly. Everything else lives in the graph as an immutable markdown entry with YAML frontmatter.

**Entry types** (two; kinds carry the nuance):

- **Signal** (`s`) — something noticed. Kinds: `gap` (default), `fact`, `question`, `insight`, `done`, `actor`, `annotation`. A `done` signal closes the decision it fulfils.
- **Decision** (`d`) — something committed to. Kinds: `directive` (default), `activity`, `plan`, `contract`, `aspiration`, `role`, `focus`.

**Actors and roles**

Participants are first-class entries. An actor signal records who someone is (canonical name plus aliases the agent should recognize). A role decision records what they do on the project. Roles bind to an actor by canonical name; retire an actor and their roles cascade closed.

**Topics**

Refs link entries one to one, but projects develop broader threads too — a domain, a recurring concern, a thematic cluster — that link-by-link doesn't reveal. Topics layer a second graph on top: hierarchical tags (like `infrastructure/cli`) grouping entries by theme. They make quieter parts of the graph discoverable on catch-up and give the agent a stable handle for drilling into a thread.

**Focus**

The graph grows in every direction, with no built-in priority — every entry stands equal. A focus decision adds that priority: a deliberate commitment to drive one specific area, at whatever cadence fits (a quarter, a cycle, a week). It connects the entries in scope with the actors on each and a time frame, so the graph becomes navigable — anyone joining sees what's being driven and where to engage.

**Layers** describe depth of thinking, not org level:

| Layer       | Abbrev | Thinking                 |
| ----------- | ------ | ------------------------ |
| Strategic   | `stg`  | Why does this exist?     |
| Conceptual  | `cpt`  | What approach?           |
| Tactical    | `tac`  | Structure and trade-offs |
| Operational | `ops`  | Individual steps         |
| Process     | `prc`  | How we work              |

**Links between entries**:

- `refs` — builds on / depends on (no status effect)
- `supersedes` — replaces; older entry no longer active
- `closes` — resolves / fulfils; signal or decision now closed

**Immutability**: entries are never edited. State is derived by traversing the graph, not by mutating files. To change direction, add a new entry that supersedes an old one.

See [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) for the full framework model and [docs/story.md](docs/story.md) for a fictional story of how SDD could work in the future.

## Multilingual graphs

Each graph has a single authoring language configured as `language: <locale>` in `.sdd/config.yaml` (set at `sdd init` time, default `en`). When the language is non-English:

- **Captured entry descriptions are written in the configured language.** Dialogue with the agent can flow freely in any language, but the text that lands in the graph is canonicalized — the `/sdd` skill translates dialogue content before running `sdd new` so the graph stays coherent across sessions. Pre-flight enforces this: an entry whose description language doesn't match is flagged as drift and blocks capture.
- **The `/sdd` skill renders translated SDD vocabulary** (types, kinds, layers, status labels) on demand, reading `references/vocabulary-<locale>.md`. Catch-up narration, playback, and grooming tables use translated terms.
- **The technical surface stays English.** YAML frontmatter, CLI tokens, entry IDs, and section headers like `## Acceptance criteria` are canonical identifiers. CLI output (`sdd info`, `sdd view`, `sdd show`) also stays English — translation is a skill concern, not a CLI concern.

German (`de`) is the only bundled locale today.

**Adding a locale locally** — drop `vocabulary-<locale>.md` into your installed skill tree (`~/.claude/skills/sdd/references/` for user scope, `.claude/skills/sdd/references/` for project scope). Follow the German file's structure. `sdd init` refreshes bundled files but leaves user-added files alone.

**Contributing a locale upstream** — drop the file into `internal/bundledskills/claude/sdd/references/` and submit a PR.

## Connected repos

Reasoning doesn't always stay in one repo. A service's decision might build on a platform team's directive; a project might ground its choices in a shared framework graph. SDD lets an entry reference across repository boundaries, so that reasoning connects instead of being silently re-derived or reduced to a local paraphrase.

**Repo identity.** Each graph has a canonical, URL-shaped identity — its `repo_id` (e.g. `github.com/acme/platform`), derived from the origin remote and recorded in `.sdd/config.yaml` at `sdd init` time. A cross-repo reference is written as `<repo-id>:<entry-id>`: the target repo's identity, then the entry within it.

**Connecting a repo.** To reference another graph, connect it once:

```bash
sdd repo add https://github.com/acme/platform.git
```

This clones the repo's graph into a local cache, verifies the `repo_id` it declares, registers the connection, and records a committed **dependency** — a `dependencies:` entry in your `.sdd/config.yaml`, like a `go.mod` line. The dependency is the portable, shared record of what your graph reaches into; the connection (where the cache lives on your machine) stays per-contributor. `sdd repo add` commits the dependency declaration for you, so a colleague who clones your repo knows what to connect.

- `sdd repo list` — the repos you've connected, and whether each is cached.
- `sdd repo sync [repo-id ...]` — refresh connected caches to their latest pushed state (all repos, or the named ones).
- `sdd repo remove <repo-id>` — drop a dependency from `.sdd/config.yaml`. It refuses when any entry in your graph still references that repo (removing it would strand those references); `--force` overrides and names each reference it would strand. Removal is project-scoped — it leaves the local cache in place.

**Referencing across repos.** With a repo connected, refer to its entries by full ID (`github.com/acme/platform:20260709-...-d-cpt-abc`) or by a **bare ID** — a short form (`d-cpt-abc`) or an unprefixed full ID — which SDD resolves across the union of your local graph and its declared dependencies. A bare ID that matches exactly one entry resolves; a genuine collision across repos is reported with the candidates so you can disambiguate. `sdd show d-cpt-abc` resolves and displays a connected repo's entry the same way it does a local one. References stored on an entry are always written in full `<repo-id>:<entry-id>` form, so the committed graph stays explicit and portable. A cross-repo reference must resolve to a declared dependency when it's captured — the same resolve-or-block rule that guards local references — and `sdd lint` re-checks it afterward as a standing invariant.

**Searching across repos.** `sdd search` and `sdd view` reach connected graphs with `--repo <repo-id>` (repeatable) or `--all-repos`:

```bash
sdd search --query "index fingerprint" --all-repos
sdd view --layout='top(20)' --repo github.com/acme/platform
```

Results from every selected graph fuse into one ranked list. Cross-repo vector search uses your **user-global embedding config** as the single shared embedder, so every connected index lives in one vector space and rankings stay comparable across repos. **Configure your embedding provider at the global layer** for this to work — `sdd config set embedding.provider ...` (see [Configuration](#configuration)). A per-repo embedding override in `.sdd/config.yaml` defines a *different* vector space and is deliberately left out of cross-repo search (`sdd lint` flags the fingerprint mismatch). The first cross-repo search embeds a connected graph on demand, which can be slow; pre-index eagerly with `sdd index --repo <repo-id>` (or `--all-repos`) so it's warm, and `sdd index --repo <repo-id> --force` rebuilds a connected index that has drifted.

**The cache is pushed state.** A connected repo's cache is a clone of the remote's *pushed* branch — not anyone's working tree. So a new entry in a connected repo becomes referenceable only after it's committed **and pushed**, and your cache picks it up on the next `sdd repo sync` (cross-repo `sdd show` and `sdd search` also freshen caches as they run). The same holds in reverse: for others to reference your entries, commit and push them. This is the one non-obvious part of the model — when a cross-repo reference won't resolve, the target is usually just unpushed, or the local cache is stale.

## Configuration

SDD config is a layered overlay — each layer overrides the one below:

1. **User-global** — `~/.config/sdd/config.yaml` (or `$XDG_CONFIG_HOME/sdd/config.yaml`). Your personal defaults across every SDD project: participant name, LLM and embedding provider, sync cooldown. Set once, apply everywhere.
2. **Committed project** — `.sdd/config.yaml`, in git. Properties of the repo itself that every contributor shares: graph directory, authoring language, skill scope, supported agents, the repo's own `repo_id`, and connected-repo [dependencies](#connected-repos). It may also carry safe project-wide defaults for the personal settings above.
3. **Machine-local** — `.sdd/config.local.yaml`, gitignored. Per-machine overrides for this checkout: API keys, a local endpoint, a provider or participant override you don't want committed.
4. **CLI flags** — per-invocation, highest precedence.

Later layers win: a provider set project-wide overrides your global default, and a machine-local override wins over both. The split is by *whose fact each setting is*: repo-identity fields (`repo_id`, `dependencies`, `graph_dir`, `language`, `supported_agents`) belong in the committed project file and can't be set globally; personal preferences (participant, LLM, embedding, sync) default best at the user-global layer, with the local file for machine-specific overrides. A key placed in the wrong file fails loud — naming the file and key — rather than being silently dropped.

Inspect and edit config with `sdd config`:

```bash
sdd config                             # effective merged config, with each value's source
sdd config get llm.model               # a single effective value
sdd config set llm.provider ollama     # write to the user-global file
sdd config set --local embedding.api_keys.openai sk-...   # write to .sdd/config.local.yaml
```

`sdd config set` writes the user-global file by default, or the machine-local file with `--local`. The committed project file is written by `sdd init` (or edited by hand), so a project property change is a reviewed commit. The provider blocks below can live in any of the three files — put shared defaults in the project file and personal defaults (API keys included) global — keys never belong in the committed project file; `--local` covers a genuinely per-machine key.

### Re-running `sdd init`

`sdd init` is safe to run again — after a binary upgrade to refresh drifted skill files, after a colleague adds something to `.sdd/`, or just to confirm setup. Pristine skill files update silently; files you've edited are preserved (`--force` to overwrite).

When an upgrade leaves sessions in an older format, interactive init offers to migrate all of them after you confirm that no `sdd serve` process is actively using the repository. Declining leaves the sessions untouched and does not stop the rest of init; a later run offers again. For non-interactive use, `--migrate-sessions` is the explicit acknowledgement and opt-in.

Pass `--bump` from a released binary to raise `.sdd/meta.json`'s `minimum_version` — this locks older binaries out of the graph after a breaking change.

Run non-interactively by passing every required flag: `sdd init --scope project --participant <name> --language en`. Missing flags produce a single error naming what's still needed.

### Multiple agents

SDD runs on more than one agent harness. Skills are authored once as agent-neutral templates and rendered per agent into that agent's own committed directory:

- **Claude Code** — `.claude/skills/`, invoked as `/sdd`.
- **OpenAI Codex** — `.agents/skills/` (the open [Agent Skills standard](https://agentskills.io)), invoked as `$sdd`.

A committed `supported_agents` list in `.sdd/config.yaml` records which agents the project renders; `sdd init` offers a multi-select on a fresh project and re-renders every listed agent on each run. Add agents with `sdd init --agents claude,codex`.

Instructions bridge through `AGENTS.md` — the cross-tool standard read by Codex and others. `CLAUDE.md` imports it via `@AGENTS.md` so Claude Code shares the same baseline, keeping only Claude-specific notes of its own. `sdd init` scaffolds this bridge for a fresh project when a non-Claude agent is selected, and never overwrites files you already have.

Claude Code is the primary, most-exercised harness; Codex support is recent and has a few rough edges:

- On an **existing** project, `sdd init --agents` doesn't yet persist the selection — add the agent to `supported_agents` in `.sdd/config.yaml` by hand (a fresh `sdd init` persists it correctly).
- Codex's sandbox prompts for approval on the network- and LLM-backed commands SDD runs (search, capture) the first time it hits them — approve them so the first capture completes.

### LLM provider (summaries + pre-flight)

SDD calls an LLM in two places — summarizing each captured entry (the short text rendered in `sdd view` and catch-up) and running pre-flight validation on every draft before it lands. Four providers supported: `anthropic` (cloud API), `openai` (cloud API), `ollama` (local), and `claude-cli` (your local Claude Code CLI authentication).

```yaml
# Anthropic API
llm:
  provider: anthropic
  model: claude-sonnet-4-6
  api_keys:
    anthropic: sk-ant-api03-...
  timeout: 5m
```

```yaml
# OpenAI API
llm:
  provider: openai
  model: gpt-4o-mini
  api_keys:
    openai: sk-...
  timeout: 5m
```

```yaml
# Local Ollama
llm:
  provider: ollama
  model: gemma4:26b
  ollama_endpoint: http://localhost:11434
  timeout: 5m
```

```yaml
# Claude Code CLI (uses your existing Claude Code authentication)
llm:
  provider: claude-cli
  model: claude-sonnet-4-6
```

Remote providers (`anthropic`, `openai`) get a conservative rate limit applied automatically, biased below tier-1 ceilings so bursty operations like `sdd summarize --all` don't trip 429s. Override with `rate_limit_rps` on higher tiers.

### Embedding provider (vector search)

Two providers supported: `openai` and `ollama`. Configuring an embedding provider activates `sdd search --query` (semantic mode) and hybrid retrieval.

```yaml
# OpenAI
embedding:
  provider: openai
  model: text-embedding-3-small
  api_keys:
    openai: sk-...
  timeout: 2m
```

```yaml
# Ollama — note query_template for instruction-tuned models like Qwen3
embedding:
  provider: ollama
  model: qwen3-embedding:8b
  ollama_endpoint: http://localhost:11434
  timeout: 2m
  query_template: |-
    Instruct: Given a query phrase, retrieve related entries from a knowledge graph
    Query:{text}
  # Qwen3 documents take no prefix — leave document_template empty
```

After configuring an embedding provider, run `sdd index` once to embed the existing graph. `sdd search` lazy-fills new entries as they're captured. `sdd info` shows `Search: vector,text` when an embedding provider is configured, `Search: text` when not.

## Browsing the graph yourself

Day to day, the agent calls the CLI for you. A few commands are useful to run yourself when you want to look around.

**`sdd view --layout='...'`** — composable pipeline. Filter, rank, and render in one expression. A few useful starters:

```bash
sdd view --layout='top(20)'                    # twenty warmest entries
sdd view --layout='focus'                      # what's in flight and who's on it
sdd view --layout='topic(infrastructure/cli)'  # a topic's neighborhood
sdd view --layout='decisions,signals'          # kind-grouped views
```

Run `sdd view --help` for the full layout grammar, macro vocabulary, filter
primitives, and worked examples. Bare `sdd view` reports the missing
`--layout` and points there.

**`sdd search`** — three-mode retrieval:

```bash
sdd search --term importer                     # keyword (live grep)
sdd search --query "variants and references"   # semantic (vector)
sdd search --term importer --query "variants"  # hybrid (RRF fusion)
sdd search --query "variants" --all-repos      # fan out across connected repos
```

Vector and hybrid modes require an embedding provider — see [Embedding provider](#embedding-provider-vector-search) above. `--repo`/`--all-repos` reach connected graphs — see [Connected repos](#connected-repos).

**`sdd show <id>`** — one entry in full, with its upstream and downstream chains:

```bash
sdd show d-cpt-vr4              # the entry plus its grounding and consumers
sdd show d-cpt-vr4 --up 4 --down 3   # widen the neighborhood
```

**`sdd recover`** — inspect a durable write whose outcome needs an explicit decision. The interactive command reconciles the concrete branch target before offering a valid action; it never replays pending work automatically. `sdd recover --history` shows terminal audit history. See [local mutation targets and recovery](docs/local-mutation-recovery.md).

For the full CLI surface, run `sdd --help`.

## Directory layout

```
your-project/
└── .sdd/
    ├── config.yaml               # graph_dir, default_branch, language, scope
    ├── config.local.yaml         # local participant name, llm + embedding config (gitignored)
    ├── meta.json                 # graph_schema_version, minimum_version
    ├── graph/
    │   ├── YYYY/MM/              # entries, e.g. 08-104102-d-prc-oka.md
    │   └── wip/                  # active WIP markers
    └── tmp/                      # scratch files (gitignored)
```

The vector index and the connected-repo caches don't live in the project tree — they're per-machine derived state in a content-addressed store under your user cache directory (keyed by repo identity and embedder fingerprint), shared across a repo's checkouts and worktrees and rebuilt on demand. Nothing to commit or clean up by hand.

`sdd init` also installs the Claude Code skills at the agent's skill directory (defaults to `~/.claude/skills/`, or `.claude/skills/` with `--scope project`). Those paths are an implementation detail of the target agent — inspect them if you're curious, but they aren't part of your project's source tree.

## Docs

- [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) — framework model
- [docs/story.md](docs/story.md) — a fictional story (Kōgen Coffee) of what SDD could become; the vision that sparked the design
- [docs/signals.md](docs/signals.md) — open design signals for the framework itself
- [docs/local-mutation-recovery.md](docs/local-mutation-recovery.md) — explicit branch authority, durable apply, and recovery states
- [CLAUDE.md](CLAUDE.md) — guidance for Claude Code working on SDD itself

## Star the repo

If SDD resonates — or if you're curious how the graph evolves — starring the repo helps others find it and lets you follow progress.

## License

MIT — see [LICENSE](LICENSE).
