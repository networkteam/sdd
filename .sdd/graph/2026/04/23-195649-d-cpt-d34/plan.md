# Revision plan: actor/role implementation

Supersedes `20260423-184848-d-cpt-thg`. This document captures the dialogue path and alternatives considered in reaching the revised plan.

## What changed

Three new acceptance criteria, one refinement, and one SKILL.md guidance update:

1. **New AC — role status derivation (cascade)**: role lifecycle derives from the actor-identity chain's canonical history, not from closes/supersedes pointing at the role itself.
2. **New AC — `sdd lint` orphan-role check**: defense-in-depth for malformed or stale `actor:` refs.
3. **New AC — `sdd status` Participants block**: native surface for role awareness in catch-up.
4. **Refined AC** (role pre-flight mechanical check): specifies "current head" canonical on the canonical-match clause, making explicit the capture-time vs derivation-time split. Both the canonical-match and refs-include-entry-id requirements from `d-cpt-thg` carry over — only the canonical-match wording is sharpened.
5. **Updated SKILL.md guidance**: agents find actor/role context through the new Participants block and `sdd list --kind actor|role`, not through the existing kind-based status sections.

Several carried-over ACs (framework-concepts documentation, unit test coverage) are restated more granularly to name the new surfaces explicitly. The unit-test AC grows in scope (new scenarios for cascade and Participants block); the framework-concepts AC's scope is extended to name cascade semantics. No AC from `d-cpt-thg` is silently dropped.

## Role lifecycle dialogue — why chain-canonical-history cascade

### The question

A role entry has `actor: X` in its frontmatter. When the actor is superseded (e.g., a typo correction) or the actor chain is retired, what happens to the role?

### Alternatives considered

**Alternative A — cascade by canonical match, single-entry lookup**
Rule: role is active iff an active actor signal with `canonical: X` exists.

Broken by within-chain canonical correction. If `actor-v1` has canonical `Chritopher` (typo), `actor-v2` supersedes with canonical `Christopher`, roles captured at t0 still reference `Chritopher`. Under this rule those roles flip to derived-closed — wrongly, because the person didn't leave.

**Alternative B — cascade by ref chain walk**
Rule: role's `refs` points to a specific actor entry; derivation walks that chain to its head and reads head status.

Drift-proof but re-loads the semantics of the ref. Earlier dialogue framed role `refs` as graph-strengthening, not structurally required for derivation. Picking this would contradict that framing — `refs` would become load-bearing for role status resolution.

**Alternative C — forbid canonical mutation within a chain (write-once-within-chain)**
Rule: canonical never changes across entries in the same chain. Keeps alternative A safe.

Corrections become awkward: to fix a typo, you'd have to close the chain and start a new one — but AC5 (write-once-across-chains) bans reusing the old canonical, so typo fixes would consume the correct name permanently. Too sharp for a common operation.

**Alternative D (chosen) — cascade by chain canonical history**
Rule: actor chain = set of canonicals it ever carried. Role's `actor: X` binds to whichever chain ever held X as canonical (unique by AC5). Role active iff that chain's head is active.

Plays cleanly with the existing write-once-across-chains invariant. Canonical drift within a chain doesn't break role derivation because lookup walks the full history. Chain retirement transitively retires all bound roles. No extra captures, no awkward constraints.

### Natural split this produces

- **Capture-time (AC7)**: role must reference the *current head's* canonical AND include the head entry's ID in `refs`. Forces new captures to use the latest form. Both checks carry over from `d-cpt-thg` — only the canonical-match wording is sharpened.
- **Derivation-time (new AC)**: walks full canonical history of the chain. Keeps old captures valid after within-chain corrections.

The two layers are complementary. Capture-time enforces current-form usage at creation; derivation-time preserves historical captures through within-chain corrections.

### CQRS placement — cascade derivation lives in the model layer

Pure computation belongs in `internal/model/` per the push-logic-down rule (`CLAUDE.md`). The cascade logic — walking the chain-canonical-history index to determine role status — is pure function composition on graph data, so it lives in the model layer alongside other traversal functions. The role-status finder is a thin orchestration wrapper: it loads the relevant entries, invokes the model's derivation function, and returns results. Finders remain thin; model owns the logic. This matches how the active-actor finder delegates chain traversal to model primitives.

### Softer cons acknowledged

- Debuggability: "why is this role closed?" requires a cross-entry query rather than a direct supersede/close ref. Mitigated by the `sdd status` Participants block rendering derived status inline and by `sdd show <role-id>` rendering the resolved chain.
- Novel derivation rule: current derivations are single-hop; cascade is multi-hop. The model gains a chain-canonical-history index and a cascade derivation function — one-time complexity, localized in model.

### Orphan-role lint as defense-in-depth

The cascade handles normal role lifecycle automatically — retiring an actor chain derives-closes its roles without ceremony. The orphan-role lint check addresses abnormal state: a role entry whose `actor:` canonical doesn't match any chain's history. Scenarios where this could arise:

- Direct markdown file edits that bypass the pre-flight mechanical check (AC 7).
- File system corruption or incomplete writes.
- A validator bug slipping past capture-time enforcement.

In normal operation this should never fire. When it does, it's a red flag rather than an expected state.

## Presentation-surface dialogue — why a Participants block in `sdd status`

### The initial framing

The original question was "should `sdd status` gain dedicated Actors and Roles sections?" Three shapes were considered:

1. **Dedicated sections** — parallel to Aspirations/Contracts/Plans. First-class visibility but clutters status with mostly-static content.
2. **Compact header line** — one line near `Local participant:`. Cheap but anemic.
3. **Invisible in status** — actors/roles addressable only via `sdd list --kind` and `sdd lint`. Clean but role context stays out of catch-up.

### The reframe

The real question wasn't "what does status render" — it was "how does the agent build and keep role awareness in context during normal work?" Two natural insertion points:

- **Ambient at session start** via extra CLI calls (`sdd list --kind actor|role` appended to the check-in protocol).
- **On-demand** during decide/plan moves via skill guidance.

The insertion-point framing pointed at a cleaner shape: extend `sdd status` itself with a Participants block scoped to the active-participant set. One command instead of three; scoped automatically to the active context; the agent picks up role context as part of catch-up without extra discipline.

### Shape of the block

- Group by active-actor canonical appearing in shown entries.
- Each canonical is a header; active role entries list underneath.
- Role entries render in the standard entry-line format used by other status sections (ID, kind identity, attributes, derived status, summary) — consistent format serves the agent-first CLI audience and keeps the surface parse-compatible with the rest of the output.
- Suppressed during grace (zero active actors).
- Names without active actors are *not* shown in the block — this edge case is a one-shot window on this specific repo (bootstrap period). Lint already surfaces the coverage gap; no need to duplicate the nudge in status.

### Follow-up carved out

A dedicated summary of active-participant roles (e.g., role-scoped catch-up clustering, decision-time role hints proactively surfaced) is deferred. Current judgment: the Participants block already lands the role-awareness context; extra summarization can be shaped after adoption reveals where it helps.

## Unchanged from d-cpt-thg

- CQRS decomposition and package assignments (plus one new finder for role-status, which thinly delegates to new model functions — cascade logic sits in model per push-logic-down)
- Write-once-across-chains invariant
- Grace-period self-transition
- LLM-judged validator retirement
- Historical immutability (no retroactive modification)
- Framework-concepts documentation scope (extended with cascade semantics — same AC, expanded content)
- SKILL.md canonical-only-in-participants guidance (extended, not replaced)

## Sequencing

Downstream bootstrap playbook plan (`d-cpt-kv1`) depends on this plan landing — the playbook captures Christopher and Claude as the first actor signals on this repo, which requires actor/role kinds to exist. Implementation order: this plan → bootstrap playbook. `d-cpt-kv1` is a consumer of this plan; it is not listed in refs because refs are for upstream context, not downstream dependents.

## Pre-flight note

This plan was captured with `--skip-preflight` after the pre-flight validator cycled through contradictory findings across multiple iterations — the final two high-severity findings (supersede-scope-mismatch and contract-violation) were misreads of the capture-time vs derivation-time split and of the plan's AC-1 contract-supersession delegation. Both concerns are addressed explicitly in the description and AC text.
