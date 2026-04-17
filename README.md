# SDD

**Signal-Dialogue-Decision** — a framework for building and traversing decision graphs through human-agent dialogue.

## What SDD does

SDD records the reasoning chain behind your project: observations (signals), commitments (decisions), and facts of execution (actions), linked together in an immutable append-only graph. Built to be used collaboratively by humans and AI agents through Claude Code skills and an SDD CLI.

See [docs/story.md](docs/story.md) for the conceptual narrative and [docs/signal-dialogue-decision.md](docs/signal-dialogue-decision.md) for the framework model.

## Install from source

```bash
git clone https://github.com/networkteam/sdd.git
cd sdd
direnv allow                    # loads Go via Devbox (optional — otherwise install Go 1.26 yourself)
go build -o bin/sdd ./cmd/sdd
```

## Quick start

Initialize a graph in an existing project:

```bash
cd your-project
/path/to/sdd/bin/sdd init
```

Then start capturing:

```bash
sdd new s tac "First signal"
sdd status
```

## License

MIT — see [LICENSE](LICENSE).
