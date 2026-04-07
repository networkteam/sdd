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

## Contracts

Contracts are process-layer decisions (`d-prc`) about who decides what. They emerge from working patterns — the system observes who has been making which decisions and suggests formalizing it. Contracts define decision *authority*, not participation boundaries — anyone can contribute signals and dialogue.

## Coverage

Coverage tracks where change is unevaluated. Every action (delta-as-change) should be evaluated against its intended gap (delta-as-gap). Open signals without decisions addressing them represent unevaluated areas.

## Evaluation Pattern

Every evaluation follows: decision (what to evaluate against) → action (who reviewed) → signals (what they found). Multiple people/agents can evaluate the same thing — each produces their own action and signals.

## The Graph

All entries are stored as small markdown files with YAML frontmatter in `docs/framework/graph/`. The graph is a DAG (directed acyclic graph) implemented on Git.

### Document ID format

```
{YYYYMMDD}-{HHmmss}-{type}-{layer}-{suffix}.md
```

Examples: `20260406-115540-d-stg-0gh.md`, `20260406-232057-s-cpt-cvz.md`

### CLI tool

The `sdd` binary (built from `framework/cmd/sdd/`) provides:
- `sdd status` — active decisions, open signals, recent actions
- `sdd show <id>` — entry with full reference chain
- `sdd list [--type d|s|a] [--layer stg|cpt|...]` — filtered listing
- `sdd new <type> <layer> [--refs id1,id2] [--supersedes id] [--closes id1,id2] [--participants p1,p2] [--confidence high|medium|low] <description>` — create entries

The binary is pre-built at `framework/bin/sdd`.
