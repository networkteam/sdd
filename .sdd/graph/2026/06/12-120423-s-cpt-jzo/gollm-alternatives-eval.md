# gollm replacement evaluation

**Date:** 2026-06-12 · **Participants:** Christopher, Claude
**Trigger:** s-tac-t96 — gpt-5-class models reject `max_tokens` (require `max_completion_tokens`), breaking the OpenAI pre-flight/summary path. Widened into: is it time to drop the networkteam/gollm fork for a maintained library?

## What our fork actually gives us (the baseline any replacement must match)

The gollm path powers **generation only** — pre-flight + summaries. Embeddings are already independent (`internal/llm/embedder.go` hits OpenAI-compatible `/v1/embeddings` and Ollama `/api/embed` directly, not via gollm).

Surface we depend on (`internal/llm/gollm/gollm.go`):

- Providers: `anthropic`, `openai`, `ollama` (claude-cli is a separate runner).
- Config knobs: provider, model, API key, Ollama endpoint, timeout, max-tokens.
- Two-part prompt (system + user); Anthropic system prefix tagged `cache_control: ephemeral`.
- **`GenerateWithUsage`** — per-call usage incl. `cache_read_input_tokens` / `cache_creation_input_tokens`. *(fork addition, s-tac-bmw)*
- **Provider-aware retry with backoff** — Retry-After parsing, transient-vs-permanent classification, jitter. *(fork addition, d-tac-4v1 / s-tac-7v0)*

The fork additions plus the gpt-5 `max_tokens` gap are the maintenance burden in question.

## Requirements

1. Anthropic + OpenAI + Ollama
2. First-class Ollama for local LLM (bonus: MLX)
3. Anthropic prompt caching — `cache_control: ephemeral` prefix **and** cache-read/creation token readback (both halves)
4. System/user prompt split
5. Per-call usage metadata
6. Retry + backoff (Retry-After, classification)
7. Correct gpt-5/o-series `max_completion_tokens`
8. **No heavyweight AWS/Google transitive trees** (d-cpt-n3y hygiene principle)
9. Well maintained

## The decisive cross-cutting finding

The official **`anthropics/anthropic-sdk-go` bundles its Bedrock + Vertex subpackages in the same module**, so its go.mod pulls `aws-sdk-go-v2` (+ smithy, sts, sso, imds) and `google.golang.org/api`/`cloud.google.com/go/auth` — even for a build that never touches Bedrock/Vertex. Therefore **any wrapper built on the official Anthropic SDK inherits the AWS SDK**. This is exactly the tree d-cpt-n3y rejected the official SDK to avoid.

**Verified today:** our current module graph is clean — `go list -m all` shows no `aws-sdk`, no `smithy`, no `google.golang.org/api` (only `cloud.google.com/go/compute/metadata v0.3.0`, a tiny stdlib-adjacent pkg). gollm uses hand-rolled HTTP, which is why. Switching to anything built on the official Anthropic SDK is a **regression** on hygiene.

## Comparison

| | any-llm-go | joakimcarlsson/ai | cloudwego/eino | zendev-sh/goai | langchaingo |
|---|---|---|---|---|---|
| Anthropic + OpenAI + Ollama | yes | yes | yes | yes | yes |
| Anthropic cache_control + cache-token readback | **none** | yes (on by default) | likely, unverified | yes | yes (own client) |
| System/user split | yes | yes | yes | yes | yes |
| Per-call usage | yes | yes | yes | yes | yes |
| Retry + backoff + Retry-After | no | yes (int seconds only) | unverified | yes | weak |
| gpt-5 `max_completion_tokens` | yes | yes | likely | unverified | unknown |
| **No AWS/Google bloat** | Google genai pulled | **AWS via Anthropic SDK** | **AWS in claude module** | **stdlib-only, clean** | **AWS+Vertex+grpc** |
| Maintenance | young, ~130★, ~4 devs | **solo**, 29★, fast cadence | mature, 11.8k★, very active | young, ~130★, near-solo | mature, slowing (last rel. Oct 2025) |

## Per-candidate notes

**mozilla-ai/any-llm-go** — DISQUALIFIED. No Anthropic prompt caching whatsoever: no `cache_control`, unified `Usage` struct lacks cache-token fields, and the `Extra` map isn't read when building Anthropic/OpenAI requests (no escape hatch). Gemini provider pulls `google.golang.org/genai` → `cloud.google.com/go`. Correctly sends `max_completion_tokens` (a win), but the caching miss is fatal for us.

**joakimcarlsson/ai** — Best *feature* match. Multi-module monorepo (import only what you use → OpenAI/Ollama modules are clean). Anthropic ephemeral caching **on by default**, full `CacheReadTokens`/`CacheCreationTokens` breakout, retry with jitter + integer Retry-After, correct `max_completion_tokens`. BUT the `llm/anthropic` module imports `anthropic-sdk-go/bedrock` → full `aws-sdk-go-v2` stack; core hard-requires an OpenTelemetry + grpc/genproto tail. Solo maintainer, 29★. Caching breakpoint is auto-placed on the *last* block per category (fine for fixed-system + varying-user, but not freely positionable). Adopting = reopening d-cpt-n3y.

**cloudwego/eino** — Most production-mature (ByteDance, 11.8k★, very active). Clean core (abstractions only, zero provider/cloud SDKs); providers live in separate `eino-ext` modules. But the `claude` module's go.mod lists `aws-sdk-go-v2/config` + `/credentials` (inherited from official Anthropic SDK), so importing Claude still drags AWS. It's also an agent/orchestration *framework* — heavier conceptual fit than a thin Runner. Retry policy, gpt-5 handling, and exact cache-token surfacing unverified at eino's API layer.

**zendev-sh/goai** — Best dependency hygiene by far: stdlib-only core (`golang.org/x/oauth2` + a tiny GCE metadata pkg), **Bedrock/Vertex hand-rolled** (SigV4 / EventStream / NDJSON) so even those providers avoid the AWS SDK. Explicitly documents automatic Anthropic cache control, cache-token tracking normalized across providers, system/user split (`WithSystem`/`WithPrompt`), per-call usage, and exponential backoff honoring Retry-After. Risks: pre-1.0 (v0.8.x), ~130★, near-solo author (self-described Go learner), and the `max_completion_tokens` switch is **likely but unverified** (has an `openaicompat.IsReasoningModel` codec path; the deciding file wasn't opened). The only option that is both feature-complete AND hygiene-clean.

**tmc/langchaingo** — Meets functional needs (incl. a hand-rolled Anthropic caching client with the cache-token fields), but **monolithic go.mod** drags `aws-sdk-go-v2` + `cloud.google.com/go/vertexai` + `generative-ai-go` + grpc unconditionally. Fails hygiene outright. Maintenance slowing.

**DIY (openai-go + anthropic-sdk-go behind a thin interface)** — maximal per-provider correctness, but inherits the Anthropic SDK's AWS/Google deps, so it's no cleaner than the fork while costing more to build. Our gollm fork already *is* a hand-rolled-HTTP thin client.

## MLX

No Go LLM library supports MLX natively. Every candidate (and gollm) reaches an MLX model the same way: point an OpenAI-compatible base URL at an mlx-lm / LM Studio server. So MLX is not a differentiator — it's already achievable via OpenAI-compat, the path the embedder uses today.

## Decision

**Patch the fork now; watch goai.**

- **Now:** in networkteam/gollm's OpenAI request builder, emit `max_completion_tokens` for gpt-5/o-series models (keep `max_tokens` for older models); bump the `replace` pseudo-version in go.mod. Small, bounded, unblocks the gpt-5 path (the real need behind s-tac-t96). Keeps the clean tree and d-cpt-n3y intact.
- **Later (watch-list):** zendev-sh/goai is the clean off-ramp from the fork. Re-evaluate when it reaches **v1.0 / broader adoption / a less solo bus factor**, and verify the `max_completion_tokens` field-switch live at that point.
- **Conditional:** joakimcarlsson/ai becomes the answer only if the AWS-SDK cost is judged acceptable — a strategic call about d-cpt-n3y, not a library call.
