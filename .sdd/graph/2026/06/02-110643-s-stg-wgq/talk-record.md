# SDD — Lightning Talk (v0.6)

A ~5-minute conference lightning talk introducing SDD to an audience of mostly
web developers, plus less-technical attendees. Source deck: "SDD Lightning Talk v2.html".
This record captures the slide content and the delivered narration.

**Tagline:** Keep humans and AI agents aligned across parallel work.

---

## 1 — Cover
SDD · signal — dialogue — decision.
> I want to spend five minutes on a tool we built called SDD — signal, dialogue,
> decision. The one-liner: it keeps humans and AI agents aligned when work is
> happening in parallel. I'll cover why we built it, where it's going, and what
> you can use today.

## 2 — Why we built it (the problem)
- Without a shared memory: the insight and decisions from each agent session get lost once the session ends.
- Most other tools scatter state across separate artifacts (PRDs, specs, roadmap, wiki, issues) that duplicate, drift, and resist new insight.
> Here's the problem. When you work with AI agents, every session produces a ton of
> reasoning — what you noticed, what you decided, and why. The moment the session
> ends, most of it evaporates, and the next person starts cold. The tools we'd reach
> for don't fix it — docs, plans, roadmaps, wikis, tickets. Each holds a slice, they
> drift apart, and when a new insight shows up nobody knows which place to update.

## 3 — From scattered artifacts to one graph (the shift)
New insight becomes a new entry. State is derived from the graph — never maintained in parallel.
Not a recording of what was said — a confirmed record of what you decided, and why
(including alternatives considered). Every entry is something a person confirmed,
labeled so it's findable, and linked to what it builds on. Reasoning you can pick up
and keep building — not a pile to search.
> So here's the shift, and it's the real differentiator. Other tools record everything
> automatically and hand you a searchable pile. SDD captures what matters, deliberately
> — every entry is confirmed by a person, labeled so it's findable, and linked to what
> it builds on. It's reasoning, not a recording: a confirmed record of what you decided
> and why, that the next person — or the next agent — can pick up and keep building.

## 4 — The loop
Signal → Dialogue → Decision → Done signal → (loop). Work sits between a decision and its done signal.
> It runs on one loop. You notice something — a signal. You talk it through. You commit
> to something — a decision. When that decision is fulfilled, a done signal closes it and
> surfaces the next thing. Dialogue and the work itself aren't recorded; everything else
> lands in the graph as an immutable entry, and the loop turns again.

## 5 — Two kinds of entry
- Signal (s) — something you noticed. Seven kinds: gap, fact, question, insight, done, actor, annotation.
- Decision (d) — something you committed to. Seven kinds: directive, activity, plan, contract, aspiration, role, focus.
- Entries are never edited; to change direction you add a new entry that supersedes the old — all tracked in Git.
> There are only two kinds of entry. A signal is something you noticed; a decision is
> something you committed to. Each has a handful of kinds for nuance — a signal might be
> a gap or a question, a decision a plan or a directive. The rule: entries are never
> edited. To change direction you add a new entry that supersedes the old, so the graph
> is a true record of how the thinking evolved — all in Git.

## 6 — How it feels to use
Live `/sdd` in Claude Code: greets you with current focus, what changed, what's waiting;
you talk, it plays back a proposed entry to approve. It can pull candidates from
transcripts and notes — but nothing lands until you confirm it.
> Here's what it feels like, because people assume it's bookkeeping. It isn't. You open
> Claude Code, type slash-sdd, and it greets you like a colleague: here's the focus,
> here's what changed, here's what's waiting. Then you just talk — 'the importer chokes
> on recipes with no servings field' — and it plays back a proposed entry to approve. And
> the honest nuance: it can mine transcripts and notes for candidates, but nothing lands
> without you confirming it — so you get the convenience of extraction without an
> unverified pile.

## 7 — Who does what
- Agents: work with information and run autonomously where they can (reading the graph, drafting entries, operational steps).
- Humans: make the calls, provide taste, raise what data can't surface (steering and reviewing).
- Dialogue, not bureaucracy, moves the graph forward.
> This works because each side does what it's good at. Agents work with information and
> run autonomously where they can — reading the graph, drafting entries, doing the
> operational steps. Humans make the calls, bring the taste, and raise what data can't
> surface. What connects them is dialogue — not forms, not bureaucracy. That's the
> principle the tool is built on.

## 8 — What's usable today (v0.6 · brew install networkteam/tap/sdd)
Append-only graph of signals & decisions; first-class participants (actors + roles);
topics (nested tags); focus (commit attention, assign actors); pre-flight LLM validator
on each capture; three-mode search (keyword, vector, hybrid); mine external material
(transcripts, articles, notes); Git-native & immutable, with multilingual authoring.
> And this isn't someday — it's shipping today, one brew install away. You get the
> append-only graph, actors and roles, topics that thread across it, focus to commit
> attention, validation on every entry before it lands, search by keyword or meaning, the
> ability to pull transcripts and notes straight in, and it's all Git-native, immutable,
> and multilingual. You can pick it up this afternoon.

## 9 — Where this is heading (the vision)
1. Beyond the terminal — engage the graph visually, as a meeting sidekick, via voice, or as a hosted agent connector without complicated setup; reports generated from the graph.
2. Include more stakeholders — anyone on the project (designer, operator, business owner) can interact and ask questions.
3. Agents that run themselves — working autonomously, guided by the decisions already in the graph, while humans review and steer.
> Where's it heading? Three shifts. Beyond the terminal — anyone on the project, a
> designer or an operator, engaging the graph through chat or voice. Capture inside the
> work — a finding while testing, a question in a meeting, recorded where it happens. And
> agents that run themselves, guided by the decisions already in the graph, while humans
> review and steer.

## 10 — Closing
brew install networkteam/tap/sdd · sdd init && /sdd · github.com/networkteam/sdd · MIT · v0.6.0
> So that's SDD — signal, dialogue, decision. Open source, MIT, two commands to start.
> If it resonates, give it a try and star the repo to follow where it goes. Thank you.
