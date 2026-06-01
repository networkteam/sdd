# Plan: Fork gollm for provider-aware LLM retry

## Context

- **Gap (s-tac-m1b):** SDD's LLM calls make one attempt and fail on any transient error. We set gollm's `MaxRetries(0)` (`internal/llm/gollm/gollm.go:49`) to stop batch-summarize WARN floods and deferred retry to "the SDD caller," but that policy was never built. gollm's own retry (v0.1.11) is shallow anyway: fixed delay, no error classification, collapses all non-200 to a generic `ErrorTypeAPI`, discards response headers (so `Retry-After` is never seen).
- **Directive (d-cpt-n3y):** fork gollm rather than adopt the official Anthropic SDK (which drags `aws-sdk-go-v2` + `google.golang.org/api` into the module graph). Dependency-hygiene principle: reject deps that pull in heavyweight transitive trees.
- **Goal:** bounded, provider-aware retry on the pre-flight and summarize paths, with no new heavyweight dependencies.

## Fork & module mechanics

- Fork `teilomillet/gollm` → `networkteam/gollm`, **keeping the module path** `github.com/teilomillet/gollm` so import paths are unchanged and the diff is a clean upstream PR.
- sdd `go.mod`: `require github.com/teilomillet/gollm v0.1.11` + `replace github.com/teilomillet/gollm => github.com/networkteam/gollm <tag>`.
- **Distribution check:** sdd ships via Homebrew + the curl installer, both built by goreleaser *from source*, which honors `replace`. The only thing `replace` breaks is `go install pkg@version`, which sdd does not advertise or use. Document this in the go.mod comment near the replace.
- **Upstream:** open a PR to `teilomillet/gollm` with the retry improvements. On merge, drop the `replace` and bump the `require`.

## Retry policy (in the fork)

Touch points in gollm: `attemptGenerate` (`llm/llm.go:294` network error, `:306` non-200 + the discarded `resp.Header`) and the `Generate` retry loop (`llm/llm.go:182-197`).

- **Classification** (new typed error carrying `retryable bool` + optional `retryAfter`):
  - *retryable-transient:* connection/network errors (the `:294` path), 408, 409, 5xx (incl. Anthropic 529).
  - *rate-limited-with-delay:* 429 rate-limit — capture the server delay (`Retry-After` / `retry-after-ms`).
  - *permanent:* 4xx except rate-limit 429; OpenAI `insufficient_quota` (a 429 that means out-of-credits) -> no retry.
- **Delay:** `delay = max(serverDelay, expBackoff(attempt) + jitter)`, where `expBackoff = base * 2^attempt` capped at `maxWait`, full jitter. Reuse gollm's existing cancellable `wait(ctx)` select.
- **Caps:** bounded `MaxRetries` (default TBD, ~3-4); ctx-cancellable.
- **Logging:** per-attempt at **Debug**, not Warn — this removes the original reason `MaxRetries(0)` was set.

The core gollm change is at `:306`: instead of collapsing every non-200 into a generic `ErrorTypeAPI` and dropping headers, read `resp.Header`, classify by status + body error type, and return the typed error the loop can act on.

## Per-provider rate-limit reference

**Anthropic**
- 429 `rate_limit_error` -> `retry-after` (seconds); wait at least that.
- 529 `overloaded_error` -> retryable, **no** trustworthy retry-after -> backoff + jitter only.
- Info headers: `anthropic-ratelimit-{requests,tokens}-{limit,remaining,reset}`.
- Retryable set (mirrors official SDK): 408, 409, 429, 5xx (incl. 529), connection errors.

**OpenAI**
- 429 -> `retry-after-ms` (milliseconds) — honor exactly on first retry, then backoff.
- Budget headers: `x-ratelimit-{limit,remaining,reset}-{requests,tokens}`; `reset-*` are duration strings (`6s`, `1m30s`), not epoch seconds.
- `insufficient_quota` (error `type`/`code` in body) is a 429 but **not retryable**.
- 5xx -> retryable with backoff.

**Ollama**
- Local; no rate-limit surface. Only network/timeout errors are transient-retryable.

## SDD-side wiring

- `internal/llm/gollm/gollm.go`: stop forcing `MaxRetries(0)`; set a sensible default and `SetRetryDelay`, letting the fork's policy run. Update the comment that documents the old rationale.
- `model.LLMConfig` / `.sdd/config.local.yaml`: optional retry block under `llm:` (`max_attempts`, `base_delay`, `max_delay`) with baked defaults.
- Both the pre-flight runner and the summarizer use the same `Runner`, so wiring it once covers `sdd new` and `sdd summarize`.

## Alternatives considered (rejected)

- **Official Anthropic Go SDK:** drags `aws-sdk-go-v2` (Bedrock) + `google.golang.org/api` (Vertex) into our module graph — rejected per d-cpt-n3y.
- **Vendor a gollm slice into `internal/`:** self-contained but a heavier in-tree copy with no clean upstream-sync path; rejected in favor of fork+replace+PR.
- **Thin in-house Anthropic client:** smallest footprint but breaks the unified multi-provider path and would need a directive change.
- **`replace` -> local filesystem path:** breaks all non-local builds; use a module-path replace to the fork's VCS instead.

## Testing

- **Fork (table-driven, no network):** classifier (status -> class, `insufficient_quota` detection, network errors); header parsing (`retry-after`, `retry-after-ms`, OpenAI `reset-*` duration strings); delay computation (backoff growth, jitter within bounds, `max(serverDelay, backoff)`).
- **SDD:** runner-level test that a transient error retries then succeeds (fake client), and that a permanent error fails fast without retrying.

## Open questions

- Default `max_attempts` (3 / 4 / 5) and base/max backoff values — calibrate.
- Whether to also add retry to the claude-cli bridge path (`internal/llm/claude`) or leave it (it is the non-gollm default).
- Config-key naming under `llm:` (`retry: {max_attempts, base_delay, max_delay}`?).
- Whether to surface rate-limit `remaining` headers for proactive throttling — likely out of scope; conservative concurrency defaults from s-tac-a4s already exist.
