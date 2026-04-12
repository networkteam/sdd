# Two-Type Design: Signal + Decision with Kinds

## Summary

Collapse the SDD type system from three types (signal, decision, action) to two types (signal, decision). Actions become signals of kind "done." Both types carry an explicit "kind" field for semantic precision. The three-letter name SDD (Signal → Dialogue → Decision) becomes literally true.

## Motivation

### The strain in three types

Analysis of 164 graph entries and the Kōgen Coffee story revealed systematic type strain:

**Signals (68 entries) were carrying five distinct things:**
- Observations of gaps (33) — the intended use
- Research findings / facts (19) — discovered knowledge, not gaps
- Questions (13) — acknowledged unknowns, not observations
- Insights (22) — patterns and connections, not gaps
- Ideas / possibilities (13) — things to explore, not things observed

**Decisions (37 entries) were carrying four distinct things:**
- Directional choices (11) — the intended use
- Tasks / work items (10) — things to drive, not choices
- Constraints / contracts (7) — standing rules, not action requests
- Plans (4) — structured breakdowns, not singular commitments

**Actions (59 entries) were the least strained** — all were "something was done," differing only in what was done (execution, research, evaluation, process work).

### The insight

Actions are signals. "Built the discovery page" is a signal to the graph that reality changed. It tells other participants: something happened, come look. It might trigger new signals (gaps, insights), which trigger new decisions.

The loop was never Signal → Decision → Action. It was always **Signal → Decision → Signal → Decision**. Two beats: observe, commit. The dialogue happens between them.

## The Two Types

### Signal — "something the graph should know about"

A signal is any input entering the graph: an observation, a discovered fact, a question, an insight, a completion report. Signals are the graph's awareness of the world.

### Decision — "something we commit to"

A decision is any commitment: a direction chosen, a constraint established, a plan structured, a task to drive. Decisions are the graph's will.

## Signal Kinds

| Kind | Definition | Drives work? | Default? |
|------|-----------|-------------|----------|
| **gap** | Delta between actual and target state | **Yes — primary driver** | Yes (default) |
| **fact** | Discovered knowledge: research findings, measurements, analytics data | Informs decisions | No |
| **question** | Acknowledged unknown requiring resolution | Awaits resolution | No |
| **insight** | Synthesized knowledge: patterns, connections, interpretations, confirmations | Enriches understanding | No |
| **done** | Work completed, fact of execution | Closes tasks/directives | No |

### Gap (default)

The gap kind preserves the original meaning of "signal" and is the engine of the sculpting philosophy. A gap says: actual state is X, target state is Y, the delta needs addressing. Gaps at every layer:

- **Strategic**: "Customers want our coffee beyond three shops, but we can't reach them"
- **Conceptual**: "The prototype feels like a generic webshop, not like Kōgen"
- **Tactical**: "Share recipients can see the discovery but can't subscribe"
- **Operational**: "Stripe embedded checkout can't handle gift shipping as specified"
- **Process**: "Agent silently drops decision requirements during implementation"

When you look at open signals of kind gap, you're looking at the sculpting surface — what needs attention.

### Fact

Discovered knowledge that informs but doesn't demand action by itself. Facts are stable reference material:

- "DHL 500g package: €4.50-5.80" (shipping research)
- "Kaffebox charges €X, Nomad €Y, both include shipping" (competitor research)
- "Jun's tasting notes are the most-viewed section, 3 min average" (analytics)
- "MemPalace uses atomic linked notes, Git-based, similar structure to SDD" (external research)

A fact may *reveal* a gap, but the fact and the gap are separate entries. The fact is stable knowledge. The gap is the tension that demands resolution.

### Question

An acknowledged unknown. Not an observation, not a commitment — a marker of uncertainty:

- "What should we charge for shipping?"
- "How can non-technical users access the graph without CLI?"
- "What adoption paths work for migrating existing projects?"

Questions are resolved by decisions (which answer them) or by facts/insights (which dissolve them).

### Insight

Synthesized knowledge — something now understood that wasn't before. Includes patterns, connections, interpretations, and confirmations from evaluation:

- "A discovery is four elements: origin story, roasting intent, tasting notes, brewing recommendation" (crystallized from Jun's narration)
- "The loop maps to the Golden Circle: Why/How/What" (connection between ideas)
- "The share-to-subscribe loop is working" (confirmation from evaluation)
- "Both real content AND story-first layout are needed" (synthesis from branch experiment)
- "SDD's differentiation is decision chains (reasoning), not memory recall (retrieval)" (positioning insight from research)

### Done

Work completed. A fact of execution that closes the loop. A done signal must reference or close at least one decision — if work was done but has no decision in the graph, the decision is missing and should be captured first.

- "Built discovery page. Four content sections, responsive." (closes the build task)
- "Integrated Stripe hosted checkout. Subscription flow works." (closes the integration task)
- "Completed shipping and competitor research." (closes the research task)
- "Started Stripe integration, stopped at gift shipping blocker." (refs the task — partial, doesn't close)

### Why "observation" dissolved

What was "observation" splits cleanly into existing kinds:
- Observations of gaps → **gap**
- Observations of patterns → **insight**
- Observations of state → **fact**

No residual need for a catch-all.

## Decision Kinds

| Kind | Definition | Lifecycle |
|------|-----------|----------|
| **directive** | Commitment to a direction | Active until closed by done signal or superseded |
| **contract** | Standing constraint or guardrail | Active until explicitly superseded |
| **plan** | Structured work breakdown | Closed when all sub-items done or superseded |
| **task** | Work item to drive to completion | Closed by done signal |

### Directive (default)

A choice between alternatives. The most common decision type:

- "We will explore a direct-to-consumer offering" (strategic direction)
- "Subscription with sharing built in" (product model choice)
- "Use Stripe hosted checkout for now" (technical choice)
- "Story-first layout is the direction" (design choice)

### Contract

Standing constraint that governs ongoing behavior. Never closed — stays active until superseded:

- "Immutability is a hard constraint — documents never modified"
- "Jun has sole authority over bean selection and discovery content"
- "Agent reliability through structural separation"

### Plan

Structured breakdown of work. Closed when all sub-items are done:

- "Build prototype: discovery page, Stripe checkout, share links, deploy to staging"
- "Pre-flight validator: six work items with verification criteria"

### Task

Work to be driven to completion. Lean — says what, not how or why. The refs chain carries context:

- "Build discovery page with placeholder content" (refs the prototype plan)
- "Integrate Stripe subscription checkout" (refs the prototype plan)
- "Research AI memory tools landscape" (refs a strategic question)
- "Evaluate the prototype" (evaluation as task — findings come back as insights/gaps)

A task doesn't need to specify how or why because it's connected in the graph. The task just says "do this." The done signal says "did it."

## The Sculpting Loop with Two Types

```
s (gap):       "Customers want beans shipped, we can't do that"
  d (directive): "Explore direct-to-consumer discovery offering"
    s (fact):      "Shipping: €4.50-5.80 per 500g package"
    s (insight):   "A discovery is four elements"
    s (gap):       "Sharing identity conflicts with subscription economics"
      d (directive): "Subscription with sharing built in"
        d (plan):      "Build prototype: four steps"
          d (task):      "Build discovery page"
            s (done):      "Built discovery page"  closes→task
          d (task):      "Integrate Stripe checkout"
            s (gap):       "Embedded checkout can't handle gift shipping"
            s (done):      "Started integration, stopped at blocker"
            d (task):      "Use hosted checkout instead"  supersedes→original task
              s (done):    "Integrated hosted checkout"  closes→new task
        s (gap):       "Prototype feels like generic webshop"  ← new gap from evaluation
          d (plan):      "Branch experiment: two layouts"
            ...sculpting continues
```

Gaps trigger decisions. Decisions spawn tasks. Tasks get done. Done signals close tasks. The work reveals new gaps. Two beats: observe, commit.

## Research as Activity, Not Kind

Research is work (driven by a task or directive), and its outputs decompose naturally into existing kinds:

```
d (task):    "Research shipping costs and competitor pricing"
s (fact):    "DHL 500g: €4.50-5.80"
s (fact):    "Kaffebox charges €X, Nomad €Y"
s (insight): "Shipping economics strongly favor subscription over one-off"
s (gap):     "Sharing identity conflicts with subscription economics"
s (done):    "Completed shipping research"  closes→task
```

No separate "research" kind needed. The task drives the work. Facts, insights, gaps, and questions come out. The done signal closes the task.

## Design Constraints

### Done signals must connect to decisions

A done signal must `closes` or at minimum `refs` a decision. If work happened without a decision in the graph, either:
- The decision exists but wasn't captured → capture it first
- The work was undirected → process gap

This is not an artificial rule — it's coherence. Pre-flight validation can enforce it.

### Gap as default signal kind

When creating a signal without specifying a kind, it defaults to gap. This reflects that gap observations are the most common signal and the primary driver of work. You must intentionally mark something as fact, question, insight, or done.

### Kind determines lifecycle expectations

- **gap** → should eventually be closed by a decision addressing it, or superseded as no longer relevant
- **fact** → stable reference material, no closure expected
- **question** → resolved by decisions or dissolved by facts/insights
- **insight** → stable knowledge, no closure expected
- **done** → closes a task/directive, stable record
- **directive** → active until fulfilled or superseded
- **contract** → active until superseded
- **plan** → active until all sub-items done
- **task** → active until a done signal closes it

## What Changes

### IDs
Entry IDs use `s` and `d` instead of `s`, `d`, `a`. Two type characters.

### Graph queries
`sdd list --type a` becomes `sdd list --kind done`. Signal and decision kind filtering becomes the primary query mechanism alongside type and layer.

### Status view
`sdd status` groups by kind within type: contracts, plans, active directives, active tasks (decisions); open gaps, open questions, recent done signals (signals).

### Pre-flight validation
Validates that a done signal claiming to close a decision covers all items. Trigger is the `closes` field, not the entry type. Same logic, kind-aware.

### The founding contract (d-cpt-o8t)
Currently says "Three types (signal, decision, action)." When this directive is implemented and validated, a new contract supersedes d-cpt-o8t with "Two types (signal, decision) with kinds."

## What Stays the Same

- Immutability — documents never modified
- Git storage — hierarchical directory layout
- Five layers — strategic through process
- Reference fields — refs, closes, supersedes with same semantics
- No redundancy — each fact in one place
- Graph traversal for current state — never maintained separately
