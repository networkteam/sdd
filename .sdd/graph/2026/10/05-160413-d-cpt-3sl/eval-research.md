> **Dialogue outcome, 2026-10-04.** This report fed the decision it is attached to. The dialogue changed it in these places:
> - Quote quality is a judge question (does the quote identify exactly one passage in the draft), not the byte-exact fidelity layer of §2.1 and §4. Quotes need not match the draft.
> - Evaluation is a built-in `sdd` command run against a matrix config of provider/model/params combinations, with keys from the normal config, not test code (§5, open question 5).
> - The judge is a provider abstraction answering with probabilities; locally run classifier models answer the data-handling concern of §3 (open question 1).
> - Results land as a done signal with the results attached (open question 2).
> - Structured output and the treatment of non-enforcing providers were decided separately (20261004-134853-d-cpt-sw8, 20261004-135748-d-tac-ndh).

# Evaluating SDD's LLM operations: research report

Scope: pre-flight, writing guide, summarize (`internal/llmops/`). Research only. No live LLM, Jev, or paid API calls were made, so every Jev property below is a documented or vendor-measured claim, not verified on SDD material.

## 0. The answer in brief

- Measure three layers separately, each with the instrument that fits it. **Format**: does the response parse and match the schema? This is mechanical and exact. **Fidelity**: do the string properties the contract fixes hold, such as a verbatim quote? Also mechanical and exact. **Judgment**: is the finding right? This takes typed questions to a judge that returns probabilities. Today's suite folds format and judgment into one pass/fail bit, has no fidelity layer, and asserts judgment by substring.
- Keep specimens and labels as data. Record every raw response. Write one result row per (specimen, run), keyed by the factors under test: provider/model/variant, prompt revision, response-structure mode. Compare runs paired, with intervals, and give one of three verdicts: regression, non-inferior, or inconclusive.
- Put the judge behind an interface shaped like Jev's questions: Noul, Choice or Score over a JSON state. Jev and an LLM judge both implement it. A labeled calibration set decides which one serves each question family.
- In this design #20 shows up in three places. The format-valid rate on a hostile-quote panel (fails today). A verbatim/substitution check, which would *still* fail after native structured output if the model keeps turning `“` into `"`: the JSON becomes valid and the quote becomes false. And judged quote relevance and finding validity.

## 1. What exists today

| Use | File | Assertions | Runs | Judge |
|---|---|---|---|---|
| Pre-flight | `internal/llmops/preflight_eval_test.go` | 37 tests. Assertions are predicates over parsed findings: `HasBlocking`, severity filters, and substring matches on category or observation (`mentionsSupersession` :768, `mentionsRefMeta` :832, `mentionsSettled` :1495) | 28 single-shot via `runEval`, where a parse error is `t.Fatalf`. 9 run as pass-rate via `runEvalPassRate`: `blockingTier` 3/3, `advisoryTier` 2/3, rescaled by `SDD_EVAL_RUNS` (:214–264) | none |
| Writing guide | `writingguide_eval_test.go` | `noFindings`, `hasAxis`. The axis set is closed (`guideAxes`, `writingguide.go`), so an exact match means something. PR #21's case checks parse only (`return nil`) | pass-rate tiers | none |
| Writing-guide sweep | `writingguide_sweep_eval_test.go` | none. It logs findings next to ground truth written as free text (`expect`, `nonFind`), and a human compares. Marked temporary | 3 per specimen | human |
| Summarize | `summarize_eval_test.go` | `checkSummaryMechanics` (empty, 25–140 words, self-ID, quotes, label, markdown) plus a judge verdict over 5 booleans | 1 per case, 3 cases | LLM on a fixed config: ollama `glm-5.3-flash:cloud`, `think=high`. Its output is parsed by first-`{`/last-`}` slice with one retry. `TestSummarizeJudgeCalibration` uses known-bad specimens only |
| Summary wild sweep | `summarize_wild_eval_test.go` | judge-only over stored summaries: deterministic stratified sample, 6 axes from 20260901-112834-d-cpt-75a, counts logged | 1 | same LLM |
| Usage | `eval_stats_test.go` | `CallStat` rows (purpose, identity, usage, duration, transport error) to JSONL, with a table in `TestMain` | — | — |
| Parser contract (non-eval) | `json_response_test.go` | scripted responses through the real `Preflight`/`WritingGuide`. PR #21 adds the typographic cases | — | — |

The runner comes from `SDD_EVAL_CONFIG`/`PROVIDER`/`MODEL`/`PARAMS` (`preflight_eval_test.go:80`). Evals go through the real operations and a production factory (`factory.New`), which matches the layer rule.

**Shortfalls for comparing configurations and prompt or structure changes:**

1. **Outcomes are not machine-readable.** The provider comparison matrix was assembled by hand (20260901-093840-s-tac-ahc, `model-eval-matrix.md`). The PR #10 evaluation needed test-only overlays that recorded requests and responses, plus Python analysis scripts. The bundle of 20260907-103331-s-tac-vsr holds `*.responses.jsonl`, `after-summary.json`, `analyze.py` and `make_report.py`. Its `SDD_EVAL_TRACE_FILE` does not exist in the repo. The recording and reporting layer was built once and thrown away.
2. **Prompt revision and response-structure mode are not part of a run's identity.** `Identity` carries provider/model/variant only (`pkg/llm/llm.go`). 20260907-103331-s-tac-vsr established "identical checker templates" between two commits by hand.
3. **Format failure and judgment failure share one bit.** `runEval` fails the case on a parse error, and the pass-rate loop counts it as a failed run. 20260907-103331-s-tac-vsr had to tally "plain JSON responses" separately (Haiku 0/64).
4. **There is no fidelity layer.** `parseWritingGuideResult` accepts any non-empty `quote`. 20260901-220355-s-tac-owp documents a quote taken from the closure edge rather than the draft and proposes a provenance assertion. #20 shows character substitution.
5. **Semantic expectations are asserted by substring over free text.** For example, `mentionsSupersession` matches "replaces". The guide's own comment says the free-form pre-flight categories "proved untrackable across runs" (`writingguide.go`).
6. **Sample sizes cannot separate configurations.** 34/37 against 35/37 is one case. 20260907-103331-s-tac-vsr: the runs "did not establish that the prompt edits caused the losses, but neither did they establish equivalent performance". Its historical baseline was not a paired run.
7. **The judge has gaps.**
   - It returns booleans with no uncertainty, and the judge itself sets the verdict.
   - Its own free-text JSON is the same failure class it judges.
   - Calibrating on bad specimens only lets a judge that fails everything pass.
   - Two judge prompts encode two contracts. `summarize_eval_test.go` carries the template's "50-100 words" (`summary_templates/summary.tmpl:6`). The wild judge follows 20260901-112834-d-cpt-75a, which forbids numeric targets. 20260901-182601-d-tac-yx6, still open, commits to reconciling them.
   - Self-preference risk: the glm-5.3-flash judge scored glm-5.3-flash candidates, and glm at `think=low` came out the best summarizer (20260901-093840-s-tac-ahc).
8. **Language coverage.** Every eval passes configured language `""`. The writing guide gets no language at all (20260901-100956-s-tac-83h). Two of the four JSON failures came from German-language graphs (20260827-224853-s-tac-giv, #20).
9. **Ground truth lives in code predicates or log strings**, not data that can be re-scored.

The four JSON recurrences, each patched where it was triggered:
- 20260606-125155-s-tac-sk1 (`record.md:44`): safe-quote instruction.
- 20260827-224853-s-tac-giv, then 20260828-154428-s-tac-j90: the `json_string_rules` partial.
- 20260906-231136-s-tac-ynt / GitHub #6: parser fix `d17b7a0c`; the prompt edits were reverted per 20260907-103331-s-tac-vsr.
- #20 / PR #21.

## 2. Recommended design

### 2.1 Measurement layers

| Layer | Questions | Instrument |
|---|---|---|
| L1 Format | Is the raw response valid JSON? Is it plain or recovered by `extractJSONObject`? Does it conform to the schema (parser accepts)? What error class if not? | mechanical, exact |
| L2 Fidelity | Each `quote` is a byte-exact substring of the draft's own fields. It comes from the draft, not from closure edges or ref descs. No code-point substitution, no truncation. Excerpts inside pre-flight `observation` are verbatim and delimited per `json_string_rules`. Enum values. Summary self-ID and the sentence ceiling | mechanical, exact, with a diagnostic classifier on failure |
| L3 Judgment | Expected concern raised. Each finding holds, is arguable, or is spurious. The quote is the text the reasoning discusses. The axis fits. The blocking label is matched. The summary axes per 20260901-112834-d-cpt-75a | typed questions to a judge, scored against labels |
| L4 Cost and latency | tokens, cost, duration | `CallStat` |

The rule for string comparison: it is the right instrument exactly when the contract *is* a string property (verbatim quote, enum, JSON validity). Topic matching such as "mentions supersession" is L3.

Report L3 over format-valid responses and always next to L1. A structured-output change that turns parse failures into valid-but-wrong answers then shows as L1 up and L3 down, instead of a single net number.

### 2.2 Specimens as data

Each use gets specimen files under testdata. A specimen holds the inputs (draft, graph fixture entries, closure targets, configured language) and the labels:

- **Pre-flight:** `expected_blocking`. `expected_concerns: [{id, description}]`. `forbidden_concerns`, for example "demands a commit for a human-attested verification", per 20260617-155740-d-prc-kqx.
- **Writing guide:** `expected_axes`, `clean`, forbidden concerns, and sweep ground truth (20260811-141038-d-tac-rvu).
- **Summarize:** `has_relationships` and the language. The axes are judge questions, not labels.

Tags: origin (synthetic, graph specimen, or captured incident), language, character class.

A **hostile-character panel** is a factor crossed with a few base drafts:
- German `„…“`, `«…»`, `‚…‘`
- English `“…”`
- ASCII `"` inside prose
- braces, a backslash or Windows path, a regex
- an excerpt containing a newline

Sources:
- the 37 pre-flight cases, with labels taken from their current predicates and comments
- the writing-guide cases and sweep specimens
- the summarize cases and the wild sample
- captured incidents: s-tac-sk1, s-tac-giv, s-tac-ynt, s-tac-owp, #20

Per 20260610-000139-s-prc-xr9 item 3, every captured leak becomes a pinned specimen before a rubric change ships. The specimen-set revision is a content hash.

External specimens must be anonymized (AGENTS.md "External material", 20260505-110748-d-prc-zb6). The #21 draft names a person and another project's product specifics.

### 2.3 Judge interface

Questions are typed and asked over a JSON state:
- **Noul** returns p(yes).
- **Choice** returns a probability per option.
- **Score** returns a probability per level.

That is Jev's shape (https://docs.typesafe.ai/api.md). TypeSafe also publishes an LLM-backed drop-in client for the same shape, with probability or discrete answer modes (github.com/typesafe-ai/system-one-adapter-python). That is precedent for one interface served by both kinds of judge.

```go
type Judge interface {
    Ask(ctx context.Context, state any, qs map[string]Question) (Answers, JudgeIdentity, error)
}
```

- **Questions are data**, kept in one reviewable place (https://docs.typesafe.ai/agent-skill.md: "Put the constants (questions and thresholds) in a single place"). The question-set revision is a hash.
- **Each state carries only the fields the question needs.** Large state with irrelevant detail costs accuracy (https://docs.typesafe.ai/model-jaggedness/jev-1.13.md #5). This helps LLM judges too.
- **Questions are atomic**: one per concern, phrased one way. Structural invariants do not hold, e.g. P(q)=0.72 and P(¬q)=0.47 on the same ticket (jaggedness #8).
- **Counting and composition happen in code** (jaggedness #2). The judge never emits a composed verdict.

### 2.4 How probabilities feed scoring

- Each question in each run yields p, oriented so that higher is better.
  - Noul: the value.
  - Choice: the mass on the passing option or options.
  - Score: the mass at or above the passing level.
- **Soft score** = p. **Hard label**: pass if p ≥ τ⁺, fail if p ≤ τ⁻; between the two it is *undecided* and goes to a human review queue. Undecided items are counted, never dropped.
- τ is set per question from the calibration set to reach a target precision, not from doc defaults. The docs agree that thresholds depend on the domain (https://docs.typesafe.ai/confidence.md).
- **Choice and Score `confidence`** only gates review. Metrics use the probabilities. The vendor says the same: "If you have a specific statistical algorithm in mind, you should probably be using probabilities instead of confidence" (agent-skill.md).
- The case verdict is composed in code: all L1/L2 invariants hold AND the required L3 items pass. Suite metrics are macro-averaged over specimens. Compare on soft scores, which have less variance than bits. Use hard labels for the readable table.

### 2.5 Judge calibration

- **A calibration set per question family**, with human labels for both directions. The existing calibration test has known-bad specimens only. Sources:
  - the real requests and responses in the 20260907-103331-s-tac-vsr bundle (4 models)
  - the 50 judged summaries of 20260901-133642-s-tac-p80, after human review
  - the s-tac-owp misattributed finding
  - the #20 raw responses
  - the sweep ground truths
- **Metrics**: accuracy at τ, AUROC, Brier score, reliability bins. Bins check the "calibrated" claim directly. Of items judged around 0.8, about 80% should be human-positive (https://docs.typesafe.ai/introduction/machine-learning-primer.md).
- **Admission.** What gets admitted for a family is a *pinned* judge identity plus a question revision. Pin `jev-1.13.0`, not `jev-latest`, because aliases move (https://docs.typesafe.ai/models.md). For an LLM judge, pin model and variant. Recalibrate when either changes. Comparisons are valid only within the same judge identity and question revision.
- **Run both judges on the calibration set.** Items where they disagree get human labels first.

### 2.6 Runs and statistics

- **Runs per specimen** must exceed today's 3. Set N so the CI half-width on each guarded metric falls below its margin.
- **Intervals:** a Wilson interval per specimen, and a cluster bootstrap for each suite metric (resample specimens, then runs).
- **Paired comparison**: same specimens, same judge, baseline and candidate interleaved in one session. 20260907-103331-s-tac-vsr notes its baseline was historical; caches and providers drift. The output is a CI on the mean per-specimen delta, plus the list of flipped specimens with links to their raw responses.
- **Zero-failure invariants** carry their sample size: 0 failures in n runs bounds the rate at about 3/n (95%).
- **Re-judging:** raw responses are stored, so new questions re-score old runs without new candidate calls. A baseline survives a judge change.

### 2.7 Recording and comparability

One JSONL row per (specimen, run) carries:

- run id, git commit and dirty flag, purpose
- candidate `Identity`
- structure mode (prompted JSON or native schema)
- **prompt revision**: a hash of the rendered `SystemPrompt+UserPrompt`, captured at the runner wrapper, so production code does not change. The suite revision is the hash over all of them.
- specimen id and set revision
- the raw response, the L1 result and error class, the L2 results with diagnostics
- judge identity, question revision, and per-question probabilities
- usage and duration

To join usage to the case, carry the case id in ctx. A `StatsSink` can read it: "ctx is the call's context, so a sink can read request-scoped facts" (`pkg/llm/stats.go`). No contract change is needed.

A manifest per result set records the factors and revisions. A report generator takes two result sets and produces the matrix and the s-tac-vsr tables. Durable record: attach the result set and report to the evaluation's done signal, as 20260901-093840-s-tac-ahc and 20260907-103331-s-tac-vsr do. The baseline is the attached set of the last accepted configuration.

### 2.8 Regression detection

| Metric class | Members | Verdict rule |
|---|---|---|
| Invariant | L1 validity, L2 fidelity | Any failure is a regression, reported with examples |
| Guarded | L3 detection recall on positive specimens; false-positive rate on clean specimens (the anti-find-something invariant, 20260415-112708-s-prc-dix); summary axes | Paired delta against margin δ: CI upper < −δ is a regression; CI lower > −δ is non-inferior; otherwise inconclusive, with the extra runs needed |
| Informative | cost, latency | reported only |

20260907-103331-s-tac-vsr was an inconclusive verdict without the name. The design makes that outcome explicit.

## 3. Jev against an LLM judge

### 3.1 Jev, as documented

- **Request.** `POST https://api.typesafe.ai/v1/systemone` with body `{state, model, questions}`.
  - `state` is a string, an object, or an array of text values.
  - `questions` is a map from your id to `{type, instructions, criteria}`. `instructions` and `criteria` accept objects, so data can sit next to the question.
  - Ids are not sent to the model, and paths into the state are named in backticks (https://docs.typesafe.ai/api.md, https://docs.typesafe.ai/concepts/state.md, https://docs.typesafe.ai/primitives.md).
- **Answers.**
  - Noul: `noul` in 0–1, with no `confidence`.
  - Choice: `choice`, `probabilities` summing to 1, `confidence`.
  - Score: `score` as an expectation across the levels, `legend`, `probabilities`, `confidence`.
  - The response's `model` field reports the versioned id (api.md).
- **Many questions in one call.** Questions are evaluated in parallel and in isolation, and answers are independent (https://docs.typesafe.ai/introduction.md). Batching left the answers unchanged and cost roughly a tenth (https://docs.typesafe.ai/cookbooks/parallel_questions.md).
- **Confidence.** It summarizes how concentrated the distribution is, about (n·p_max−1)/(n−1). It is not a correctness estimate (https://docs.typesafe.ai/confidence.md).
- **Calibration.** Jev is trained with "RLCD" for calibrated probabilities, and calibration "is measured across groups of predictions" (machine-learning-primer.md, https://docs.typesafe.ai/concepts/system-one.md). I found no published calibration metric in the docs.
- **Determinism.** Not claimed. Answers are "designed to return stable answers across repeated evaluations" (https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md). The vendor measured a mean per-question std of 0.0102 over 15 repeats, against LLMs that varied even at temperature 0. One answer spanned 0.43–0.53, across a 0.5 threshold (https://docs.typesafe.ai/cookbooks/consistency_noul_cookbook.md, sampled 2026-09-11).
- **Limits** (https://docs.typesafe.ai/models.md, api.md):
  - 64k tokens per request; 32k for state plus the longest question
  - Choice up to 255 options; Score 2–10 levels
  - 100K tokens/s and 40 requests/s, "adjusting dynamically"
  - text only
- **Language.** English is primary; other languages are handled less accurately (models.md).
- **Jagged edges** (model-jaggedness/jev-1.13.md):
  - literal reading
  - indirection hurts
  - unreliable counting and numbers
  - distractor state
  - no text generation, so there is no "problems" list to read
- **Price.** $0.042 per million input tokens; output tokens are free (models.md).
- **SDKs.** Python and JS only; HTTP from any language (https://docs.typesafe.ai/sdk.md). There is no Go client in the github.com/typesafe-ai repos.
- **Data handling.**
  - Not trained on requests (models.md).
  - The services are hosted in the United States (https://typesafe.ai/legal/privacy-policy).
  - DPA with Standard Contractual Clauses Module 2 under Irish law; retention "as long as necessary", with no period stated (https://typesafe.ai/legal/data-processing).
  - Zero data retention for enterprise customers only (https://docs.typesafe.ai/legal.md).

Your own observations all match the docs: JSON as the state, many questions per call, a probability per answer. One nuance: for Noul the value *is* the probability, and for Choice and Score the probabilities, not `confidence`, are the evaluation signal.

### 3.2 Comparison

| Aspect | Jev | LLM judge (existing pattern) |
|---|---|---|
| Fit | Atomic yes/no and choice questions over a structured state. SDD's judged items decompose this way (per finding, per expected concern, per summary axis) | Fits multi-hop questions and explanations |
| Probability output | Native, claimed calibrated. Soft scores and thresholds come straight from it | Verbalized probabilities (calibration unknown), or k samples (k× cost), or logprobs where a provider exposes them |
| Independence from candidates | A different model class from every candidate | Same-family self-preference risk (glm judging glm, 20260901-093840-s-tac-ahc) |
| Run-to-run noise | Low, by vendor measurement | Varies even at temperature 0, by the same vendor's comparison |
| Explanation | None | Natural-language reasons. Spot-checking verdicts relied on them ("verdicts spot-checked verbatim", matrix) |
| Non-English (German specimens) | Lower accuracy, per vendor | Depends on the model |
| Cost | Negligible | Per-token, or subscription-capped |
| Data | US-hosted, retention unspecified. Public-repo specimens are fine; external-project specimens fall under 20260921-181638-d-cpt-cnu reasoning | Under whatever terms the chosen provider has |
| Maintenance surface | One HTTP adapter we own (3 question types, 401/422/429/529, backoff). One v1 endpoint and model versions to track; a "migrating to v1" guide (https://docs.typesafe.ai/migrating-to-v1.md, linked from SKILL.md) implies a past break. Question wording and thresholds need re-tuning on each pinned version bump | The prompt rendering of typed questions, a schema per question set, probability elicitation. The providers are already tracked (gollm fork, claude CLI) |
| Dependency weight | No Go dependency (stdlib `net/http`). An external account and vendor continuity (young: jev-1.13, dynamic limits) | Nothing new |
| Reversibility | High behind the interface. Drop the adapter and the questions, labels and calibration data survive | — |

### 3.3 Recommendation

- **Build the interface and implement both judges.** Assign each question family on calibration evidence.
- **The expected default is Jev for atomic L3 questions.** It gives native probabilities, is independent of every candidate family, and costs almost nothing.
- **Keep the LLM judge** for three jobs: cross-checking calibration items, any family where Jev misses the bar (German semantic questions are the likely candidate), and on-demand explanations of failed items.
- **Neither judge composes a verdict.**

## 4. Issue #20 expressed in this design

**Specimens.** The #21 German done draft, anonymized, crossed with the hostile panel (`„…“`, `«…»`, `‚…‘`, an ASCII close, English `“…”`). The specimens run through both the writing guide and pre-flight, since both quote the draft. Language is configured to `de` where the operation takes it. Labels: no judgment expectation (the clean label is unknown), only invariants.

**L1, mechanical:**
- `json.Valid` on the trimmed raw response, giving plain, recovered or invalid.
- Diagnostic class `typographic-close-as-ascii`: an ASCII `"` in the raw response at a position whose surrounding text matches the draft where the draft has U+201C. This is #20's shape: "all five „ were kept, while every “ had become "".
- Expected today: about 1/6 valid on claude-sonnet-4-6 (#20).

**L2, mechanical, per finding:**
- **verbatim**: the quote's bytes are a substring of the draft's own fields.
- **substitution**: a match only after folding typographic quotes, recording the pairs (e.g. U+201C→U+0022). Do *not* fold for the gate itself. The TypeSafe citation cookbook folds curly quotes before matching (https://docs.typesafe.ai/cookbooks/citation_check.md), which would hide exactly this failure.
- **truncation**: the quote ends at a quotation mark the draft span continues past, has an unbalanced `„`/`“` pair the draft span balances, or ends in `…`/`...`.
- **provenance**: the quote is found only in closure edges or ref descs (20260901-220355-s-tac-owp).
- **fabricated**: no match anywhere.
- **pre-flight `observation`**: draft excerpts of at least k words must be verbatim and set off with `'…'` or backticks, the rule in `shared_templates/json_string_rules.tmpl`.

**L3, judged.** The state is
`{draft: {kind, body}, context: {closure_edges}, finding: {reasoning, axis, quote, repair, severity}, axis_definitions}`.
Questions:
- `quote_matches_reasoning` (Noul): "Does `finding.quote` contain the words of `draft.body` that `finding.reasoning` is about?"
- `quote_whole_unit` (Noul): "Does `finding.quote` stop where the phrase of `draft.body` it is taken from stops, rather than mid-phrase?"
- `finding_holds` (Choice): `holds` / `arguable` / `does_not_hold`, with the axis definition carried in structured instructions.
- `axis_fits` (Noul): "Does `finding.axis` name the problem `finding.reasoning` describes, per `axis_definitions`?"
- For labeled specimens, one Noul per `expected_concern`: "Does any entry of `findings` raise `expected_concern`?"

Because the draft is German and Jev's accuracy on German is lower, this family is a calibration candidate for the LLM judge.

**What the result shows.** Under native structured output, L1 should reach 100% by construction. The L2 substitution rate then tells whether the guarantee removed the failure or moved it: `„Code einlösen"` inside valid JSON, escaped as `\"`, is false to the draft.

A boundary-level echo conformance test (§5) separates "the transport or structured mode preserves characters" from "the model normalized them". Recorded #20 responses also become deterministic fixtures in the style of `json_response_test.go`.

## 5. Where it lives

The rule (20260819-152950-d-prc-h1m): a test lives at the layer that owns what it asserts, and only true externals are scripted.

- **`internal/llmops/*_eval_test.go`** (eval tag, `package llmops_test`): the per-operation suites. llmops owns the operations, prompts and parsed result types whose quality is asserted. One generic run loop replaces the duplicated `runEvalPassRate`/`runGuideEvalPassRate`.
- **`internal/llmops/testdata/eval/<purpose>/`**: specimens and labels. Next to it: `testdata/eval/judge/<family>/` for the calibration set and `testdata/eval/questions/` for the question sets.
- **`internal/llmeval`** (new): the harness. It holds the `Judge` interface, question and answer types, result rows, JSONL I/O, manifests, statistics, report rendering, and the LLM judge over `pkg/llm.Runner`. Its unit tests run over synthetic data, since the harness owns its statistics. Stats are eval-only, not SDD domain logic, so they do not belong in `internal/model`.
- **`internal/llmeval/jev`**: the HTTP Judge. Unit tests against an `httptest` server for the wire shape; Jev is a true external.
- **Fidelity checks**: if a post-parse provenance guard ships in production (a repair candidate in 20260901-220355-s-tac-owp), they become an exported pure function in `internal/llmops`, reused by the eval (AGENTS.md "Single path"). Otherwise they live in `internal/llmeval`.
- **The structured-output guarantee**, if it becomes a `pkg/llm` contract property (`Request` carries a schema):
  - a conformance suite in `pkg/sddtest` (package doc: "reusable conformance suites for SDD public ports"), beside `RunEmbedderTests`: an echo task over the hostile panel that asserts schema validity and a byte-exact round trip
  - run live from `internal/llm/gollm` and `internal/llm/claude` under an eval tag, following the `internal/llm/gollm/ollama_eval_test.go` pattern
  - the adapter owns the character-preservation assertion; llmops owns verbatim quoting by the model
- **The report**: a test entry point (`-tags=eval -run TestEvalReport`, given baseline and candidate dirs) keeps it out of the shipped binary. A shipped command is the other option if evaluation becomes user-facing (§6).
- **AGENTS.md "Commands"** gains the eval invocations; it already lists `TestPreflightEval`.

## 6. Open questions for your decision

1. Commit to Jev now, or let the calibration set choose per question family? Either way, which specimens may leave for a US-hosted service with unspecified retention, given that external-project specimens like #21's draft come from another project?
2. Where do baselines and result sets live: graph attachments (today's practice), committed testdata, or an untracked results dir with only the reports attached?
3. Run budget and gates: N per specimen, the margins δ, and which metrics are invariant, guarded or informative.
4. Who labels specimens, and how labels are versioned when a decision recalibrates them (e.g. 20260617-155740-d-prc-kqx changed what durability counts as blocking).
5. Should evaluation be user-facing, i.e. users judging their own model choices (20260901-093840-s-tac-ahc names the matrix a baseline "for sdd users")? That moves it from tests to a command, and users would lack Jev keys.
6. Prompt revision as part of the public `Identity`/`CallStat` (a `pkg/llm` contract change, 20260830-234501-d-cpt-q6n), or eval rows only?
7. Should L2 checks become production guards (drop or flag out-of-draft quotes)? If so, the eval measures the pre-guard rate and the guard's drop rate.
8. How the LLM judge elicits probabilities: verbalized, k samples, or logprobs where available. This becomes part of its identity.
9. Production stats stay blind to semantic failures (20260831-093815-s-tac-k7d). The eval rows make that moot for evals only. Should the operation's outcome reach `CallStat` too?
10. Retire the substring predicates in the pre-flight cases in favor of labeled concerns judged by Noul, case by case, or wholesale?
11. Reconcile the two summary judges onto 20260901-112834-d-cpt-75a now, as part of this work, or keep that in 20260901-182601-d-tac-yx6?
