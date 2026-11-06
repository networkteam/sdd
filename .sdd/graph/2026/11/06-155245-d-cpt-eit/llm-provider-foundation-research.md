# LLM provider foundation: research record (2026-11-01)

Researched with Christopher, Claude. This record updates the June library evaluation (the attachment of 20260612-120423-s-cpt-jzo). Library and SDK claims were checked in source at the versions named, in the libraries' `go.mod` files, and with probe modules. No live API calls were made.

## 1. Requirements, from what the code uses today

Call shape and request:
- One non-streaming call serves pre-flight, the writing guide and summarize (`internal/llmops`): a system prompt and one user prompt go in, text comes out. No tools, multi-turn, images or streaming.
- The system prompt is sent once, in the provider's system slot, and stays byte-stable. On Anthropic it carries `cache_control: {type: ephemeral}`.
- A JSON schema on the request is enforced natively: OpenAI strict `response_format`, Anthropic `output_config.format`, Ollama `format`. The schema bytes go out unchanged, because constrained decoding generates properties in schema order. On the schema path the prompt split and usage are kept, and a path without native support fails loudly (20261004-134853-d-cpt-sw8, 20261004-135748-d-tac-ndh).
- Configured parameters reach the provider as typed request fields: `reasoning_effort`, Ollama `think` (a boolean or a model-defined level string), Anthropic `output_config.effort`. Nothing is inferred from model names: `max_completion_tokens` by default, `temperature` only when configured. Provider-prefixed gateway names such as `openai/gpt-5-mini` work unchanged.
- Every path takes a base URL and a key: OpenAI-compatible endpoints with a bearer key, Anthropic with `x-api-key`, Ollama local without a key or Ollama Cloud with a bearer key (20260921-181638-d-cpt-cnu).

Result:
- Usage is reported, never reconstructed: input, output, cache read, cache write. Anthropic `cache_read_input_tokens` / `cache_creation_input_tokens`, OpenAI `prompt_tokens_details.cached_tokens`, Ollama `prompt_eval_count` / `eval_count` / `prompt_eval_cached_count`. Cost is included where a provider or gateway reports it, for example in a response header.
- Identity carries provider, model and variant (the canonical parameter string).
- Ollama reasoning stays in `message.thinking`, apart from the answer (20261005-172136-s-tac-c8r).

Runtime:
- Safe for concurrent calls through one client. Batch summarize runs calls in parallel, and a long-running host serves several projects through one runner.
- Honours the context deadline (`llm.Bounded`). No rate limiting of its own (`llm.RateLimited`). Logs through slog from the context and never writes to stderr.
- Retries transient failures (408, 409, 429, 5xx, 529, network), honours `retry-after-ms` and `Retry-After`, stops fast on permanent ones (`insufficient_quota`, other 4xx), with configurable attempts and delays.
- Typed errors carry the HTTP status, the provider's error type and `Retry-After`. Retry can then be a caller-side decorator, as the `Runner` contract states ("no internal retries"), and a failure can be classified by who must act before a retry succeeds (20260829-113416-d-cpt-nut).

Evaluation (20261005-160413-d-cpt-3sl, 20261031-174216-d-tac-p83):
- System One: `POST {base}/v1/systemone` with `{model, state, questions}`. Questions are typed `noul`, `choice` or `score`, answered with probabilities, plus usage. Served by TypeSafe directly and by Opper at base `https://api.opper.ai/v3/compat`. Errors 400/401/402/403/413/422/429/502/503/504/529; 429 carries `Retry-After`.
- OpenAI Decisions: `POST /v1/decisions` on OpenAI (GA, model `gpt-6-luna`), and `POST {base}/decisions` on Opper. Questions are typed `predicate`, `choice` or `score`, answered with probabilities, plus usage. A declined question returns a `refusal` answer.
- Both use the same transport needs as chat: base URL, bearer key, retry, typed errors, usage, observation.

Tests and constraints:
- Wire-testable against `httptest` through base-URL injection, as `internal/llm/gollm/wire_test.go` does today.
- The dependency-hygiene rule of 20260601-153050-d-cpt-n3y: no heavyweight transitive trees (aws-sdk-go-v2, google.golang.org/api, cloud.google.com, grpc, genproto, OpenTelemetry) in the module graph.
- The Claude CLI adapter stays separate. Embeddings are already hand-rolled over `net/http` (`internal/llm/embed`, OpenAI-compatible `/v1/embeddings` and Ollama `/api/embed`), without retry.

## 2. The gollm fork since June

21 commits, about 2,000 changed lines, all fitting the fork to SDD:
- 2026-06: provider-aware retry with backoff; `GenerateWithUsage`; `max_completion_tokens` for gpt-5.
- 2026-08: Ollama Cloud bearer auth; Ollama token counts; a doubled user prompt in `Prompt.String()`; no default `temperature` for reasoning models; OpenAI `cached_tokens`; Anthropic `effort` mapped to `output_config.effort`.
- 2026-09: configurable OpenAI endpoint (an outside contribution), with its validation fixes.
- 2026-10-31 (for 20261031-141038-d-tac-p8l): configurable Anthropic base URL; the system prompt sent once, as the call's own (it was sent twice, and a logic race on shared client options could send another call's); parameters picked without model names; native schema on the regular request path for OpenAI, Anthropic and Ollama.

Further facts:
- The fork's Ollama path calls `/api/generate` with the system prompt flattened into the prompt text, and it streams NDJSON. That call shape is one of the candidate causes in 20261005-172136-s-tac-c8r.
- Upstream `teilomillet/gollm` has had no maintainer commit since 2026-02-05 (v0.1.11). SDD's PRs #60 (retry) and #61 (gpt-5) and two other contributors' PRs are open and unanswered.
- The fork carries about 18k non-test lines (15 providers, memory, templates, optimizer, mixture-of-agents) for SDD's one call shape. It compiles 23 modules into the SDD binary, among them `testify`, `go-spew`, `go-difflib`, `tiktoken-go`, `validator` and `mimetype`.
- A host composing over `pkg/llm` cannot import SDD's internal adapters. It has copied them, with a fork pin of its own that lags SDD's.

## 3. Candidates

### Official SDKs: anthropic-sdk-go v2.4.0, openai-go v3.91.0, ollama/api v0.40.13

Protocol tracking is good: typed fields for structured output (`OutputConfig.Format`, GA with no beta header), cache control, effort and thinking, `max_completion_tokens`, `reasoning_effort` and Ollama `Think`; base-URL options; extra-body escape hatches; retry that honours `Retry-After` and can be disabled with `option.WithMaxRetries(0)`; errors that expose status and the response. openai-go has a typed Decisions client since v3.73.0, and its generic `client.Post` can reach System One through the same retry and error handling.

Measured with probe modules that import only the core client:

| Probe | Modules in graph | Heavy modules in graph | Compiled in |
|---|---|---|---|
| anthropic-sdk-go v2 | 73 | 27: aws/smithy ×14, cloud.google.com ×3, google.golang.org/api, grpc, genproto, OpenTelemetry ×7 | 18 modules, including OpenTelemetry (otel, trace, metric) |
| openai-go v3 | 34 | 18: aws/smithy ×14, Azure azcore, azidentity, MSAL | 6 modules |
| ollama/api | 86 (gin, bubbletea, sqlite, quic-go, protobuf, cobra …) | 0 | 8 modules; it raises the importer's Go floor to 1.26.10 |

- anthropic-sdk-go is still one module with `bedrock/`, `vertex/`, `googlecloud/`, `aws/` and `foundry/` inside. Its root package's tests put full aws zip hashes into an importer's `go.sum`. v2.0.0 pins `anthropic-version: 2026-10-21`, so an SDK upgrade moves the API version with it.
- openai-go split Vertex into its own module but keeps Azure and AWS in the root. Azure arrives through a realtime test file, a regression of its own isolation fix.
- Every route still needs System One written by hand.
- Verdict: fails 20260601-153050-d-cpt-n3y for all three providers, worse than in June.

### zendev-sh/goai v0.11.0 (2026-10-31)

- Clean: 6 modules in the graph, no third-party package compiled in.
- Maintained: 28 releases since June, issues closed within a day or two, no exported API removed since v0.8.5. About half the commits come from one author; v0.11.0 is still pre-1.0. It has grown to about 35 providers (+36.8k/−2k lines since June).
- Its defaults run against SDD's requirements:
  - Anthropic structured output defaults to a forced tool: a synthetic `json_response` tool with the schema as `input_schema`, forced through `tool_choice`, without `strict`. It doesn't enforce the schema, and forced `tool_choice` returns a 400 on Claude Opus 5.5, Sonnet 5.5 and Fable 5.1. Native `output_config.format` needs `structuredOutputMode: "outputFormat"` per call, or "auto" with a hard-coded model list.
  - OpenAI defaults to the Responses API; Chat Completions needs `useResponsesAPI: false`.
  - `strict` defaults to false.
  - `IsReasoningModel` matches name prefixes (`o<digit>`, `gpt-5` except `gpt-5-chat`, `codex-`), so `openai/gpt-5-mini` gets `max_tokens`.
  - Ollama always sends `think`, `false` when unset.
  - Setting prompt caching on a non-Anthropic provider prints to stderr.
- No System One and no Decisions; its Opper provider is chat over the compatible route only.

### voocel/litellm v1.9.6 (2026-10-18)

- Zero dependencies, Go 1.26.
- Native Anthropic `output_config.format` with system-block caching; strict OpenAI `json_schema`; explicit thinking and effort with no model-name inference; typed errors with `StatusCode` and `RetryAfter`; opt-in retry; base URL on every provider.
- Reaches Ollama only through its OpenAI-compatible `/v1` endpoint, so there are no native `format` or `think` fields.
- The Anthropic provider accepts six named extra keys and no general escape hatch.
- One maintainer (212 of about 219 commits) broke the API within v1 (v1.9.0). No System One.

### Screened out

- On dependency hygiene, mostly by inheriting anthropic-sdk-go: charmbracelet/fantasy (163 modules; Anthropic JSON mode also falls back to tools), cloudwego/eino-ext, mozilla-ai/any-llm-go (also still without Anthropic `cache_control`), joakimcarlsson/ai (worse than in June, Ollama now via `/v1`), tmc/langchaingo, jetify-com/ai, google/adk-go, firebase/genkit, maximhq/bifrost and several smaller ones.
- On Anthropic caching or base URL: maruel/genai and modfin/bellman. Both also require Go 1.27.

## 4. Binary size

Stripped release builds (`-trimpath -ldflags='-s -w'`, go1.26.10, darwin/arm64). Each probe makes the calls SDD needs: system and user prompt, schema, caching, usage, and System One where the route supports it.

| Probe | Size | vs hand-rolled |
|---|---|---|
| Hand-rolled `net/http`: three providers and System One | 5.8 MB | — |
| goai v0.11.0 | 6.3 MB | +0.5 |
| litellm v1.9.6 | 6.4 MB | +0.6 |
| ollama/api | 6.6 MB | +0.8 |
| gollm fork, as SDD uses it | 10.1 MB | +4.3 |
| anthropic-sdk-go v2.4.0 | 11.5 MB | +5.7 |
| openai-go v3.91.0 | 22.3 MB | +16.5 |
| all three official SDKs | 30.4 MB | +24.6 |

SDD's release-style binary measured 26.0 MB, so the official route would put it at roughly 46 MB (estimate). The aws, Google Cloud and Azure modules never reach any binary; they burden the module graph and `go.sum` only. The size comes from services that are never called: the openai probe holds about 500 Decisions and 1,264 Realtime symbols without calling either.

## 5. What to take from goai

goai's code for SDD's three paths, as non-test lines at v0.11.0:
- `internal/httpc`: 144 (the transport, a typed `APIError`, the retry headers).
- `internal/openaicompat`: 1,871 (one chat-completions codec shared by every compatible provider).
- `provider/anthropic`: 3,403.
- `provider/ollama`: 771.
- `provider/*.go`: 3,359 (shared `GenerateParams`, messages, tools and streams).

Taken:
- **The cut.** A small shared transport, one OpenAI-compatible codec that every compatible endpoint reuses, and one package per native protocol.
- **The wire details.** Field names, `Retry-After` handling, cache-token normalisation, Ollama `think` values, and test fixtures.

Not taken:
- **`provider.LanguageModel` with `GenerateParams`.** A request model for tools, streaming and multi-turn; `pkg/llm.Request` stays the only request model.
- **`ProviderOptions map[string]any`** and model-name capability lists.

Porting: of goai's 19 Anthropic commits since June, about three touch SDD's call shape by their subjects: native `output_config.format`, the cache TTL option, and a sync with the latest API schemas. The rest are thinking replay, streaming, tool search and file upload. Changes are carried over by reading release notes for the three protocols, not by mirroring files. goai is MIT-licensed: code copied verbatim keeps its copyright notice.

## 6. Placement

Options weighed:
- **A separate repository.** It repeats the fork's hops (PR, tag or pseudo-version, bump) and puts every fix behind a release decision in a second repo.
- **A nested module in this repository.** It gives a non-SDD consumer a clean module graph, but needs a `replace` in the root `go.mod` or tags of its own.
- **`internal/`.** It keeps the clients out of reach of a host composing over `pkg/llm`, which already copies the adapters.
- **Chosen: `pkg/llm` in the root module.** A host that imports `pkg/application` gains no module-graph cost. The clients import only the standard library, so a later move to a nested module stays a directory move.

20260902-114838-d-tac-cov kept adapters internal because code coupled to the unmaintained gollm was not worth publishing; that reason falls away. Its second reason, that factories read host-private configuration, stands.

## 7. Wire notes for an own client

- **Anthropic.**
  - API version `2026-10-21` exists alongside `2023-06-01`; under it `temperature`, `top_p` and `top_k` return a 400 on every model.
  - `output_config.format` is GA, and the old `output_format` is deprecated.
  - Minimum cacheable prefix: 512 tokens on the 5.x models, 1024 on Opus 4.8 and Sonnet 4.6 among others, 4096 on Opus 4.6/4.5 and Haiku 4.5.
  - 529 means overloaded and is transient.
- **Ollama `/api/chat`.** `format` takes `"json"` or a schema object. `think` takes a boolean or a model-defined string. The response carries `message.thinking` apart from `message.content`, and usage in `prompt_eval_count`, `prompt_eval_cached_count` and `eval_count`.
- **OpenAI.** Chat Completions has no announced shutdown, and OpenAI recommends the Responses API for new projects. SDD reaches providers through the chat-completions protocol (20260921-181638-d-cpt-cnu).
