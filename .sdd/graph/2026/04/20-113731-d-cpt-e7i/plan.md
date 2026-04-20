# Two-Type Redesign + Aspiration: Implementation Plan

Operationalizes `d-cpt-omm` (two-type directive) and `d-cpt-xqt` (scope expansion to include aspiration), folding in refinements from session dialogue that the directive-level entries did not pre-commit.

## Context

`d-cpt-omm` committed to collapsing three types (signal, decision, action) to two (signal, decision) with explicit kinds. `d-cpt-xqt` expanded the scope to include an aspiration kind, avoiding a second round of type-system churn. Session dialogue (2026-04-19 / 2026-04-20) produced further refinements:

- **Activity retained as a distinct decision kind** — validated against story and graph evidence via the WHAT-vs-THAT distinction
- **Universal retirement rule** — the immutability contract governs files, not relevance; every entry must be retireable
- **Short-loop closure calibrated** — signal-closes-signal allowed at low layers, gated at higher ones
- **Pre-flight validates structure, not reasoning** — checks presence of dialogue-captured rationale; dialogue validates content quality
- **`sdd rewrite` plumbing command** — makes `d-cpt-e1i`'s "mechanical fixes permitted" rule concretely actionable
- **Phased migration with teardown** — build support alongside legacy, migrate data, remove scaffolding

## Target type system

### Types (2)

| Type | Purpose |
|---|---|
| signal | Something the graph should know about |
| decision | Something we commit to |

### Signal kinds (5)

| Kind | Question it answers | Default? |
|---|---|---|
| gap | What needs attention? | yes |
| fact | What do we know? | no |
| question | What do we not know? | no |
| insight | What have we synthesized? | no |
| done | What was accomplished? | no |

### Decision kinds (5)

| Kind | Question it answers | Default? |
|---|---|---|
| directive | Which way do we go? | yes |
| activity | What's next to do? | no |
| plan | What must be true when done? | no |
| contract | What must always hold? | no |
| aspiration | What are we pulling toward? | no |

## Distinguishing tests

When drafting an entry, the kind emerges from dialogue. Pre-flight enforces consistency by asking these questions in order for decisions:

1. Does the entry push against a constraint that should always hold? → **contract**
2. Does it pull toward a direction with no completion criterion? → **aspiration**
3. Does the narrative justify a choice against alternatives? → **directive**
4. Does it shape the WHAT (define verifiable outcomes)? → **plan** (requires `## Acceptance criteria`)
5. Does it dispatch THAT work happen (shape known from context)? → **activity**

### Strategic directive vs aspiration

The most common ambiguity. Test: does the entry have a plausible completion criterion?

- Yes (closable by done or supersede) → **directive**
- No (perpetual pull, every decision aligned against it) → **aspiration**

Confidence often signals this — directives can go high-confidence once settled; aspirations sit at medium indefinitely.

### Activity vs plan (WHAT vs THAT)

- **Plan** shapes the WHAT — defines what must be true when complete. ACs are the mechanism.
- **Activity** dispatches THAT — specifies work whose shape is already known from context (parent plan, refs, or self-evident narrative).

Boundary test: is validation a single self-evident "did you do the thing?" → activity. Does the AC specify a testable outcome separable from the work itself? → plan.

## Lifecycle and retirement

Every entry is retireable. Two primitives:

- **supersedes** — same-kind successor replaces it
- **closes** — new entry retires it without replacement

| Entry | Supersede path | Close path |
|---|---|---|
| gap | refined gap | decision addressing it; or done signal (short-loop, see calibration) |
| fact | corrected fact | directive: "no longer true / no longer relevant" |
| question | refined question | directive: "answered as X" or "won't pursue" |
| insight | corrected insight | directive: "noted, no action needed" |
| done | corrective done (rare) | (terminal — facts of execution) |
| directive | replacement directive | done signal (standard); directive retiring it |
| activity | replacement activity | done signal (standard); directive retiring it |
| contract | replacement contract | directive retiring it |
| plan | restructured plan | done (via ACs); directive retiring it |
| aspiration | evolved aspiration | directive retiring it |

**Retirement rationale is required.** Pre-flight on a closing decision against a stable-kind entry (fact, insight, contract, aspiration) checks that the narrative states *why* — not whether the why is correct.

## Short-loop closure calibration

Signal-closes-signal (done-closes-gap) is allowed. Pre-flight severity by layer × narrative scope-shape:

| Layer of gap | Execution-shaped done | Approach-shaped done |
|---|---|---|
| ops / prc | low — proceed | medium — surface: "reads like a choice, want to capture a decision?" |
| tac | low — proceed | high — block |
| cpt | medium — surface | high — block |
| stg | high — block | high — block |

- **Execution-shaped**: narrative describes what was done (*"updated X to prevent Y"*)
- **Approach-shaped**: narrative describes a choice or justification (*"changed the approach to Z because W"*)

The `medium — surface` band is the interesting one: pre-flight asks the question without forcing capture, matching the "dialogue, don't adjudicate" principle.

## Pre-flight principle

Pre-flight validates **structural presence**; dialogue validates **content quality**. Templates check:

- Required fields present (kind; ACs on plan; retirement rationale on stable-kind closures)
- Narrative shape matches kind claim (per the distinguishing tests)
- Ref chains resolve

Pre-flight does **not** argue with the reasoning participants committed to. This prevents the validator from drifting into adjudication (`s-prc-hpa` class failure).

## Structural validation (model / CLI layer)

Structural checks run in the model / CLI layer as typed errors *before* LLM pre-flight is invoked. The existing `signal_capture.tmpl` states the principle directly: *"kinds on signals are structural metadata and may evolve; prose-level type policing belongs to the kind system, not pre-flight."*

- **Signals**: `kind` is one of gap / fact / question / insight / done; for `kind: done`, at least one `closes` or `refs` is present (semantic validation of the target — decision for standard cases, signal for short-loop — is pre-flight's job)
- **Decisions**: `kind` is one of directive / activity / plan / contract / aspiration; for `kind: plan`, description contains `## Acceptance criteria` with at least one checklist item
- **All entries**: `refs` / `closes` / `supersedes` resolve to existing entries; supersede targets share the entry's type

No LLM required for these.

## LLM pre-flight — checks to add or modify

Templates organize by check transaction, not by kind. The post-plan set: `signal_capture`, `decision_refs`, `aspiration_capture` (new), `closing_decision`, `closing_done` (renamed from `closing_action`), `short_loop` (renamed from `action_closes_signals`), `dissolution` (new), `supersedes`. Plus shared partials: `unrelated_refs`, `unusual_close`, existing `contracts`, `entry_quality`, `durability`, `verdict`.

**New shared partial `unrelated_refs` (per `s-prc-uy2`):** invoked by every capture template (`signal_capture`, `decision_refs`, `aspiration_capture`, `closing_decision`, `closing_done`, `short_loop`, `dissolution`). Flags refs that don't topically connect to the entry. Calibration:

- `high` — ref points at an entry with no topical connection to this entry's concern (e.g., plan about CSV parsing refs a WIP-marker improvement signal without explaining the link)
- `medium` — ref is tangentially related but the "builds on" relationship isn't evident
- `low` — ref is related but a one-sentence note would sharpen the connection
- no finding — refs that are upstream context the entry genuinely builds on; conceptual grounding that doesn't need a connective sentence

### Template design principles

Two guiding rules for template organization:

1. **Prefer separate templates over conditional branches** when the check task or calibration shape differs. Agents read templates top-to-bottom; conditionals inside a template make it harder to see which path applies. A narrow additional finding (same calibration shape) can stay as a branch; a different task or a different severity shape (e.g., binary vs gradient) gets its own template.
2. **Close = different role resolves; supersede = same role replaces.** Supersede is the same-kind-successor primitive. Close is for cross-kind resolution and retirement. Unusual close patterns are allowed-and-flagged: severity is gated by whether rationale is captured — with rationale, `low` (surface for awareness); without rationale, `high` (block until explained).

### Close-pattern design rule

`closes` means *"this entry resolves the closed entry's purpose"*. Each kind has something different to resolve:

- **gap** — delta to close; resolved by a decision, or by a done signal (short-loop)
- **question** — unknown to dissolve; resolved by decision (answer), fact (knowledge), or insight (synthesis)
- **directive / activity / plan** — commitment to fulfill; resolved by done signal
- **fact / insight / contract / aspiration** — stable-kind, nothing intrinsic to resolve; retired by a directive with rationale

Supersede (same-kind successor refilling the role) is distinct from close (different entry resolves or retires). Unusual close patterns are *allowed-and-flagged*: severity gated by rationale presence (see `unusual_close` calibration below).

### `short_loop` template (rename of `action_closes_signals`)

Applied when a `kind: done` signal closes a `kind: gap` signal directly (bypassing a decision). Other done→signal patterns (e.g., done closes question) are unusual and trigger `unusual_close` instead. Checks:

1. **Gap coverage** — the done's narrative addresses what the gap identified
2. **Short-loop appropriateness** — at this layer, with this narrative scope, is bypassing a decision appropriate?
3. **No smuggled decision** — the narrative does not describe a choice that would have warranted a decision

Calibration by layer × narrative scope-shape:

| Layer of gap | Execution-shaped done | Approach-shaped done |
|---|---|---|
| stg | `high` | `high` |
| cpt | `medium` | `high` |
| tac | no finding | `high` |
| ops / prc | no finding | `medium` |

Bands:

- `high`:
  - Gap at stg closed by done regardless of scope-shape — strategic gaps require a captured decision
  - Gap at cpt or tac closed by approach-shaped done — the choice is smuggled into the done; should have been a decision
  - Done does not address the gap it claims to close (coverage failure)
  - Example: gap "pre-flight over-rejects on wording" closed by done "Changed pre-flight to allow solution-space thinking because signals explore, decisions commit" — approach-shaped at cpt, should have been a decision capturing the reframing

- `medium`:
  - Gap at cpt with execution-shaped done — conceptual scope, no explicit choice in prose, but worth surfacing "reads like it could be a decision?"
  - Gap at ops / prc with approach-shaped done — low-layer choice worth capturing as a decision; surface without blocking
  - Example: gap "session startup is slow" closed by done "Reduced synthesis to recent entries because full depth was slow" — approach-shaped at ops, medium surface

- `low`:
  - Done technically addresses the gap but could be more specific about which aspect
  - Example: gap "pre-flight over-rejects on wording" closed by done "Improved pre-flight prompt" — general fix without naming the specific aspect

- no finding:
  - Gap at ops / prc with execution-shaped done describing pure execution
  - Standard short-loop fix at low layers — observation → fix
  - Example: gap "catch-up cites superseded decision" closed by done "Updated skill example to reference current decision"
  - Deferral explicitly acknowledged ("minimal fix; deeper rework deferred") — trust the author's dialogue-grounded judgment

**Extend `decision_refs`:**

- Directive-reads-aspiration-shaped check (inverse of aspiration-capture) — when proposed `kind: directive` at stg or cpt layer lacks a plausible completion criterion. Calibration:
  - `medium` — strategic or conceptual directive with perpetual framing ("ongoing pull toward X", "we orient around Y") and no "done when" condition
  - `low` — directive at any layer where the commitment could read as a perpetual pull rather than a bounded choice
  - no finding — directive with a clear bounded choice, even if high-level (e.g., "we will pursue direct-to-consumer over wholesale" — bounded despite being strategic)

**Extend `closing_decision`:**

- Retirement rationale check — when the closed target is stable-kind (fact, insight, contract, aspiration), verify narrative states retirement rationale. Calibration:
  - `high` — retirement claim with no why-retired statement (e.g., closing a contract with narrative "No longer relevant." and nothing more)
  - `medium` — reasoning restates what was closed without naming what changed (e.g., "Retiring the immutability contract. It said documents don't change.")
  - `low` — reasoning present but thin; a sentence would sharpen
  - no finding — cites a specific reason (new evidence, scope change, superseded by aspiration, etc.)

**Rename `closing_action` → `closing_done`:** action references stripped; AC coverage calibration preserved. Applied when a `kind: done` signal closes a decision.

### `dissolution` template (new)

Applied when a fact or insight closes a question. The close-semantics are benign (knowledge dissolving unknown) — but pre-flight checks that **dialogue-captured context** is present, not whether the reasoning is correct.

- `medium` — closing entry's narrative makes no connection to the closed question (narrative talks about fact X alone; never names the question or framing it addresses)
- `low` — connection implicit; a naming phrase would sharpen future readability
- no finding — narrative references the question by content or framing (e.g., "answers the shipping-cost question with DHL €4.50-5.80")

Same shape as retirement-rationale: presence of dialogue-captured context, not adjudication of reasoning quality.

### Unusual-close-pattern check (shared partial)

Applied by every close-carrying template (`closing_decision`, `closing_done`, `short_loop`, `dissolution`). Unusual patterns are allowed-and-flagged — severity is gated by whether retirement / intent rationale is captured in prose. Same principle as retirement-rationale: presence of dialogue-captured context is the test, not reasoning quality.

- `high` — close pattern is unusual (same-kind close where supersede fits, or atypical cross-kind close not in the standard matrix) AND narrative lacks clear rationale explaining the intent of this specific pattern
  - Example: `fact` closes `fact` with narrative only describing the new fact, no mention of why retiring without supersede
  - Example: `insight` closes `fact` with narrative describing the insight but never explaining why the old fact is being closed
- `low` — close pattern is unusual BUT rationale is clear in prose — valid close, still surface the atypical pattern for reader awareness
  - Example: `insight` closes `fact` with *"Retiring this fact because the measurement was taken pre-launch and no longer applies post-market-shift"*
  - Example: `fact` closes `fact` with *"Not superseding — no updated measurement available; just retiring the stale value"*
- no finding — standard patterns per the close matrix (decision→signal, done→decision, done→gap short-loop, fact/insight→question dissolution, directive→stable-kind retirement)

### `aspiration_capture` template (new — own template per design principle 1)

Applied when `kind: aspiration` is being captured without closes. Gradient calibration shape is structurally different from `decision_refs`'s binary shape, so it gets its own template.

- `medium` — narrative proposes direction that contradicts an active aspiration without acknowledging the tension (e.g., "ship v1 the fastest way" while an active aspiration pulls toward "dialogue shapes decisions")
- `low` — decision is neutral or partially aligned; surface for dialogue without blocking
- no finding — decision moves toward an aspiration, or tension is acknowledged in narrative
- **never `high`** — aspirations are gradient; binary blocking would be the wrong shape

## Calibration discipline

Every severity band in every template — existing and new — follows this discipline:

1. **Names a concrete observable pattern** in the entry (not a vague qualifier like "unclear" or "ambiguous")
2. **Includes at least one concrete example** so the LLM can match form
3. **"No finding" section enumerates non-findings explicitly** — patterns that look flaggable but are legitimate (e.g., "confidence the author deliberately set", "standard 'per d-tac-X plan' opening for closing actions")
4. **Distinguishes structural from prose concerns** — structural checks belong to the model layer; prose-level policing of kinds or required fields belongs there, not in the LLM template

This discipline was earned through multiple rounds of pre-flight tuning. The key lineage:

- `s-prc-316` → `d-prc-g1h` — introduced confidence-scored findings (high/medium/low + no-finding) over binary PASS/FAIL, decoupling observation from verdict. Architectural backbone of the discipline.
- `s-prc-2nh` (closed by `a-tac-eqc`) — original over-correction: validator re-argued framing/wording after dialogue had confirmed closure. Fixed by folding dialogue context into the prompt.
- `s-prc-q2m` (closed by `a-prc-sbc`) — over-rejected signals that explored solution-space as "smuggled decisions". Fixed by redefining the test to "does the entry prescribe a commitment?" rather than "does it mention solutions?".
- `s-prc-okf` (closed by `d-prc-8vh`) — over-applied the regression-test contract to template refinements that aren't bug fixes. Led to the bug-vs-refinement distinction.
- `s-prc-6hw` (closed by `a-prc-jg3`) — 60-120s × 3-4 rejections per entry made workflow unsustainable; "most rejections were either genuinely useful or flow-killing overreach".
- `s-prc-as6` (closed by `a-prc-68m`) — category errors the validator cannot assess by design; some checks are outside pre-flight's structural competence.
- `s-prc-hpa` (open) — deterministic field checks (missing kind, missing confidence) wasting LLM budget on judgments that belong in the CLI parse layer.

**Operational rule for this plan:** before drafting any new calibration band, read `signal_capture.tmpl`, `closing_action.tmpl`, `closing_decision.tmpl`, and `entry_quality.tmpl` for the established form. A band that reuses vague qualifiers like "unclear" or "ambiguous" without a concrete pattern + example re-invites the over-correction history above.

## Migration approach (phased)

### Phase 1 — Build two-type support alongside legacy

- Extend frontmatter parser to accept both `type: action` (legacy) and `type: signal` + `kind: done` (new)
- Add signal kinds (gap / fact / question / insight / done) and decision kinds `activity` / `aspiration` to the model
- Add pre-flight templates per check transaction (renames + new templates listed above) with the calibration rules specified
- Add `sdd new s --kind <k>` / `sdd new d --kind <k>` paths; add `--missing-kind` filter to `sdd list`
- Build `sdd rewrite` plumbing command

`sdd new a` continues to work in Phase 1 so the graph stays operational during transition.

### Phase 2 — Migrate data

**Capture the target-state contract first.** Before any rewrites commit: a fresh `kind: contract` decision stating the two-type system as the standing rule (supersedes the type-count claim in `d-cpt-o8t` independently of the immutability claim already handled by `d-cpt-e1i`).

Then proceed in batches:

**Batch 1 — Mechanical rewrites** (via `sdd rewrite`):
- All 98 `a-*` → `s-*` with `kind: done`
- All 115 kindless signals → add `kind: gap` (most cases mechanical; a small judgment sweep catches fact / question / insight where narrative is unambiguous)

**Batch 2 — Judgment pass** (activity-shaped directives):
- `sdd list --kind directive` enumerates candidates
- Skill walks through `tac` / `ops` layer candidates, applying the WHAT-vs-THAT test in dialogue
- Confirmed retags → `sdd rewrite <id> --type d --kind activity`
- Ambiguous cases stay as directives (safe default)

**Post-migration:** one `sdd lint` + one `sdd summarize --all`.

### Phase 3 — Teardown

- Remove `TypeAction` (and equivalents) from the model
- Remove `sdd new a` CLI path and legacy frontmatter parsing
- Verify no `type: action` remains anywhere in the graph
- Update CLI help, README, skill docs to reflect the new vocabulary

`sdd rewrite` stays — it's genuinely useful plumbing beyond this migration, and it makes `d-cpt-e1i` concretely actionable for future mechanical fixes.

## `sdd rewrite` specification

```
sdd rewrite <id> --type <s|d> --kind <k> [--dry-run] [--no-commit] [--message <text>]
```

**Behavior:**

1. Compute new ID (swap type character; timestamp / layer / suffix preserved — e.g., `20260418-101837-a-tac-r8l` → `20260418-101837-s-tac-r8l`)
2. `git mv` the file to its new path
3. Rewrite frontmatter: `type`, `kind`, anything else specified
4. Find every entry with `refs` / `closes` / `supersedes` pointing at the old ID; update those references in place
5. Stage changes; commit atomically unless `--no-commit`

**Does not:**

- Run pre-flight (mechanical operation, not a capture)
- Re-run summaries (agent runs `sdd summarize --all` once at end of batch)
- Run lint (agent runs `sdd lint` once at end of batch)

**Orchestration is the outer agent's job.** `sdd rewrite` does one thing per invocation; the skill batches it and runs lint / summarize at the batch boundary.

## Judgment-pass protocol (activity-shaped directives)

For Batch 2 of Phase 2:

1. `sdd list --kind directive --layer tac` then `--layer ops` enumerates candidates
2. Skill walks each in sequence — show the entry's full content via `sdd show`, apply the WHAT-vs-THAT test in dialogue
3. Confirmed retags → `sdd rewrite <id> --type d --kind activity --no-commit` (or commit per entry, agent's call)
4. Entries too borderline to judge confidently stay as directives — the WHAT-vs-THAT test is lossy at the boundary; leaving ambiguous cases as directives is the safe default
5. End of pass: `sdd lint` + `sdd summarize --all`

If the judgment pass grows beyond ~30 entries requiring retag, carve it off as a follow-up action — don't hold the plan.

## CQRS decomposition

Implementation decomposes across the CQRS layers per `d-cpt-ah1`.

### `internal/model/` — pure domain

- Kind constants: `KindActivity`, `KindAspiration` (decision); `KindGap`, `KindFact`, `KindQuestion`, `KindInsight`, `KindDone` (signal)
- Kind validation rules (signal kinds vs decision kinds; defaults)
- Short-loop calibration matrix as pure function `(Layer, ScopeShape) → Severity`
- Retirement-rationale structural detection predicate
- Extended `DerivedStatus` for universal retirement — closes-by-directive counts as retirement on stable-kind entries

### `internal/command/` — write-intent structs

- New `RewriteEntryCmd` carrying entry ID, new type, new kind, optional message, `DryRun` flag, `NoCommit` flag
- Existing `NewEntryCmd` extended to accept new kinds

### `internal/query/` — read-intent structs

- Existing list query extended with `MissingKind bool` filter
- Pre-flight remains a query per `d-cpt-ah1`'s note: pure read intent at the domain level despite the LLM runner's side effect

### `internal/finders/` — pure reads

- New **inbound-reference finder** — given an entry ID, returns all entries with `refs` / `closes` / `supersedes` pointing at it (used by rewrite handler)
- List finder extended with missing-kind filter
- Pre-flight finder extended to select kind-specific templates and apply the calibration matrix

### `internal/handlers/` — write-side, side effects

- New **rewrite handler** — receives `RewriteEntryCmd`, calls inbound-reference finder, applies `git mv`, rewrites frontmatter, updates inbound references, commits atomically. Bypasses pre-flight.
- Existing new-entry handler extended to validate new kinds

### `internal/presenters/` — view rendering

- Status presenter extended: aspirations as a distinct grouping from contracts; activities distinguishable from directives

### `internal/llm/` — pre-flight templates

- Existing templates stay organized per check transaction; the new kind vocabulary is context, not a reason for per-kind template proliferation
- `closing_action.tmpl` renamed to `closing_done.tmpl`; action references stripped; AC coverage calibration preserved
- `action_closes_signals.tmpl` renamed to `short_loop.tmpl`; retargeted for done-kind signals closing signals; calibrated by layer × scope-shape matrix with gap-coverage and smuggled-decision checks
- New `dissolution.tmpl` — fact or insight closes question; dialogue-presence check (not content-correctness)
- `signal_capture.tmpl` and other capture templates unchanged in core logic — only shared partials are added
- New shared partial `unrelated_refs.tmpl` invoked by all capture templates — flags topically disconnected refs (per `s-prc-uy2`)
- New shared partial `unusual_close.tmpl` invoked by all close-carrying templates (`closing_decision`, `closing_done`, `short_loop`, `dissolution`) — surfaces atypical close patterns for confirmation at medium/low severity
- New `aspiration_capture.tmpl` — applied when kind=aspiration captured without closes; gradient calibration (never high)
- `decision_refs.tmpl` extended with directive-reads-aspiration-shaped check (medium/low at stg/cpt for kind=directive with no completion criterion) — kept as a branch since its calibration shape matches the template's existing binary form
- `closing_decision.tmpl` extended with retirement-rationale check (stable-kind targets)
- All new and modified calibrations follow the discipline in "Calibration discipline"

### `cmd/sdd/` — CLI thin shell

- New `sdd rewrite` subcommand parses flags, constructs `RewriteEntryCmd`, dispatches to handler
- Existing `sdd new` accepts new kinds; existing `sdd list` accepts `--missing-kind`

### Rule discipline

- Side effects confined to handlers — the rewrite handler is the only place touching files / git for this plan
- Handlers return errors only; result data flows back through CLI-invoked finders
- Finders pure — inbound-reference finder reads the graph, returns IDs, doesn't mutate
- Model carries all pure computation (validation, calibration matrix, retirement detection)

## Dependencies

- `d-cpt-vt1` (structural separation) — honored; pre-flight runs separately from capture
- `d-cpt-ah1` (CQRS planning contract) — honored; implementation decomposes across `command/` / `query/` / `finders/` / `handlers/` / `model/`
- `d-cpt-e1i` (immutability contract) — interpreted: type-system migration is a mechanical fix per the "reader understanding unchanged" test; documented in the new two-type contract decision

## Out of scope (explicit)

- **Retro-labeling strategic directives as aspirations.** `s-cpt-wiv` sketches this (aspiration #4 closes `s-stg-gtu`); other strategic entries may be aspiration-shaped. Handled as follow-up actions after the aspiration kind lands. Each is a single-entry judgment call, naturally dialogued.
- **Additional kind proposals.** The settled set is 5 signal kinds + 5 decision kinds. Future additions go through the normal dialogue-before-capture loop.
