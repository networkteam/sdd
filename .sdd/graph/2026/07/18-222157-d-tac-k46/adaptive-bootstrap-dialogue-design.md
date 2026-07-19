# Adaptive bootstrap procedure — dialogue design record (v2, review-refined)

This is the revised design record behind the superseding bootstrap plan. Version 1 rode the original plan (20260718-175730-d-tac-3y2); this version folds in a dedicated review of the user stories, UX, and engine mechanics (2026-07-18), including verification of the engine claims against the Go source. Changes against v1 are marked inline as **[v2]**.

## Purpose

Bootstrap helps the user and agent come to terms with the new project's world. It establishes enough identity and framing that later signals and decisions have meaningful context, without turning initial orientation into onboarding paperwork, a form, or a grilling session.

A sparse but meaningful bootstrap is valid. Later bootstrap runs can deepen the graph.

**[v2]** Bootstrap users are often — not always — at their first touchpoint with SDD (an experienced user bootstrapping a new project is normal too). The design consequence: confirmation surfaces deliver meaning over mechanics, and playback fidelity adapts to the user rather than assuming SDD fluency.

## User stories

### Orientation and repetition

- As a user entering a fresh or sparse project, I want bootstrap to feel like jointly understanding the project's world, so that I gain useful orientation without ceremony.
- As a returning user, I want bootstrap to summarize what the graph already establishes and what appears incomplete, so that I can continue a partial bootstrap without repeating covered ground.
- As a user with limited time or appetite, I want to decide whether to continue, deepen, move elsewhere, or finish, so that bootstrap is only as deep as I want.
- As an agent, I want a slim capped readiness view rather than the full graph, so that I can assess framing without exhausting the context window.
- **[v2]** As a user on a truly empty graph, I want a strong opening question that produces a first pointer, so the dialogue starts from my answer rather than from a lens menu.
- **[v2]** As a user with existing captures, I don't want bootstrap pushed at me — after the graph has content, deepening runs are mine to initiate.

### Host and engine responsibilities

- As a local agent on a brownfield project, I want the procedure to tell me which repository evidence to inspect and synthesize, so that README files, agent instructions, stack, structure, and recent activity can inform the dialogue.
- As the hosted workflow engine, I must not be responsible for browsing Git or repository files. Hosted bootstrap is a separate design problem.

### Interview experience

- As a user, I want the agent to use a journalist posture and treat me as the project expert, so that questions build from what I say rather than follow a questionnaire.
- As a user, I want WHAT, HOW, WHY, brownfield context, and actors/roles used as optional lenses, not required phases.
- As a user, I want concise answers accepted when they establish useful meaning.
- As a user, I want the agent to prompt for capture when a coherent cluster forms, rather than capturing every useful sentence or waiting for an undefined "natural pause."
- As an agent, I want a stock of possible questions as a reservoir, not a checklist, so that I can follow the user's energy and stop probing when sufficient shared understanding exists.

### Cluster capture

- **[v2 — reshaped]** As a user, I want a formed cluster played back as a meaning-level replay — the agent's understanding in the prose that would be captured — so I can verify the agent got the idea and framing right. The field taxonomy (kinds, layers, topics, refs) is the agent's job, not my review burden; I may not know SDD vocabulary yet, and even when I do, the words carry the alignment.
- As a user, I want the whole stable cluster captured while its context is fresh, while every immutable entry still receives exact playback and explicit confirmation — **[v2]** with playback fidelity instructions adapted so it reads as recognition of already-approved words, not a second full review.
- As the outer procedure, I want cluster captures executed in dependency order and each generated ID reported back into my state, so later entries can reference earlier entries without placeholders.
- As the outer procedure, I want unsettled candidates kept in my phase synthesis rather than opened as a backlog of parked captures.
- As a user interrupted during a particular capture, I want that capture parkable and faithfully resumable.
- **[v2]** As a user on a fresh graph, I want the topic vocabulary founded deliberately after the first captured cluster — a small nested landscape I can discuss against concrete entries — not taxonomy talk before anything exists.

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
- actors and roles;
- **[v2]** focus — only when the user describes a "what are the next steps toward …" direction; never solicited for completeness.

These are not mandatory ordered gates. The graph overview and current dialogue determine the warmest useful lens. The agent may recommend one, while the user can redirect or stop.

**[v2] The cold open.** On a truly empty greenfield graph, the fixed opener is the briefing frame:

> "Imagine you're bringing someone on board to help you with every aspect of this project. What's the one thing they need to understand first?"

The answer is a pointer-generator, not a lens answer: the agent classifies where it lands (WHAT, WHY, context, actors) and follows it. When brownfield synthesis exists, the grounded opener replaces the abstract one: "Here's what I understood from the repo — does that match how you'd describe it, and what does it miss?" Opener candidates considered and set aside for the spec's default: the stakes frame ("what's different if it works?"), the origin frame ("what itch does this scratch?"), the moment frame ("where does the project stand right now?") — the moment frame works better as a second, routing beat.

**[v2] Frame before self-description.** The legacy ordering insight (d-prc-cgc — participants self-describe within the project frame, not before it) is retained as an encoded default recommendation bias, not left to agent judgment: on a sparse graph, warm up project framing before inviting the user to describe themselves.

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

**[v2] The confirmation split.** The cluster proposal is the alignment gate: the agent plays back its understanding as the actual prose that would be captured — "here's what I'd record, in these words." The user checks framing and meaning there. Per-entry capture playback then reads as recognition — the approved words, now going in — with fidelity instructions adapted accordingly. The capture invariant (exact playback, explicit confirmation) is untouched; what shifts is the content the user is asked to review, and that field detail is not duplicated into the cluster proposal.

Entries are materialized in dependency order:

```text
phase dialogue
    ↓
candidate cluster A, B, C (meaning-level replay, user accepts)
    ↓
capture A → produced ID A reported into outer state
    ↓
capture B with typed ref to A → produced ID B reported
    ↓
capture C with typed refs to A/B
    ↓
refresh grounding + synthesis → resume bootstrap
```

**[v2] Produced-ID flow.** The engine has no child-to-parent write-back, and deliberately none is added. The agent reports each produced ID and the refreshed synthesis into outer state at the beats where it already talks to the engine — cluster proposal, each capture completion, the direction junction. The beats are free carriers: no extra ceremony, and the state machine rails the agent into the sequence more reliably than prose instructions alone.

The outer procedure owns candidates that are not ready. A parked capture is durable procedure state but not a graph entry; it is used only when one specific capture has begun and must be interrupted. It is not the default cluster queue.

## Topic landscape founding **[v2]**

On a fresh graph there is no topic vocabulary — the first captures mint it. Timing options considered:

- **Settle topics before the first capture** — rejected: taxonomy talk before any entry exists is friction, exactly the ceremony bootstrap avoids.
- **Annotate entries with topics afterwards** — rejected: more ceremony, and annotation is a heavier mechanism than getting labels right at capture.
- **Found the landscape after the first captured cluster** — chosen: the labels exist from capture, and the discussion has concrete material.

Mechanism: during first-cluster capture the agent mints labels as usual (nested `family/member` preferred). Once the cluster lands, the agent proposes a small topic landscape — stable bases derived from the captured entries and the dialogue — the user reshapes or accepts it, and subsequent captures in the run reuse those bases rather than minting flat ad-hoc labels. Historical note: graph topics postdate the legacy sdd-bootstrap skill entirely (its only "topic" mentions are incidental dialogue-navigation prose), so this is new bootstrap substance, not a port.

## Repeated runs and "complete enough"

Every run injects a capped view of:

- active actors and roles;
- conceptual guiding directives that frame the project;
- strategic guiding directives that express direction;
- aspirations.

The agent summarizes what is established and what looks missing or incomplete. A fresh run reconstructs coverage from the graph; a resumed run also retains its transcript and phase synthesis **[v2]** as of the last report beat.

**[v2] Offer policy.** Bootstrap is auto-recommended only on a truly empty graph. As soon as captures exist, deepening runs are user-initiated — so "deliberately skipped lenses get re-offered" cannot become nagging: nothing re-offers.

There is no numeric readiness score. The indispensable foundation for meaningful handoff is one actor plus at least one signal or decision providing project framing. Further identity, direction, and aspiration improve context but may be added in this run or a later one.

## Outer state — why it exists **[v2 rewritten]**

The agent only talks to the engine at report and junction moments; between beats, nothing durable exists outside the agent's context. Outer state therefore exists for three reasons:

1. **Beats are free carriers** — synthesis, unsettled candidates, and produced IDs ride reports the agent sends anyway.
2. **Guards need fields** — lens, direction choice, produced IDs for seeding child captures.
3. **Rails** — the state machine pushes the agent through the sequence more consistently than written instructions and hope.

**The honest resume promise:** a resumed run recovers the graph (captured entries) plus state as of the last beat. Dialogue-only material since then is re-derived — acceptable and consistent with capture-while-fresh: anything stable should already be in the graph, and unsettled candidates are by definition not stable. There is no lossless-dialogue-recovery guarantee, and no test burden for one. (This also resolves the mid-cluster staleness question: in live context the agent knows what landed; on cold resume, the last-beat state is the carrier — which is why synthesis rides every beat.)

State fields: current lens, readiness/current-state synthesis, optional brownfield synthesis, running transcript, phase synthesis, unsettled candidate cluster, produced child-capture IDs, dispatch grounding for seeding children, user direction. The candidate cluster remains textual structured synthesis; no new general-purpose capture-draft domain type in v1.

## Brownfield boundary

The engine injects graph context only. For a brownfield project, served instructions ask the local host agent to inspect relevant project evidence and return a compact synthesis. The likely evidence includes project/agent instructions, README material, manifests and stack, top-level structure, and recent activity. The exact inspection adapts to the host and dialogue.

A future hosted-engine bootstrap that can inspect repositories requires separate authority and design and is outside this procedure.

## Engine mechanics **[v2 — verified against source]**

Confirmed supported (verified, not assumed):

- procedures are graph entries; a base procedure entry with `canonical: bootstrap` auto-lists as a startable move — no Go registration (`application/application.go` Procedures);
- injection is computed fresh on every serve — loop-backs re-ground automatically (`internal/engine/instance.go` renderUnit);
- park/resume replays full typed state from the JSONL event log (`internal/engine/session.go` ReplaySession);
- loops are freely expressible; transitions may target any earlier step;
- dispatch seeding: a parent can seed any state field the child declares — capture's params (anchor, supersedes, closes, kind) plus all its declared state (body, refs, topics, …) (`internal/engine/session.go` seedFromParent, store SetStart).

Corrected from v1's "already supported" list:

- **Produced entry IDs are NOT returned to the parent's state.** `Serve.Produced` goes to the agent; seeding is strictly parent→child. Bootstrap uses agent relay at report beats (see Cluster execution). This is a spec-encoded discipline, not an engine feature.

Required engine work:

- **Actor/role/focus writes are impossible through the engine today.** `application.EntryDraft` and the `newEntry` command (`application/workflow_registry.go` runWorkflowNewEntry) carry no canonical/aliases (actor), actor linkage (role), or involvement/focus-actors/when (focus); `model.ValidateEntry` hard-fails without them. The CLI-side `NewEntryCmd` already carries all these fields — the gap is engine wiring only.
- **Capture spec extensions:** new state fields for the kind-specific data, new presence predicates (`internal/engine/predicates.go` has nothing like hasCanonical/actorResolves), playback-unit rendering, and a **kind-conditional assemble gate** — the current `hasRefs and hasTopics` requirement structurally blocks actor capture (an actor signal has no natural refs).
- **Actionable gate errors:** write-gate validation failures must re-serve naming the issue and routing to a fixable step, not wedge the instance (the s-prc-g0j failure mode).
- **Capped readiness layout:** no existing layout splits conceptual vs strategic guiding directives; composable from existing `layer()`/`intent()` view vocabulary as a new layout string in the bootstrap spec — likely no Go change.
- Slicing: actor/role first (bootstrap's hard dependency — handoff needs an actor), focus second (heavier: involvement triples with resolvable targets, per-involvement overrides, `when` ranges, possibly a new engine variable type). Focus support closes s-prc-g0j.
- Procedure tests: empty and partially bootstrapped graphs, skipped lenses, shallow and deep interviews, multiple clusters per lens, dependency-ordered references, cluster deferral, child interruption/resume including cold resume from last-beat state, repeated fresh runs, resumed runs, actor/role/focus capture, catch-up handoff.

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

### **[v2]** Full field-level cluster proposal (kinds, layers, topics, refs presented for user review before capture)

Rejected: it duplicates capture playback, doubles the confirmation touchpoints per entry (~3 per entry), and asks users — possibly at their first SDD touchpoint — to review taxonomy they cannot meaningfully verify. The meaning-level replay in the words to be captured is the alignment gate; field structure is the agent's job.

### **[v2]** Engine child→parent produced-ID write-back

Rejected: agent relay at report beats covers the need with zero engine surface, is park-safe (IDs land in state at the same beat the capture completes), and keeps the parent-child model's one-directional seeding simple.

### **[v2]** Lossless dialogue resume (turn-by-turn fidelity of unsettled candidates)

Rejected: the engine can only persist what reports carry, the beats already carry everything stable, and capture-while-fresh makes dialogue-only loss acceptable by design. A stronger guarantee would add test burden for a rare case the philosophy explicitly tolerates.

## Tone contract

Bootstrap should feel generative and clarifying: two participants coming to terms with the project's world. It may recommend a useful next direction, but the user owns depth. It stops being useful when it feels like a form, completeness audit, or interrogation.
