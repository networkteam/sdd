# Adaptive bootstrap procedure — dialogue design record

## Purpose

Bootstrap helps the user and agent come to terms with the new project's world. It establishes enough identity and framing that later signals and decisions have meaningful context, without turning initial orientation into onboarding paperwork, a form, or a grilling session.

A sparse but meaningful bootstrap is valid. Later bootstrap runs can deepen the graph.

## User stories

### Orientation and repetition

- As a user entering a fresh or sparse project, I want bootstrap to feel like jointly understanding the project's world, so that I gain useful orientation without ceremony.
- As a returning user, I want bootstrap to summarize what the graph already establishes and what appears incomplete, so that I can continue a partial bootstrap without repeating covered ground.
- As a user with limited time or appetite, I want to decide whether to continue, deepen, move elsewhere, or finish, so that bootstrap is only as deep as I want.
- As an agent, I want a slim capped readiness view rather than the full graph, so that I can assess framing without exhausting the context window.

### Host and engine responsibilities

- As a local agent on a brownfield project, I want the procedure to tell me which repository evidence to inspect and synthesize, so that README files, agent instructions, stack, structure, and recent activity can inform the dialogue.
- As the hosted workflow engine, I must not be responsible for browsing Git or repository files. Hosted bootstrap is a separate design problem.

### Interview experience

- As a user, I want the agent to use a journalist posture and treat me as the project expert, so that questions build from what I say rather than follow a questionnaire.
- As a user, I want WHAT, HOW, WHY, brownfield context, and actors/roles used as optional lenses, not required phases.
- As a user, I want concise answers accepted when they establish useful meaning.
- As a user, I want the agent to prompt for capture when a coherent cluster forms, rather than capturing every useful sentence or waiting for an undefined “natural pause.”
- As an agent, I want a stock of possible questions as a reservoir, not a checklist, so that I can follow the user's energy and stop probing when sufficient shared understanding exists.

### Cluster capture

- As a user, I want a formed cluster presented in SDD vocabulary—candidate signals and decisions with proposed kinds, layers, topics, participants, and typed references—so that I can reshape, defer, or approve it knowingly.
- As a user, I want the whole stable cluster captured while its context is fresh, while every immutable entry still receives exact playback and explicit confirmation.
- As the outer procedure, I want cluster captures executed in dependency order and each generated ID returned, so later entries can reference earlier entries without placeholders.
- As the outer procedure, I want unsettled candidates kept in my phase synthesis rather than opened as a backlog of parked captures.
- As a user interrupted during a particular capture, I want that capture parkable and faithfully resumable.

### Completion and handoff

- As later SDD work, I need at least one actor and one framing signal or decision to give the graph meaningful project context.
- As a user, I want identity, direction, and aspiration to deepen when useful rather than satisfy a mechanical completeness score.
- As a user finishing bootstrap, I want catch-up to re-read the updated graph and provide the handoff into ordinary SDD work.

## Dialogue model

Bootstrap is a persistent outer orchestration procedure. Its conversational lenses are:

- readiness and current graph orientation;
- optional brownfield repository context;
- WHAT: what exists or is being built;
- HOW: principles, posture, and differentiators;
- WHY: purpose, direction, and aspiration;
- actors and roles.

These are not mandatory ordered gates. The graph overview and current dialogue determine the warmest useful lens. The agent may recommend one, while the user can redirect or stop.

Within a lens:

1. Ask one balanced question grounded in the current graph and transcript.
2. Integrate the answer into the running synthesis.
3. Follow up only when the answer invites depth or a small clarification is needed to preserve meaning.
4. Once the agent can explain one or more standalone graph entries whose relationships are reasonably clear, stop questioning and propose the cluster.
5. Let the user reshape it, keep exploring, defer it, or accept it.
6. Capture every accepted stable candidate while the context is fresh.
7. Refresh graph grounding and offer to continue, deepen, move to another apparent gap, or finish.

A coherent cluster is a conversational judgment, not a count. It exists when the agent can distinguish observations from commitments, articulate candidate entries with standalone meaning, and propose meaningful topics and resolvable references without inventing substance. It does not require exhausting the lens.

## Cluster execution

A cluster is not an atomic graph write. Each entry uses the shared capture procedure and retains its playback, explicit confirmation, pre-flight gate, and summary verification.

Entries are materialized in dependency order:

```text
phase dialogue
    ↓
candidate cluster A, B, C
    ↓
capture A → generated ID A
    ↓
capture B with typed ref to A → generated ID B
    ↓
capture C with typed refs to A/B
    ↓
refresh graph → resume bootstrap
```

The outer procedure owns candidates that are not ready. A parked capture is durable procedure state but not a graph entry; it is used only when one specific capture has begun and must be interrupted. It is not the default cluster queue.

## Repeated runs and “complete enough”

Every run injects a capped view of:

- active actors and roles;
- conceptual guiding directives that frame the project;
- strategic guiding directives that express direction;
- aspirations.

The agent summarizes what is established and what looks missing or incomplete. A fresh run reconstructs coverage from the graph; a resumed run also retains its transcript and phase synthesis.

There is no numeric readiness score. The indispensable foundation for meaningful handoff is one actor plus at least one signal or decision providing project framing. Further identity, direction, and aspiration improve context but may be added in this run or a later one.

## Brownfield boundary

The engine injects graph context only. For a brownfield project, served instructions ask the local host agent to inspect relevant project evidence and return a compact synthesis. The likely evidence includes project/agent instructions, README material, manifests and stack, top-level structure, and recent activity. The exact inspection adapts to the host and dialogue.

A future hosted-engine bootstrap that can inspect repositories requires separate authority and design and is outside this procedure.

## Proposed outer state

The exact names may follow the procedure schema conventions, but the outer state needs to retain:

- current lens or area of inquiry;
- readiness/current-state synthesis;
- optional brownfield synthesis;
- running interview transcript;
- current phase synthesis;
- unsettled candidate cluster;
- IDs produced by completed child captures;
- enough dispatch grounding to seed child captures;
- whether the user wants to continue, deepen, redirect, or hand off.

The candidate cluster can remain textual structured synthesis; a new general-purpose capture-draft domain type is not required for the first version.

## Engine mechanics

Already supported:

- persistent procedure state and loops;
- query injection before a step is served;
- parent-child procedure lineage;
- agent-mediated dispatch of capture sub-moves;
- produced entry IDs returned by capture;
- park/resume fidelity;
- graph refresh after child completion.

Required work:

- add the canonical bootstrap procedure;
- provide a slim capped readiness injection, likely using the existing view-layout query rather than introducing a scoring predicate;
- extend the shared capture/newEntry state and command path for actor canonical identity and aliases;
- extend it for role-to-actor linkage;
- ensure those kind-specific fields appear in exact playback and write validation;
- cover repeated runs, skipped lenses, multiple clusters per lens, dependency-ordered capture, interruption/resume, and catch-up handoff in procedure tests.

## Alternatives rejected

### Mandatory phase sequence

Rejected because WHAT/HOW/WHY and actor work are lenses, not universal gates. Repeated runs and existing graph coverage must alter the route.

### Capture every crystallized statement immediately

Rejected because it turns dialogue into micromanagement and prevents thoughts from settling into a meaningful graph thread.

### Wait until the end of a phase

Rejected because stable intermediate understanding should become durable while context is fresh, and phases may have an indefinite number of clusters.

### Park every candidate capture until phase end

Rejected because parked drafts are not graph-visible, unresolved future refs cannot be written, and multiple child threads create a stale backlog. The outer synthesis is the appropriate owner for unsettled candidates.

### Engine-side repository inspection

Rejected for the local-agent procedure. Repository inspection is host work directed by the procedure; hosted engine inspection is another story.

## Tone contract

Bootstrap should feel generative and clarifying: two participants coming to terms with the project's world. It may recommend a useful next direction, but the user owns depth. It stops being useful when it feels like a form, completeness audit, or interrogation.
