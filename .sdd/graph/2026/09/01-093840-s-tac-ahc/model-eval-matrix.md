# Provider/model/variant evaluation — full matrix (2026-08-31/09-01)

Nine candidate identities ran the live eval suites (pre-flight calibration: 37 tests with 3-run
pass-rates on blocking/advisory tiers; writing guide: 3 tests incl. the anti-find-something
clean-draft invariant; summarize: 3 specimen cases judged by a fixed LLM judge). Per-call usage
recorded through the production Observed decorator into per-candidate JSONL
(provider/model/variant attribution). Judge: ollama `glm-5.3-flash:cloud` at `think=high` (subscription-covered),
independent of the candidate; verdicts spot-checked verbatim and substantive.

Harness: `SDD_EVAL_PROVIDER` / `SDD_EVAL_MODEL` / `SDD_EVAL_PARAMS` (variant) /
`SDD_EVAL_CONFIG` (explicit config path) / `SDD_EVAL_STATS_DIR` (usage sink) /
`SDD_EVAL_JUDGE_*`. Reliability read from eval results, not recorded rows (s-tac-k7d).

## Capability

| Candidate | Pre-flight (37) | Writing guide | Summarize (judged cases) |
|---|---|---|---|
| gpt-5.6-luna (default = medium) | 37/37 | 3/3 | 8/12 |
| claude-sonnet-5 effort=low | 37/37 | clean-draft flake | 1/3 |
| glm-5.3-flash:cloud think=high | 36/37 | 3/3 | 1/3 |
| glm-5.3-flash:cloud think=low | 36/37 | clean-draft flake (1 in 3) | 15/18 |
| gpt-5.6-luna reasoning_effort=low | 35/37 | 3/3 | 3/3 (n=3) |
| claude-sonnet-5 (default) | 35/37 | clean-draft flake | 2/3 |
| claude-sonnet-4-6 (baseline) | 34/37 | clean-draft flake | 2/3 |
| claude-haiku-4-5 | 29/37 | clean-draft flake | 1/3 |

Failure characters: haiku invents facts (e.g. "S3" never in source) and misses ref-meta cases
broadly; the baseline misses 3 pre-flight cases on its own tuned prompts; glm-low missed the
no-durable-artifact check once (under-detection) and fires a spurious minor finding on a clean
draft ~1 in 3; sonnet-5 at default effort is slow, not wrong. Summarize is a tight bar after the
sharpened judge contract (first sentence standalone, terse 50–100 words): every model wobbles by
cramming clauses into the lede — a prompt-tuning finding as much as a model ranking. No German
appeared in 18 judged glm-low summaries under the configured-language prompt.

## Timing (per purpose, P50/P90 seconds)

| Candidate | Pre-flight | Writing guide | Summarize | Suite wall |
|---|---|---|---|---|
| glm think=low | 1.4 / 3.6 | 4.1 / 4.6 | 1.5 / 1.6 | 2m25s |
| sonnet-5 effort=low | 2.1 / 5.3 | 6.8 / 7.6 | 2.3 / 2.5 | 3m38s |
| luna reasoning_effort=low | 2.1 / 4.9 | 5.0 / 7.8 | 2.1 / 2.2 | 3m20s |
| sonnet-4-6 | 2.2 / 9.9 | 11.8 / 17.5 | 3.2 / 3.6 | 6m0s |
| haiku-4-5 | 2.5 / 4.4 | 5.6 / 8.2 | 1.4 / 1.7 | 3m26s |
| luna (default) | 2.6 / 6.8 | 7.3 / 8.2 | 2.1 / 2.8 | 4m25s |
| glm think=high | 3.1 / 7.8 | 7.3 / 18.8 | 1.1 / 1.1 | 5m26s |
| sonnet-5 (default) | 10.5 / 24.6 | 26.9 / 45.2 | 2.7 / 2.8 | 16m7s |

glm at default effort (max) ran as a timing probe only: worst call 44s (the >200s from initial
use did not reproduce), and it burns ~1.2k output tokens on a trivial summary vs ~85 at think=low.

## Cost (full suite ≈ 67 candidate calls, cache-aware)

| Candidate | Suite cost | Pricing basis ($/MTok in/out) |
|---|---|---|
| glm (any effort) | $0 per-token | Ollama Cloud subscription (usage-capped; cap generous at this profile) |
| luna reasoning_effort=low | ~$0.07 | 0.20 / 1.20 |
| luna (default) | ~$0.10 | 0.20 / 1.20 |
| haiku-4-5 | ~$0.52 | 1 / 5 |
| sonnet-5 effort=low | ~$1.33 | 2 / 10 |
| sonnet-4-6 | ~$1.45 | 3 / 15 |
| sonnet-5 (default) | ~$2.08 | 2 / 10 (output-heavy: 85k thinking tokens) |

Accounting notes for future cost reads: Anthropic `input_tokens` EXCLUDES cache reads/writes
(separate fields, 1.25× write / 0.1× read pricing, ~5-min window); OpenAI `prompt_tokens`
INCLUDES `cached_tokens`, and cache WRITES are billed (1.25× on gpt-5.6) but never reported
per call — only reads appear in usage. Short eviction window: a 25-min idle gap lost the cache.
Raw IN columns are not cross-provider comparable without the cache columns.

External reconciliation: the OpenAI console for the run day reports 1,337,462 input tokens
with 787,233 cache reads, 536,306 cache writes, 13,923 uncached — the reads and the total match
the recorded per-call rows (two Luna suites plus smokes) to within rounding, validating the
harness numbers against the provider's billing view; the write split exists only billing-side. Console cache profile for the eval workload: 58.9%
hit rate, 1.47 cache reads per write — a conservative floor for production amortization, since
the eval's 37 distinct test prompts give far higher prefix diversity than a capture session
reusing one byte-stable prefix. The Anthropic console reconciles the same way: Sonnet 5 (42.2%
read ratio, 14.8× reads/write) and Haiku 4.5 (40.3%, 21.6×) match the recorded rows; the
Sonnet 4.6 row mixes in non-eval traffic and an aborted first run, so it does not read as an
eval figure. Anthropic amortizes markedly better than OpenAI here for a structural reason:
caches are model-scoped, not variant-scoped — the two Sonnet 5 effort variants ran back-to-back
and cross-read each other's cache. Effort variants of one model share a cache namespace.

## Reading guide for future model evaluations

Re-run per candidate: `SDD_EVAL_CONFIG=~/.config/sdd/config.yaml SDD_EVAL_PROVIDER=<p>
SDD_EVAL_MODEL=<m> SDD_EVAL_PARAMS=<k=v,...> SDD_EVAL_STATS_DIR=<dir> go test -tags=eval
-run 'TestPreflightEval|TestWritingGuideEval|TestSummarizeEval$' ./internal/llmops/ -v`.
Each run prints a per-identity usage table; the JSONL rows carry full attribution. Capability
comes from test pass/fail, reliability from the pass-rate tiers, cost from tokens × pricing.
