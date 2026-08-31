---
type: signal
layer: process
kind: fact
override: closed
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
    names, how a run receives its initial values and hands work to another
    procedure, a worked example valid by construction, and the live ability
    registry a step's guards, injects, and ops draw from.
---

# Writing a procedure spec

A procedure entry's frontmatter declares the workflow the engine executes. Three fields carry it, beside the entry's usual ones: `params`, `state`, and `steps`. The engine runs the workflow over a per-run variable store; params and state declare its fields.

## `params` — what a caller passes at start

Each entry is a declaration, `name: {type, optional?, desc}`. Params are written into the store once, at start.

## `state` — the working fields of the run

Same declaration shape as params, and steps collect and read these fields as the run advances. A field that arrives from a dispatching parent rather than the caller is state, not a param — a parent's seed writes state only.

**Field names are a vocabulary, not a free choice.** The commonest gate is a presence check: it passes once one specific field is present and non-empty, and each check is bound to its field by name — `hasAnchor` reads `anchor`, and nothing reads a name outside this vocabulary. So a field is gateable on presence only when it carries one of these names:

{{ .GateableFields }}

Prefer these names over inventing near-synonyms (`brief`, not `timeline`); a field outside the vocabulary can still be collected and served, but no gate can hold on it. Declaring stays your job: a guard naming a check whose field the spec never declares loads cleanly and then never advances — the run waits on a field nothing can collect. This list — like the type list below — renders from the running engine's own declarations each time the fact is served, so it is the running version's truth and cannot disagree with the live registry.

The declaration attributes:

- `type` — a domain type, or `list<T>` around one; see the closed set below.
- `optional` — `true` marks the field as not required where it is collected.
- `desc` — one line, served to the running agent when the field is asked for; write it as the instruction it is.
- `default` — state only: applied at start when no channel supplied the field — the one literal a spec carries, written in the type's natural YAML form (`true`, a quoted string, a list).

**How a run's store fills.** Initial values arrive through three channels, and naming them apart matters because one word — "seed" — is casually used for all three. In rank order: a value passed explicitly at start wins (params always arrive this way, and a start may also carry any *declared state field* the caller chooses to set); a dispatching parent's armed seed writes next (state only, see Dispatching below); `default` applies last, only where nothing else supplied the field. Everything after start is collected by steps.

## `steps` — the walk itself

A list in execution order. Each step has an `id` and advances in one of three ways.

### Gate steps

A gate declares `transitions`: ordered `{when: <guard>, to: <step id>}` clauses, optionally closed by `{otherwise: <step id>}`. The guard is a boolean combination of ability names written infix — `when: hasBrief and hasSynthesis` — with `and`, `or`, `not`, and parentheses, and nothing else: no comparisons, no literals, no field access. A gate may also run one effect first (`op: <command name>`).

### Chooser steps

An agent chooser (`chooser: agent`) or user chooser (`chooser: user`) declares `options`: `{choice, collect?, call?, dispatch?, to}` — the fields that option collects, an optional command it calls, an optional seed declaration for a handoff (below), and where it goes. A user chooser stops the run until the user answers. An option may route back to an earlier step, which then serves again — a confirm-or-correct loop needs no machinery beyond a user chooser whose options route on the answer.

### On any step

- `collect` — the state fields reportable here; a `?` suffix marks one optional (quote the marker: `"anchor?"`), and required ones hold the step until reported.
- `inject` — a list of `{fn: <query name>, args?, maxBytes?, maxItems?}` calls: each result is served into the step's instructions under its function's name. Size is the engine's job — an inject without a declared cap is bounded by the engine's defaults; `maxBytes`/`maxItems` are overrides.
- `render` — the name of an extra `## unit:` section from the body, served with the step (typically presenting one collected field's content).

A run ends by transitioning to {{ .EndTargets }}.

### Serve budget

Every automatic serve is bounded per part, but individually capped parts can still sum past a host's response budget. Capture pre-flight and `sdd lint` size each step at its declared worst case and raise an advisory finding when a step exceeds the engine's default serve budget. The finding is a risk note, never a load failure — the spec still runs. To accept the trade, declare `serveBudget: <bytes>` at the spec's top level (beside `params`/`state`/`steps`): a declared total at or above the worst case silences the finding and records the decision on the spec itself.

## Variable types

A declaration's `type` is a domain type, or `list<T>` around one. You declare the semantic type only. The concrete shape a value must take — object fields, identifier patterns, the accepted values of closed enumerations such as the ref-kind set or the confidence grades — is generated from the declaration and served as the step's report schema while the procedure runs, then enforced when the value arrives. Nothing about shape needs restating in the spec. The closed set:

{{ .VarTypes }}

## Abilities

Guards name checks, `inject` names queries, `op` and `call` name commands — and the engine serves the live inventory of all three, with each ability's contract (what it reads, what it writes), through its function registry. Where the engine is connected, its `registry` tool returns that inventory by class (predicate, query, command), including each query's argument names; what it lists is exactly what a spec can name, for the running version. Beyond the presence checks printed above, this reference deliberately carries no ability list — pull the registry when you write steps. A step that asks the user is always available for whatever no ability covers.

## Dispatching another procedure

A step never starts another procedure, and no command does. The handoff lives in the unit's prose: the instruction tells the running agent to start the named procedure as a sub-move of this run, and the product comes back the same way — the agent reports it into a field this spec declares, and the next gate holds on that field. Nothing else flows back: the child writes nothing into this run's store, so the reported field is the only bridge.

An option's `dispatch` block declares seeding for that handoff, never the start itself: `dispatch: {procedure: <canonical>, seed: {<child field>: <parent field>}}` on the answered option arms the mapping, and the next procedure started under this run receives each value into the *state* field it declares under the child name — a value passed explicitly at start wins, and an empty source seeds nothing.

A delegate procedure (class `task`) is dispatched the same way, with its inputs resolved by the dispatching run; it asks the user nothing and is offered nowhere on its own, so the dispatching unit's prose is where it is discoverable.

## Instruction units

The entry's body carries one `## unit: <step id>` section per step, served as the instructions the agent works from while that step runs — write it as what to do there and how to report the step's fields. A step's `render` field may name an extra unit section beyond the per-step ones. Units may use template placeholders over the store's fields and the step's injected data — Go-template form, each field by its declared name: `The anchor under review: {{"{{.anchor}}"}}` serves with the store's `anchor` value in place, and an inject's result appears under its function's name the same way. A field not yet collected renders empty.

## A worked example

A small review move, written for width rather than realism: an optional param and an optional collect, two injects with differently shaped arguments, compound guards, a user chooser that loops back for correction, a dispatch seed on the confirming option, and a closing gate on the field the dispatched recording hands back. It ships from the same source the engine's own tests load, so it is valid by construction:

```yaml
# one frontmatter: the entry's identity fields, then the workflow it runs
type: decision
kind: procedure
layer: process
canonical: example-review
class: move
{{ .Example }}```

The workflow sits in the same frontmatter as the entry's usual fields (references, topics, confidence). The body carries one `## unit:` section per step — here `## unit: scope`, `## unit: account`, `## unit: review`, `## unit: record` — each served verbatim as the instructions the agent works from while that step runs: what to do there, and how to report the step's fields. The review unit, for example, presents the account for the user's ruling and instructs that a confirming answer leads to starting the recording procedure as a sub-move.

## Validation

The spec's structure — step wiring, guard grammar, that every named ability exists — is validated when the engine loads the entry, and a broken spec fails loudly there. A procedure that loads becomes startable by its canonical and appears in the served inventory of moves. Start small, load it, run it, and grow it by superseding the entry — the workflow's identity is its supersession chain, and the canonical keeps resolving to the chain's newest version.
