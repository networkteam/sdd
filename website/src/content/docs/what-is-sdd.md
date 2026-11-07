---
title: What is SDD?
description: The problem SDD solves, how it works, what it gives you, and who it is for today.
---

SDD is agent-native memory for your project's decisions and the why behind them. Your agents get exactly the project context they need: what was noticed, discussed and decided. New knowledge is captured in dialogue with the people who create it, as the work happens.

## The problem

A session with an AI agent produces reasoning: what you noticed, which options you weighed, what you decided and why. When the session ends, that reasoning is gone. The next session, yours or a colleague's, starts from the project files and whatever you remember to tell the agent. Decisions made in meetings and chat fare no better. They get argued again, and every newcomer rebuilds the picture by asking around.

Tools that try to keep this reasoning fail in one of two ways. Some record everything and leave a searchable pile of what was said, with no way to tell what was decided. Others spread the project's state across documents, plans, roadmaps and tickets that repeat each other and drift apart.

## How SDD works

SDD stands for Signal, Dialogue, Decision. It keeps a project's reasoning in a graph: a network of small entries linked to each other. There are two types of entry:

- A **signal** records something noticed, such as a problem, a fact, an open question or finished work.
- A **decision** records something committed to, such as a direction, a plan or a standing rule.

Each entry states its reasoning and links to the entries it builds on. Entries are plain text files in your project's git repository, readable without SDD, and sdd commits each one.

The graph grows in a loop. A signal meets dialogue between you and your agent and becomes a decision. When the work is done, a done entry closes the decision, and what the work turned up feeds the next signals.

You take part by talking. The agent reads what the graph holds on the subject before it answers, drafts an entry when something is worth keeping, and plays it back to you. Nothing reaches the graph until you confirm it.

## What you get

- **Agents start from what was decided.** When a session opens, the `sdd` server gives your agent the project's direction, focus, participants and recent changes, so you don't explain the project again.
- **A confirmed record of why.** You confirm every entry, and a pre-flight check compares it with the graph and stops the write on a serious problem such as a contradiction.
- **Status nobody maintains.** Entries are never edited. A later entry closes or replaces an earlier one, and what is open, active or closed follows from those links. There is no status field to update.
- **A briefing when you return or join.** Ask where things stand, and the agent briefs you from the graph: current focus, recently finished work, open questions and gaps, work in progress.
- **Parallel work without overwrites.** Entries are only ever added, so several people and agents can write to the graph at once.

## Who it is for today

SDD runs inside coding agents, currently Claude Code and Codex, on a project kept in git. It is for developers and teams who work with those agents, alone or together. SDD's open source core, the `sdd` program and its server, is on [GitHub](https://github.com/networkteam/sdd) under the MIT license.

The aim reaches further. Product owners, designers and domain experts should take part directly, through chat, voice or a hosted version that needs no setup. Agents should work on their own within the decisions and guidelines the graph holds, while people review what comes back. These are directions, not shipped features. The [Vision](/sdd/vision/landscape/) marks what works today and what is still being built.

## What SDD is not

- **Not a recorder.** The graph holds reasoning, not a recording. SDD does not store the dialogue, only the entries you confirm.
- **Not a ticket system or a set of documents to maintain.** There are no assignment queues, status workflows or boards. The graph itself answers what is open, who is on it and what waits next.
- **Not an agent writing your record on its own.** The agent reads, drafts and runs the steps. Today you confirm every entry, and only you can override a pre-flight finding.

## Next steps

- [Getting started](/sdd/getting-started/) installs sdd, sets up a project and walks you through a first dialogue.
- [Working modes](/sdd/working-modes/) describes the moves your agent offers in a session.
