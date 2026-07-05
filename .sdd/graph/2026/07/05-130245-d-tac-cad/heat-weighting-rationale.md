# Target-class-aware heat weighting rationale

This plan treats weights as attention strength, not truth, importance, or correctness. A newer entry raises an older entry's heat when it refs it; the weight answers whether that ref means the target is being actively worked through, or only being cited as background.

The score contribution stays separable:

```text
contribution = refWeight(refKind, targetClass) * decay(refAge)
```

Decay remains the time horizon: how quickly attention fades. The weight remains semantic strength: how much this relationship should count as attention to the target.

## Why target class is the right first axis

A full `entry kind x ref kind` table would be more expressive, but it would also create a wide calibration surface. Every new entry kind or ref kind would require fresh judgment across the grid. The model already has a smaller target classifier for ref applicability: live decision, live signal, terminal done, and retired. Reusing that axis captures the main difference Christopher raised — the same ref kind can mean different attention depending on what receives it — while keeping the first implementation understandable and reversible.

## Initial weights

| Ref kind | live decision | live signal | terminal done | retired |
| --- | ---: | ---: | ---: | ---: |
| `grounded-in` | 0.25 | 0.5 | 0.5 | 0.25 |
| `related` | 0.5 | 0.5 | 0.5 | 0.5 |
| `builds-on` | 0.75 | 0.75 | 1.0 | 0.75 |
| `refines` | 1.0 | 0.75 | n/a | n/a |
| `addresses` | 1.0 | 1.0 | n/a | 0.75 |
| `depends-on` | 1.0 | 1.0 | 0.75 | 0.75 |
| `required-by` | 1.0 | 1.0 | 1.0 | 0.75 |
| `surfaces` | 1.0 | 1.0 | 1.0 | 0.75 |
| `surfaced-by` | 1.0 | 1.0 | 1.0 | 0.75 |
| `unknown` | 1.0 | 1.0 | 1.0 | 1.0 |

`grounded-in` is lightest for live decisions because reasoning from a directive, plan, contract, or aspiration is often background alignment rather than action on that target. It counts more for live signals and terminal done signals because facts, insights, and done outcomes often carry empirical weight. It is damped again for retired targets because old closed or superseded material should remain findable without dominating current attention.

`related` stays medium everywhere. It means genuine nearby context, but deliberately less than a sharper relationship.

`addresses`, `refines`, `depends-on`, `required-by`, `surfaces`, and `surfaced-by` are full-strength for live work because they normally mean the target is being acted on, sharpened, blocked by, needed by, or producing follow-up.

`builds-on` is full-strength for terminal done because that is its home case: later work continues a finished chain. It is slightly lower elsewhere because it can also carry a softer next-step reading.

Retired targets are generally damped. They should remain discoverable as historical basis, but should not dominate current catch-up unless a fresh active entry carries the new work.

`unknown` remains full-strength for legacy parity. Legacy bare refs lack semantic weight data, so damping them would change old rankings without evidence.

## Coldness

Coldness is heat's inverse as a catch-up question, but not a mirror formula. Heat asks what is receiving attention. Coldness asks what has not yet been acted on.

The weighted version should therefore use the same relationship weights in the denominator:

```text
coldness = decay(entryAge) / (1 + weightedInDegree)
```

A weak background citation should demote an open loop less than a full `addresses`, `refines`, or dependency edge. This keeps coldness aligned with the same attention semantics as heat while preserving its purpose: finding commitments that have not really been worked through yet.

## Evaluation expectation

The closing done should compare current and weighted rankings on the live graph. It should include old versus new `heat` top lists, old versus new `coldness` open-loop lists, sensitivity for the most uncertain buckets (`grounded-in`, `related`, and retired-target damping), and notes on any surprising movement. The findings should decide whether the starting weights hold or need adjustment before closure.