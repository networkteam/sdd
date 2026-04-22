# SDD

**Keep humans and AI agents aligned across parallel work.**

SDD records your project's reasoning as an immutable decision graph — signals (what you noticed) and decisions (what you committed to). At any moment, anyone (human or agent) can see what's in flight, which decisions are active, and what's still open. Built for developers and teams shipping with AI agents.

**Why SDD**

**Without SDD:** insights and decisions from each agent session get lost or hard to discover once the session ends.

**Most other systems:** scatter state across separate static artifacts — docs, plans, phases, roadmaps, trackers, specs, tickets — that duplicate across layers, go stale, and resist new insight.

**With SDD:** an append-only graph where new insight → new entry (`refs`, `supersedes`, `closes`). State is *derived from the graph*, never maintained in parallel docs. Tracked in Git, so humans and agents work against the same graph — each session runs independently, and signals and decisions are captured by the work itself.

![SDD in action — parallel feature work without losing context](docs/assets/screencast.gif)

---

You start in Claude Code with the `/sdd` skill. It shows the current state of the graph and carries the playbook moves for taking it forward — capture, decide, implement, explore, groom — all through conversation. The `sdd` CLI underneath stores entries and derives current state from the graph — the same view humans and agents both read. You rarely invoke it directly.

## Philosophy

SDD lets each party do what they're good at. Agents work with information and run autonomously where they can — reading the graph, synthesizing context, drafting entries, running operational steps. Humans make decisions, provide taste, and raise observations data can't surface. Dialogue, not bureaucracy, moves the graph forward.

## What a session looks like

### Catch up

```
> /sdd

Claude: Fork landed yesterday (s-tac-q94, kind: done). Two fresh plans
        today, plus a new process signal. No active WIP.

        Release pipeline — just captured, ready to build
         1. GitHub Actions release pipeline (d-tac-lg1) — plan, medium. …
        Participant-name drift — plan waiting for build
         2. Participant correlation + drift validation (d-tac-q5p) — …
        Pre-flight refinement — active signal cluster
         4. Dry-run reflex / retry-hint UX (s-prc-w5r) — captured today. …
         5. CLI-level validation for Confidence (s-prc-hpa) — …
         …

        Where I'd start: the release pipeline plan (1) — fork's done,
        plan is fresh. Want to kick off d-tac-lg1 implementation, dig
        into the pre-flight cluster, or something else?

> Implement 1.
```

Opens every session. Invoking `/sdd` loads the skill; Claude then runs `sdd status` + `sdd wip list`, clusters active entries by project thread, and presents a numbered list. You reference items by number — "implement 1", "dig into 3" — and the dialogue takes it from there.

### Implement

```
> Implement 1.

Claude: [sdd wip start 20260417-173310-d-tac-lg1 --exclusive
         --participant Christopher "release pipeline"]
```

A WIP marker signals the work is in flight and discourages parallel edits to the same entry. During implementation Claude captures operational sub-decisions as they come up.

When the work finishes, Claude plays back the closing done signal and takes the WIP marker down:

```
> Done. Tests pass, the v0.1.0-rc release went out cleanly.

Claude: [plays back a closing done signal referencing the final commit
         and addressing each AC in d-tac-lg1. Confidence high.]

> Looks right.

Claude: [sdd new s tac --kind done --closes 20260417-173310-d-tac-lg1 …
         then sdd wip done <marker-id>]
```

For exploratory work or multi-participant projects, add `--branch` to isolate the entry chain on a git branch until you decide to merge or discard.

### Capture a signal

Signals usually surface while you're in the work — testing what was just built, noticing a gap, hitting an unexpected trade-off. Capturing keeps the insight durable instead of losing it at session end.

```
> Testing the release pipeline, the Homebrew formula update didn't
  trigger automatically. Not blocking the release — worth capturing?

Claude: [plays back a tactical signal refsing the closing done signal
         for d-tac-lg1, confidence medium; asks to confirm wording.]

> Looks right.

Claude: [runs sdd new s tac ...]
```

Before writing, Claude plays back the proposed entry. A pre-flight validator (a second Claude, run by the CLI) then reviews the draft against its refs and flags contradictions, missing acceptance criteria on plans, unrelated references, and similar calibration gaps. `high` findings block the write; `medium` and `low` are advisory.

### Make a plan decision

Plan decisions (`--kind plan`) carry an `## Acceptance criteria` section — each AC is a verifiable outcome that becomes the contract between plan author, implementing agent, and the pre-flight validator that checks the closing done signal. Claude helps draft the ACs and can attach a longer design doc via `--attach`.

### Groom

```
> Let's groom.

Claude: [invokes the /sdd-groom sub-skill — returns a table of candidates:
         stale entries, missing `closes` links, superseded-in-practice
         relationships — then walks through them one at a time]
```

The goal is to keep `sdd status` reflecting reality.

### Explore

```
> Dig into s-prc-ljg.

Claude: [invokes the /sdd-explore sub-skill — pulls upstream, downstream,
         and semantically related entries, then dialogues toward the next
         graph move]
```

The goal of exploration is always a graph change, not just understanding.

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

Installs to `~/.local/bin/sdd` by default (XDG-compliant, user-scoped — no `sudo`). Re-run to upgrade. Pass `-b <dir>` to change install location, e.g. `curl -sL ... | sh -s -- -b /usr/local/bin`.

### Curl installer — verified (recommended for CI / security-conscious setups)

Build provenance attestations are produced by GitHub Actions on every release. Verify before execution with the `gh` CLI:

```bash
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh -o install.sh
gh attestation verify install.sh --repo networkteam/sdd
sh install.sh
rm install.sh
```

This confirms the installer was signed by the `networkteam/sdd` release workflow via GitHub's native artifact attestations (Sigstore-backed).

### From source

Requires Go 1.26.

```bash
git clone https://github.com/networkteam/sdd.git
cd sdd
direnv allow                    # loads Go via Devbox (optional)
go build -o bin/sdd ./cmd/sdd
```

The binary ends up at `./bin/sdd`. Add it to your `$PATH`, or reference it by absolute path from other projects.

## Quickstart

### 1. Initialize the project

```bash
cd your-project
sdd init
```

One idempotent command. On a fresh tree it prompts for the graph directory (default `.sdd/graph`), writes `.sdd/config.yaml` and `.sdd/meta.json`, adds `.sdd/tmp/` to `.gitignore`, and installs the Claude Code skills under `~/.claude/skills/` (the default, user-global scope). Pass `--scope project` to install into this repo's `.claude/skills/` instead.

Run `sdd init` again after a binary upgrade to refresh drifted skill files. Pristine files update silently; files you've edited yourself are preserved (add `--force` to overwrite them, or `--scope project` + `--force` to rebuild a repo-local installation).

### 2. Start a session

Open Claude Code in your project and run:

```
/sdd
```

The skill runs `sdd status` + `sdd wip list`, clusters the graph state by project thread, and suggests where to start. Everything after that is dialogue.

## Concepts in a minute

**The loop**: Signal → Dialogue → Decision → Done signal → Signal...

Dialogue is the work that turns signals into decisions — it isn't recorded directly. Everything else lives in the graph as an immutable markdown entry with YAML frontmatter.

**Entry types** (two; kinds carry the nuance):
- **Signal** (`s`) — something noticed. Kinds: `gap` (default), `fact`, `question`, `insight`, `done`. A `done` signal records a completed commitment and closes the decision it fulfils.
- **Decision** (`d`) — something committed to. Kinds: `directive` (default), `activity`, `plan` (multi-step scope with acceptance criteria), `contract` (standing rule), `aspiration` (perpetual direction).

**Layers** describe depth of thinking, not org level:

| Layer | Abbrev | Thinking |
|-------|--------|----------|
| Strategic | `stg` | Why does this exist? |
| Conceptual | `cpt` | What approach? |
| Tactical | `tac` | Structure and trade-offs |
| Operational | `ops` | Individual steps |
| Process | `prc` | How we work |

**Links between entries**:
- `refs` — builds on / depends on (no status effect)
- `supersedes` — replaces; older entry no longer active
- `closes` — resolves / fulfils; signal or decision now closed

**Immutability**: entries are never edited. State is derived by traversing the graph, not by mutating files. To change direction, add a new entry that supersedes an old one.

See [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) for the full framework model and [docs/story.md](docs/story.md) for a story of how SDD could work in the future.

## CLI cheat sheet

Day to day you rarely run these directly — the skill does. Useful to know:

```bash
sdd status                                            # catch-up view: active contracts/plans/decisions, open signals, recent done signals
sdd list [--type d|s] [--layer stg|cpt|tac|ops|prc] [--kind gap|fact|question|insight|done|directive|activity|plan|contract|aspiration] [--all]
sdd show <id> [--downstream] [--max-depth N]          # entry + upstream/downstream summary chain
sdd new <type> <layer> [flags] "<description>"        # create an entry (runs pre-flight)
sdd wip start <entry-id> --exclusive --participant <name> "<description>" [--branch]
sdd wip done <marker-id> [--force]
sdd wip list
sdd lint                                              # dangling refs, broken attachments, stale summaries
sdd summarize [<id> | --all]
```

IDs accept both full form (`20260408-104102-d-prc-oka`) and short form (`d-prc-oka`). Short IDs are a human convenience; agents always use full IDs.

## Directory layout

```
your-project/
└── .sdd/
    ├── config.yaml               # graph_dir, etc.
    ├── meta.json                 # graph_schema_version, minimum_version
    ├── graph/
    │   ├── YYYY/MM/              # entries, e.g. 08-104102-d-prc-oka.md
    │   └── wip/                  # active WIP markers
    └── tmp/                      # scratch files (gitignored)
```

`sdd init` also extracts the Claude Code skills to the agent's skill directory (defaults to `~/.claude/skills/`, or `.claude/skills/` with `--scope project`). Those paths are an implementation detail of the target agent — inspect them if you're curious, but they aren't part of your project's source tree.

## Docs

- [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) — framework model
- [docs/story.md](docs/story.md) — a fictional story (Kōgen Coffee) of what SDD could become; the vision that sparked the design
- [docs/signals.md](docs/signals.md) — open design signals for the framework itself
- [CLAUDE.md](CLAUDE.md) — guidance for Claude Code working on SDD itself

## Star the repo

If SDD resonates — or if you're curious how the graph evolves — starring the repo helps others find it and lets you follow progress.

## License

MIT — see [LICENSE](LICENSE).
