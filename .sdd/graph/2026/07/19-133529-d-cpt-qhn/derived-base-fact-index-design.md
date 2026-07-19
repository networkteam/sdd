# Derived base-fact discovery index — design record

## Problem

The first shipped base fact is usable once found, but a real agent did not discover it through ordinary task flow. A first-view breadcrumb arrives too late when the agent needs reference knowledge before constructing the first view call.

The base-facts directive already commits the session shell to carrying a small index that points to pullable facts without pushing their bodies.

## Alternatives considered

### Hard-code fact IDs in the session shell

Rejected. The shell would become coupled to the fact catalog, and every addition, supersession, or project override would require a second synchronized edit. That recreates the drift the base-facts split is meant to prevent.

### Derive every active fact automatically

Rejected as the sole rule. The catalog can grow, while only some facts need bootstrapping exposure in the opening serve. Index inclusion needs explicit author intent.

### Boolean `index: true`

Rejected. A generated summary or terse entry name is not necessarily enough for an agent to understand when pulling the fact will help. The index needs a deliberately authored retrieval cue.

### Short IDs

Rejected for this machine-facing handoff. Full IDs avoid ambiguity and an extra resolution step; compactness is less important for a deliberately small index.

## Chosen shape

```yaml
topics:
  - engine/base-facts
  - cli/view

index:
  title: "How to compose graph views (view tool): layout grammar, filters, ranking, quoting, and examples"
  topic: cli/view
```

- Presence of `index` enrolls an active `kind: fact` entry.
- `index.title` is required and must stand alone as the agent's retrieval cue: it explains enough of the fact's purpose to know when to pull it.
- `index.topic` is required and selects exactly one primary grouping key.
- The selected topic must also occur in the entry's ordinary `topics`; it is a presentation selector, not duplicate classification.
- Exactly one index topic keeps each fact in one future group. The flat initial rendering may ignore grouping while preserving the field for later.
- The shell derives the index after active/supersession resolution and renders the full entry ID plus title.
- The shell contains no fact-specific IDs and no duplicated fact bodies.

## Presentation scope

The session opening serve renders the derived index. `sdd show` exposes the frontmatter field. No special `sdd view`, catch-up, or skill-rendering behavior is required by this directive; any later grouping presentation remains a separate decision.
