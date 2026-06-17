# Capturing evaluation findings — the gap

The framework commits to evaluation but the skill never says how to *record* it. Two recent sessions hit different faces of the same hole.

## The model we already have

- `references/evaluation.md`: two lenses, **always both** — inner (sound on its own terms) and outer (works in use) — at three points (during implementation, at the landing gate, after landing). "Each finding worth keeping becomes a signal."
- Meta-process Evaluate mode: decision (what to evaluate against) → done (who reviewed) → signals (findings).
- `s-cpt-9qn`: a done signal is not really terminal. Evaluation is a further loop; "done done" confidence accumulates from downstream evaluation refs — a *reading convention over refs*, not a schema change. The graph becomes a map of validated vs. unvalidated work (`s-cpt-l2q`, "coverage evaluation").
- The evaluate move is sketched (`s-prc-fc0`: two anchor points, inner/outer axes, a lightweight-vs-heavy ceremony test) and committed for crystallization (`d-prc-iqw`, now unblocked since the engage refactor shipped). But it has **no written form** — no `playbook-evaluate.md`, no capture section.

## Face 1 — coverage: no marker, no relation

Multiple inner/outer evaluations of one change is the *intended* pattern, not redundancy. But nothing marks an entry as "an evaluation of <target> from <lens>", so an evaluation reads as a floating execution fact; and there is no convention to relate sibling reviews (an inner correctness review + an outer deploy/behavior review of the same change) so the coverage is navigable instead of two unlinked entries hanging off the plan.

## Face 2 — attestation vs. the durability gate

The artifact-durability check has three sanctioned paths — prior commit, entry attachment, upstream attachment (`s-prc-0ka`, closing the `d-prc-22i` design; restored in `s-prc-8nl`). It has no path for a **human-attested manual verification**: an outer-lens check ("confirmed X works in a fresh session") where nothing was committed because nothing changed in code. Pre-flight blocks it, correctly on its own terms.

Sharpening: the **entry-attachment path is an existing, non-lossy escape** — attach the verification record and a documented check passes today with full done/closure semantics. So the genuinely-blocked case is the *bare* attestation: a participant's word in the body, no artifact at all.

**Determination:** bare attestation should count as a durable trace **when it is a human-in-the-loop dialogue** — a participant explicitly confirming in the loop. The human-in-loop condition is the anti-loophole: an agent writing "I confirmed it works" on its own is exactly the bypass the gate was built to stop. This preserves the rigidity fix the three-path design delivered (`s-cpt-ix1` — validation rigidity that had forced `--skip-preflight` bypasses).

This wall is not new: in April a non-code knowledge-capture act was rejected the same way (`s-prc-ql7`), because the durability paths assume an artifact and acts that produce only an observation have no home.

## Face 3 — which kind carries a finding

No guidance tells an author whether an evaluation finding is a **fact** (behavioral observation), an **insight** (synthesis — but an insight implies something still to act on), or a **done** that `addresses` the evaluated commitment without closing it. The fact-signal route has been used ad-hoc — e.g. the two MCP experiment runs recorded as open facts refed to the experiment (`s-tac-9od`, `s-tac-o47`) — with no rule behind the choice.

## Why these are one gap

A manual smoke test *is* an outer-lens evaluation. So Face 2 is the outer half of Face 1 hitting the durability gate. Under it all: SDD treats a done as "work that produced a durable artifact," but evaluation is a legitimate completed act whose evidence is often an observation. The skill never names the capture pattern, so each session improvises.

## Open design questions (deferred to the evaluate-move work)

- **Marker vs. convention.** Does an evaluation get a first-class marker (kind/flag) that both tags it as an evaluation-from-a-lens and, gated by that marker, lets human-in-the-loop attestation satisfy durability — or does it stay a reading convention over refs?
- **Sibling relation.** How do inner/outer reviews of one change relate so coverage is navigable: `related` edges, an annotation clustering the reviews, or a new ref kind / "evaluates" marker? (Weigh against the standing caution that every new ref kind is another capture-time judgment.)
- **Durability-gate change.** Confirm the current closing-template behavior against the source, then decide whether the human-in-the-loop path is a template rule or needs a structural marker to avoid re-opening the bypass.
- **Surface.** `playbook-evaluate.md`, an `evaluation.md` capture section, the durability template — and whether this extends `d-prc-iqw`'s evaluate disposition or spawns its own plan.
