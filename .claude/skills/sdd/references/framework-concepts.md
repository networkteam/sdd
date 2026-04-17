# SDD Framework Concepts

## The Loop

One universal loop: **Signal → Dialogue → Decision → Action → Signal...**

- **Signal** (`s`): An observation. Something noticed — from users, data, agents, or the world. Suggests a gap between actual and target state.
- **Dialogue**: Collaborative reasoning about what a signal means and what to do. Happens between humans, agents, or both. Not recorded directly — its outputs become signals and decisions.
- **Decision** (`d`): A commitment to a direction. Immutable once recorded. The durable asset of the framework.
- **Action** (`a`): Something that was done. A fact of execution. References the decisions it implements.

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
- `closes`: "resolves / fulfills" — the referenced entry is no longer active/open. Decisions close signals, actions close decisions and signals.

**Open signal** = not superseded, not closed. **Active decision** = not superseded, not closed.

## Proposals vs Facts

Open entries — signals, unclosed decisions, open plans — describe *where the graph might go*, not where it is. Only a closing action declares what was done, turning proposal into fact.

## Decision Kinds

Decisions have an optional `kind` property:

- **`directive`** (default, omitted): Requests action. Closed when fulfilled by an action.
- **`contract`**: Standing constraint. Never closed — stays active until superseded. A directive can become a contract via a new decision with `supersedes` + `kind: contract`.

The `sdd status` view separates contracts from active directives. `sdd list --kind contract|directive` filters by kind.

## Contracts

Contracts are decisions marked `kind: contract`. They define standing constraints — architectural rules, authority boundaries, process agreements. They emerge from working patterns: a directive that hardens into a permanent rule can be reclassified. Contracts define constraints, not participation boundaries — anyone can contribute signals and dialogue.

