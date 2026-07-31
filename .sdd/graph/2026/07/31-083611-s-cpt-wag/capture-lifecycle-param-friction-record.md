# Capture's start-only lifecycle fields — investigation record

Dialogue between Christopher and Claude, 2026-07-31, triggered by an external bug
report from Benno. All code claims verified against HEAD (b265a885), not against
the reporter's version (v0.16.2, 254 commits behind).

## 1. The external report (verbatim, as received)

> Bug report: sdd lint misses lifecycle edges asserted in prose
>
> Version: sdd 0.16.2 (macOS, darwin/arm64)
> Severity: low-medium — silent, and it corrupts derived status rather than failing loudly
>
> **Summary**
>
> An entry whose body and summary state that it closes another entry, but whose
> frontmatter carries no `closes:` key, passes `sdd lint` without a word. The
> target keeps `status: open` forever while the graph's own prose says it is
> resolved.
>
> **How it happened**
>
> During a capture of a done signal, the `closes` start parameter was not passed
> to `start_procedure`. The resulting entry `20260730-134536-s-tac-ak2` was
> written with:
>
> - Body, first paragraph: „… und nach grüner Pipeline nach main gemergt.
>   Schließt den Gap 20260730-105911-s-tac-khc."
> - Generated summary: „Der Fix schließt den Gap (20260730-105911-s-tac-khc) und
>   korrigiert dessen falsche Beschreibung …"
> - Frontmatter: `refs:` contained the target with `kind: addresses` — but no
>   `closes:` key.
>
> The pre-flight gate accepted it, the summary generator happily reproduced the
> claim, and `sdd lint` reported:
>
>     No issues found.
>     Index:
>       fingerprint: ollama/qwen3-embedding:8b
>       entries indexed: 190
>       no fingerprint drift
>
> The gap remained `status: open`. It only surfaced because someone re-read the
> target with `show` and noticed the mismatch.
>
> **Reproduction**
>
> 1. `sdd new s tac --kind done --refs '{"id":"<some-open-gap>","kind":"addresses"}' "Fixes X. Closes <some-open-gap>."` — note: no `--closes`
> 2. `sdd lint` → No issues found
> 3. `sdd show <some-open-gap>` → still `status: open`
>
> **Why it matters**
>
> Derived status is the whole point of an append-only graph: nothing is edited,
> everything is inferred from edges. A missing edge is therefore invisible by
> construction — no field is wrong, one is simply absent. The failure mode is
> that resolved work keeps showing up in catch-up lanes and open-loop views,
> which quietly erodes trust in exactly the surfaces meant to answer "what's
> still open?".
>
> It is also self-inflicted-error-prone in the MCP path: `closes` and
> `supersedes` are start parameters of the capture move, while everything else
> the author supplies is a report field. An author who reaches the assemble step
> without having set them has no way to add them there — and the step's own
> instruction text reads
>
>     every ID in refs, supersedes, and closes must have been served to this session in full
>
> which lists all three together as if they were equally available at that point.
> That wording nudges toward looking for `closes` in the report schema, where it
> correctly is not.
>
> **Suggested fixes**
>
> 1. Lint rule: flag an entry whose body or summary asserts a closing/superseding
>    relationship to an ID that is present in `refs` but absent from
>    `closes`/`supersedes`. A cheap first cut: entry text matches a close-verb
>    near an entry ID (locale-aware — closes, schließt, supersedes, löst … ab)
>    and that ID is not in a lifecycle field.
> 2. Pre-flight: the same check at write time is better than at lint time — it
>    catches it while the author is still in the dialogue and can restart the
>    capture with the parameter set.
> 3. Wording: in the assemble step, separate the lifecycle fields from refs and
>    say plainly that `closes`/`supersedes` are fixed at move start and cannot be
>    added at this step.
>
> **Non-issue, for the record**
>
> `closes` itself works exactly as designed. Passing
> `params: {"kind": "done", "closes": ["<id>"]}` to `start_procedure` is accepted
> and the assemble step then states:
>
>     This capture retires 20260730-105911-s-tac-khc. The body must state the retirement rationale, not just point at the target.
>
> An earlier suspicion that 0.16.2 had dropped the field from the capture path
> was wrong.

## 2. Verification of the reported facts (all confirmed at HEAD)

**`sdd lint` has no prose-vs-frontmatter check.** `internal/finders/lint.go`
runs exactly three things: `graph.Lint()` (collecting warnings produced at graph
construction), `validateCrossRepoDeps`, and `validateSummaries`. Every
construction-time validation in `internal/model/graph.go:780-1500` compares
*fields against fields* — dangling refs, malformed IDs, close-matrix legality,
type mismatch, kind presence, topic shape, involvement resolution. Nothing in
the codebase reads body prose against frontmatter. This is not a missing rule;
it is an absent category.

**Pre-flight cannot reach the case.** `selectCheckType`
(`internal/llm/preflight.go:190`) routes on structure. Every lifecycle-aware
rubric — `closing_done`, `short_loop`, `dissolution`, `closing_decision`,
`unusual_close` — is gated behind `len(entry.Closes) > 0`. A done signal without
`closes` falls through to `signal_capture.tmpl`, which checks only type
correctness, layer appropriateness, and confidence honesty. The check that would
catch a missing `closes` is gated behind the field being present.

**A done signal with refs but no closes is deliberately legal.**
`internal/model/graph.go:1314` requires "at least one closes **or** refs (target
of the completion claim)". Two valid shapes exist, and only the prose
distinguishes them — so the frontmatter is not "missing a required field".

**The wording complaint is accurate.** `d-prc-cap` assemble unit: "every ID in
`refs`, `supersedes`, and `closes` must have been served to this session in
full" — while the same step's `collect` list holds `refs` and no lifecycle
field.

## 3. The mechanism: why `closes` cannot be added at assemble

Two engine rules, both real:

- `internal/engine/spec.go:375` — a step's `collect` entry must name a field
  declared under `state:`. Naming a param fails spec validation at load.
- `internal/engine/store.go:166` — `setParams` is reachable only from
  `SetStart`. Params are start-only by construction.

**But the constraint is on params, not on `closes`.** `SetStart`
(`store.go:91-103`) splits its inputs: declared params → params, declared state →
seed. Capture already uses this: `captureBranch` is declared under `state:` with
the description "explicitly seeded by the dispatching workflow" — set at start
*and* resident in state.

So moving `closes` from `params:` to `state:` in `d-prc-cap` would:

- keep every existing start-time call working byte-identically
  (`start_procedure` with `closes` still seeds it), and
- additionally make it collectible at assemble.

One declaration line. Not an engine limitation.

Downstream effects checked:

- The instruction unit `{{if .closes}}This capture retires…{{end}}` renders per
  step, so the retirement-rationale instruction would appear on the re-serve
  after collection rather than at start. The author declares the closure, then
  is told to write the rationale. The `assemble → playback → adjust → assemble`
  loop already exists for exactly this shape.
- Pre-flight routing reads the *written entry's* `Closes` at the write step, not
  the start param, so `selectCheckType` is unaffected.

## 4. The friction hypothesis

The observed capture was a done signal. The closure *was* the substance being
recorded — it surfaced while drafting the body, not before starting the move.

At assemble, the only field open for that target ID was `refs`. And `addresses`
is explicitly sanctioned there: `d-prc-cap` defines it as "responding to a
gap/question/insight". So the author picked a legitimate kind from the only
field it was permitted to write, and the body said "schließt" because that is
what happened.

The step's own instruction then named `closes` as if it were in scope, pointing
at a field the schema does not offer there.

The only escape is abandon-and-restart. Note the irony: a restart *could*
re-seed `body`, `refs`, `topics` as state (`SetStart` permits it), so the draft
need not be lost — but nothing documents this, and no author would guess it.

Conclusion: this is not "the author forgot a parameter". The procedure's field
availability shaped a false entry, and the author produced the best-conforming
thing available under the constraint.

Status of this hypothesis: **reasoned, not validated.** We are inferring the
author's reason from an external report without the session log. The mechanism
claims in §2 and §3 are code-verified; the causal story is not.

## 5. Prior art in the graph

- **`s-prc-g0j`** (closed by `s-tac-kdd`) is the same class of defect: capture's
  assemble step collected no `involvement`, so a focus decision could not be
  written at all. It was fixed by adding the fields to `collect` — direct
  precedent for the fix shape here. The difference in failure mode matters: g0j
  *wedged loudly* at the write gate, this one *succeeds quietly* with a
  degraded entry. Silent conformance is the worse failure.
- **`s-prc-ema`** established the body↔edge consistency rule: every ID typed in
  the body is one of its edges, every edge visible in the narrative. The
  reported entry **passed** this rule — the ID was in `refs`. The rule governs
  edge *presence*; this failure is edge *kind*.
- **`d-tac-b7f`** committed to mechanical enforcement of that rule in pre-flight
  (extract body IDs, diff against refs ∪ closes ∪ supersedes, LLM verdict on
  each dangling mention). **`s-tac-dxh`** (open) records that it was never
  built. Verified at HEAD: `internal/finders/preflight_mechanical.go` runs
  ref-kind, applicability, cross-repo, supersede-fork, participant, actor, role,
  and procedure checks — no body-ID extraction. (dxh cites the file under
  `internal/llm/`; it has since moved package, the substance is unchanged.)
  **Critically: even fully built, b7f would not catch this case** — it diffs
  against the union of all edges, and the ID was in `refs`. This gap sits one
  notch finer than the enforcement that was already promised.
- **`s-cpt-hcp`** frames the procedure authoring model — state declarations,
  collect semantics, expression routing, projection — as one connected problem
  at its v1 ceiling. The param/state split is part of that authoring contract,
  so a redesign should absorb this rather than have it patched twice.

## 6. Options weighed

**A. Move `closes`/`supersedes` from `params:` to `state:` and collect them at
assemble.** One declaration line, start-compatible, removes the dead end
entirely. Cost: weakens a discipline the current design enforces — that you know
you are retiring something *before* you draft, so the targets are read up front
and the body is written as retirement rationale from the first word. That
discipline is worth defending on its merits; it should not be enforced by an
accident of which YAML key the field landed under.

**B. Wording fix only** (the reporter's #3). Separate the lifecycle fields in the
assemble instruction and state plainly that they are fixed at start. Nearly
free, strictly correct, and removes the misleading nudge. Does not remove the
dead end — an author who discovers closure mid-draft still has to abandon and
restart, but at least now knows that.

**C. Pre-flight check for asserted-but-absent closure** (the reporter's #2).
Right instrument — "does the body assert something the frontmatter does not
carry" is a semantic judgment, which is what pre-flight is for. Unmentioned
cost: you cannot *route* on absence, so this must fold into the generic
fall-through checks (`signal_capture`, `decision_refs`) that every entry hits —
precisely the checks most sensitive to false-positive noise, and the surface
where `s-prc-dix` measured 35–57% false-positive rates historically.

**D. Regex lint rule** (the reporter's #1). Rejected in dialogue. A locale-aware
close-verb-near-an-ID matcher chases every natural-language phrasing in every
language a graph might use (this graph is already bilingual), false-positives on
"this does *not* close X" and "X was closed by Y", and fires only after the
entry is immutable — so the remedy is a follow-up entry, the ceremony the
framework exists to avoid.

**Tempting and wrong:** `sdd lint --fix` already patches files in place for
mechanical defaults (`internal/handlers/handler_lint_fix.go`), so autofixing a
missing `closes` will look available. A lifecycle edge is a semantic claim, not
a defaultable field.

No option was selected — this record captures the gap, not its resolution.
