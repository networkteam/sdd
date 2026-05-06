# Pre-flight findings — augment-plan directive capture (2026-05-06)

## Context

While implementing the type-system 7+7 plan (`d-tac-gvn`), an inconsistency surfaced between Plan 1's design body and one of its acceptance criteria over who ships the `topic(L)` filter primitive. The agreed resolution was an augmenting directive (per the augment-plan pattern, `d-prc-9ti`) refining Plan 1's AC contract without superseding the plan.

Two capture attempts were made; both were rejected by pre-flight with findings that contradict the augment-plan pattern's core mechanics. Capturing was eventually completed via `--skip-preflight`. The findings are recorded verbatim below for template-tuning.

## Attempt 1 — directive description named the augment-plan pattern explicitly and cited `d-prc-9ti`

CLI invocation outline:

```
sdd new d tac --kind directive --confidence high \
  --refs 20260506-151849-d-tac-gvn,20260506-151345-d-tac-uww \
  --participants Christopher,Claude \
  "<description claiming augment-plan pattern, citing d-prc-9ti by name in prose>"
```

Findings:

```
[high] missing-ref-on-augmenting-directive: The proposed directive (d-tac-ldp) references d-tac-gvn and d-tac-uww to clarify ownership of the `topic(L)` filter primitive, but does not reference d-prc-9ti (the augment-plan pattern it claims to follow). Per the entry's own statement — 'This is an augmenting directive per the augment-plan pattern (`d-prc-9ti`)' — the directive should explicitly ref d-prc-9ti to ground its use of that pattern.

[medium] scope-smuggling-across-plans: The proposed directive asserts a hard boundary ('Plan 1 ships the `topic(L)` filter primitive... Plan 2 does not re-implement') that narrows Plan 2's scope retroactively. Plan 2's AC list includes '`sdd list --topic <label>` filters via prefix-match on topic-path components, case-insensitive; reuses `topic(L)` filter primitive from Plan 2's shared internals' — which d-tac-ldp now reinterprets as Plan 2 consuming Plan 1's work rather than contributing its own. While the directive's reasoning is sound (avoiding duplication), this materially constrains Plan 2's implementation contract. A closing done signal on Plan 2 should explicitly confirm the reuse path, or the directive should be paired with an augmenting directive to Plan 2 settling the same boundary.

[medium] directive-shape-ambiguity: The directive reads as both a design clarification (settling architectural ownership) and a behavioral commitment ('Plan 1 ships X, Plan 2 consumes X'). The 'bounded choice' test for directive shape is unclear: is the commitment 'we have decided the architectural boundary' (bounded, done when both plans acknowledge it) or 'maintain this ownership boundary across implementations' (perpetual orientation)?

[low] opening-sentence-reference-lean: The opening sentence is reference-heavy and assumes familiarity with the inconsistency being resolved.
```

The high finding is itself the issue: SDD's own skill guidance is "Don't cite graph-mechanics rules in prose or refs ... contracts that govern all entries apply universally." The augment-plan pattern is exactly that — a universal pattern that all directive-on-plan refinements follow. Citing it in any individual entry adds noise; pre-flight asking us to ref it pulls in the wrong direction.

## Attempt 2 — meta-claim and `d-prc-9ti` reference both removed

Description rewritten to drop pattern naming entirely; just states what the directive does (resolves Plan 1's AC ambiguity in Plan 1's favor, scopes to Plan 1, names closure trigger).

Findings:

```
[high] missing-ref-to-directive-target: The proposed directive d-tac-z6t resolves an ambiguity within d-tac-gvn's design and ACs, but d-tac-gvn does not yet ref this directive. The directive's closure trigger requires Plan 1's done signal to list both d-tac-gvn and d-tac-z6t in `--closes`, which creates a forward reference dependency. For structural integrity, d-tac-gvn should be amended (via mechanical edit, per d-cpt-e1i) to add d-tac-z6t to its refs now, so the dependency is visible at plan-capture time rather than discovered at closure.

[high] scope-creep-in-plan-1: D-tac-gvn's AC list includes 'reuses `topic(L)` filter primitive from Plan 2's shared internals,' but the directive resolves this by placing the primitive in Plan 1 instead. The AC as written is now incompatible with the directive's resolution. Either the AC must be superseded/amended, or the directive's scope must narrow to acknowledge that Plan 1 *may* ship the primitive but Plan 2 retains the option to diverge. Current phrasing leaves the AC ambiguously binding.

[medium] closure-sequencing-ambiguity: D-tac-z6t's closure trigger requires Plan 1's done signal to close both d-tac-gvn and d-tac-z6t. This creates a temporal dependency: d-tac-z6t cannot close until Plan 1 is complete. However, d-tac-z6t is being captured now, before Plan 1's implementation. This is not structurally invalid (directives often precede execution), but the closure trigger should clarify whether d-tac-z6t remains open during Plan 1's work, or if there is an expectation of periodic progress signals addressing the directive's commitment.

[medium] plan-2-underconstrained: D-tac-uww (Plan 2) carries an AC that 'Pipeline primitives in shared internal packages, callable from other CLI commands (`sdd list --topic` consumes `topic(L)` filter).' The directive resolves Plan 1's internal inconsistency by mandating Plan 1 ships the primitive, but Plan 2's AC is worded as a passive observation of what happens, not an active commitment to consume or re-export it. If Plan 2 is expected to actively consume Plan 1's primitive, that should be explicit in Plan 2's ACs or in a companion directive clarifying the consumption boundary.

[low] phrasing-precision: The phrase 'other CLI commands' is slightly ambiguous.
```

## Why these findings argue against the augment-plan pattern

The augment-plan pattern (`d-prc-9ti`, shipped 2026-05-05 per `s-prc-a9n`) commits to:

- An augmenting directive refs the plan it refines — relationship is one-way
- Plan stays immutable (per `d-cpt-e1i`)
- Downstream surfacing handles visibility: `sdd show <plan-id> --downstream` shows augmenting directives
- Closing done signal on the plan addresses each AC and each augmenting directive's commitment, listing both in `--closes`

Pre-flight findings observed:

| Finding | Conflict with augment-plan pattern |
|---|---|
| `missing-ref-on-augmenting-directive` (attempt 1) | Asks for the meta-pattern to be cited as a ref. Skill explicitly says graph-mechanics rules should not be cited per-entry. |
| `missing-ref-to-directive-target` (attempt 2) | Asks for the **plan** to be amended to backward-ref the directive. Plan immutability + downstream traversal makes backward refs neither necessary nor allowed. |
| `scope-creep-in-plan-1` (attempt 2) | Asks for the AC to be superseded *or* the directive to weaken to "may ship." This is the exact case the augment-plan pattern exists to handle: refine an AC without superseding the plan. |
| `closure-sequencing-ambiguity` (attempt 2) | Treats the directive's open-during-implementation, closed-by-plan-done lifecycle as ambiguous. This is the standard augmenting-directive lifecycle. |
| `directive-shape-ambiguity` / `scope-smuggling-across-plans` / `plan-2-underconstrained` | Reframe a Plan-1-scoped clarification as needing to constrain Plan 2 as well. The directive narrative explicitly scopes to Plan 1. |

## Connection to existing pre-flight gap signals

This sits within `s-prc-vvd`'s broader observation that pre-flight oscillates on supersession-shape rubrics. The augment-plan blindspot is a specific instance: the templates apparently reach for "supersede or amend" as the resolution path for AC inconsistencies, missing the augment-plan pattern that explicitly avoids that move.

Worth considering as part of pre-flight template tuning:

- Recognize "augmenting directive" shape: a directive that refs an active plan and refines its AC contract without claiming to supersede.
- Don't ask for backward refs on the plan when an external entry refs it.
- Don't ask for graph-mechanics meta-pattern citations.
- Treat AC-refinement-via-directive as a valid resolution path alongside supersession.

## Resolution this session

Captured the directive with `--skip-preflight`. Implementation continues per the agreed sequence (Plan 1 → Plan 2).
