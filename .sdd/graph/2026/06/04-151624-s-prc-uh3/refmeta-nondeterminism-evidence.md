# Pre-flight ref-meta advisory — non-determinism on a terminal-`done` borderline

External reproduction, sdd v0.7.0. All entry IDs and domain nouns below are
generic placeholders; the original was observed in a downstream user's graph.

## The borderline shape

A ref whose **target is a terminal `done` signal that flagged a follow-up in
its body** legitimately reads as more than one kind:

- `builds-on` — "next step after a finished chain" (target is terminal).
- `addresses` — "acting on a concern the prior entry flagged."
- `grounded-in` — "cite the done as the empirical basis for the claim."

`framework-concepts.md → Ref kinds` gives no tie-break for this shape, so the
advisory's choice is unstable run to run.

Placeholders:
- `<T>` — a terminal `done` signal recording completed work, whose body named
  a sub-item as still unresolved and tracked separately.
- `<S1>` — a later `done` signal recording the now-completed follow-up item
  (Case 1 source).
- `<S2>` — a `gap` signal naming a related unresolved concern from the same
  work (Case 2 source).

## Case 1 — `<S1>` → `<T>`, body identical across all three runs

| Run | ref `kind` | ref `desc`            | Verdict                              |
|-----|-----------|------------------------|--------------------------------------|
| 1   | builds-on | A (same as run 2)      | `[high]` blocked — "use `addresses`" |
| 2   | addresses | A (same as run 1)      | `[high]` blocked — "use `builds-on`" |
| 3   | builds-on | A′ (reworded)          | `[low]` passed (entry created)       |

Runs 1 and 2 share body **and** `desc`; only the kind differs. Both blocked at
`[high]`, each recommending the kind the other run had just been rejected for.
Severity then moved on a `desc` reword alone (run 3, `[low]`, body unchanged).

## Case 2 — `<S2>` → `<T>` (same target)

| Run | ref `kind`   | Verdict                                |
|-----|--------------|----------------------------------------|
| 1   | builds-on    | `[high]` blocked — "use `grounded-in`" |
| 2   | grounded-in  | passed (entry created)                 |

Across one session the same terminal-`done` target `<T>` was assigned three
different recommended kinds — `addresses`, `builds-on`, `grounded-in` —
depending on run and source.

## Paraphrased validator output (domain nouns stripped, reasoning preserved)

- **Case 1, run 1 (`builds-on` → blocked):** "Target's derived status is
  terminal, so `builds-on` is applicable on status grounds — but the body
  frames this as *addressing* the follow-up the prior entry flagged.
  `addresses` names that relationship; `builds-on` implies extending a
  finished chain." → recommends `addresses`.
- **Case 1, run 2 (`addresses` → blocked):** "The target is a `done` signal
  (terminal), not a gap or decision — `addresses` is for acting on open
  gaps/decisions. Since the target is closed/terminal, `builds-on` is correct
  here." → recommends `builds-on`. (Directly reverses run 1.)
- **Case 1, run 3 (`builds-on`, reworded `desc` → passed):** same objection as
  run 1, now emitted as `[low] ref-kind-sharpness` rather than a `[high]`
  block. Entry created.
- **Case 2, run 1 (`builds-on` → blocked):** "`builds-on` on a terminal target
  is defensible, but the body reads as grounded in the prior entry as
  empirical evidence — `grounded-in` names it more precisely." → recommends a
  *third* kind, `grounded-in`, at `[high]`.

The mechanical checks (closed-set kind, dangling refs, canonical participants,
AC presence, language-drift) were deterministic across every run. Only the
semantic ref-meta advisory oscillated.

## Directions the reporter suggested (for maintainer dialogue — not decided)

1. Make the semantic ref-meta advisory **non-blocking** — cap below `[high]`,
   leaving the deterministic mechanical checks as the only blocking gate.
2. If it must block, **pin determinism** (temperature 0 / fixed seed) and/or
   require **N-run agreement** before raising `[high]`.
3. Add a **spec tie-break** for "source acts on a follow-up flagged inside a
   terminal `done`" — pick one canonical kind so validator and authors
   converge.

## Note on model choice

The reporter had switched their pre-flight model from Haiku 4.5 to Sonnet 4.6
shortly before observing this. The oscillation pattern predates the switch
(s-prc-vvd documents it in the Haiku era), so the model change at most alters
the variance's character (e.g. more confident high-severity blocks), not its
existence.
