---
title: Getting started
description: Install sdd, set up a project, found its graph through a first dialogue, and record your first observation.
---

By the end of this page your project has an SDD graph, founded in a first dialogue, and holds your first recorded observation. You work in Claude Code or Codex. Your agent does the graph work through the `sdd` server; you take part by talking with it.

## Before you start

- **A git repository with a branch checked out.** For a new project, `git init` is enough. sdd commits everything it records, so git needs your `user.name` and `user.email`.
- **Claude Code or Codex.**
- **An API key for a model provider**, such as Anthropic, OpenAI or the Opper gateway, or a local [Ollama](https://ollama.com). Step 2 shows where it goes.

## 1. Install sdd

With Homebrew:

```bash
brew install networkteam/tap/sdd
```

Or with the install script, which puts the binary in `~/.local/bin`:

```bash
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh | sh
```

Check that it is on your `PATH`:

```bash
sdd --version
```

Your agent starts sdd as the plain command `sdd serve`, so a binary outside your `PATH` will not be found.

## 2. Connect a language model and search

sdd uses a language model whenever it records an entry: to check the draft's wording, to check it against the graph, and to write its one-line summary. An embedding model makes the graph searchable by meaning, not only by keyword. Set both once for your user, and they apply to every project. `sdd config set` writes to `~/.config/sdd/config.yaml`; add `--local` to set a value for the current project on this machine only.

### Language model

For example, with the Anthropic API:

```bash
sdd config set llm.provider anthropic
sdd config set llm.model claude-sonnet-4-6
sdd config set llm.api_keys.anthropic <your-api-key>
```

For hosted open-weight models, or one key for many models, use an AI gateway such as [Opper](https://opper.ai) through its OpenAI-compatible route:

```bash
sdd config set llm.provider openai
sdd config set llm.endpoint https://api.opper.ai/v3/compat/chat/completions
sdd config set llm.model greenpt/glm-5.3-flash
sdd config set llm.api_keys.openai <your-opper-key>
```

The other providers are `openai` for OpenAI itself, and `ollama` for models running on your machine. The README's [LLM provider](https://github.com/networkteam/sdd#llm-provider-summaries--pre-flight) section has a configuration for each.

Every check asks the model for a fixed response format, so the provider and model must support structured output. With one that does not, such as Ollama Cloud, the checks fail.

Without a provider, sdd falls back to `claude-cli`, which runs every check through your local Claude Code installation. It needs no key, but it is slow, and checks can time out.

### Embedding model

For example, with OpenAI:

```bash
sdd config set embedding.provider openai
sdd config set embedding.model text-embedding-3-small
sdd config set embedding.api_keys.openai <your-api-key>
```

If your language model already uses an OpenAI key, the embedding model uses it too, and you can skip the last line. For embeddings on your machine with Ollama, see [Embedding provider](https://github.com/networkteam/sdd#embedding-provider-vector-search) in the README.

Without an embedding model, search by keyword still works. When you join a project that already has entries, run `sdd index` once to make them searchable by meaning; new entries are added as they are recorded.

### Check the result

```bash
sdd config
```

This lists every effective setting and where it comes from. The `sdd` server reads the configuration when it starts, so restart your agent after a change.

## 3. Set up the project

Run `sdd init` in the project root:

```bash
sdd init
```

On a new project it asks for:

1. the graph directory, `.sdd/graph` by default
2. the graph language, `en` by default
3. where to install the skill: choose **project**
4. your participant name, by default your git user name
5. which agents to set up: Claude Code, Codex, or both

It then writes these files and commits them as `sdd: init .sdd/ metadata directory`:

| File | What it is |
| --- | --- |
| `.sdd/config.yaml` | Project settings, shared through git |
| `.sdd/config.local.yaml` | Your participant name, on this machine only |
| `.claude/skills/sdd/` or `.agents/skills/sdd/` | The `sdd` skill for your agent |
| `.mcp.json` and `.claude/settings.json` | Claude Code: starts the `sdd` server and allows its tools |
| `.codex/config.toml` | Codex: starts the `sdd` server |

In a repository that already uses SDD, `sdd init` only asks for your name and installs the skill as the project has it configured.

If you chose **user** instead of project for the skill, sdd does not register the server. Register it yourself, for Claude Code with `claude mcp add sdd -- sdd serve`.

:::note[What just happened]
The graph is empty so far. The skill is only a pointer: it tells your agent to open a session with the `sdd` server. The working process itself, how a capture or a check-in runs step by step, ships inside the sdd binary and is served to the agent as the session goes on.
:::

## 4. Open a session

Start your agent in the project directory. If Claude Code asks whether to enable the project's `sdd` server, approve it. Then type:

- `/sdd` in Claude Code
- `$sdd` in Codex

The agent opens a session. The server gives it an orientation: the working principles, the project's direction, what is in focus, who takes part, and what changed recently. On a new project most of that is empty. From here on, you talk; you don't need to know any commands.

## 5. Found the graph with bootstrap

On an empty graph the agent suggests bootstrap, the dialogue that founds the graph. You can also ask for it: "Let's bootstrap this project."

Bootstrap runs as an interview:

- It first asks whether you already know SDD, and explains what you need as it comes up.
- In an existing repository, the agent reads the README, docs and recent history first, so you correct its picture instead of starting from nothing.
- It asks one question at a time: what the project is, how it is approached, why it exists, who is part of it.
- When an answer forms a coherent piece, the agent plays it back as the text it would record. You accept it, reshape it, or set it aside.
- Accepted pieces become entries, recorded one by one. Typically these are an actor entry for you, your role on the project, and the aspirations and directives that frame it.
- After the first entries, the agent proposes a few topic labels, such as `product/vision`, so related entries can be found together.
- It ends with a check-in: a short briefing on the new graph.

A sparse bootstrap is fine. One participant and one entry that frames the project are enough to start; you can deepen it in any later session.

:::note[What just happened]
These first entries are what every later session starts from. When the agent opens a session, it reads the project's direction and participants from the graph, so you do not explain the project again.
:::

## 6. Record your first observation

Tell the agent something you noticed, in plain words, for example that a step in your setup fails and what you expected instead. The agent recognizes an observation and starts a capture:

1. **Draft.** The agent searches the graph for related entries, reads them, and drafts the entry: its kind (a gap, for something that falls short of what was expected), its text, and links to the entries it relates to.
2. **Writing check.** A separate model reads the draft as a stranger would and points out what is unclear. The agent revises.
3. **Playback.** The agent shows you the exact entry. You confirm it, ask for changes, or drop it. Nothing is written before you confirm.
4. **Pre-flight check.** Another check compares the entry with the graph, for example for contradictions with the entries it links to. A serious finding stops the write: you revise the entry, or override the check. Overriding is your call alone, never the agent's.
5. **Write.** sdd generates a one-line summary, writes the entry as a markdown file under `.sdd/graph/`, and commits it as `sdd: capture <id>`. The agent compares the summary with the entry and corrects it if it drifted.

If a model call fails, the step stops and the agent tells you. Nothing half-done is written.

:::note[What just happened]
The gap is now open in the graph. The next check-in lists it among the things the project owes attention, until a decision or a completed piece of work closes it. The [entry kinds](/sdd/reference/entry-kinds/) reference explains what each kind is for.
:::

## How knowledge flows

One loop drives the graph. A signal, something noticed, meets dialogue and becomes a decision, something committed to. When the work is done, a done entry records it and closes the decision, and what the work turned up feeds the next signals.

- **The graph prepares the dialogue.** Before drafting or answering, the agent reads what the graph holds on the subject, so a conversation starts from the project's recorded state instead of your memory.
- **The dialogue moves the graph.** Nothing is recorded without your confirmation, and every entry links to the entries it builds on.
- **Entries are never edited.** A later entry closes or replaces an earlier one. What is open, active or closed is worked out from those links, and git keeps the full history.

## Next steps

Ask your agent in plain words; it starts the right move:

- "Where do we stand?" gives a check-in: what is in focus, what changed, what is open.
- Naming an entry, or "let's work on this", digs into it and offers the next steps.
- Agreed work runs as an implementation, which ends with a done entry citing the commits.
- Finished work can be evaluated, and the graph can be groomed of entries that are resolved but still open.

Search by keyword works out of the box. Search by meaning needs an embedding provider, described under [Embedding provider](https://github.com/networkteam/sdd#embedding-provider-vector-search) in the README.

To look up what an entry kind is for, see [Entry kinds](/sdd/reference/entry-kinds/). For where SDD is heading, see the [Vision](/sdd/vision/landscape/).
