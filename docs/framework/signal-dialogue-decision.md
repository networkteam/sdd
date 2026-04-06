# Signal → Dialogue → Decision

A framework for human-agent collaboration in product development.

## Core Idea

One universal loop governs all productive work:

**Signal → Dialogue → Decision**

This loop operates across layers of abstraction and involves both humans and agents as participants. It replaces sequential phase models (plan → build → test → release) with continuous, concurrent feedback loops.

## The Loop

- **Signal**: An observation from any source — a user complaint, a metric shift, an agent's code review finding, a market trend, a test failure. Signals are inputs that suggest a gap between actual and target state.
- **Dialogue**: Collaborative reasoning about what the signal means and what to do. Involves the participants defined by the contract for that layer and context. Dialogue happens between humans, between agents, and between humans and agents.
- **Decision**: A commitment to a direction, recorded immutably. Decisions are the durable asset — not code, not specs, not tickets. A decision captures what was decided, why, and in what context.
- **Action**: Something that was done — a fact of execution. Actions reference the decisions they implement. They close the loop: actions produce new signals, which trigger new dialogues and decisions.

The full loop is: **Signal → Dialogue → Decision → Action → Signal...**

The loop is fractal: it operates the same way whether a CEO is responding to a market shift or an agent is responding to a test failure. What varies is the layer, the participants, and the tempo.

## Layers

The loop runs at five layers of abstraction. These are not org chart levels — they describe the depth and time horizon of the thinking involved.

| Layer | Thinking | Horizon | Typical human/agent balance |
|-------|----------|---------|----------------------------|
| **Strategic** | Why does this exist? What future are we shaping? | Long | Humans dominant, agents provide data/signals |
| **Conceptual** | What approach? What are the key ideas and boundaries? | Medium | Heavy dialogue between humans and agents |
| **Tactical** | How do we realize it? What structures and trade-offs? | Short | Agents can lead, humans review at decision points |
| **Operational** | Making it happen, reacting to immediate signals | Near-instant | Agents largely autonomous within guardrails |
| **Process** | How do we work? Contracts, review rules, release process | Evolving | Team decides, system enforces |

Signal → Dialogue → Decision happens at every layer. The matrix of layers x loop stages is the full picture — not a pipeline but a two-dimensional space where work happens concurrently.

Cross-layer flow is essential:
- Signals flow **up**: operational friction clusters into tactical questions, tactical patterns challenge conceptual assumptions, conceptual dead-ends inform strategy
- Decisions flow **down**: strategic decisions constrain conceptual options, conceptual choices constrain tactical approaches, tactical decisions set operational guardrails

## Participants

Participants are not fixed roles. They are defined by **contracts** — contextual agreements about who participates in which dialogues and who has decision authority.

**What humans bring:**
- Taste — knowing what feels right before you can articulate why
- Strategic judgment — which gaps matter, which to ignore
- Stakeholder empathy — understanding real needs vs. stated needs
- Coherence — holding the big picture, sensing drift
- Accountability — owning outcomes under uncertainty

**What agents bring:**
- Throughput — exploring options, generating variants, executing patterns
- Vigilance — watching for drift, checking contracts, tireless attention
- Memory — holding large context, noticing inconsistencies
- Compression — distilling large amounts of information into decision-relevant signals

**What stakeholders bring:**
- Reality — actual needs, actual pain, actual usage patterns
- Feedback — signals from the world the product exists in

## Key Mechanisms

### Contracts

A contract defines, for a given context: who participates in the dialogue, and who has decision authority. Contracts vary by layer, by risk level, and by domain.

Contracts emerge through the same loop as everything else. As people work, patterns form — who decides what, who reviews what, who owns which domain. A review agent observes these patterns and surfaces them as signals: "Based on the last 15 decisions, Jun has been deciding all content and experience questions. Should this be made explicit?" The team discusses and captures it as a contract decision — a decision about the process itself.

Contract decisions live in the graph like any other decision — immutable, referenceable, and enforceable by the system. They can be challenged later by new signals and superseded by new decisions. A new team member can read the contract decisions to understand how the team works. The system uses them to route questions and enforce review requirements.

The contract is the real process definition. Not a sprint plan or ticket workflow, but "for this kind of change, these perspectives are required, and this level of authority is needed to decide."

### Coverage

Coverage tracks where change is unevaluated. With agents producing changes at high throughput, unevaluated change accumulates fast. Coverage makes the unknown space visible and bounded.

Two kinds of delta to track:
- **Delta-as-gap**: the identified difference between actual and target state — an evaluation signal, the compass
- **Delta-as-change**: a resulting change to the system (UI, architecture, code) — an artifact of action that needs review

Every delta-as-change should be evaluated against a delta-as-gap. Coverage answers: what percentage of changes have been assessed against their intended gap?

Coverage applies at every layer: you can have thorough code review (tactical) but zero architecture evaluation (conceptual). Making this visible turns unknown space from anxiety into managed risk.

### Distillation

Raw output is not a signal. A 3000-line spec is not a signal. A multi-hour agent session is not a signal. Distillation compresses results into decision-relevant form for the receiving layer.

Principles:
- Shape output for the **decision needed**, not as a summary of activity
- Multiple agent perspectives provide triangulation — calibrate number of perspectives to risk
- Agents track decisions and uncertainty **as they work**, not as post-hoc summaries
- Humans assess whether the distillation is trustworthy and the coverage sufficient — not review every line

### Guardrails

Guardrails are constraints that enable autonomous action within boundaries. They emerge from decisions, harden over time, and can be challenged by new signals.

Lifecycle: signal triggers dialogue → decision establishes a constraint → constraint becomes a guardrail → guardrail enables agents to act without per-decision human approval → new signals may challenge the guardrail → new dialogue and decision.

### Exploration over Planning

Exploration IS evaluation. Instead of exhaustive upfront planning, test hypotheses through cheap experiments. Agents make this viable because the cost of exploration drops dramatically.

You don't need alignment before exploring — you need alignment before **committing**. Let multiple hypotheses be explored in parallel, then converge on a decision. Alignment happens at decision points, not at ideation.

## The Decision Graph

Decisions and signals are stored as an evolutionary DAG (directed acyclic graph), implemented on Git.

### Analogy to Git

| Git | Decision Graph |
|-----|---------------|
| Commit | Decision, Signal, or Action (immutable, small, with context) |
| Branch | Experiment / parallel exploration |
| Merge / PR | Experiment validated → decision integrated into main line |
| HEAD | Current shared understanding |
| Diff | Delta between any two states of understanding |
| Log | Full traceable history of how we got here |

### Immutability Constraint

**Documents are never modified after creation.** This is a hard constraint.

- A decision is never updated. It is **superseded** by a new decision that lists the old one in its `supersedes` field (distinct from `refs`, which means "builds on").
- The old document does not know about its successor — just like a Git commit doesn't know what comes after it.
- "Current state" is always a query result: all decisions with no successor that supersedes them. Whether something is "active," "experimental," or "superseded" is not a field on the document — it's a property of its position in the graph.
- Current state is **reconstructed by traversing the graph**, never maintained as a separate document.

### No Redundancy Across Layers

**Each fact lives in exactly one place.** This is a hard constraint.

- A tactical decision **references** the conceptual decision it's based on — it does not restate it.
- If a strategic decision is superseded, the chain of downstream decisions that reference it is visibly affected.
- There is no vision document, PRD, spec, task description, and ticket all restating the same information at different levels of detail. There is one decision per fact, linked by references.

### Document Format

### Document IDs

Each document's filename is its ID, formatted as:

```
{YYYYMMDD}-{HHmmss}-{type}-{layer}-{xxx}.md
```

Type abbreviations: `d` (decision), `s` (signal), `a` (action).
Layer abbreviations: `stg` (strategic), `cpt` (conceptual), `tac` (tactical), `ops` (operational), `prc` (process).

Where `xxx` is a short random suffix to prevent collisions. Examples:
- `20260405-143022-d-stg-k7x.md`
- `20260405-150301-s-ops-m2p.md`

IDs are human-readable, chronologically sortable, self-describing, and require no coordination across branches.

### Document Format

```markdown
---
type: decision | signal | action
layer: strategic | conceptual | tactical | operational | process
refs: [filenames of entries this builds on / depends on]
supersedes: [filenames of decisions this replaces]
participants: [who was in the dialogue]
confidence: high | medium | low
---

[Short, specific content. No restating context from refs.]
```

### Materialized Views

No one reads the graph directly for day-to-day work. Agents and tools materialize views on demand:

- "Show me all active decisions affecting auth"
- "What's the coverage for the billing domain?"
- "Materialize current context for an agent starting work on payments"
- "What experiments are in flight and what signals have they produced?"

These views are ephemeral — derived from the graph, never maintained as separate documents.

## What This Replaces

**Deliberately absent:**
- Backlogs, sprints, fixed iterations — cadence is driven by signal density, not a calendar
- Prescribed roles (Product Owner, Scrum Master, Architect) — contracts define participation contextually
- Mandatory artifacts (specs, PRDs, design docs) — decisions are the only durable artifact
- Sequential phases (plan → build → test → release) — all layers operate concurrently
- Ticket lifecycle (todo → in progress → done) — the unit of work is the loop, not the ticket

**What replaces the coordination function of Scrum/Agile:**
- The decision graph provides shared context — anyone can read HEAD to understand current state
- Coverage tracking provides visibility — where is attention needed?
- Contracts provide clarity — who participates in what?
- Signals provide cadence — loops turn when signals warrant, not on schedule

## Insights

- Coding becoming cheap inverts the bottleneck: the scarce resource is knowing whether what was built is right, not getting it built.
- Code may be less like a cathedral to maintain and more like a rendered output of decisions + constraints. Technical debt is not messy code — it's lost decisions.
- The human skill shifts from "review all the code" to "assess whether the distillation is trustworthy and the coverage is sufficient for the risk."
- The framework applies beyond development to the whole product lifecycle: discovery, marketing, operations, support. The loop is the same; the layers, participants, and contracts change.
- Existing frameworks (OODA, Cynefin, Viable System Model, Lean) share DNA with this approach but were designed for humans-only. The novel element is participants that operate at wildly different speeds, hold vast context, and run in parallel.

## Open Questions

- **Bootstrapping**: Explored in the Kōgen story — a person introduces the system, initial signals and direction are captured through onboarding dialogue. Needs more exploration for larger teams joining an existing graph.
- **Graph growth**: The graph grows indefinitely. Materialized views help, but attention management (decay surfacing, parking decisions, relevance filtering) is an unsolved design challenge.
- **Trust calibration**: When does a human trust an agent's distillation enough to decide without going deeper? How does this evolve over time?
- **Conflict resolution**: Explored partially — dialogue to narrow disagreement, then branch experiments to test. Needs stress-testing in higher-stakes settings.
- **Tooling**: What are the minimal tools/skills needed to make this workable? Graph traversal, coverage dashboards, signal routing, materialized views, session management with graph-based context reconstruction.
- **Adoption**: Explored in Kōgen — organic adoption by one person, then team invitation. Needs exploration for larger orgs with existing processes.
- **Compliance and regulation**: How do mandatory external constraints (legal, regulatory) enter the graph? How do mandatory review contracts differ from voluntary ones? (See signals.md for details.)

## Next Steps

- Write a second story in a compliance-heavy setting (healthcare or fintech) to stress-test mandatory reviews, external guardrails, audit trails, multi-team coordination, and risk-based escalation.
- Identify minimal tooling needed to make the framework workable in practice.
