# Plan Requirements and Pre-flight Gates

## Plan decisions (kind: plan)

Implementation of a decision requires a plan decision (`kind: plan`) captured in the graph before work begins. Plans are always required — the playbook does not offer a skip path. A minimal plan (one item for a simple decision) is fine; the discipline of writing it is what catches gaps.

The plan is a decision that refs the parent decision it implements. The WIP marker is created for the plan, not the parent decision.

## What a plan must contain

A plan must:

1. **Map every explicit requirement from the parent decision** to at least one concrete, verifiable plan item. If the decision lists 4 checks, the plan has at least 4 items. Each item must be concrete enough that the action pre-flight can answer "was this done? yes/no."

2. **Ground every item in active (non-superseded) decisions, current dialogue, or baseline facts** (CLAUDE.md, codebase patterns, existing code). Items not traceable to these sources are either missing decisions (capture first) or unauthorized scope (remove).

3. **Introduce no silent omissions.** A decision requirement without a corresponding plan item is a gap. It must be dialogued — either add the item or explicitly defer it with a captured rationale.

4. **Introduce no silent additions.** Plan items not grounded in the parent decision are flagged as conscious scope expansion. They may be valid, but they require explicit dialogue and may need their own decision first.

## Pre-flight gates

The /sdd skill enforces type-specific pre-flight checklists as hard gates before any `sdd new` capture. These are walked through explicitly in dialogue — not skipped because the answer "seems obvious."

### Implementation pre-flight (before starting work)

- [ ] Plan decision exists in the graph (kind: plan, refs parent decision)
- [ ] Every parent decision requirement has at least one plan item
- [ ] Every plan item traces to an active decision, current dialogue, or baseline fact
- [ ] No silent omissions or additions — user has confirmed plan is exhaustive
- [ ] WIP marker created for the plan

### Action pre-flight (before capturing closing action)

- [ ] Each plan item verified against what was actually built — yes/no per item
- [ ] Gaps identified and surfaced (not glossed over)
- [ ] If gaps exist: action does not close the plan; signal captured for the gap

### Decision pre-flight (before capturing a decision)

- [ ] Relevant signals were explored with genuine dialogue, not surface-level
- [ ] Refs are complete — all signals/decisions that informed this decision are linked
- [ ] No decisions introduced without dialogue (agent did not silently commit to a direction)

### Closure pre-flight (grooming / closing entries)

- [ ] Evidence actually supports the closure
- [ ] Referenced entries and context verified as current
