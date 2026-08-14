---
type: signal
layer: process
kind: fact
confidence: high
topics:
    - engine/base-facts
    - engine/base-procedures
refs:
    - id: 20260813-170000-s-prc-prd
      kind: related
      desc: the procedure kind's authoring fact — what a procedure is and when to reach for the kind; this reference covers how to write its spec
summary: >-
    The procedure spec reference: the frontmatter fields a runnable procedure
    declares — typed params and state, the step list with its gates, choosers,
    and transitions — the closed set of variable types, an annotated skeleton,
    and how to pull the live registry of abilities a step can name.
---

# Writing a procedure spec

A procedure entry's frontmatter declares the workflow the engine executes. Three fields carry it, beside the entry's usual ones: `params`, `state`, and `steps`.

## `params` — what a caller passes at start

Each entry is a declaration, `name: {type, optional?, desc}`. Params seed the store once, at start.

## `state` — the working fields of the run

Same declaration shape as params, and steps collect and read these fields as the run advances.

The declaration attributes:

- `type` — a domain type, or `list<T>` around one; see the closed set below.
- `optional` — `true` marks the field as not required where it is collected.
- `desc` — one line, served to the running agent when the field is asked for; write it as the instruction it is.
- `default` — state only: applied at start when the caller left the field unset — the one literal a spec carries.

## `steps` — the walk itself

A list in execution order. Each step has an `id` and advances in one of three ways.

### Gate steps

A gate declares `transitions`: ordered `{when: <guard>, to: <step id>}` clauses, optionally closed by `{otherwise: <step id>}`. The guard is a boolean combination (`and`, `or`, `not`) of ability names. A gate may also run one effect first (`op: <command name>`).

### Chooser steps

An agent chooser (`chooser: agent`) or user chooser (`chooser: user`) declares `options`: `{choice, collect?, call?, to}` — the fields that option collects, an optional command it calls, and where it goes. A user chooser stops the run until the user answers.

### On any step

- `collect` — the state fields reportable here; a `?` suffix marks one optional, and required ones hold the step until reported.
- `inject` — `{fn: <query name>, args?}`: served data rendered into the step's instructions.
- `render` — a state field shown with the serve.

A run ends by transitioning to {{ .EndTargets }}.

## Variable types

A declaration's `type` is a domain type or `list<T>` around one. The closed set: {{ .VarTypes }}.

## Abilities

Guards name checks, `inject` names queries, `op` and `call` name commands — and the engine serves the live inventory of all three, with each ability's contract (what it reads, what it writes), through its function registry. Pull the registry by class (predicate, query, command) whenever you write steps: what it lists is exactly what a spec can name, for the running version. A step that asks the user is always available for whatever no ability covers.

## Instruction units

The entry's body carries one `## unit: <step id>` section per step — the guidance served while that step runs. Units may use template placeholders over the store's fields and the step's injected data.

## Skeleton

A minimal, valid shape — one working step gated on its collected field, one user confirmation:

```yaml
params:
    goalHint: {type: text, optional: true, desc: what the caller wants examined}
state:
    synthesis: {type: text, desc: the outcome the run hands back}
steps:
    - id: examine
      collect: [synthesis]
      transitions:
          - when: hasSynthesis
            to: confirm
    - id: confirm
      chooser: user
      options:
          - {choice: accept, to: end(completed)}
          - {choice: abort, to: end(abandoned)}
```

With a body carrying `## unit: examine` and `## unit: confirm` sections that tell the runner what each step wants.

## Validation

The spec's structure — step wiring, guard grammar, that every named ability exists — is validated when the engine loads the entry, and a broken spec fails loudly there. Start small, load it, run it, and grow it by superseding the entry under the same canonical.
