---
sdd-content-hash: 38f78f78e08e248529981a3f9f656882cad4d71cf3cc89840799dff159610a7f
sdd-version: dev
---
# SDD Framework Concepts

## The Loop

One universal loop: **Signal → Dialogue → Decision → Done signal → Signal...**

- **Signal** (`s`): Something the graph should know about — an observation, gap, fact, open question, synthesis, or a record of completed work.
- **Dialogue**: Collaborative reasoning about what a signal means and what to do. Happens between humans, agents, or both. Not recorded directly — its outputs become signals and decisions.
- **Decision** (`d`): Something we commit to. Immutable once recorded. The durable asset of the framework.

Completed work is itself a signal — a `kind: done` signal — which closes the decision it fulfils and adds to the pool of observations future loops draw from.

## Entry Types

The graph has **two entry types** — signal and decision. Each carries an explicit **kind** that sharpens what the entry is for. Pre-flight enforces that kind matches the narrative shape.

### Signal kinds (5)

| Kind | Question it answers | Default? |
|---|---|---|
| `gap` | What needs attention? | yes |
| `fact` | What do we know? | no |
| `question` | What do we not know? | no |
| `insight` | What have we synthesized? | no |
| `done` | What was accomplished? | no |

A `done` signal records a commitment fulfilled — it must carry at least one `closes` or `refs` pointing at the commitment it completes.

### Decision kinds (5)

| Kind | Question it answers | Default? |
|---|---|---|
| `directive` | Which way do we go? | yes |
| `activity` | What's next to do? | no |
| `plan` | What must be true when done? | no |
| `contract` | What must always hold? | no |
| `aspiration` | What are we pulling toward? | no |

Plan decisions require a `## Acceptance criteria` section with at least one checklist item. Each AC is a verifiable outcome — the contract between plan author, implementing agent, and the pre-flight validator that checks the closing done signal.

## Distinguishing tests

When drafting a decision, the kind emerges from dialogue. Apply these tests in order:

1. Does the entry push against a constraint that should always hold? → **contract**
2. Does it pull toward a direction with no completion criterion? → **aspiration**
3. Does the narrative justify a choice against alternatives? → **directive**
4. Does it shape the WHAT (define verifiable outcomes)? → **plan** (requires `## Acceptance criteria`)
5. Does it dispatch THAT work happen (shape known from context)? → **activity**

### Strategic directive vs aspiration

Test: does the entry have a plausible completion criterion?

- Yes (closable by done or supersede) → **directive**
- No (perpetual pull, every decision aligned against it) → **aspiration**

Confidence often signals this — directives can go high-confidence once settled; aspirations sit at medium indefinitely.

### Activity vs plan (WHAT vs THAT)

- **Plan** shapes the WHAT — defines what must be true when complete. ACs are the mechanism.
- **Activity** dispatches THAT — specifies work whose shape is already known from context (parent plan, refs, or self-evident narrative).

Boundary test: is validation a single self-evident "did you do the thing?" → activity. Does the AC specify a testable outcome separable from the work itself? → plan.

## Layers

Every entry has a layer describing the depth of thinking:

| Layer | Abbrev | Thinking |
|-------|--------|----------|
| Strategic | `stg` | Why does this exist? What direction? |
| Conceptual | `cpt` | What approach? Key ideas and boundaries |
| Tactical | `tac` | How to realize it? Structures and trade-offs |
| Operational | `ops` | Making it happen. Individual implementation steps |
| Process | `prc` | How do we work? Contracts, review rules, release process |

## Immutability

**Documents are never modified after creation.** This is a hard constraint.

- A decision is superseded by a new decision with an explicit `supersedes` field (distinct from `refs` which means "builds on").
- Current state is reconstructed by traversing the graph, never maintained separately.
- Each fact lives in exactly one place — no redundancy across layers.

## Reference Fields

Three fields with distinct semantics:

- `refs`: "builds on / depends on" — context or foundation, **no status effect**
- `supersedes`: "replaces" — the referenced entry is no longer active/open
- `closes`: "resolves / fulfills" — the referenced entry is no longer active/open. Decisions close signals; done-kind signals close decisions and gap signals.

**Open signal** = not superseded, not closed. **Active decision** = not superseded, not closed.

## Retirement primitives

Every entry is retireable. Two primitives:

- **supersedes** — same-kind successor replaces it
- **closes** — new entry retires it without replacement

Per-kind retirement paths:

| Entry | Supersede path | Close path |
|---|---|---|
| gap | refined gap | decision addressing it; or done signal (short-loop, see below) |
| fact | corrected fact | directive: "no longer true / no longer relevant" |
| question | refined question | directive: "answered as X" or "won't pursue"; or fact / insight (dissolution) |
| insight | corrected insight | directive: "noted, no action needed" |
| done | corrective done (rare) | — (terminal — facts of execution) |
| directive | replacement directive | done signal (standard); directive retiring it |
| activity | replacement activity | done signal (standard); directive retiring it |
| contract | replacement contract | directive retiring it |
| plan | restructured plan | done signal (via ACs); directive retiring it |
| aspiration | evolved aspiration | directive retiring it |

**Retirement rationale is required** when closing a stable-kind entry (fact, insight, contract, aspiration). Pre-flight checks that the narrative states *why* — not whether the why is correct.

## Short-loop closure

A `kind: done` signal may close a `kind: gap` signal directly, bypassing a decision. Use for narrow execution work where no choice was made — *"updated X to fix Y"*.

**Smell test before drafting:** if you'd have to describe *a choice or justification* to capture what was done, stop and capture a decision first. Approach-shaped narratives (*"changed the approach to Z because W"*) read like smuggled decisions; pre-flight flags them — more strictly at higher layers (strategic / conceptual) than at operational / process. At strategic layer, any short-loop closure is blocked: strategic gaps require a captured decision.

## Proposals vs Facts

Open entries — signals, unclosed decisions, open plans — describe *where the graph might go*, not where it is. Only a closing done signal (or a retirement directive) declares what was done, turning proposal into fact.

## Contracts

Contracts are decisions marked `kind: contract`. They define standing constraints — architectural rules, authority boundaries, process agreements. They emerge from working patterns: a directive that hardens into a permanent rule can be reclassified via supersedes + `kind: contract`. Contracts define constraints, not participation boundaries — anyone can contribute signals and dialogue.

## Rendering Conventions

Entry lines in `sdd status`, `sdd list`, and summary chains carry three kinds of information, visually distinguished by notation:

- **Identity (kind, layer, type)** renders as plain qualifiers: `tactical plan decision`, `process gap signal`. Kind acts like a sub-type — identity, not an attribute.
- **Stored attributes** live in the entry's YAML frontmatter — written at creation, immutable afterwards. Rendered with square brackets: `[confidence: medium]`.
- **Derived attributes** are computed from graph relationships on every read — never written on the entry itself. Rendered with curly braces: `{status: active}`, `{status: open}`, `{status: closed-by <full-id>}`, `{status: superseded-by <full-id>}`. Done signals don't carry `{status: ...}` — they're terminal facts of execution with no lifecycle to track.

The stored-vs-derived split is what makes the immutability contract practical: state changes as the graph grows (a signal becomes closed when a closing done signal lands), but the entry file never changes. Reading `{status: ...}` tells you the current computed state; reading stored attrs tells you what was written originally.

Do not edit entries to "update" status — the graph computes it. To change status, add a new entry: a done signal or directive that `closes`, or a same-kind entry that `supersedes`.
