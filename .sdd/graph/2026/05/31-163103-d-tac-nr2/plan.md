# Implementation plan — principle-based ref-kind vocabulary (d-cpt-s9j)

## Goal

Ship the vocabulary redefinition decided in d-cpt-s9j: rename `grounds` → `grounded-in`, merge `evidence` into `grounded-in`, add `required-by`, generalize `addresses`, keep `surfaces`. Final capturable set (8): `grounded-in, builds-on, refines, addresses, surfaces, depends-on, required-by, related`. Define the set by **principle** across skill, framework-concepts, and the pre-flight rubric, and (pulled into scope) thread referenced-entry derived status into pre-flight so the LLM check can tell `grounded-in` / `builds-on` / `refines` apart.

The reference note attached to d-cpt-s9j (`ref-kinds.md`) is the canonical source text for every definition below.

## Surfaces map

### 1. Vocabulary + parse transform — `internal/model/ref.go`
- Add constants `RefKindGroundedIn = "grounded-in"`, `RefKindRequiredBy = "required-by"`.
- `RefKindValues()` (capturable set) becomes the eight: grounded-in, builds-on, refines, addresses, surfaces, depends-on, required-by, related.
- `grounds` and `evidence` become **parse-time aliases** → `grounded-in`, applied in `Ref.UnmarshalYAML` before kind validation, so `r.Kind` above the parser is always canonical. Same shape as the legacy bare-string → `RefKindUnknown` fallback.
- Drop them from the capturable set so new writes can't emit them; `IsCapturableRefKind` rejects them.
- Tests: alias parse (`grounds`/`evidence` → `grounded-in`), `required-by` round-trips, `MarshalYAML` emits canonical, old names absent from the capturable list.

### 2. Rendering — verify only (no code change expected)
- `internal/presenters/render_list.go:refVerb` and `internal/presenters/show.go:formatRelations` build the verb from `string(kind)`. After the parse alias, legacy `grounds`/`evidence` render as `grounded-in` automatically; `required-by` renders for new entries. AC verifies output; no verb-map edit needed.

### 3. Pre-flight rubric — `internal/llm/preflight_templates/`
- Full rewrite of `ref_meta_consistency.tmpl` to the principle-based definitions: why-the-pointer-exists framing, generalized `addresses`, `grounded-in` absorbing empirical citation, forward-class `surfaces`/`required-by` + no-back-ref, `related` as floor.
- Sweep the other templates that mention the vocabulary inline (`signal_capture`, `closing_decision`, `durability`, `closing_done`, `entry_quality`, `verdict`, `actor_capture`, `decision_refs`) for `grounds`/`evidence` tokens and the kind list.
- Preserve d-prc-2is calibration: defensible-but-sharper demoted from medium to low; body-keyed selection.

### 4. Target-status threading (the "include it" scope) — `internal/llm/preflight.go`
- In `assembleContext`, the referenced-entries block must carry each target's derived status (`graph.DerivedStatus(e)`), rendered as `active` / `closed-by <id>` / `superseded-by <id>`.
- **Constraint:** do NOT modify the shared `FormatEntryForPrompt` (`internal/llm/summary.go`) — it feeds summary-prompt hashing, and a change there would trigger summary regeneration across the graph (the hash-stability concern d-tac-4ub guarded). Append status in a pre-flight-local wrapper instead.
- Update `ref_meta_consistency.tmpl` to use the now-available target status to separate `grounded-in` (basis) / `builds-on` (closed or next-step) / `refines` (active, in-place).
- Tests: referenced-entry block includes derived status; summary-prompt hash unchanged (regression guard).

### 5. Skill source of truth — `internal/bundledskills/claude/`
- `sdd/references/framework-concepts.md` — rewrite the Ref kinds table + the `refines`/`builds-on` test to the principle set + direction/forward-class/capture-order/no-back-ref + completeness principle.
- `sdd/SKILL.md` — rewrite "Refs matter" to the new set + generalized `addresses`, consistent with d-prc-2is.
- `sdd/references/cli-reference.md` — `--refs` doc; check for an inline kind list.
- Sweep all bundled skills (catchup/explore/groom) for `grounds`/`evidence` ref-kind tokens.
- Rebuild (`go build -o bin/sdd ./cmd/sdd`), reinstall (`sdd init --scope project`); commit bundle source + re-stamped `.claude/skills/` together.

### 6. Integrity
- `sdd lint` clean on the existing graph (legacy refs parse to `grounded-in`, stay valid).
- No entry files rewritten.
- `sdd new --refs` help text auto-derives from `RefKindValues()` via `refKindList()` — verify it shows the eight without `grounds`/`evidence`.

## Suggested slice order
1. Model: constants + parse alias + `RefKindValues` + tests. (Render follows; verify.)
2. Pre-flight status threading + `ref_meta_consistency` rewrite + tests.
3. Remaining template sweep.
4. Skill rewrite (framework-concepts, SKILL.md, sweep) + rebuild + reinstall.
5. Full `go vet` / `go test` / `golangci-lint` / `sdd lint` pass.

## Carve-outs / coordination
- **d-tac-cpw (label-aware heat weighting)** — unbuilt; no weight table exists. This plan hands it the clean vocabulary (`grounded-in`, no separate `evidence`). No code here.
- **No migration command** — read-layer transform only, per d-cpt-s9j; history is never rewritten.
- **d-prc-2is** — active; rubric/skill rewrites stay consistent with its body-keyed selection, related-as-floor, and recalibrated severity.

## Alternatives considered
- *Rewrite history (migrate on-disk `grounds`/`evidence`)* — rejected by d-cpt-s9j; violates immutability and is unnecessary given the parse transform.
- *Add status to the shared `FormatEntryForPrompt`* — rejected: perturbs summary-prompt hashes and forces graph-wide summary regeneration. The pre-flight-local append avoids it.
