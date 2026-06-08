# Process rules: unify contracts into a `rule` kind; capture + discover learnings

Directive-level design record. Detailed decomposition and acceptance criteria belong to the follow-up plan; this captures the committed shape, the alternatives weighed, the surfaces to touch, and the open questions.

## Decision

- Introduce a decision kind `rule` with an `enforcement` attribute: `binding` (blocks) or `advisory` (surfaces for consideration).
- Fold the existing `contract` kind into `rule` as `enforcement: binding`. A rename + generalize, not an addition — the type system stays 7 signal + 7 decision kinds.
- A rule self-describes three things:
  - **When to apply** — the situation/trigger. Doubles as the text the discovery query matches against.
  - **What to consider** — the substance of the learning.
  - **How to calibrate** — guidance on weighing it (and, for binding, what it blocks).
- Learnings become rules. Rules are captured once and rediscovered at the relevant phases of the working modes.

## Why one kind, not two

- The real axis is **enforce vs. surface**: a contract is enforced (the system blocks); a rule is surfaced (you weigh it). Same shape underneath — a standing conditional "when X, then Y" — differing only in bindingness. That is the same severity axis pre-flight already applies to one-off findings, lifted onto the standing statement.
- "Contract" broadcasts "binding agreement," and that connotation suppressed capture — most learnings are judgment calls, not blocks, so forcing them into contracts felt wrong and they went uncaptured (or drifted into CLAUDE.md). Naming the spectrum after its heaviest, rarest end was backwards.
- `rule` + an explicit strength reads cleanly at both ends ("binding rule" / "advisory rule"), and giving guidance a home should *reduce* contract misuse rather than add a competing concept.

## Discovery mechanism (in scope — the system is inert without it)

- A "widen"-style retrieval over the rule corpus, run at junctures: plan, implement, evaluate, close.
- The skill supplies search **angles** — which feature the work provides, which layer it touches, topic — and vector search returns candidate rules (`sdd search --kind rule`).
- A **sub-agent** sifts candidates down to the applicable few and returns them *with body*, so poor candidates never reach the main context (same shape as `sdd-explore`).
- The **juncture is an angle, not a declared field** on the rule — keeps recall, hard-pins nothing, and stays forward-compatible with rules later hooking into any seam (s-cpt-k8i).
- v1 is **text-driven** (a skill instruction, like "widen", which already works); structural gating is a later hardening. Text is unreliable, not useless.
- Precision risk: over-broad matches breed alert-fatigue and get tuned out — the same over-eagerness already seen in pre-flight. Calibration is part of each rule, not a global knob.

## Alternatives considered

- **Reuse `contract` + a strength attribute** (keep the name): rejected — the name is the problem; a "low-enforcement contract" is a contradiction in terms.
- **Two distinct kinds** (contract and rule side by side): rejected — they are one spectrum; two overlapping concepts is the "feels weird" the dialogue started from.
- **Rules declare `applies_at: [juncture]`**: rejected as too rigid — juncture is a query angle; if precision proves bad, a *soft* ranking boost, never a hard filter.

## Surfaces to make consistent (scope for the follow-up plan)

### A. Type-system core (`internal/model/`)
- `entry.go` — `KindContract` const + `decisionKinds` set + `IsContract()` → add `KindRule`, an `Enforcement` field (`binding|advisory`), `IsRule()`. Decide: drop `contract` or keep as a parse-time alias for existing files.
- frontmatter parse/serialize/roundtrip for `enforcement` (same plumbing as role's `actor:` / focus's `involvement:`).
- `graph.go` — `Contracts()` collection (the single chokepoint, ~13 callers) → `Rules()` / `BindingRules()`.
- `d-cpt-ni0` — the 7+7 type-system contract enumerates the kind set, so it must be **superseded by a revised contract** (rule replacing contract; count stays 7+7). It remains in force until that supersession ships.

### B. Pre-flight (`internal/llm/preflight.go`)
- `ActiveContracts` (loads `graph.Contracts()` into the cached system preamble for byte-stability) → follows the rename; feeds only `enforcement: binding` rules. This is the load-everything spot flagged by s-tac-osb, and the seam where retrieval would later replace "load all".
- New rubric for the `rule` kind: valid enforcement value + the when/consider/calibrate sections present (same shape as the `## Acceptance criteria` check on plans).

## C. Read / render
- `presenters/status.go` + `query/status.go` + `finders/status.go` — the "Contracts" section → "Rules" (open: split binding vs advisory?).
- `query/macros.go` — the `contracts` macro + header → `rules`.
- `finders/view.go` — `kind()` accepts `rule`; `not(kind(...))` help text.
- `cmd/sdd/` — `--kind` enum, a new `--enforcement` flag, `sdd list` / `sdd search` kind filters.

### D. Skill + docs
- `framework-concepts.md` — decision-kinds table, distinguishing test #1 ("constraint that always holds" → `rule`, set enforcement), the Contracts section, retirement-primitives table, role↔contract orthogonality note.
- `SKILL.md` + `cli-reference.md` — kind-for-decisions guidance, `--enforcement`, macros.
- playbooks (`engage`, `augment-plan`, `groom`, `meta-process`) + `ref-kinds.md` (`grounded-in` cites "a contract").
- `vocabulary-de.md` — translated term for `rule`.
- project `CLAUDE.md` — the CQRS "planning contract" framing.

### E. Migration
- Supersede each active contract → a `rule` at `enforcement: binding` (existing reclassify-via-supersede move).
- Old `grounded-in <contract>` refs auto-resolve to the new rule head via supersede-chain resolution (s-tac-4ly) — no dangling edges.

### F. Discovery in the working modes (required for usability)
- Wire the rule-discovery move into the skill playbooks at each juncture: plan capture, `playbook-engage.md` (engage→implement), `playbook-implementation.md`, `playbook-augment-plan.md`, and the evaluate flow.
- The "widen" section in `SKILL.md` is the natural sibling — discovery-for-rules is widen-for-rules.
- A discovery sub-skill (analogous to `sdd-explore`) that takes context + angles, searches `--kind rule`, sifts, and returns applicable rules with body.
- Angle definitions live in the playbook instructions (feature / layer / topic), not in code.
- CLI likely needs nothing new beyond `sdd search --kind rule`; a convenience `sdd rules --for <context>` is optional later.

## Open questions (for planning)

- **Provenance** — how does a learning become a rule? Emit a candidate on closing a done? A capture-time prompt? Manual only at first?
- **Discovery precision/recall** — angle design; sub-agent sift criteria; whether a soft juncture boost is needed.
- **Status rendering** — one "Rules" section, or split binding vs advisory?
- **Enforcement phasing** — ship the kind + advisory discovery first; wire binding-blocks in pre-flight second.
