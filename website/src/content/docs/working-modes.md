---
title: Working modes
description: The moves your agent offers in an SDD session, when to use each, and what it leaves in the graph.
---

In an SDD session you don't pick a mode or type commands. You talk with your agent. When a move fits what you are doing, the agent recommends it, and you decide whether to start it. A move runs step by step, served by the `sdd` server, and anything it writes to the graph is played back to you first and needs your confirmation.

## How a session opens and ends

Type `/sdd` in Claude Code or `$sdd` in Codex. The agent opens a session, and the `sdd` server gives it an orientation: the project's direction, what is in focus, who takes part, and what changed recently. You can talk in any language; entries are written in the graph language set for the project.

Questions about the graph need no move: the agent searches and reads entries as you ask. When you ask for a move directly, for example to record something, the agent starts it. Otherwise it suggests one and waits for your answer.

A running move can be parked and picked up later in the same session. When something comes up that you want to record later, the agent can start a capture and park it, so it waits as an open thread instead of a promise.

At a natural stop, the agent offers to wrap up and names each thread that is still open. Wrapping up ends the session for good and leaves open threads behind, so finish or drop them first. If you leave without wrapping up, the session stays open, and your agent can resume it when you give it the session ID.

## Catch-up

Use it when you want to know where things stand: "Where are we?" or "What changed this week?" The agent recommends it for that. It does not run by itself when a session opens.

- The server gathers a fresh view of the graph: the current focus, recently finished work, active work drawing attention, commitments not yet acted on, open questions and gaps, and work marked as in progress.
- The agent turns this into a short briefing grouped by the project's main threads, with numbered items and their entry IDs.
- Below the threads it lists other open areas you can ask to drill into.
- It ends with one question: what do you want to move forward?
- Your answer usually starts an engage on the entry you picked, or a capture.

A catch-up only reads. It leaves nothing in the graph.

## Capture

Use it when you notice something or settle something: "The export drops rows without a date" or "Let's release every two weeks." A [plan](/sdd/reference/entry-kinds/#plan) is captured the same way, with acceptance criteria: the outcomes that must be true when the work is done.

- The agent searches the graph and drafts one entry: its [kind](/sdd/reference/entry-kinds/), its text, and links to the entries it builds on.
- A separate model reads the draft as a stranger would and points out what is unclear. The agent revises.
- The agent plays back the exact entry. You confirm it, ask for changes, or drop it.
- A pre-flight check compares the entry with the graph. A serious finding stops the write until you revise the entry or override the check. Only you can override.
- sdd writes the entry with a generated one-line summary and commits it. The agent checks the summary against the entry.

It leaves one new entry. The entry can also close or replace an earlier one, and then says why. [Getting started](/sdd/getting-started/#6-record-your-first-observation) walks through a first capture.

## Engage

Use it to work on one specific entry: "Let's look at the onboarding plan", or an entry ID such as `d-tac-ry0`. Picking an item from a catch-up usually leads here. Closed entries work too, for example to look back on finished work.

- If you describe the entry, the agent searches and lets you pick from the candidates.
- It reads the entry and the entries around it in full.
- It gives you a brief shaped by the entry and your goal. For a plan you want to build, that is a table of its acceptance criteria: done, partly done or open. For an observation, it is what happened since and what it needs.
- It proposes next moves. Some act on the entry, such as implementing, replacing or closing it. Others check other work against it, for example against a [contract](/sdd/reference/entry-kinds/#contract).
- You pick one, and it runs as its own move.

Engage itself writes nothing. The move you pick may.

## Implementation

Use it when work is ready to build, usually a [plan](/sdd/reference/entry-kinds/#plan), a [directive](/sdd/reference/entry-kinds/#directive) or an [activity](/sdd/reference/entry-kinds/#activity). A [gap](/sdd/reference/entry-kinds/#gap) you want fixed or another entry works too. "Let's implement the export plan."

- The agent reads what the work must deliver, including later decisions that add to it, and says whether enough is decided to start. If a decision is missing, you capture it first.
- You choose how to run it: on the main branch, on a separate branch, in a separate git worktree, or as a quick change too small to track. Except for a quick change, the entry is marked as in progress.
- The agent builds in slices and checks in between them. A choice no decision covers stops the work until you settle it together, and what you settle is captured.
- At the end it commits, records a done entry, and shows which criteria are met. You give the go-ahead to land it, and it offers an evaluation.

It leaves a [done](/sdd/reference/entry-kinds/#done) entry citing the commits, plus any decisions captured along the way. A done that covers everything closes the entry it implements; a partial one says what remains.

## Evaluate

Use it to judge finished work: "Did the new export hold up?" The agent offers it when an implementation ends, and you can start it later on any done entry.

- The agent reads the work and what happened since it landed, including earlier evaluations.
- It proposes which lens this run covers. The inner lens asks whether the work is sound by the project's own standards. The outer lens asks whether it is the right thing, working in use for the people it serves. One lens is enough, and your project's guidelines define what each means.
- The agent inspects and runs the work, and asks for your judgment or other people's feedback, which count as evidence.
- It tells you what held and what didn't.
- You decide whether to record the evaluation and which findings to keep.

Recorded, it leaves a done entry for the evaluation that names the lens it covered, plus one entry for each finding you chose. A clean pass is worth recording too.

## Groom

Use it when the graph's status looks out of date: "Let's tidy up the graph." Grooming decides nothing new. It records what the graph already implies.

- The agent sweeps the graph for entries that look resolved but are still open, in-progress markers on work that has ended, entries a newer one restates without replacing them, and directives that contradict each other.
- It shows a numbered table: each candidate with its evidence and a proposed cleanup.
- You go through it together, most valuable first.
- Each cleanup you agree to is an ordinary capture, such as a done that closes finished work or a decision that replaces an older one, played back for your confirmation. A stale in-progress marker needs no entry; the agent removes it once you agree.

It leaves the closing and replacing entries for the cleanups you agreed to. A clean sweep with nothing to fix is a fine result.

## Interview

Use it when you hold knowledge the graph doesn't, or when things don't fit together: "Interview me about how pricing should work." For a quick check, the agent restates its understanding and asks you to confirm instead.

- The agent first searches the graph and notes the assumptions it wants you to correct.
- It asks one open question per turn, with a few sentences of background.
- It checks each answer against the graph, connecting it to entries or pushing back where it conflicts with them.
- When nothing is left to ask, it sums up what became clear and what stays open. You correct the summary.
- You choose which outcomes to record.

Each outcome you choose becomes its own capture, such as a decision, an [insight](/sdd/reference/entry-kinds/#insight) or a [gap](/sdd/reference/entry-kinds/#gap). An interview can also end with shared understanding and nothing recorded.

## Bootstrap

Use it when the graph is new or still thin. The agent suggests it then, or you ask: "Let's bootstrap this project." [Getting started](/sdd/getting-started/#5-found-the-graph-with-bootstrap) walks through a first run.

- The agent asks whether you know SDD and adjusts how much it explains.
- In an existing repository it first reads the README, docs and recent history, and asks you to correct its picture.
- It asks one question at a time about what the project is, how it is approached, why it exists and who is part of it, following your lead.
- When a group of entries has formed, it plays back the text it would record. You accept it, reshape it or set it aside.
- It records accepted entries one by one. After each group you choose to continue or stop.

It typically leaves an [actor](/sdd/reference/entry-kinds/#actor) and a [role](/sdd/reference/entry-kinds/#role) for each person taking part, the [aspirations](/sdd/reference/entry-kinds/#aspiration) and [directives](/sdd/reference/entry-kinds/#directive) that frame the project, and a first set of topic labels. It ends with a catch-up. You can run it again later to go deeper.
