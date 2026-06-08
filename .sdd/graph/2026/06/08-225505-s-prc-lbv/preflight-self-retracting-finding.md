# Pre-flight failure mode: self-retracting `[high]` finding (ref-kind)

Observed 2026-06-08 in a separate external graph. Captured here as a process
gap — part of the collection of real pre-flight failure modes.

## Context

Creating a tactical directive that references an **open** `gap` signal with
ref-kind `addresses` (the directive is a scope answer to the gap). The
ref-kind precondition plainly holds: the framework reserves `addresses` for
responding to a gap/question/insight.

`sdd new` invocation (reduced to the relevant shape):

    sdd new d tac --kind directive --confidence medium \
      --refs '{"id":"<open-gap-id>","kind":"addresses","desc":"<scope answer to the gap>"}' \
      "<description>"

- Target ref: an open `kind: gap` signal, derived status `open`.
- Ref-kind used: `addresses`.

## Validator output (verbatim)

    [high] ref-kind-inapplicable: The ref to <gap-id> carries kind `addresses`,
    but <gap-id> is a `kind: gap` signal with `Derived status: open` —
    `addresses` is the correct kind for responding to an open gap. No finding
    on the kind itself. However, re-reading the vocabulary: `addresses` on an
    open gap is exactly correct. Retracting — this is not a finding.
    pre-flight validation blocked: 1 high-severity finding(s)
      ✕ pre-flight rejected entry

## Failure mode

- The finding **retracts itself** in its own text ("Retracting — this is not
  a finding") yet is still scored `[high]` and **blocks** creation.
- Severity score and reasoned conclusion **diverge within a single emission**:
  the check concludes textually "addresses on an open gap is exactly correct /
  no finding," but the severity label stays `[high]`.
- `addresses` on an **open** `gap` is correct per the framework — the finding
  should not have fired, or at minimum should not have blocked.
- Workaround that passed: `--skip-preflight` (entry created with
  `preflight: skipped`).

## Candidate directions (not yet chosen)

- Couple severity to the finding's textual conclusion — detect a
  self-retraction and downgrade it.
- Pull the mechanical rule "`addresses` on an open `gap` is always applicable"
  ahead of the LLM advisory, so applicable cases are never scored by it.
