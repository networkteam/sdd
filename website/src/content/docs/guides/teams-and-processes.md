---
title: Teams and processes
description: How teams use SDD today in product development and IT operations, what you learn by using it, how work reaches the others, meetings, working in small steps, tickets, and what is still rough.
---

This page collects how teams work with SDD today. It describes what works now. Where SDD is heading is on the Vision page [How will that feel?](/sdd/vision/scenes/). The page grows as SDD does.

## What teams use it for

Most use is in product development with software development, in small teams of two or three people and, more recently, in larger setups. Teams use SDD to:

- work out requirements, UX and technical concepts until they become plans that go into development [step by step](#working-in-small-steps),
- turn meetings into recorded decisions and [shared understanding](#meetings-and-dialogue-between-people),
- get new people oriented by [asking the agent](#how-work-reaches-the-others),
- write tickets and merge request descriptions [with the background included](#tickets-and-merge-requests-as-output),
- keep IT operations knowledge, meaning what happened to the systems and how problems were solved, next to a [reference wiki](#operations-knowledge-with-a-wiki).

Each person works in their own sessions with their own agent. The graph is what they share.

## Who takes part

Developers, and more and more people in product, UX and project lead roles. They run the same kind of agent session but start from a different question. Not "How can I implement that?" but "This is the customer need. Why does the customer need it? What do we have? How could we do it? What are the approaches?" Ask why again of each answer until you understand the background well. That understanding helps the team align later.

That gives them direct access to the technical background, so they can shape requirements without involving a developer at every point. What they work out goes back into the graph as observations, questions and directives.

Bringing in people without a technical background has only started. It is too early to describe how that works.

## What you learn by using it

### You choose the ceremony

You are in the driver's seat. Tell the agent what you want from the session, and it works out how to get there from the guidance sdd serves. Sometimes you only want to record your current understanding as it is, without working the material further. Other times you dig deep, research and follow several directions. That can pull you into a rabbit hole where it feels like you could never stop, which helps and hurts.

The agent can also propose more structure than the work needs: three entries where one would do, or a plan for a gap you could simply fix and close with a done. Say what you want instead, for example "Capture this as one entry" or "Let's just fix this gap".

There is no wrong way. Use whatever gets you to the next step while you keep the knowledge and decisions in the graph current.

### Step back from the details

You also steer the level a session works at. When it gets dragged into details, tell the agent to step back and look at the bigger picture. Work that picture out first, then decide where to go deeper, possibly in follow-up sessions.

### One angle per session

A graph with many open threads can be a lot at once. It is better to focus on one angle at a time. A terminal multiplexer with coding agent integration, such as Herdr, makes splitting easy. Start one session for an overview, sort out what is open and group it, then ask the agent to start a few sdd sessions, one per group, each with a handoff. Each session then follows one thread. As in any parallel work, [each agent gets its own sdd session](/sdd/guides/working-with-coding-agents/#one-sdd-session-per-conversation).

### You think more about the why

Working this way leads you to ask why more often. Why do we do this? What exactly is wrong? "Something doesn't work" is not a good [gap](/sdd/reference/entry-kinds/#gap). A good one sets what is actually the case against what was expected. Over time this makes your own thinking more structured.

## How work reaches the others

- **Push and pull the graph.** sdd commits every entry it records, but it does not push or pull. Do that with git, as you do for code.
- **New entries reach running sessions.** When entries land in the graph while a session runs, for example after a pull, the agent sees them among the recent changes and can bring them into the dialogue.
- **Answer with an entry ID.** When someone asks about something that is already captured, send them the entry ID. They open a session with it, for example `/sdd let's look at d-tac-ry0`, and get the entry together with how it connects and the reasoning around it.
- **Ask the agent.** To find your way into a topic, tell the agent your goal and what you need to understand. It searches the graph and answers from what it finds. If the answer is not there, record a [question](/sdd/reference/entry-kinds/#question). It stays open and shows up in catch-ups until someone answers it.

## Meetings and dialogue between people

Dialogue between people works well with SDD, and transcripts carry it into the graph. A loop that works well:

1. Prepare the meeting in a session. Name the topic and the participants, and let the agent gather what the graph holds on it and what is open.
2. Hold the meeting with that preparation.
3. Transcribe the meeting, work through the transcript in a session, and apply it to the graph.
4. Follow up with sessions for targeted captures.

This turns even a dense two-hour meeting into focused entries and puts the shared understanding in the graph. When you apply a transcript, name the people who took part so the entries list them as participants, and attach the passages an entry rests on. No move is dedicated to transcripts yet. The agent proposes entries in the dialogue, and you confirm each capture as usual.

A second pattern helps when some people in a team have gone deeper into a topic than the others. They work through it with their agents, prepare a document for a dialogue with the others, hold that dialogue, and bring the feedback back into the graph. It aligns understanding, carries knowledge across the team and leads to better decisions made together.

Clients are not connected directly yet. Their input arrives through tickets, calls and meetings and takes the same path into the graph.

## Working in small steps

The graph tracks work, but not the way an issue tracker does. No entry moves through states. Instead the graph grows: new knowledge, experiences and decisions come up while you work, and you record them as they appear. Starting from one [directive](/sdd/reference/entry-kinds/#directive), a typical sequence is:

1. Write a first [plan](/sdd/reference/entry-kinds/#plan) from what you know now.
2. Work on it in iterations.
3. Record [done](/sdd/reference/entry-kinds/#done) entries, partial ones too.
4. Hold a new dialogue or run an [evaluation](/sdd/working-modes/#evaluate).
5. Shape the next plan.
6. Refine earlier decisions where needed.

The pull toward planning everything up front is always there. Plan from what you understand now, take the next step that teaches you more, and check that you are still heading where the aspirations and strategic directives point.

## Tickets and merge requests as output

Teams still create tickets, as output of the graph. Once the graph holds a good picture of something, the agent writes the issue or merge request description from it, with the background included. With the tracker's CLI or MCP server available, the agent creates them directly. Tickets keep people who don't use SDD up to date, and the graph stays the source of truth. A team that works fully with SDD would not need an issue tracker.

Copying information from one tool to another by hand is friction. Look for these points and remove them. The aim is not to automate everything. People stay in the loop, make the decisions and build the understanding. Less friction lets a team choose how much it hands to agents.

## Operations knowledge with a wiki

For IT operations, one setup keeps the graph and a Markdown wiki, for example an Obsidian vault, in the same repository. The team browses the wiki for hosts, services and applications. The agent uses both to answer questions about problems. The graph holds what happened, what was observed and how earlier problems were solved, so recurring problems become visible. The wiki gives an overview that costs little work. Together they help new people get started, let the team share responsibility, and let agents take on more operations work, because they understand the legacy systems and the plans to change them.

A coding agent sets this up in a few minutes. What matters is a few rules in the agent's instruction file, such as `AGENTS.md` or `CLAUDE.md`:

- **The graph is the source of truth.** It holds what happened, what was observed and what was decided, in order. Measurements are evidence for entries.
- **The wiki describes the current state.** It holds reference material such as host lists, versions and network details, and links to entry IDs instead of retelling history. A sentence that is only true as of a past date belongs in the graph.
- **Wiki pages stay small.** A page covers what is not obvious and points to the configuration or the graph for the rest.
- **Every change updates the wiki.** When work changes the current state, the agent updates the affected pages, commits them, and references them in the entry it records, for example the done.
- **The agent researches both.** Before it states a fact about the systems, it reads the graph and the wiki.

## What is still rough

- **Sync.** Everyone pushes and pulls the graph by hand. Automatic sync is decided but not built.
- **Forwarding.** Nothing notifies people when the graph needs them, so you forward entry IDs yourself. Notifications are planned.
- **Who may decide what.** Nothing checks who may record which decision. Agree on it in the team. A [role](/sdd/reference/entry-kinds/#role) entry can state it, but nothing enforces it.
- **Setup.** Everyone needs git, sdd and a coding agent on their machine. A hosted version is being built, so someone in a non-technical role only connects their agent to it over MCP. The [Vision](/sdd/vision/landscape/) marks where it stands.
