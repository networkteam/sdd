# LLM boundary architecture

Concrete design settled in dialogue (Christopher, Claude, 2026-08-30). Replaces the `application` LLM port mirror. The guiding analogy is `net/http`: `Runner` is our `RoundTripper`, with divergences noted below.

## Package map

```
pkg/llm                  NEW public contract package. Pure vocabulary plus one
                         one-method interface. No deps, no I/O, no machinery.
internal/llm             Client machinery: observing decorator (timing, debug
                         log, CallStat records), timeout decorator,
                         CallStat + StatsSink types, embed plumbing (unchanged
                         for now).
internal/llm/factory     Local implementation: model.LLMConfig -> one composed
                         pkg/llm.Runner (purpose mux, rate-limit decorator).
internal/llm/gollm       Provider adapters implementing pkg/llm.Runner.
internal/llm/claude      Keeps its per-model/turn detail as its own debug logging.
internal/llmops          The LLM operations: preflight, summarize, writing guide.
                         Each owns its template dir + parse; shared prompt
                         fragments live here. Every op sets its Purpose constant.
internal/llmstats        FileSink implements the internal StatsSink; owns the
                         JSONL wire shape (.sdd/stats/llm.jsonl rows stay readable).

DELETED  application/llm_port.go (LLMRequest, LLMResult, LLMUsage, LLMIdentity,
         LLMExecutor), runtimeLLMRunner (write_api.go), the serve.go executor
         bridge, LLMExecutorFuncs + Capabilities (functional_adapters.go), and
         the internal llm.Request/RunResult/LLMMetadata/Identity mirrors.
```

Dependency line: `llmops -> internal/llm -> pkg/llm`. Machinery knows no operation, operations know no provider.

## pkg/llm: the complete public surface

```go
package llm

// Purpose names what a call is for: a routing key for implementations and an
// observability dimension, the same value for both so they cannot drift. The
// set is closed: purposes are minted only by application operations. Hosts
// route on these constants and never invent values.
type Purpose string

const (
    PurposePreflight    Purpose = "preflight"
    PurposeSummarize    Purpose = "summarize"
    PurposeWritingGuide Purpose = "writing-guide"
)

// Request carries everything an implementation may route on: ctx (who) and
// Purpose (what for), plus the two-part prompt.
type Request struct {
    Purpose      Purpose
    SystemPrompt string // stable prefix, cacheable by providers that can
    UserPrompt   string // per-call variable part
}

// Identity names what served a call, reported per call on Result. It is never
// a static property of an implementation, because implementations may route.
type Identity struct {
    Provider string
    Model    string
    Variant  string // behaviour-affecting config, e.g. "reasoning_effort=high"
}
func (i Identity) String() string // "model (variant)"

// Usage is the common format for provider-reported consumption. A field the
// provider does not report stays zero: reported, never reconstructed.
type Usage struct {
    InputTokens       int
    OutputTokens      int
    CacheReadTokens   int
    CacheCreateTokens int
    CostUSD           float64
}

type Result struct {
    Text     string
    Identity Identity // required on success: what actually served this call
    Usage    Usage
}

// Runner is the single port. Contract, stated RoundTripper-style: fill
// Result.Identity on success; report Usage, never invent it; no internal
// retries (retry is caller policy); bound your own calls, because deadlines
// are configuration and configuration is host-private; ctx carries
// request-scoped facts a routing implementation may use (tenant, logger).
type Runner interface {
    Run(ctx context.Context, req Request) (Result, error)
}

// RunnerFunc adapts a function to Runner (test doubles, error stubs).
type RunnerFunc func(context.Context, Request) (Result, error)

// Error optionally attributes a failed call. An implementation that knows what
// it routed to wraps its error so failures measure like successes.
type Error struct {
    Identity Identity
    Err      error
}
func (e *Error) Error() string
func (e *Error) Unwrap() error
```

Nothing else. No sink, no recorder, no client: observability types are not part
of the contract, because no host outside the module consumes our recorder.

## Observability: composed by the host, not served by application

Recording is a decorating Runner. Everything it needs is in the data: Purpose in
the Request, Identity and Usage in the Result, failure attribution in the typed
error, duration measured around the inner call. There is no fact a framework
hook would hold that a decorator lacks, so there is no framework hook.

```go
// internal/llm. The local host's observing decorator: one debug log line and
// one CallStat row per call, success or failure.
func Observed(r pkgllm.Runner, sink StatsSink) pkgllm.Runner
```

`CallStat{Purpose, Identity, Usage, Items, DurationMS, Error}` and `StatsSink`
stay internal beside it. A hosted deployment observes with its own decorator
and its own record shape (tenant metering, billing); it never needs ours.

Recording is therefore a composition convention, not a framework guarantee: a
host that skips the decorator gets no stats, and the failure mode is a visibly
empty stats file, never silent corruption. For the local host the guarantee is
as strong as before, enforced at its one composition site.

What remains inside `application` is only the facts it alone holds: the
Purpose set by its operations, and the prompts. Deadlines are configuration,
may vary by purpose exactly like the model, and compose as a timeout decorator
in the host's stack; the local factory always applies the configured timeout,
so a host that passes an unbounded runner gets a visible hang, never an
invented default. The internal `op` string argument dies; Purpose is
the op. This also ends a live drift: the engine routes on "summary" today while
stats record "summarize". Canonical value: "summarize" (stats-history
continuity); proctest updates.

## Composition root and shells

```go
// application
type ProjectRuntimeOptions struct {
    ...
    LLM llm.Runner // pkg/llm; required, instances only; arrives already
                   // decorated for observability and bounded by its timeout
}
```

```go
// cmd/sdd: the local host's whole LLM wiring
cfg, _ := resolveLLMConfig(cmd)   // unchanged: flags + config file overlay
base, err := factory.New(cfg)     // internal, in-module; mux, rate limit,
                                  // and the configured timeout, all decorators
sink, _ := llmstats.NewFileSink(statsDir)
runner := internalllm.Observed(base, sink)
sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{LLM: runner, ...})
```

`model.LLMConfig` and the factory stay internal. The local config schema
(user-global + project + project-local + flag overrides) never goes public;
each host adapts its own config scheme to instances at its own composition site.

## Requirements replayed against the design

**Per-purpose model strength** (writing guide cheap, summary strong). Host-private config section; `factory.New` builds the mux. No public surface change:

```yaml
llm:
  provider: ollama
  model: glm-5.3-flash:cloud
  purposes:
    writing-guide: { model: qwen3:4b }
```

**Hosted sdd, per-org API keys.** The host implements the one method; `application` untouched:

```go
func (h *tenantRouter) Run(ctx context.Context, req llm.Request) (llm.Result, error) {
    org := tenantFromContext(ctx)
    return h.clientFor(org.APIKeys, req.Purpose).Run(ctx, req)
}
```

**Variant evaluation** (the GLM-5.3-Flash case that exposed the mirror). gollm fills `Identity{Provider: "ollama", Model: "glm-5.3-flash:cloud", Variant: "think=high"}` on every Result; the observing decorator copies it into CallStat verbatim. A field drop is unrepresentable: Identity exists once, so adding a field is one struct edit the compiler carries to every implementer. No copy site is left to forget.

**Observability composed.** The host wraps its runner; a hosted deployment meters per tenant with its own decorator and record shape. Nothing is reconstructed, because everything observable travels in the data the port already carries.

**Decorators.** Rate limiting stops being factory-special; it is a wrapping Runner, like observation itself and any host-side caching or retry wrapper.

**proctest, read-only commands, recover.** Each is a RunnerFunc: proctest switches on req.Purpose as today, minus dead fields; read-only paths return an error from one line.

## Parity with current behavior

| today | new shape |
|---|---|
| stats JSONL, `sdd stats` | FileSink owns the wire shape; rows unchanged |
| failure rows with provider/model | llm.Error wrap in local runners; parity locally, honest degradation for foreign implementations that skip it |
| one debug log line per call | observing decorator, applied at the local composition site |
| timeout | decorator composed by the local factory from config, per-purpose capable |
| claude-cli cost and multi-model detail | Usage.CostUSD common; per-model detail logged by the claude runner itself (only log.go ever read the Models map) |
| rate limits | decorator Runner in the factory composition |
| engine and CLI recording identically | both paths receive the same composed observed runner and vocabulary |
| examples/extendingsdd | implements one method instead of three, none dead |
| embedding stats | unchanged: ctx-carried sink until embeddings get the same treatment |

## Deliberate divergences from net/http

1. Response-carried attribution instead of a transport method: net/http answers "where did this go" with resp.Request after redirects; we answer with Result.Identity. Same principle: ask the data, not the implementation.
2. No public Client at all: net/http exports Client because arbitrary callers are its public; our only caller is application's operations, and everything a Client would own is either a fact application alone holds (Purpose) or a decorator composed by the host (observation, timeout).

## Rejected along the way

- Reflective parity test over the mirror types: scaffolding around a bad construct (rejected by Christopher before this dialogue).
- Public type aliases to internal types: pins the public API to internal structure.
- A public config struct / public factory: config schemas are host-private; surface for imagined hosts.
- Per-purpose runner map on ProjectRuntimeOptions: routing moved wholly across the port.
- Identity() on the Runner interface: false for anything that routes, including the local purpose mux on day one of per-purpose config.
- Open-set Purpose: hosts never mint purposes; only application operations do, so the set is closed.
- A StatsSink option on ProjectRuntimeOptions, then a public Observed decorator: recording needs no fact the port data does not already carry, and no host outside the module consumes our recorder, so both retreated to host-side composition over internal machinery.
- An LLMTimeout option on ProjectRuntimeOptions: the deadline is configuration, may vary by purpose like the model, and belongs to the same host config the runner is built from; keeping it in application would split one config across two consumers.

The test that carried the design: each hardening requirement simplified the surface instead of growing it. Per-purpose selection deleted Identity() from the interface; tenant-key routing deleted routing from application; host-owned observability deleted the stats hook and the Client; per-purpose deadlines deleted the timeout option; the port ended at one method over pure data types, and the runtime options ended at one field: the Runner.
