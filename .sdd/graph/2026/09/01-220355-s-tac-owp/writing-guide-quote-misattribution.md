# Writing-guide quote misattribution — reproduction record (2026-08-31)

Live case from the capture of 20260831-200636-s-tac-eav (done closing 20260811-234304-s-tac-ual), driven through `sdd serve` on the rebuilt LLM boundary. Model: provider `ollama`, model `glm-5.3-flash:cloud`, variant `think=high`. The guide call recorded in `.sdd/stats/llm.jsonl` at 2026-08-31T18:03:07Z (in=3604, out=7907, 91.6s).

## The finding, verbatim

```json
{
  "axis": "form",
  "quote": "Capture-time LLM completions driven through the engine record no usage or cache metrics: across eight captures invoking the writing guide, pre-flight, and summarization, only embedding-side op counts moved, while completion calls left no trace in the stats file despite the sink being demonstrably active.",
  "reasoning": "The first sentence, read alone as the entry's summary, states the problem in the present tense — 'record no usage or cache metrics' — which is the opposite of what this done records; the act confirms completions 'now record'. The paragraph reproduces the closed gap's pre-rebuild findings, including a leftover checking count ('eight captures') that binds later readers to reconcile a figure that is not part of this act's claim, and all of it already sits in the gap one step away via the closure edge. Cut the paragraph so the done opens with its own act, which the following paragraph already states as a clean lead.",
  "repair": "cut",
  "severity": "substantive"
}
```

The quoted text does not occur in the draft. It is, verbatim, the first summary sentence of the closure target 20260811-234304-s-tac-ual. The reasoning treats it as the entry's opening sentence; the draft's actual opening states the opposite ("Live verification … confirms capture-time LLM completions now record …").

## The draft as submitted

Kind `done`, layer tactical, confidence high, closes `20260811-234304-s-tac-ual`, one ref (builds-on `20260831-195807-s-tac-tje`). Body:

> Live verification on a real engine session confirms capture-time LLM completions now record, closing the engine-recording gap (20260811-234304-s-tac-ual): Christopher reconnected the sdd MCP server on the rebuilt binary and a done capture was driven through it end to end, after which `.sdd/stats/llm.jsonl` holds attributed rows for all three chat purposes of that capture — writing-guide (the first such row ever recorded), preflight, and summarize — each carrying provider, model, and variant (ollama, glm-5.3-flash:cloud, think=high) with token counts and duration.
>
> The act is exactly the resolution the gap named — record completions on this path, then read the numbers — and it settles the gap's open why: the same capture flow on the pre-rebuild binary, minutes earlier, recorded preflight and summarize but no writing-guide row, matching the mechanism analysis in the boundary-rebuild done (20260831-195807-s-tac-tje), whose flagged follow-up this read-back completes. The gap's second candidate, whether the guide's prompt prefix falls under the provider cache floor, is now measurable from these rows but not yet measured — the verified run used Ollama, which reports no cache breakdown.

## How the guide's prompt renders this draft

`formatDraftForWritingGuide` (`internal/llmops/writingguide.go`) emits one block, placed under a single `## Draft entry` heading by `writing_guide.tmpl`; closure edges sit above the body, each with the target's `FirstSummarySentence()`:

```
Type: signal
Layer: tactical
Kind: done
Refs:
  - 20260831-195807-s-tac-tje (kind: builds-on): the boundary-rebuild done whose flagged live read-back this verification completes
Closure edges:
  - closes 20260811-234304-s-tac-ual (gap signal)
    Capture-time LLM completions driven through the engine record no usage or cache metrics: across eight captures invoking the writing guide, pre-flight, and summarization, only embedding-side op counts moved, while completion calls left no trace in the stats file despite the sink being demonstrably active.

Live verification on a real engine session confirms capture-time LLM completions now record, …
```

## Established mechanics vs. assumption

Established by code reading:

- The closure summary is inside the same `## Draft entry` block the model is told to judge; nothing structurally marks it as context (`internal/llmops/writingguide.go` `formatDraftForWritingGuide`; `writingguide_templates/writing_guide.tmpl` user block).
- The "one summary sentence" bound delivered a ~60-word paragraph: `FirstSummarySentence` (`internal/model/entry.go`) cuts at the first `". "`, and this target's summary is one colon-structured sentence.
- Quote provenance is unchecked: `parseWritingGuideResult` accepts any string as `quote`; the prompt's only guards are the one-line quoting rule ("the draft's words verbatim") and the stranding axis' note that closure-edge targets anchor mentions.

Assumption, not established: that the rendering shape (context above the body, paragraph-sized, unmarked) is what caused the misread. Model-specific weakness under this layout is a competing explanation; both are testable per model through the eval suite.

## Reproduction and verification path

The writing-guide eval suite (`internal/llmops/writingguide_eval_test.go`, `go test -tags=eval -run TestWritingGuideEval ./internal/llmops/...`) pins drafts and asserts on findings; `test_helpers_internal_test.go` builds entries and `runGuideEvalPassRate` measures per-model pass rates.

- Pinned case reproducing this shape: a done draft closing a target whose first summary sentence is long and states the pre-act problem in present tense; assert no finding's `quote` contains text that occurs only in the closure edge.
- Suite-wide provenance assertion: every finding's `quote` must occur verbatim in the draft's own rendered fields — mechanical, no LLM judgment, measurable per model, and a regression guard once a repair lands.
- Repair candidates the same case verifies: separate or relabel the closure-edge block so it cannot read as entry prose, move it below the body, or add a post-parse provenance check that drops or flags out-of-draft quotes.

Constraint a repair must keep: closure edges entered this prompt because the guide misreads correct retirement dones without them (20260811-183929-s-tac-fu8).
