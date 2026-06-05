# Ref-meta calibration — validation results

Validated against the pre-flight eval suite (`go test -tags=eval -run
TestPreflightEval_RefMeta`) on Sonnet (claude-cli provider, the new default),
the model the defect was reported on.

## Sonnet outcomes (key ref-meta cases)

| Case | Expectation | Result |
|---|---|---|
| WrongKind_High — `grounded-in` on an open gap | high | PASS — fired `ref-kind-inapplicable`: "a gap is something you address, not a premise you reason from → addresses" |
| DescContradicts_High — desc "extends" vs body "retires" | high | PASS — `desc-body-contradiction` + `ref-kind-inapplicable` |
| BuildsOnActiveSharpened_High — `builds-on` on an active target sharpened in place | high | PASS |
| BuildsOnTerminalDoneFollowup_NotHigh — the reported shape: `builds-on` on a terminal `done` that flagged a follow-up | not high | PASS — zero findings; no `addresses` recommendation |
| BuildsOnClosedTarget_NoFinding — `builds-on` on a closed plan | no ref-meta finding | PASS (after the detector fix below) |

Real precondition violations still block at high; applicable/defensible kinds no
longer do. The exact oscillation case from the report produces zero findings.

## Haiku flake — model noise, not over-correction

An early run on Haiku 4.5 failed WrongKind_High (Haiku did not fire high on
`grounded-in` over a gap). The same case passes reliably on Sonnet, with
identical rubric guidance — Haiku is simply less reliable on the ref-meta
judgment. This is itself evidence for the report's thesis: the advisory's
reliability is model-dependent, which is why the eval now defaults to (and was
validated on) the stronger model.

## Detector false-positive (fixed)

The eval helper `mentionsRefMeta` substring-matched "ref " / "kind" in finding
*observation* prose, so an unrelated `[low] ac-specificity` note whose text said
"each ref by a per-kind weight" tripped a "no ref-meta finding" assertion.
Retightened to key on the finding *category* (`ref-kind-*` / `desc-*`), removing
the false positive.
