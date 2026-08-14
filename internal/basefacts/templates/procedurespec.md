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
    and transitions — the closed sets of variable types and gateable field
    names, how a run hands work to another procedure, a worked example valid
    by construction, and how to pull the live registry of abilities a step
    can name.
---

# Writing a procedure spec

A procedure entry's frontmatter declares the workflow the engine executes. Three fields carry it, beside the entry's usual ones: `params`, `state`, and `steps`.

## `params` — what a caller passes at start

Each entry is a declaration, `name: {type, optional?, desc}`. Params seed the store once, at start.

## `state` — the working fields of the run

Same declaration shape as params, and steps collect and read these fields as the run advances. A field that arrives from a dispatching parent rather than the caller is state, not a param — seeding writes state only.

**Field names are a vocabulary, not a free choice.** The commonest gate is a presence check: it passes once one specific field is present and non-empty, and each check is bound to its field by name — `hasAnchor` reads `anchor`, and nothing reads a name outside this vocabulary. So a field is gateable on presence only when it carries one of these names:

{{ .GateableFields }}

Prefer these names over inventing near-synonyms (`brief`, not `timeline`); a field outside the vocabulary can still be collected and served, but no gate can hold on it. Declaring stays your job: a guard naming a check whose field the spec never declares loads cleanly and then never advances — the run waits on a field nothing can collect.

The declaration attributes:

- `type` — a domain type, or `list<T>` around one; see the closed set below.
- `optional` — `true` marks the field as not required where it is collected.
- `desc` — one line, served to the running agent when the field is asked for; write it as the instruction it is.
- `default` — state only: applied at start when the caller left the field unset — the one literal a spec carries.

## `steps` — the walk itself

A list in execution order. Each step has an `id` and advances in one of three ways.

### Gate steps

A gate declares `transitions`: ordered `{when: <guard>, to: <step id>}` clauses, optionally closed by `{otherwise: <step id>}`. The guard is a boolean combination of ability names written infix — `when: hasBrief and hasSynthesis` — with `and`, `or`, `not`, and parentheses, and nothing else: no comparisons, no literals, no field access. A gate may also run one effect first (`op: <command name>`).

### Chooser steps

An agent chooser (`chooser: agent`) or user chooser (`chooser: user`) declares `options`: `{choice, collect?, call?, dispatch?, to}` — the fields that option collects, an optional command it calls, an optional seed declaration for a handoff (below), and where it goes. A user chooser stops the run until the user answers. An option may route back to an earlier step, which then serves again — a confirm-or-correct loop needs no machinery beyond a user chooser whose options route on the answer.

### On any step

- `collect` — the state fields reportable here; a `?` suffix marks one optional (quote the marker: `"anchor?"`), and required ones hold the step until reported.
- `inject` — a list of `{fn: <query name>, args?}` calls: each result is served into the step's instructions under its function's name.
- `render` — a state field shown with the serve.

A run ends by transitioning to {{ .EndTargets }}.

## Variable types

A declaration's `type` is a domain type, or `list<T>` around one. The closed set:

{{ .VarTypes }}

You declare the semantic type only. The concrete shape a value must take — object fields, identifier patterns, the accepted values of closed enumerations — is generated from the declaration and served as the step's report schema while the procedure runs, then enforced when the value arrives. Nothing about shape needs restating in the spec.

## Abilities

Guards name checks, `inject` names queries, `op` and `call` name commands — and the engine serves the live inventory of all three, with each ability's contract (what it reads, what it writes), through its function registry. The registry is askable on demand, the same way entries are read — request it by class (predicate, query, command) whenever you write steps: what it lists is exactly what a spec can name, for the running version, including each query's argument names. A step that asks the user is always available for whatever no ability covers.

## Dispatching another procedure

A step never starts another procedure, and no command does. The handoff lives in the unit's prose: the instruction tells the runner to start the named procedure as a sub-move of this run, and the product comes back the same way — the runner reports it into a field this spec declares, and the next gate holds on that field. Nothing else flows back: the child writes nothing into this run's store, so the reported field is the only bridge.

An option's `dispatch` block declares seeding for that handoff, never the start itself: `dispatch: {procedure: <canonical>, seed: {<child field>: <parent field>}}` on the answered option arms the mapping, and the next procedure started under this run receives each value into the *state* field it declares under the child name — a value passed explicitly at start wins, and an empty source seeds nothing.

A delegate procedure (class `task`) is dispatched the same way, with its inputs resolved by the dispatching run; it asks the user nothing and is offered nowhere on its own, so the dispatching unit's prose is where it is discoverable.

## Instruction units

The entry's body carries one `## unit: <step id>` section per step — the guidance served while that step runs. Units may use template placeholders over the store's fields and the step's injected data.

## A worked example

A small review move, written for width rather than realism: an optional param and an optional collect, two injects with differently shaped arguments, compound guards, a user chooser that loops back for correction, a dispatch seed on the confirming option, and a closing gate on the field the dispatched recording hands back. It ships from the same source the engine's own tests load, so it is valid by construction:

```yaml
{{ .Example }}```

With a body carrying one `## unit:` section per step — `## unit: scope`, `## unit: account`, `## unit: review`, `## unit: record` — telling the runner what each step wants, and the review unit instructing that the confirming answer leads to starting the recording procedure as a sub-move.

## Validation

The spec's structure — step wiring, guard grammar, that every named ability exists — is validated when the engine loads the entry, and a broken spec fails loudly there. A procedure that loads becomes startable by its canonical and appears in the served inventory of moves. Start small, load it, run it, and grow it by superseding the entry — the workflow's identity is its supersession chain, and the canonical keeps resolving to the chain's newest version.
