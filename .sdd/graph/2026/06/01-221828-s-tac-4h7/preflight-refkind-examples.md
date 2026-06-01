# Pre-flight ref-kind over-eagerness — examples for calibration

Captured during one session (worktree + sync work, 2026-06-01). Four entries
were created; the ref-kind consistency check flagged three with `[medium]`
ref-kind-mismatch and one with a `[low]` ref-kind-precision note. In each case
the chosen kind was defensible per the strict vocabulary and the suggested kind
was weaker or wrong. Read each entry's full body via `sdd show <id>`.

## 1. s-prc-8bp — chose `builds-on` (target s-prc-tha, closed)
> [medium] ref-kind-mismatch: The ref to s-prc-tha is marked `builds-on`, but
> the target is closed (derived status shows it was closed by
> 20260411-233406-s-tac-p5x). Per the ref-kind vocabulary, a closed target
> should use `builds-on` only when extending it as a next step; here the
> proposed signal observes a new friction point that emerged *after* the prior
> gap was closed and addressed. The body describes this as a *new discovery*
> arising from implementation of the prior decision, which reads more like
> `grounded-in` (the prior gap's closure created the conditions that revealed
> this one) or `related` (parallel friction in the same workflow). `builds-on`
> implies the proposed entry is the next step in a continuing chain, but the
> relationship is actually causal discovery, not continuation.

## 2. s-tac-qun — chose `related` (target d-tac-hsu, closed plan)
> [medium] ref-kind-mismatch: The ref to d-tac-hsu carries kind `related`, but
> the body frames the relationship more specifically: the signal directly
> observes that d-tac-hsu's background-sync implementation creates one of the
> two compound root causes (mid-wrap-up history rewrite). The body says the
> sync 'can rewrite history on its cooldown between any two steps' of the
> conclude flow, making d-tac-hsu a prerequisite context the gap depends on
> understanding. `grounded-in` would more accurately name that the gap's
> diagnosis is founded on how d-tac-hsu's behavior interacts with the conclude
> flow.

## 3. d-tac-4ff — chose `related` (target s-prc-jpx, open insight)
> [medium] ref-kind-mismatch: The ref to s-prc-jpx uses `kind: related`, but
> the body frames it as a refinement that requires updating guidance in place —
> the directive's implementation will necessitate changes to s-prc-jpx's
> conflict-resolution approach. This is closer to a `refines` relationship (the
> directive sharpens the rebase-shaped guidance into a merge-shaped equivalent)
> or `depends-on` (the directive's mechanics require updates to s-prc-jpx to
> remain correct). `related` is defensible as a sibling concern but undersells
> the structural dependency.

## 4. d-tac-fla — chose `builds-on` (target d-tac-ar2, closed directive)
> [low] ref-kind-precision: The d-tac-ar2 ref uses `kind: builds-on`, but the
> relationship is reversal: the new directive undoes the prior stance rather
> than extending it. `kind: supersedes` would be more precise, though the
> current `builds-on` is defensible if read as 'the conditions enabling this
> reversal are built on the prior decision.' Consider clarifying in the
> description that this is an intentional reversal, not a continuation.

## Why each chosen kind was defensible
- **s-prc-8bp / builds-on**: the vocabulary defines `builds-on` as "target
  closed, or next step in time after it" — this is the next observation in the
  same directory-bound-session lineage. `grounded-in` is for standing structure
  (contract / aspiration / active directive), which a closed gap is not.
- **s-tac-qun / related**: the gap diagnoses a flaw *arising from* the sync
  infra; d-tac-hsu (a closed plan) is not standing structure, so `grounded-in`
  does not fit. No "caused-by" kind exists, so `related` is the honest catch-all.
- **d-tac-4ff / related**: the directive triggers a *future* update to
  s-prc-jpx; `refines` is scoped to augmenting an active directive's commitments
  in place (lifecycle-split), which does not apply, and `depends-on` is
  backwards (s-prc-jpx is not a prerequisite for the directive).
- **d-tac-fla / builds-on**: deliberate — the directive reverses only the
  worktrees-out-of-CLI part of d-tac-ar2 while keeping plain `--branch`, so a
  full `supersedes` would overstate it.
