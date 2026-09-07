# Issue #6 prompt evaluation

The wording changes correct the output instructions, but these runs do not establish a reliability improvement. The fixed parser remains necessary. No production or PR source was changed during evaluation.

## Comparison and scope

- After: PR #10 commit `7d2dc2e1`. Before prompts: `d17b7a0c`. Both use the fixed parser.
- Historical full-suite baseline: `20260901-093840-s-tac-ahc`, dated 2026-09-01. Its `model-eval-matrix.md` attachment supplies the before totals and is included in the evidence bundle.
- Source comparison from `19c40958` to `d17b7a0c` found identical checker templates, embedded facts, and writing-guide fixtures. Pre-flight fixture changes only move the observation decorator import to `pkg/llm`; test inputs and assertions are unchanged. The baseline record names the same final gollm provider fixes.
- Each after suite ran 37 pre-flight cases and 3 writing-guide cases. Existing three-run checks were retained: 55 pre-flight calls and 9 writing-guide calls per model. Summarize and exploratory sweeps were excluded because their prompts were not changed.
- Full-suite before totals are historical, not a randomized paired experiment. The small brace panel is a fresh paired comparison, with identical user prompts verified from captured requests.
- API credentials came from the user-selected config through the existing harness. Keys are absent from this evidence bundle. Test-only Go overlays recorded requests and responses without changing the repository.

## Full-suite results

| Model | Historical pre-flight | After pre-flight | After writing guide | Clean draft runs passing |
|---|---:|---:|---:|---:|
| Anthropic claude-haiku-4-5 | 29/37 | 29/37 | 2/3 | 1/3 |
| Anthropic claude-sonnet-4-6 | 34/37 | 33/37 | 2/3 | 0/3 |
| Ollama glm-5.3-flash:cloud, think=low | 36/37 | 35/37 | 2/3 | 2/3 |
| OpenAI gpt-5.6-luna, reasoning_effort=low | 35/37 | 35/37 | 3/3 | 3/3 |

The historical writing-guide matrix already reports clean-draft failures for Haiku, Sonnet 4.6, and GLM low; Luna low passed all three cases. All models passed the stranding and conflation cases in this run. Every full-suite command exits nonzero because at least one behavioral assertion failed. These are not all-green evals.

## Output format

| Model | Plain JSON responses, after suite | Checker parse/schema failures |
|---|---:|---:|
| Anthropic claude-haiku-4-5 | 0/64 | 1 |
| Anthropic claude-sonnet-4-6 | 59/64 | 0 |
| Ollama glm-5.3-flash:cloud, think=low | 64/64 | 0 |
| OpenAI gpt-5.6-luna, reasoning_effort=low | 64/64 | 0 |

Haiku used surrounding Markdown or prose in all 64 responses. One response emitted `severity: "no-finding"`, which the existing severity validation correctly rejected. Sonnet used surrounding prose or fences in five responses; the parser recovered their JSON. GLM and Luna returned plain JSON throughout. Transport errors: zero recorded.

## Fresh before/after brace panel

Three inputs per model and prompt version: a pre-flight draft containing `Options{Zebra: true}`; a closing entry whose target contains that literal; and a writing-guide draft containing braces, ASCII quotes, and a Windows-style path. Assertions check checker parsing, not judgment accuracy. No model was instructed by the harness to add a preamble.

| Model | Before parsed | After parsed | Before plain JSON | After plain JSON |
|---|---:|---:|---:|---:|
| Anthropic claude-haiku-4-5 | 3/3 | 3/3 | 0/3 | 0/3 |
| Anthropic claude-sonnet-4-6 | 3/3 | 3/3 | 2/3 | 3/3 |
| Ollama glm-5.3-flash:cloud, think=low | 3/3 | 3/3 | 3/3 | 3/3 |
| OpenAI gpt-5.6-luna, reasoning_effort=low | 3/3 | 3/3 | 3/3 | 3/3 |

All 24 panel calls parsed successfully. Sonnet changed from one prose-wrapped response before to plain JSON after; one sample per input cannot establish a formatting benefit. Haiku remained noncompliant with the no-surrounding-text instruction on both versions. This panel did not reproduce the original bug with a live model; the deterministic regression tests reproduce that response shape.

## Follow-up on the additional settled-decision misses

The initial after suites missed `TestPreflightEval_Settled_Unjustified_Medium` on GLM and Sonnet. Only that case was checked again: three old-prompt calls, two new-prompt calls, reusing the initial new-prompt call as the third sample. This follow-up was selected after observing failures, so it is diagnostic rather than an unbiased rate estimate.

| Model | Old prompt | New prompt, including initial result | Interpretation |
|---|---:|---:|---|
| GLM low | 3/3 | 2/3 | The initial miss did not repeat; no persistent regression demonstrated. |
| Sonnet 4.6 | 0/3 | 0/3 | The miss also occurs with the old prompt. |

Initial failures remain in the full-suite totals. Follow-ups do not replace them.

## Failed full-suite cases

### Anthropic claude-haiku-4-5

- `TestPreflightEval_ActionClosesSignal_NoDurableArtifact`
- `TestPreflightEval_ActionVerification_HumanAttestation`
- `TestPreflightEval_AugmentingDirective_GenuineSupersessionFlagged`
- `TestPreflightEval_RefMeta_WrongKind_EvidenceMedium`
- `TestPreflightEval_RefMeta_BuildsOnActiveSharpened_EvidenceMedium`
- `TestPreflightEval_RefMeta_RelatedFloor_NoFinding`
- `TestPreflightEval_Settled_Justified_NoFinding`
- `TestPreflightEval_ClosingSignal_StatedWhy_NoBlocking`
- `TestWritingGuideEval_CleanDraftStaysClean`

### Anthropic claude-sonnet-4-6

- `TestPreflightEval_RealGraphHistory_SilentScopeOut`
- `TestPreflightEval_ActionClosesSignal_NoDurableArtifact`
- `TestPreflightEval_AugmentingDirective_GenuineSupersessionFlagged`
- `TestPreflightEval_Settled_Unjustified_Medium`
- `TestWritingGuideEval_CleanDraftStaysClean`

### Ollama glm-5.3-flash:cloud, think=low

- `TestPreflightEval_ActionClosesSignal_NoDurableArtifact`
- `TestPreflightEval_Settled_Unjustified_Medium`
- `TestWritingGuideEval_CleanDraftStaysClean`

### OpenAI gpt-5.6-luna, reasoning_effort=low

- `TestPreflightEval_RealGraphHistory_SilentScopeOut`
- `TestPreflightEval_RefMeta_BuildsOnActiveSharpened_EvidenceMedium`

## Run accounting and evidence

290 recorded calls: 256 in four after suites, 24 in the matched brace panel, and 10 targeted follow-ups. No full before suite was repeated. Every test stream was saved in full; requests, responses, and per-call usage were retained. Provider usage columns differ in cache accounting and should not be compared as billing totals.

| Model | Calls across all runs | Reported input tokens | Reported output tokens | Cache reads | Cache writes |
|---|---:|---:|---:|---:|---:|
| Anthropic claude-haiku-4-5 | 70 | 445,443 | 13,606 | 294,435 | 18,028 |
| Anthropic claude-sonnet-4-6 | 75 | 457,348 | 11,637 | 340,788 | 20,630 |
| Ollama glm-5.3-flash:cloud, think=low | 75 | 423,847 | 15,673 | 0 | 0 |
| OpenAI gpt-5.6-luna, reasoning_effort=low | 70 | 702,358 | 12,165 | 407,837 | 0 |

Artifacts: `*.log` contain complete test output; `*.responses.jsonl` contain test names, rendered requests, results, and transport errors; `*/stats/llm.jsonl` contain usage; `after-summary.json` contains machine-readable suite results; `manifest.json` identifies source versions and artifact hashes. Temporary Go overlays and their source are included for reproduction.

The compiled test executables used the existing eval harness, with `SDD_EVAL_CONFIG`, `SDD_EVAL_PROVIDER`, `SDD_EVAL_MODEL`, `SDD_EVAL_PARAMS`, and distinct `SDD_EVAL_STATS_DIR` / `SDD_EVAL_TRACE_FILE` per run. Suite selector: `^(TestPreflightEval|TestWritingGuideEval)`. Brace selector: `^TestIssue6PromptEval$`. Follow-up selector: `^TestPreflightEval_Settled_Unjustified_Medium$`.

## Assessment

The initial recommendation to retain the wording edits was withdrawn in dialogue: the observed full-suite totals were slightly worse, and no measured benefit offset the losses. The follow-ups do not establish that the new prompts are equally good. Following the confirmed direction, corrective commit 791cb44f reverted prompt-only commit 7d2dc2e1 while preserving parser fix d17b7a0c. The resulting llmops source matches d17b7a0c, and its tests pass. PR #10 remains unmerged. Future dialogue can revisit the wording weaknesses separately, using paired evaluations before restoring changes. The clean-draft false positives and settled-decision misses remain calibration concerns; this record does not claim they were fixed.
