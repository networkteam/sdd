# Plan: teach `decision_refs.tmpl` the augmenting-directive shape

## Context

The augment-plan pattern (`d-prc-9ti`, shipped 2026-05-05) added skill-side guidance for a third path between mechanical fixes and full plan supersession: a directive that refs an active plan and refines its AC contract without superseding it. The pattern is documented in `playbook-augment-plan.md` and folded into `playbook-implementation.md` (which now requires `sdd show <plan-id> --downstream` before implementation).

The pre-flight templates were not updated. The validator has no model of the pattern. Two capture attempts on a Plan 1 augmenting directive (resolving topic-filter ownership in `d-tac-gvn`) were blocked at high severity — recorded verbatim in `s-prc-vko`'s attachment. Capture went through with `--skip-preflight`.

This plan addresses the local frame deficit. The broader asymmetry (skill grows patterns; templates lag) stays open under `s-prc-vvd`.

## Dispatch analysis

`internal/llm/preflight.go:selectCheckType` routes by entry shape. An augmenting directive has:

- `Type: decision`
- `Kind: directive` (default)
- `Supersedes`: empty
- `Closes`: empty
- `Refs`: includes the active plan being refined

Walking the dispatch in order:

1. Not actor / role.
2. `len(entry.Supersedes) == 0` — skip supersedes.
3. Not a done signal — skip closing-done / short-loop.
4. Not a fact/insight closing — skip dissolution.
5. `len(entry.Closes) == 0` — skip closing-decision.
6. Decision, not aspiration — falls through to **`checkDecisionRefs`** → `decision_refs.tmpl`.

The template's checks: ref completeness, grounding, no scope smuggling, AC presence (plan-only, doesn't fire), directive shape (stg/cpt-only, doesn't fire). Plus injected partials: `unrelated_refs`, `contracts`, `entry_quality`, `language`, `verdict`. Plus the `OpenSignals` block.

None of these carry a positive frame for "directive refines an active plan's AC contract without superseding."

## Per-finding root-cause walk (from `s-prc-vko`'s attachment)

The two `[high]` blockers and surrounding `[medium]` findings stem from a single cause: the validator has no positive frame for the augmenting-directive shape, so the LLM constructs resolutions from its known repertoire (supersede, amend, backward-ref).

- `missing-ref-on-augmenting-directive` [high] (attempt 1) — Ref completeness check pulled `d-prc-9ti` as a logical dependency because the prose named it. The skill's "don't cite graph-mechanics rules" guidance lives in the skill, not the template.
- `missing-ref-to-directive-target` [high] (attempt 2) — Free extrapolation under verdict's "structural completeness" frame. The augment pattern is one-way by design (plan stays immutable, downstream traversal carries visibility); this is not in any template.
- `scope-creep-in-plan-1` [high] (attempt 2) — Real structural observation: the plan's AC text and the directive's resolution conflict. Without the augment frame, the only resolutions in the LLM's repertoire are supersede the AC or weaken the directive. AC-refinement-via-directive is invisible.
- `scope-smuggling-across-plans` [medium] — Augmenting directives commonly narrow scope of the refined plan in light of new context; the check has no exception.
- `directive-shape-ambiguity` [medium] — Bounded vs perpetual lens applied where it doesn't fit. Directive-shape check is gated to stg/cpt; this fires looser via verdict's general framing.
- `closure-sequencing-ambiguity` [medium] — Augmenting-directive lifecycle (open during plan implementation, closed by plan's done signal alongside the plan) is non-default and looks like a dangling commitment.
- `plan-2-underconstrained` [medium] — LLM inferred the directive should also constrain the unrelated Plan 2; augmenting-directive scope is one plan.

The high findings are the blockers; the mediums are the same frame deficit producing softer construction.

## Design notes for the template change

Two surfaces:

**`decision_refs.tmpl`** — primary. Add a section that names the augmenting-directive shape and the AC-refinement-via-directive resolution path. Adjust check 1 (ref completeness) and check 3 (no scope smuggling) so they don't fire on the augment shape. Concrete moves:

- Define the shape early in the template: an augmenting directive refs an active plan, has no `supersedes` or `closes` on the plan, and refines or clarifies one of the plan's acceptance criteria. The plan stays immutable; the directive joins the plan's implicit AC chain. Closing happens via the plan's done signal listing both the plan and the augmenting directive in `--closes`.
- Add a calibration line under ref completeness: graph-mechanics meta-patterns named in prose to identify the entry's shape are not refs — they are framework-level rules, not logical dependencies for this individual entry.
- Add a calibration line under no-scope-smuggling: an augmenting directive may narrow or sharpen the scope of the ref'd plan's ACs — this is the pattern's purpose. Tension between directive and plan AC is resolved by the directive refining the contract; the plan stays immutable. Don't propose plan amendment, AC supersession, or directive weakening as findings.
- Add an explicit no-finding line for the augmenting-directive lifecycle: the directive remains open during plan implementation and is closed by the plan's done signal (alongside the plan) via `--closes`. Don't flag this as closure-sequencing or dangling commitment.

**`verdict.tmpl`** — possibly. The "Dialogue context is grounding, not argument" section already discourages structural-integrity construction, but it's general. If post-change runs still produce `missing-ref-to-directive-target`-style extrapolations, sharpen the verdict-level guidance with an explicit example: don't construct backward-reference requirements (asking the ref'd entry to be amended to ref this entry); refs are one-way and immutable, downstream traversal makes backward-reference unnecessary.

Hold this surface unless tests show the verdict-level reflex still fires.

## Test approach

`internal/llm/preflight_eval_test.go` is the LLM-rubric test. Add fixtures:

- Positive augment — directive refs an active plan, refines an AC, no supersede/close. Expect: no `[high]` findings tied to the augment-plan pattern.
- Negative supersession masquerading as augment — directive refs an active plan but actually replaces a core AC commitment (claims the AC is wrong, not narrower). Expect: `[high]` finding suggesting supersession of the plan or the AC, since this is genuine supersede shape and shouldn't slip through.
- Reconstruction of the original blocked case — directive resolving topic-filter ownership across `d-tac-gvn` Plans 1 and 2. Expect: no `[high]` findings tied to the augment-plan pattern (mediums about cross-plan boundary clarity acceptable; the user can weigh them).

The reconstruction case is AC 7's evidence; without it we don't know the local fix landed on the actual blocker.

## Out of scope

- Symmetric pattern-shipping (every skill pattern ships with template-side awareness). This would address `s-prc-vvd`'s broader concern and possibly `s-prc-ljg`/`s-prc-7i7`'s phase-aware severity threads. Larger plan; logged.
- Phase-aware severity (advisory at capture, blocking at close). Orthogonal axis.
- Other check types that may carry similar frame deficits (e.g. `closing_decision.tmpl` if a done signal addresses augmenting directives oddly).

## Open questions

- Does the augment-aware recognition need to run on the proposed entry's own shape (refs-an-active-plan + no-supersede/close) or also on the ref'd plan's surfaceable AC list? The latter is heavier (template would need the plan's AC text injected) but gives sharper calibration on AC tension. Start with shape-only; reconsider if positive-case tests still produce `[high]` on real AC tension.
- The `verdict.tmpl` "Immutability as calibration context" section already suggests "use supersede as the remedy for ambiguity rather than expanding the blocking surface." This phrasing nudges toward supersede-or-nothing. Worth adjusting to acknowledge the augment path as a non-blocking resolution? Track during implementation.
