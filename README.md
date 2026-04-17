# SDD

**Signal-Dialogue-Decision** — a framework for building and traversing decision graphs through human-agent dialogue.

SDD records the reasoning chain behind your project: observations (signals), commitments (decisions), and facts of execution (actions), linked together in an immutable Git-based graph. The `sdd` CLI provides the plumbing; a set of Claude Code skills (`/sdd`, `/sdd-catchup`, `/sdd-explore`, `/sdd-groom`) turn it into a collaboration surface you drive through conversation.

## Concepts in a minute

**The loop**: Signal → Dialogue → Decision → Action → Signal...

Dialogue is the work that turns signals into decisions — it isn't recorded directly. Everything else lives in the graph as an immutable markdown entry with YAML frontmatter.

**Entry types**:
- **Signal** (`s`) — an observation, a gap, a question raised.
- **Decision** (`d`) — a commitment to a direction. Kinds: `directive` (default), `contract` (standing rule), `plan` (multi-step scope with acceptance criteria).
- **Action** (`a`) — a fact of execution. Actions `close` the decisions and signals they fulfil.

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

See [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) for the full framework model and [docs/story.md](docs/story.md) for a narrative walkthrough (Kōgen Coffee).

## Install from source

Requires Go 1.26.

```bash
git clone https://github.com/networkteam/sdd.git
cd sdd
direnv allow                    # loads Go via Devbox (optional)
go build -o bin/sdd ./cmd/sdd
```

The binary ends up at `./bin/sdd`. Add it to your `$PATH`, or reference it by absolute path from other projects.

## Quickstart

### 1. Initialize a graph in your project

```bash
cd your-project
/path/to/sdd/bin/sdd init
```

`sdd init` prompts for the graph directory (default `.sdd/graph`), writes `.sdd/config.yaml`, creates the graph directory, and adds `.sdd/tmp/` to `.gitignore`.

### 2. Install the Claude Code skills

There is no `sdd install` yet. For now, copy the four skills from this repo to one of the standard Claude Code skill locations:

- **User-global** (available in every project): `~/.claude/skills/`
- **Project-level** (scoped to one repo): `.claude/skills/` in the project

```bash
cp -r /path/to/sdd/.claude/skills/sdd*  ~/.claude/skills/
# or
mkdir -p .claude/skills && cp -r /path/to/sdd/.claude/skills/sdd*  .claude/skills/
```

The skills invoke the binary at `./bin/sdd` relative to the working directory — convenient for dogfooding inside this repo, but not portable. Until a proper install story lands, using the skills from another project means editing the binary path in each `SKILL.md` (and `sdd/references/cli-reference.md`) to wherever your `sdd` binary lives, or putting it on `$PATH` and doing the same edit to drop the `./bin/` prefix.

### 3. Start a session

Open Claude Code in your project and run:

```
/sdd
```

The skill runs `sdd status` + `sdd wip list`, clusters the graph state by project thread, and suggests where to start. Everything after that is dialogue — you don't run the CLI directly in normal use.

## What a session looks like

### Catch up

```
/sdd
```

Opens every session. The skill presents a numbered list of active contracts, plans, decisions, and open signals, grouped by thread. You reference items by number — "let's dig into 3" — and the skill takes it from there.

### Capture a signal

```
me:   Pre-flight wastes LLM budget checking for field presence — that
      should be a CLI-level validation.
/sdd: [plays back a proposed signal: type, layer, text, refs, confidence]
me:   Looks right.
/sdd: [runs sdd new s prc with the description and refs]
```

The skill always plays back what it would write before running `sdd new`. A pre-flight validator (a second Claude) reviews the draft against the refs and flags contradictions, missing acceptance criteria on plans, unrelated references, and similar calibration gaps. `high` findings block the write; `medium` and `low` are advisory.

### Make a plan decision

Plan decisions (`--kind plan`) carry an `## Acceptance criteria` section — each AC is a verifiable outcome that becomes the contract between plan author, implementing agent, and the pre-flight validator that checks the closing action. The skill helps draft the ACs and can attach a longer design doc via `--attach`.

### Implement

```
me:   Let's build d-tac-lg1.
/sdd: [sdd wip start 20260417-173310-d-tac-lg1 --exclusive
       --participant Christopher "release pipeline"]
```

A WIP marker signals the work is in flight and discourages parallel edits to the same entry. During implementation the skill captures operational sub-decisions as you hit them. When you finish, it captures the closing action (addressing each AC if the plan had one) and runs `sdd wip done`.

For exploratory work or multi-participant projects, add `--branch` to isolate the entry chain on a git branch until you decide to merge or discard.

### Groom

```
me:   Let's groom.
/sdd: [invokes /sdd-groom — returns a table of candidates: stale entries,
       missing `closes` links, superseded-in-practice relationships —
       then walks through them one at a time]
```

The goal is to keep `sdd status` reflecting reality.

### Explore

```
me:   Dig into s-prc-ljg.
/sdd: [invokes /sdd-explore — pulls upstream, downstream, and
       semantically related entries, then dialogues toward the next
       graph move]
```

The goal of exploration is always a graph change, not just understanding.

## CLI cheat sheet

Day to day you rarely run these directly — the skill does. Useful to know:

```bash
sdd status                                            # catch-up view: active contracts/plans/decisions, open signals, recent actions
sdd list [--type d|s|a] [--layer stg|cpt|tac|ops|prc] [--kind contract|directive|plan] [--all]
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
├── .sdd/
│   ├── config.yaml               # graph_dir, etc.
│   ├── graph/
│   │   ├── YYYY/MM/              # entries, e.g. 08-104102-d-prc-oka.md
│   │   └── wip/                  # active WIP markers
│   └── tmp/                      # scratch files (gitignored)
└── .claude/skills/               # if using project-level skills
    ├── sdd/
    ├── sdd-catchup/
    ├── sdd-explore/
    └── sdd-groom/
```

## Docs

- [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) — framework model
- [docs/story.md](docs/story.md) — narrative walkthrough of SDD in use
- [docs/signals.md](docs/signals.md) — open design signals for the framework itself
- [CLAUDE.md](CLAUDE.md) — guidance for Claude Code working on SDD itself

## License

MIT — see [LICENSE](LICENSE).
