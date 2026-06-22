# Graph-resident rule system — v1 spec

Draft spec for the v1 rule system. Becomes the attachment for the owning plan decision once reviewed. Realizes `20260608-190808-d-cpt-30v` (rule directive) and folds in `20260611-085648-d-cpt-0ah` (pre-flight as second consumer), with two conscious deviations from the original design records noted in the decisions below.

Status: **draft — all markers resolved**. Ready for a readiness review / task breakdown. Decisions settled this session are listed at the foot.

---

## 1. Goals & Requirements

### What we're building

The mechanism by which **any** SDD project grows its own process layer on a lean core. A project captures process learnings as graph-resident `rule` entries; the right ones resurface at the moment they apply — as guidance during work and as enforcement at capture. SDD's own contracts are the first tenant and the test corpus, not the deliverable.

Rules are domain-neutral: technical (CQRS-on-plan), procedural (regression-test-on-bugfix), or organizational (touch security → second pair of eyes; UI change → UX expert vets).

### Functional requirements

1. A new `rule` decision kind: a stored `enforcement` flag (`binding | advisory`), a bundled `activation` block (decision 2), and body sections **What to consider / How to calibrate**.
2. A **dual trigger** bundled under `activation`: `when` (free-text, matched semantically) and `matches` (structured, matched mechanically by pre-flight); at least one required.
3. **Discovery** — an always-loaded activation index plus a two-agent semantic discovery move, run at the junctures that already exist (planning, pre-implementation, review/close).
4. **In-flow delivery is uniform advisory** — rules surface as dialogue pushback and as injected context for an acting agent. Never blocks mid-work.
5. **Pre-flight is the second consumer** — at capture, binding rules whose `activation.matches` matches produce blocking (high) findings; advisory rules weight severity. This is the one place a binding rule gates, and it's the existing, overridable capture gate.
6. **Capture** — two paths: user-initiated ("that's a rule") and a graduated system-proposed nudge (light, ignorable suggestion → full proposal only on recurring-or-verified friction).
7. **Usage tracking** — positive-only: an entry shaped by a rule refs it `grounded-in`; existing heat/decay turns that into a usefulness signal.
8. **Transitional coexistence** — `rule` and `contract` kinds coexist; this plan **deprecates** `contract` on implementation. Removing it (and migrating existing contracts) is out of scope — a later effort, ending at 7+7.
9. **Rule quality is first-class** — both the skill capture path and pre-flight know what a good rule looks like (specific + checkable, recurring-or-verified, non-duplicating); see decision 10.

### Out of scope for v1 (deferred)

- Migrating SDD's own active contracts into rules, **and removing the `contract` kind** (v1 only *deprecates* it) — gated on the intent plan `20260610-000354-d-tac-n9k`; the always-on → guiding-directive branch needs `intent: guiding` to exist (`20260610-000444-s-cpt-iuh`).
- Rules pointing at roles/actors (organizational handoff targets).
- The working-mode / transition architecture (`20260523-114322-d-prc-nkw` and kin) — rules ride existing junctures.
- Deterministic Go evaluation of binding rules beyond the few already-mechanical checks; structural-trigger optimization beyond the minimal `activation.matches` selector.
- Auto-clustering/consolidation of the rule corpus (topics already bucket it).

### Grounding entries

`20260608-190808-d-cpt-30v` (directive + design.md), `20260611-085648-d-cpt-0ah` (pre-flight consumer), `20260608-232836-s-cpt-smn` (discovery design synthesis), `20260608-235644-s-cpt-7ca` (prototype eval + 8-rule seed corpus), `20260610-000444-s-cpt-iuh` (intent coordination), `20260604-150714-d-cpt-cyx`/`20260610-000354-d-tac-n9k` (guiding-directive/intent dependency), `20260506-191632-d-cpt-ni0` (the 7+7 contract to supersede), `20260609-001150-s-cpt-ov7` (base process ships bundled, not as rules), `20260610-000311-d-tac-oei` (`sdd status` retirement — rendering carve-out).

---

## 2. Architecture & Design Decisions

### 1. New `rule` decision kind; transitional coexistence with `contract`

**Decision.** Add `rule` as a decision kind alongside the existing seven. On implementation, this plan **marks `contract` deprecated** — the kind still parses and existing contract entries stay valid, but the skill stops proposing new contracts and steers to rules. The plan supersedes `20260506-191632-d-cpt-ni0` with a transitional enumeration carrying both kinds, `contract` flagged deprecated. **Removing the `contract` kind and migrating the seven existing contracts is explicitly out of scope** — no trigger, no commitment to when/how; a later effort owns it. End state (7+7, rule replaces contract) is the direction, not a deliverable here.

**Reasoning.** **Conscious deviation** from `20260608-190808-d-cpt-30v`/design.md, which specified a rename-in-place ("a rename + generalize, not an addition — stays 7+7"). A big-bang rename forces migrating all seven active contracts at once, and that migration's always-on → guiding-directive branch is gated on the intent plan `20260610-000354-d-tac-n9k` (`20260610-000444-s-cpt-iuh`). Deprecate-now-remove-later keeps the migration (and its `20260610-000354-d-tac-n9k` coupling) **entirely out of this plan** — v1 ships the rule kind and deprecates contract; the removal effort handles migration on its own timeline. Precedent for adding a decision kind cleanly: `focus` and `annotation` (the add-a-kind checklist below).

### 2. Rule representation — `enforcement` + a bundled `activation` block; substance in body

**Decision.** A rule entry carries:
- **Frontmatter**: `enforcement: binding | advisory` (required), and a required `activation` block bundling the two triggers:
  - `activation.when` — free-text, one concise sentence; the conversational trigger (semantic discovery + index).
  - `activation.matches` — structured mechanical filter (`kinds`, `operations`); the pre-flight trigger.
  - At least one of `when` / `matches` is required. Either alone = conversational-only or mechanical-only; both = both surfaces.
- **Body**: only `## What to consider` and `## How to calibrate` — the substance the agent reads when the rule applies. No trigger lives in the body.

**Reasoning.** Keying activation off a body heading couples the mechanism to text shape (a rule titled "When this applies" breaks the index). Moving both triggers into a structured `activation` block means the index and pre-flight read fields, never prose. Bundling under one `activation` key (rather than two siblings) groups the triggering concern and matches the "activation" framing in `20260608-232836-s-cpt-smn`. Frontmatter plumbing follows the kind-specific-field precedent (focus's `involvement:`).

### 3. The two triggers — `activation.when` (semantic) + `activation.matches` (mechanical)

**Decision.** The bundled `activation` block carries two independently-optional triggers:
- **`when`** (free-text) → matched **semantically** by the in-flow discovery move and embedded for search.
- **`matches`** (structured) → matched **mechanically** by pre-flight to select which rules apply to the entry being captured.

Schema:

```yaml
activation:
  when: "Shipping a change when other parts depend on or encode it."
  matches:
    kinds: [done]            # entry kinds this fires on; absent = any
    operations: [closing]    # capture | closing | supersede | dissolution; absent = any
```

Pre-flight selects a rule when `entry.Kind ∈ matches.kinds` (or absent) **and** the check-type ∈ `matches.operations` (or absent). Selection only — what the rule *requires* lives in its body.

**Reasoning.** **Conscious deviation** from `20260608-232836-s-cpt-smn`, which treated structural triggers as a "later optimization, not the v1 mechanism." Pulling pre-flight enforcement into v1 requires a machine-checkable discriminator, because pre-flight "assembles validation context deterministically, not via the semantic sift" (`20260611-085648-d-cpt-0ah`). `matches` targets **entry-detectable** conditions only (kind, operation) — pre-flight sees the entry, not the code diff. Rules whose real trigger is about external work (e.g. seed rule #2, "new `.sdd/` path needs a gitignore entry") carry only `when` and stay conversational. `matches` is intentionally minimal (`kinds` + `operations`); `layers` can be added later if a real need appears.

### 4. Discovery — activation index + two-agent move at existing junctures

**Decision.**
- An **activation index**: a compact, always-loaded section over active rules **that carry a `when`** (the discoverable set), listing each rule's id, enforcement, and `activation.when` — read directly from the field, no body parsing. Mechanical-only rules (`matches` but no `when`) are **excluded** — they fire deterministically at pre-flight and need no query orientation; `matches` never appears in the index. Slim, indented output (id + enforcement on a header line, `when:` indented beneath — the `sdd show` slim aesthetic):

  ```
  Rules — activation index · 3 discoverable

    d-prc-a1b  advisory
      when: Editing skill or prompt text.
    d-tac-e5f  binding
      when: Adding a new machine-local path under .sdd/.
  ```

  Generated on demand by a new CLI command and injected into the skill via the existing `inject` helper (`{{ inject "sdd ..." }}` → `` !`sdd ...` ``), refreshed each session load — same pattern as `sdd info` / aspirations / focus.
- A **discovery move** in the playbooks at planning, pre-implementation, and review/close: the outer agent uses the index to compose activation queries; a sub-agent runs `sdd search --kind rule` over activation chunks, reranks, and returns the applicable few *with body*; the outer agent judges applicability. Same compression shape as `/sdd-explore`.

**Reasoning.** The index solves the bootstrapping problem (the agent must know which activations exist to query well) — the "tiny always-loaded top-level index, detail on demand" pattern (`20260608-232836-s-cpt-smn`). `sdd search --kind rule` already exists once the kind does; the genuinely new build is the index command + injection and the discovery sub-skill. Junctures are query angles, not declared fields on the rule (`20260608-190808-d-cpt-30v`). Index runs at every session load, so it is **deterministic, no LLM in the path** (verbatim/truncated `when`, not a summarization pass). The `when`-only scope is principled: a `when` *is* the author's declaration "surface me during work," so the discoverable set is exactly the `when`-bearing rules; a `matches`-only rule deliberately chose capture-time gating and has nothing for the orientation index.

**Resolved**: a dedicated `sdd rules` command namespace (groups all rule operations; avoids bloating `sdd view`), with `sdd rules index` as the index subcommand. Other rule queries land under `sdd rules` later.

### 5. In-flow delivery — uniform advisory, never blocks

**Decision.** During work, rules guide; they never block. Same rule body, two delivery points:
- **Dialogue** — surfaced as pushback ("we settled this; did you account for it?").
- **Agent work** — injected into the acting agent's context to steer implementation/review.

`enforcement` shapes *push strength*, not blocking: advisory = a consideration; binding = a must-reckon-with pushback the agent surfaces and, on the agent-work surface, flags before violating. Both let work proceed.

**Reasoning.** This is the UX shaped this session and the correct model for organizational rules (SDD can't verify a human sign-off). **Refines** `20260608-190808-d-cpt-30v`/design.md's "binding (blocks)" — binding blocks *only at pre-flight* (decision 6), not in-flow. Delivery is skill-text/playbook; no enforcement engine in the in-flow path.

### 6. Pre-flight as second consumer — binding enforces, advisory weights

**Decision.** At capture, after the check-type is selected:
- Select active rules whose `activation.matches` matches `(entry.Kind, check-type)` — deterministic, sorted by ID for byte-stability.
- **Binding** matched rules → injected into the check as binding criteria; a violation yields a **high** (blocking) finding, overridable via `--skip-preflight` like any pre-flight finding.
- **Advisory** matched rules → injected as severity-weighting context (no independent block).
- A new **rule-capture** check validates a `rule` entry itself in two tiers (see decision 10): *mechanical* (valid `enforcement`; `activation` with ≥1 of `when`/`matches`; `matches` well-formed; body sections present) and *calibration* (LLM advisory — is `when` specific enough, does it duplicate an existing rule/check, is it recurring-or-verified).

**Reasoning.** Pre-flight is the existing before-capture gate with a findings/severity model and a high-blocks-creation rule (`internal/finders/preflight.go`, `handler_new_entry.go`). Active contracts already load into the cache-stable system block via `graph.Contracts()`; binding rules follow that seam (`20260608-190808-d-cpt-30v` flags this as the load-everything spot, `20260604-162749-s-tac-osb`). The deterministic `activation.matches` selection keeps the injected set scoped and the prefix cache-stable.

**Resolved**: prose binding rules are LLM-evaluated (injected as criteria into the matched check template). The existing code checks (canonical, plan-needs-ACs, …) **stay in code** — v1 adds enforcement only for *new* binding rules in the corpus, no re-homing (avoids duplicating existing enforcement — `20260608-235644-s-cpt-7ca` #5).

### 7. Capture — two paths, mostly skill-text

**Decision.**
- **User-initiated**: "that's a rule" → dialogue → `sdd new d <layer> --kind rule --enforcement <b|a> [--when ...] [--matches ...]`. The user is the verification.
- **System-proposed, graduated**: a light one-line nudge at process-shaped friction (fires early, ignorable, no entry if waved off) → a full proposal only when the user bites or the friction is recurring-or-verified.

The rule-worthiness test (recurring-or-verified) and the graduated nudge live in skill-text/playbooks (capture discipline + the after-completion move). Code adds only the `--kind rule` capture surface and flags.

**Reasoning.** Provenance reuses the existing dialogue-first capture discipline prompted at evaluate/close (`20260608-232836-s-cpt-smn`). The nagging budget sits on the full proposal; the light nudge's only gate is "process-shaped" (`20260608-235644-s-cpt-7ca`).

**Resolved**: `enforcement` is **required** — no default. Capturing a `rule` without `--enforcement` fails; the skill must choose binding/advisory explicitly. (Validation already requires it.)

### 8. Usage tracking — positive-only via `grounded-in`

**Decision.** An entry shaped by a rule adds a `grounded-in` ref to it. No new code beyond the convention and a skill-text instruction; existing heat/decay and `rank(heat)` surface recently-useful rules. Demotion is passive (unused rules decay).

**Reasoning.** Reuses typed refs + heat already in the graph (`20260608-232836-s-cpt-smn`). No negative-signal tracking.

### 9. Rendering surfaces — on `sdd view` / `sdd show`, not `sdd status`

**Decision.** Rules render through `sdd view` and `sdd show`, **not** `sdd status` — that command is being retired by `20260610-000311-d-tac-oei` (active, high) and is off the catch-up/`/sdd` hot path, so it gets no Rules section. Concretely:
- A `rules` view macro (`active:kind(rule):name-prefix("Rules"):as-list`) — precedent: the `contracts` macro.
- Add `rule` to the `decisions` macro's kind set; `kind(rule)` filter already works.
- `enforcement` renders as a stored attribute on entry lines — `[enforcement: binding|advisory]`, mirroring `[confidence: ...]`; `activation` shows in the `sdd show` envelope.
- Binding/advisory separation is **on-demand** via `group(by(enforcement))` (add `enforcement` to the groupable fields) — no permanent split.
- Plus `--kind rule` help on `new`/`list`, the MCP tool schema, and `framework-concepts.md` / `cli-reference.md` / vocabulary. The `contracts` macro stays during coexistence.

**Reasoning.** The earlier one-section-vs-split question dissolves once the surface is `sdd view`: grouping is a layout choice the reader makes, not a baked-in section. Building a `sdd status` Rules section would invest in a surface `20260610-000311-d-tac-oei` removes. Surfaces enumerated per the type-system-plan-surfaces-presentation rule (CLAUDE.md), with `sdd status` consciously carved out.

### 10. Rule quality — what a good rule looks like, on both surfaces

**Decision.** The system must know what a *good* rule looks like, surfaced on two **bundled** (framework-level, not graph-resident) surfaces:
- **Skill / capture discipline** — guidance shaping both authoring paths (user-initiated and the graduated nudge): favor **specific + checkable over broad** (an over-broad rule fires on everything and breeds alert-fatigue; its narrow, checkable specialization is what's useful); **recurring-or-verified** worthiness; **fill an enforcement gap, don't duplicate** a check already enforced elsewhere; write a concise `activation.when` that names a recognizable situation.
- **Pre-flight `rule_capture` check** (decision 6) — the same criteria as a validation tier: mechanical structure (blocking) + calibration-for-text (LLM advisory: specificity, non-duplication, provenance).

**Reasoning.** Grounded in `20260608-235644-s-cpt-7ca` #3 (the dilution risk is real — the capture procedure needs "rules about writing good rules") and #5 (rules fill gaps, don't duplicate existing enforcement). This guidance is the lean-core base process, so it ships **bundled in the skill + pre-flight templates**, not as graph rules (`20260609-001150-s-cpt-ov7`: the base ships via skills/binary). Projects may later add their own rule-quality rules on top; v1 ships the base.

---

## 3. Implementation Changes

### Model — `internal/model/`

- `entry.go`:
  - Add `KindRule Kind = "rule"` to the kind consts and `KindRule: true` to `decisionKinds` (`entry.go` ~67–109).
  - Add `IsRule()` predicate (~202–225).
  - Add fields to `Entry` and the `frontmatter` struct: `Enforcement string` (`yaml:"enforcement,omitempty"`) and `Activation *Activation` (`yaml:"activation,omitempty"`). Define `Activation struct { When string; Matches *RuleMatches }` and `RuleMatches struct { Kinds []Kind; Operations []string }`.
  - Route the new fields in `ParseEntry` and `FormatFrontmatter` (the same lift/route pattern as focus's `Involvement`, ~267–361 / 488–540).
- `graph.go`:
  - Add `Rules() []*Entry` and `BindingRules() []*Entry` accessors (follow `Contracts()` ~122–136).
  - Add `validateRuleFrontmatter(e, g)` and call it from `ValidateEntry` (~765–768): require valid `enforcement`; an `activation` block with ≥1 of `when`/`matches`; `matches` well-formed if present; `## What to consider` and `## How to calibrate` body sections present.
  - Add a body-section helper (pure) for the rule-section presence check.
  - Update the valid-kinds error message (~862).

### Pre-flight — `internal/llm/` and `internal/finders/`

- `llm/preflight.go`:
  - Add a `checkRuleCapture` check type + `rule_capture` template entry (dispatch ~149–162, `selectCheckType` ~190–246).
  - In `assembleContext`, select applicable rules by `activation.matches` and add `BindingRules`/`AdvisoryRules` fields to `preflightContext` (~172–185, ~255–270), sorted by ID for byte-stability.
  - Inject advisory rules and matched binding criteria into the relevant templates.
- `llm/preflight_templates/`: add `rule_capture.tmpl` carrying the rule-quality calibration criteria (decision 10: specificity, non-duplication, provenance) — plus `_system` if structural — and a `rules.tmpl` partial included in `universal_system.tmpl` after `contracts.tmpl`.
- `finders/preflight.go` / `preflight_mechanical.go`: the deterministic `validateRuleFrontmatter`-style structure check for rule entries; if any binding rule is deterministically checkable, emit high findings here (precedent: the mechanical canonical / ref-applicability checks). v1 default per decision 6 is LLM-evaluated, so this stays minimal.

### Discovery & activation index — new CLI surface + skill

- New `sdd rules` command namespace with an `index` subcommand: `cmd/sdd/` + `internal/query/` (`RulesIndexQuery`) + `internal/finders/` (iterate active `graph.Rules()` carrying a `when`, read `activation.when` — no body parsing) + `internal/presenters/` (slim, deterministic: id + enforcement, indented `when:`).
- **Indexer** (`internal/handlers/handler_index.go` / `internal/index/`): for `rule` entries, embed `activation.when` as a dedicated chunk (e.g. `<id>#activation`) so semantic discovery matches situation-against-situation from the field, not a parsed body section.
- `internal/bundledskills/templates/sdd/SKILL.md.tmpl`: add an always-loaded `### Rules` section via `{{ inject "sdd rules index" }}` (precedent: the `sdd info` / aspirations / focus injections).
- New discovery sub-skill template (analogous to `sdd-explore`) that takes context + angles, runs `sdd search --kind rule`, sifts, returns applicable rules with body.
- Playbook edits wiring the discovery move at junctures: `playbook-engage.md`, `playbook-implementation.md`, `playbook-augment-plan.md`, and the plan-capture / evaluate flows; a "widen-for-rules" sibling in `SKILL.md`.

### CLI capture — `cmd/sdd/` + `internal/command/` + `internal/handlers/`

- `main.go`: add `rule` to `--kind` help on `new` and `list` (~513, ~623); add `--enforcement` (required when `--kind rule`, no default), `--when`, and `--matches` flags (precedent: focus's `--involvement`/`--when`, ~645–656); parse and route (~753–820).
- `command/new_entry.go`: add `Enforcement` and `Activation` fields to `NewEntryCmd` and route in `BuildEntry`.
- `handlers/handler_new_entry.go`: no structural change beyond carrying the new fields.

### Rendering — view / show / list / MCP (not status)

- `query/macros.go`: add a `rules` macro (`expandRules`, precedent `expandContracts` ~239) and add `rule` to `expandDecisions`' kind set (~154).
- `cmd/sdd/view.go`: macros help + examples include `rule` (~124, ~179).
- Entry-line presenter (shared `as-list` / `sdd list` line) + `sdd show` envelope: render `[enforcement: …]` as a stored attribute; surface `activation` in the `sdd show` envelope.
- `internal/finders/aggregation.go`: add `enforcement` to the groupable fields (~14) so `group(by(enforcement))` works on demand.
- `internal/mcpserver/tools.go`: add `rule` to the kind enum in the tool schema (~62).
- **Not touched**: `sdd status` (`query`/`finders`/`presenters` status layer) — retired by `20260610-000311-d-tac-oei`, consciously carved out.

### Skill, docs, graph entries

- `framework-concepts.md`: decision-kinds table, distinguishing test, retirement-primitives table, the `rule` vs `contract` coexistence note; `ref-kinds.md` (`grounded-in` "a contract" → "a rule/contract").
- Capture discipline (skill-text / a `rules` reference): the "what a good rule looks like" guidance from decision 10, shaping user-initiated authoring and the graduated nudge.
- `cli-reference.md`, `SKILL.md`: `--enforcement`/`--when`/`--matches`, the `rules` macro, the activation index, the discovery move.
- `vocabulary-de.md`: translated term for `rule`.
- Graph: supersede `20260506-191632-d-cpt-ni0` with the transitional enumeration; capture the 8 seed-corpus rules (`20260608-235644-s-cpt-7ca`) as the first rules + pre-flight/discovery fixtures.

---

## 4. Test Cases

Organized by layer; assertions to be filled per decision as markers resolve.

### Model — `internal/model/`

| Test | Setup | Action | Expected |
|---|---|---|---|
| Rule roundtrip | `rule` with `enforcement` + `activation` (when + matches) + 2 body sections | Parse → Format → Parse | fields preserved byte-stable |
| Valid kind | `d` + `rule` | `IsValidKindForType` | true |
| Missing enforcement | `rule`, no `enforcement` | `validateRuleFrontmatter` | error |
| Missing section | `rule`, no `## How to calibrate` | `validateRuleFrontmatter` | error |
| No trigger | `rule`, `activation` with neither `when` nor `matches` | `validateRuleFrontmatter` | error |
| Malformed `matches` | unknown operation | `validateRuleFrontmatter` | error |
| `Rules()` / `BindingRules()` | mixed corpus, some closed | accessors | only active rules; binding subset correct |

### Pre-flight — `internal/llm` + `internal/finders`

| Test | Setup | Action | Expected |
|---|---|---|---|
| Binding match → block | binding rule `matches: {kinds:[plan]}`; plan violating it | preflight | high finding, `HasBlocking` true |
| Advisory no block | advisory rule matches; entry "violates" | preflight | finding ≤ medium, no block |
| `matches` miss | rule `matches.kinds:[done]`; capturing a plan | preflight | rule not selected |
| Rule-capture structure | malformed `rule` entry | preflight | mechanical rule_capture findings |
| Rule-capture: over-broad | `rule` with vague `when` ("shipping a change") | preflight rule_capture | calibration advisory finding |
| Rule-capture: duplicate | `rule` duplicating an existing rule/check | preflight rule_capture | calibration advisory finding |
| Byte-stability | same rule set, two captures | render system prompt | identical bytes |

### Discovery / index

| Test | Setup | Action | Expected |
|---|---|---|---|
| Index content | 3 active `when` rules + 1 closed + 1 matches-only | `sdd rules index` | the 3 `when` rules only; closed and matches-only excluded |
| Index is field-read | rule with no body sections, only `activation.when` | `sdd rules index` | renders correctly (no parse dependency) |
| Activation chunk | rule with `activation.when` | index the entry | a dedicated `#activation` chunk embedding the `when` text |
| `--kind rule` search | rule corpus | `sdd search --kind rule` | rules ranked, bodies retrievable |

### Rendering / CLI

| Test | Setup | Action | Expected |
|---|---|---|---|
| `rules` macro | rule corpus | `sdd view --layout='rules'` | rules listed, each line `[enforcement: …]` |
| group by enforcement | mixed binding/advisory | `kind(rule):group(by(enforcement))` | two groups |
| `decisions` includes rule | mixed decisions | `decisions` macro | rule rows present |
| Capture | `sdd new d prc --kind rule --enforcement binding --matches ...` | run | entry written, frontmatter correct |

---

## Open markers summary

- *(none open — all markers resolved)*

Resolved this session: §2.1 (deprecate `contract` on implementation; removal + migration out of scope) · §2.2/§2.3 (bundled `activation`: `when` + `matches`, ≥1 required; `matches` = `kinds` + `operations`) · §2.4 (`sdd rules index` under a `sdd rules` namespace) · §2.6 (binding LLM-evaluated, no re-homing) · §2.7 (`enforcement` required, no default) · §2.9 (render on `sdd view` / `sdd show`, not the retiring `sdd status`; enforcement a stored attribute, split on-demand) · §2.10 (rule quality, bundled, both surfaces).
