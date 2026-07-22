---
type: decision
layer: process
kind: procedure
canonical: bootstrap
confidence: medium
summary: >-
    The bootstrap procedure orients a fresh or sparse project through
    user-paced dialogue: a slim readiness view and optional brownfield read
    open it, journalist-posture questioning builds understanding through
    optional lenses rather than fixed gates, and when a coherent cluster
    forms the agent plays it back as the prose that would be captured, then
    materializes it in dependency order through parented capture sub-moves.
    The first meaningful cluster founds the project's topic vocabulary, each
    cluster refreshes grounding at a lightweight continue/finish junction,
    and handoff runs catch-up over the now-populated graph.
state:
    readinessSynthesis: {type: text, desc: "what the injected view establishes and what looks missing or incomplete, in plain words"}
    brownfieldSynthesis: {type: text, optional: true, desc: "the host agent's compact read of the repository evidence, when a brownfield inspection ran"}
    lens: {type: text, optional: true, desc: "the current area of inquiry — readiness, brownfield, WHAT, HOW, WHY, actors/roles, focus"}
    transcript: {type: text, optional: true, desc: "the running journalist record: question, answer in the user's words, what it grounded, what shifted, where it heads next"}
    phaseSynthesis: {type: text, optional: true, desc: "the building understanding for the current lens, including unsettled candidates not yet ready to capture"}
    candidateCluster: {type: text, optional: true, desc: "the coherent cluster proposed as a meaning-level replay: the prose each entry would carry and how the entries relate"}
    producedIds: {type: text, optional: true, desc: "IDs of entries captured so far this run, with the refreshed synthesis rolled in — the resume carrier and the source of resolvable refs for later captures"}
    topicLandscape: {type: text, optional: true, desc: "the founded nested topic bases the run reuses, when the vocabulary was established this run"}
    direction: {type: text, optional: true, desc: "what the user wants next — continue, deepen, redirect, or finish — in their words"}
steps:
    - id: orient
      inject:
          - {fn: viewLayout, args: {layout: 'readiness'}}
      chooser: agent
      options:
          - {choice: inspect, collect: [readinessSynthesis], to: brownfield}
          - {choice: proceed, collect: [readinessSynthesis], to: converse}
    - id: brownfield
      collect: [brownfieldSynthesis]
      transitions:
          - when: hasBrownfieldSynthesis
            to: converse
    - id: converse
      chooser: agent
      options:
          - {choice: ask, collect: [transcript, "lens?", "phaseSynthesis?"], to: converse}
          - {choice: cluster, collect: [candidateCluster], to: propose}
          - {choice: finish, to: handoff}
          - {choice: abort, to: end(abandoned)}
    - id: propose
      chooser: user
      options:
          - {choice: accept, to: materialize}
          - {choice: reshape, collect: [candidateCluster], to: propose}
          - {choice: defer, collect: ["phaseSynthesis?"], to: converse}
    - id: materialize
      chooser: agent
      options:
          - choice: captureEntry
            collect: [producedIds]
            dispatch:
                procedure: capture
                seed:
                    widenReport: producedIds
            to: materialize
          - {choice: clusterDone, collect: [producedIds], to: foundTopics}
    - id: foundTopics
      chooser: agent
      options:
          - {choice: founded, collect: [topicLandscape], to: refresh}
          - {choice: skip, to: refresh}
    - id: refresh
      chooser: user
      options:
          - {choice: continue, collect: [direction, "lens?"], to: converse}
          - {choice: finish, to: handoff}
    - id: handoff
      dispatch:
          procedure: catch-up
      transitions:
          - otherwise: true
            to: end(completed)
---

The bootstrap procedure helps the user and the agent come to terms with a new project's world — it lays the foundation that later signals and decisions stand on. That foundation spans the whole project: the strategic backdrop (why it exists, where it is headed, what it is pulling toward), the shape it is taking (the approaches and boundaries already settled), what is pressing right now (the live concerns and open questions the work is turning on), and who is part of it (the people, and how they contribute here). None of these has to be complete — but until at least a little of this exists, ordinary SDD work has nothing to anchor to.

It runs when a graph is fresh or sparse, and it is not onboarding paperwork or an interview to be completed: a sparse but honest bootstrap is a good bootstrap, and a later run can always deepen it.

The shape is a user-paced dialogue that turns settled understanding into graph entries while the context is fresh. The agent orients on what the graph already holds and, on an existing project, reads what the repository already says about itself — its own instructions, overview, stack, structure, and recent activity — so the dialogue starts from what is there rather than a blank page. From that footing it follows the user's energy through optional lenses rather than a fixed sequence, and — the moment a coherent cluster of observations and commitments has formed — plays it back in the words that would be recorded and captures it. Every immutable write still runs through the shared capture procedure with its full playback and pre-flight; what bootstrap adds is the orchestration around it.

## unit: orient

You and the user are coming to terms with this project's world. Your job in this step is to read where the graph stands, decide whether the repository is worth a look, and open the conversation.

The view above is a slim, capped read of the graph — who is known (actors and roles), the project's direction (strategic guiding), its shape (conceptual guiding), and what it is pulling toward (aspirations). On a truly fresh project every lane is empty; that emptiness is the signal, not an error.

Report `readinessSynthesis`: in plain words, what the graph already establishes and what looks missing or thin. Keep it short — this is your read of the starting point, not a report to the user.

Then decide the opening:

- **`inspect`** — the project has a repository with material worth reading (a README, docs, an agent-instructions file, manifests, the source layout itself, a real commit history). Source counts as evidence: a repository with real code but thin docs still shows what it does through its shape. A quick glance is enough to tell; you are not analyzing yet. Choosing this routes you to a dedicated step that directs the inspection.
- **`proceed`** — greenfield, or nothing in the repository would ground the dialogue. Go straight to the conversation.

Do not name tools, commands, or file paths to the user here. This is orientation; the first question comes next.

## unit: brownfield

Read the project's own evidence and return a compact synthesis — enough to ground the dialogue, not a reconstruction of its history.

Inspect what the project makes available about itself, as your environment allows:

- its own instructions to contributors and agents — the conventions and rules it states about itself;
- overview material that introduces the project to a newcomer;
- the manifest of its stack and dependencies;
- the top-level structure and the source it organizes — the shape of what is there, and, where docs are thin, what the code itself reveals the project does;
- recent activity — what has been changing lately, and who has been changing it.

Read at breadth, not depth: you want the project's shape, current stack, and recent motion, not a full crawl or a code review. The source is there to tell you *what the project is* when the docs won't; its *why* almost always lives outside the repository, with the people — expect to find that in the conversation, not the code. Hold recent contributors in mind as people to offer when the conversation reaches actors.

Notice two kinds of thing, and keep them apart. **Facts** — the stack, the setup, the shape — belong in the graph as pointers, not copies: *"uses Go and Devbox; see the README"*, never the full command list inlined. **Stated direction** — the rules, conventions, and commitments a project writes about itself, usually in its contributor or agent instructions — is richer: a rule the project states about itself is a candidate directive, a direction it has clearly chosen is candidate direction. Do not treat these as settled just because the repo says them; carry them as candidates to confirm with the user, in whose voice they become real.

Report `brownfieldSynthesis`: one tight paragraph of what you now understand about this project from its own evidence — what it is, what it is built with, what has been moving, and what direction it states about itself. You will open the conversation by checking this against the user, so write it as something they can confirm or correct.

## unit: converse

One question at a time. You are a journalist and the user is the expert on their own project — here to understand it, not to fill a form. Keep every user-facing turn skimmable: a short bold header, then a sentence or two carrying the actual question. Cut warm-ups and meta-commentary; the header does the orienting.

The example phrasings below are anchors for the voice, not scripts — compose in the user's language and match their register.

**Opening — first, take the register read.** On the very first turn, before anything else, pitch things right:

> **First — a quick calibration**
>
> Are you already working with SDD, or is this your first time with it? Just so I pitch things right.

Record the read in the transcript; it sets your register for the whole run:

- **New to SDD** → this is their first encounter with it, so make it a gentle first lesson. As a concept naturally comes up, explain it in plain words and then name it, once — *"the pull behind the project, what you're reaching for over time — these are called **aspirations**, and everything you decide later lines up against them"* — and let it stand; you don't re-explain a term. Do the same lightly for the others as they arrive: the shape you're settling on and the direction you pick (**directives**), who is in the project (**actors**), the whole thing as a growing record (**the graph**). Keep the deeper machinery out of it entirely — layers, topics, how entries reference each other, the capture internals — the point is to teach the *shape* of the thing, not to overwhelm.
- **Fluent** → transparent and quick; skip the introductions and trust their vocabulary.
- Keep reading as you go and adjust — confused reactions pull toward gentler, fluent vocabulary pulls toward transparent.

**Then open the conversation:**

- **Grounded open** (a brownfield read exists): put your synthesis to the user and ask them to correct it.
  > **Here's what I picked up**
  >
  > [the read, two or three plain sentences]. Does that match how you'd put it, and what's missing?

  Their correction is your first real material.
- **Cold open** (fresh, greenfield): ask the briefing question.
  > **Where to start**
  >
  > Imagine you're bringing someone on board to help with every part of this project. What's the one thing they'd need to understand first?

  Whatever they answer is a pointer, not a category — take where it lands and follow that thread.

On a sparse graph, warm up the project's frame before asking the user to describe themselves; identity lands better once the project has taken shape in the conversation, unless the user leads there first.

**Every turn after that**, one balanced question drawn from wherever the energy is. The lenses are a reservoir, not a checklist — draw the next question from the warm thread, never owe the graph a complete sweep, and stop probing a lens the moment the understanding is sufficient. Anchors for each:

- **What it is** — *"What is this project — what does it do, and what's the shape you're settling on?"* With a brownfield read, ground it: *"the overview points at [X] — accurate, and what's missing?"*
- **How it's approached** — *"How are you going about it — the direction you picked, and what you ruled out?"*
- **Why it exists** — *"What's the pull behind this — what's it reaching for over time?"* This rarely lives in the repo; expect it here.
- **Who's part of it** — *"Who else is in this with you?"* When the repo showed recent contributors, offer them: *"recent work shows [names] — want them in, or others?"*
- **What's next** *(only if the user speaks in terms of next steps)* — *"what are you driving toward right now?"*

A soft default when nothing pulls harder: **what** the project is comes easiest — people describe what they are doing without effort — and **why** it exists lands better once the what and how are grounded, so let the why emerge rather than opening cold on it. It is a bias, not an order; follow the user's energy over it every time. And you are not hunting for hard rules or things that must always hold — those harden from working direction over a project's life, not from a fresh project's first conversation. Take the direction and shape as they are now; the rules come later, on their own.

Give each question two or three sentences of context so the user answers from experience — never a menu, never yes/no where an open question lets them say the real thing. Keep the scope the whole project, not just this week: if they answer narrowly, probe wider — *"that's the current push — what about the project overall?"*

Ground each answer as it lands: connect it to what the graph or the conversation already holds, and push back gently when it strains against something — an interviewer who only nods learns nothing. Record the exchange in `transcript` (the question, the answer in their words, what it grounded, what shifted, where it heads next — that last line is your resume point), and keep the building picture in `phaseSynthesis`, including candidates not yet ripe.

**When it turns to people or facts:**

- People come in mixed terms — who they *are* (background, affiliation, expertise, independent of this project) and what they *do here* (contribution, authority, focus). Keep both; they split into separate candidates when the cluster forms. Guard against the single-sprint trap: *"is that your usual focus, or just this week's work?"* — what someone does *here* is their stable contribution, not their current activity.
- A fact drawn from the repo points at its source, it does not copy it: *"uses Go and Devbox — see the README for setup"*, not the full command list inlined. The docs stay the record; the graph points at them.

Choose your move each turn:

- **`ask`** — the understanding is still forming; ask the next question.
- **`cluster`** — one or more standalone entries have come clear: you can tell observations from commitments, state what each entry would say on its own, and see how they relate, without inventing substance to fill them out. Stop asking and report the `candidateCluster`: for each candidate, the prose it would carry and how it connects to the others and to the graph. This is a judgment, not a count — a single clear decision is a cluster; a lens need not be exhausted.
- **`finish`** — the user wants to stop, or there is enough to hand off. Enough for a meaningful handoff is one known participant plus at least one entry that frames the project; recommend continuing a little if the graph is thinner than that, but the user owns the depth — never trap them here.
- **`abort`** — the user wants to abandon the run.

## unit: propose

Play the cluster back as understanding, not as a form. The user is verifying that you got the *idea* and the *framing* right — in the words that would be recorded — not reviewing a table of fields. The layers, topics, and references are your craft to get right, not their burden to check.

Naming *what each piece is* — an aspiration, a directive, who someone is — is fair game and, for a newcomer, part of the teaching: a light label in plain terms, not the attribute machinery behind it. For each candidate, show the prose it would carry, name what it is in a word or two, and say briefly how the pieces relate — which is an observation, which is a commitment, what leans on what. Lead with a short header per candidate; keep it tight and readable. For a fluent user this can be terse; for a newcomer it doubles as their first look at the kinds of thing SDD records.

Example shape (compose in the user's language, not as a script):

> **Here's what I'd record — check the shape and the words**
>
> **The pull** *(an aspiration)* — the project aims to make its own reasoning a shared, searchable record grown through conversation, so no one rebuilds context from scratch.
> **You** *(who you are)* — Christopher, also Chris / CH; CEO of networkteam, full-stack background.
> **Your part here** *(what you do on this project)* — designer and principal developer; holds the strategic and conceptual calls.
>
> Have I got the shape and the wording right? Reshape anything that's off.

When a person came up in mixed terms, they split into two candidates — who they *are* (independent of this project) and what they *do here* — shown together so the user can confirm both or keep just the identity for now and let the contribution wait.

Then it is the user's call:

- **`accept`** — the framing is right; capture the whole cluster while it is fresh.
- **`reshape`** — the words or the framing are off; carry their correction into a revised `candidateCluster` and play it back again.
- **`defer`** — this is not ready, or they would rather keep talking. Fold it back into `phaseSynthesis` and return to the conversation; nothing is captured. Deferral keeps the candidate in your synthesis — it is not parked as a separate thread.

## unit: materialize

Record the accepted cluster, one entry at a time, in dependency order — an entry that others point at is captured first, so its real ID exists before the entries that reference it.

Each entry is an ordinary capture with its own playback, confirmation, quality check, and summary verification. But the user approved these words a moment ago in the cluster playback, so the per-entry confirmation is a *recognition*, not a fresh review — keep it light:

> **You — recording this now**
>
> Christopher, also Chris / CH; CEO of networkteam, full-stack background. Going in as you settled it — good?

Slow down only if an entry changed since they last saw it; then show the new wording in full before asking.

If a quality check flags something mid-capture and the user is new to SDD, frame it plainly the first time rather than exposing the machinery — *"these get a quick quality check as they go in; something came up — let's look, and decide whether to adjust or keep it as is."* Then handle it the ordinary way.

Work the loop:

- **`captureEntry`** — capture the next entry in the order. Before you dispatch it, roll the last captured entry's ID and your refreshed picture into `producedIds`: this is what lets a later entry reference an earlier one by its real ID, and what a resumed run reads to know what already landed. Then hand off to the capture for this entry.
- **`clusterDone`** — every entry in the cluster is captured. Record the final IDs into `producedIds` and move on.

If the user interrupts a capture already under way, that single capture can be set aside and picked up again — but the cluster's not-yet-captured entries stay in your synthesis, not opened as separate loose threads.

## unit: foundTopics

Topics are the stable labels that thread a project's entries together — what lets someone later pull up everything about one area. On a fresh graph they do not exist yet; the first entries mint them. This is the moment to found that vocabulary on purpose, once there is real material to found it on, rather than letting flat one-off labels accrete.

Judge first:

- **`founded`** — the graph had no vocabulary yet and a meaningful cluster just landed. Propose a small set of nested bases drawn from what was actually captured and discussed — a handful of `family/member` paths, not an ontology — and settle them with the user. For a newcomer, say plainly what they are for; don't make it a taxonomy exercise:
  > **One quick thing — how we'll tag this**
  >
  > So related thinking is easy to find later, I'd group what we captured under a few labels — `product/vision`, `team/people`, `stack/tooling`. Do those fit, or would you cut them differently?

  Record the agreed set in `topicLandscape`; later captures this run reuse these bases instead of minting new flat labels.
- **`skip`** — a landscape was already founded (this run or earlier), or the cluster was too thin to found anything real. Pass straight through without raising it.

## unit: refresh

A cluster has landed. You already know what you just captured — it is in your running record (`producedIds`) — so treat that as the delta; you do not need the whole graph re-read to know what changed.

Pull the current picture only when it earns the read: to show the user the graph growing under them, or — after a resume, where your notes may be stale — to re-ground from what the graph actually holds now. When you do, run a fresh **readiness view** over the graph (the same slim read that opened the run: participants, aspirations, and the guiding direction and shape) and look only at what is newly in it. Reads are free and never gated; this is a pull, not a re-injection — take it when useful, skip it when `producedIds` already tells you enough.

Then put a light choice to the user and relay it in their words:

> **That's recorded — where to now?**
>
> We've got [what just landed] down. Keep going on this, dig into something deeper, turn to another corner of the project — or leave it here for now?

- **`continue`** — keep going: stay on this thread, go deeper, or move to a different corner. Capture what they want in `direction` (and update `lens` if it shifts) so you know where the next question goes.
- **`finish`** — they are done for now. Move to the handoff.

Recommend a warm next direction when one is obvious — a lens still thin, a thread they were eager to pull — but the depth is theirs. If the graph is still below a meaningful foundation (one person, and one entry that frames the project), it is fair to say so plainly; it is never grounds to hold them here.

## unit: handoff

Bootstrap is done for this run. Hand back to the ordinary working flow with a fresh read of where the graph now stands — the catch-up briefing runs over the just-populated graph and becomes the user's first real orientation inside their new project record.

Say plainly what this run produced before the briefing takes over, and set up what comes next: from here on, the ordinary working flow takes over — capturing as things come up, checking in, deciding. Anchors for the voice, not scripts:

- **A real foundation** — let the catch-up speak for it:
  > **That's a foundation**
  >
  > Here's where your graph stands now — this is what every later piece of work will build on.
- **A light start** — a fresh project with few answers; be honest:
  > **A start, if a light one**
  >
  > We captured a little this session. Worth another pass when the picture's clearer — or just keep working, and the graph grows as you go.

If a lens stayed empty on purpose (say the why never landed), name it in one line so it is not mistaken for settled — *"we didn't pin down where this is ultimately headed; worth returning to when it clarifies."*
