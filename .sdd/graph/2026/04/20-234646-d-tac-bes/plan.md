# LLM Provider Abstraction — Design Plan

## Goal

Replace the monolithic `claude -p` invocation with a pluggable Runner abstraction so SDD can use other providers (local Ollama, Anthropic API, OpenAI, etc.) while keeping `claude -p` as the default. Address the `sdd summarize --all` throughput pain through Anthropic prompt caching (where applicable) and errgroup-pooled concurrency.

## Current state

`internal/llm/` already defines a clean `Runner` interface used across handlers and finders via DI. Only one implementation exists: `internal/llm/claude` which shells out to `claude -p --model ... --output-format json`. The claude path piggybacks on the logged-in Claude Code session (no API key needed) — a real onboarding advantage we want to preserve.

`sdd summarize --all` is fully sequential: 30 s × N entries. No response caching, no prompt caching, no parallelism.

## Target state

- Two Runner implementations live side by side: `internal/llm/claude` (shell bridge, default) and `internal/llm/gollm` (adapter for Ollama / Anthropic API / OpenAI / etc.).
- A tiny factory resolves the Runner from resolved config (CLI flag > `.sdd/config.local.yaml` > `.sdd/config.yaml` > built-in default).
- For Anthropic via gollm: prompt caching via structured messages with `ephemeral` cache_control on the stable prefix.
- For batch throughput: `sdd summarize --concurrency N` using `errgroup.SetLimit`, with a `rate.Limiter` gating remote providers.

## Alternatives considered

### 1. Replace `claude -p` entirely with gollm-anthropic

**Rejected.** `claude -p` has a real onboarding advantage: it uses the logged-in Claude Code session, no API key management. Users who install SDD and are already running Claude Code can do pre-flight and summarize with zero extra setup. Routing all Anthropic calls through gollm-anthropic forces an `ANTHROPIC_API_KEY` on them. Two paths is worth the maintenance cost.

### 2. Client-side response-cache decorator

**Rejected.** The idea: wrap any Runner with a disk-keyed memoization layer at `.sdd/tmp/llm-cache/{provider}/{model}/{sha256}.json`. In practice:

- Pre-flight prompts are unique per entry (they embed the entry content).
- Summarize prompts are unique per entry (they embed the entry content + its refs).
- Hits would be near-zero in normal use; the cache would be dead weight with invalidation cost.

The only scenario where it would help is repeated runs of identical prompts during dev/test, which is rare enough that dropping the feature is the right call.

### 3. Anthropic Message Batches API (async, 50% discount)

**Rejected.** Anthropic's batch API submits many prompts at once and returns results asynchronously (up to 24 h). Good for very large backfills. But:

- Interactive `sdd summarize` needs low-latency results.
- gollm has no wrapper; we would have to call the API directly.
- Concurrent calls + prompt caching cover current throughput targets.

If batch re-summarize scale grows past where prompt caching + concurrency can handle, this is the next lever.

### 4. Runner takes a single `prompt string` (keep current interface)

**Rejected.** Anthropic prompt caching relies on prefix detection. To mark the stable prefix explicitly — which is how gollm exposes caching, via `AddStructuredMessage(..., "ephemeral")` — the Runner must know which part of the input is stable and which is variable. A richer `Request{SystemPrompt, UserPrompt}` captures that without leaking provider specifics into callers.

For providers without caching (Ollama, OpenAI via gollm, claude-cli), the adapter simply concatenates — zero cost.

### 5. Keep all LLM config in the shared `.sdd/config.yaml`

**Rejected.** API keys must not be committed. The `d-tac-q5p` decision already established `.sdd/config.local.yaml` (gitignored) as the local-override slot — this plan extends that schema. The shared file holds safe defaults (provider name, model name, timeout, concurrency); the local file holds API keys, Ollama endpoint, and per-machine overrides.

### 6. Use gollm's own concurrency primitive (`BatchPromptOptimizer`)

**Rejected.** `BatchPromptOptimizer` is a specialized utility for prompt-optimization workflows (rate-limited concurrent generation, then selecting best). Reusing it for generic batch summarize would couple us to its specific shape. Rolling our own `errgroup.SetLimit(N)` + `rate.Limiter` is ~20 lines of code and stays under our control.

## Ordering / dependencies

`d-tac-q5p` (participant-name drift plan) establishes `.sdd/config.local.yaml` and the idempotent `sdd init` housekeeping rule. This plan extends that schema but does not re-invent it. Either d-tac-q5p lands first, or the two plans share the minimal `config.local.yaml` introduction.

## Caching mechanics (implementer reference)

Anthropic prompt caching has a 5-minute default TTL (1 hour with the `extended-cache-ttl-2025-04-11` beta header). Each call:

1. Client sends a message marked `cache_control: {type: "ephemeral"}`.
2. Server computes a content hash of the prefix up through that marker.
3. If the hash was seen in the last 5 minutes, the prefix is served from cache at ~10% of input-token cost.
4. The variable tail (the proposed entry, refs, etc.) is always re-processed.

For this to work across calls, the stable prefix must be byte-identical. The template refactor should therefore isolate any context that varies per-call (timestamps, entry IDs, participant lists) to the user-prompt portion.

## Open implementation details

These are deferred to implementation time, not open architectural questions:

- Exact default `rate_limit_rps` for remote providers (likely 4 RPS for Anthropic to stay under typical tier limits).
- Whether gollm's `Memory` wrapper has per-call overhead worth measuring before making it default for anthropic-provider calls.
- Whether to expose a `--no-cache` flag for debugging (probably yes — useful when testing prompt changes).
