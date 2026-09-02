# Public LLM composition surface (2026-09-02)

Signatures and composition examples for the directive that completes the public LLM surface in `pkg/llm` and `pkg/llm/embed`. Written against HEAD `fc36753d` (v0.17.0-beta.1).

## Today

`cmd/sdd/main.go` composes the chat runner:

```go
runner, _ := factory.New(cfg)                          // provider adapter, rate limiter, Bounded(timeout)
sink, _ := llmstats.NewFileSink(sddDir + "/stats")     // writes .sdd/stats/llm.jsonl
runner = internalllm.Observed(runner, sink)            // one log line + one CallStat row per call
```

`Bounded`, `Observed`, `CallStat`, `StatsSink`, and the rate limiter live in `internal/llm` and `internal/llm/factory`. A host sees `llm.Runner` only.

Embedding has a five-method internal interface (`internal/llm.Embedder`: EmbedDocuments, EmbedQueries, Dimensions, Fingerprint, BatchSize) built by `internal/llm/embed.New(cfg)`. Its stats are recorded inside the OpenAI and Ollama adapters via `llm.RecordEmbedCall(ctx, CallStat{...})`, which reads the sink from the context that `cmd/sdd` set with `llm.WithStatsSink`. The public port is a different shape:

```go
// pkg/application/search_ports.go
type EmbeddingExecutor interface {
    Spec(ctx) (EmbeddingSpec, error)                         // fingerprint
    Embed(ctx, []EmbeddingInput) ([]EmbeddingVector, error)  // inputs carry ID, Text, Purpose (document|query)
}
```

The result carries vectors only, so nothing outside the adapter can observe identity or usage; a host implementing the port gets no stats.

## After: `pkg/llm` (chat)

Existing contract unchanged (`Purpose`, `Request`, `Result`, `Identity`, `Usage`, `Runner`, `RunnerFunc`, `Error`). Added, moved from `internal/llm` with the same shape:

```go
func Bounded(r Runner, timeout time.Duration) Runner   // from internal/llm/timeout.go
func RateLimited(r Runner, rps float64) Runner         // from internal/llm/factory
func Observed(r Runner, sink StatsSink) Runner         // from internal/llm/observed.go

type CallStat struct {
    Purpose    string     // llm.Purpose for chat, embed.Purpose for embedding
    Identity   Identity
    Usage      Usage
    Items      int        // inputs per call; embedding batches set it, chat leaves 0
    DurationMS int64
    Error      string     // failure text; empty on success
}
type StatsSink interface { RecordCall(CallStat) }      // must be safe for concurrent use

func ByPurpose(routes map[Purpose]Runner, fallback Runner) Runner   // new
```

`Observed` wraps a runner; after each call it fills a `CallStat` and hands it to the sink. `ByPurpose` dispatches on `Request.Purpose` and uses `fallback` for purposes not in the map.

## After: `pkg/llm/embed` (embedding)

Parallel to the chat side; imports `pkg/llm` for `Identity`, `Usage`, `CallStat`, `StatsSink`.

```go
type Purpose string
const (
    PurposeDocument Purpose = "embed-document"
    PurposeQuery    Purpose = "embed-query"
)

type Request struct { Purpose Purpose; Texts []string }
type Result  struct { Vectors [][]float32; Identity llm.Identity; Usage llm.Usage }

type Embedder interface {
    Embed(ctx context.Context, req Request) (Result, error)
    Fingerprint() string   // identifies the vector space; stable across calls
}
type EmbedderFunc ...

func Bounded(e Embedder, timeout time.Duration) Embedder
func RateLimited(e Embedder, rps float64) Embedder
func Observed(e Embedder, sink llm.StatsSink) Embedder   // records Items=len(Texts), Usage from the result
```

Deleted: `internal/llm.RecordEmbedCall`, `internal/llm.WithStatsSink`, the context-carried sink lookup. The OpenAI and Ollama adapters return `Result` with identity and usage instead of recording themselves. Document and query templates stay an adapter concern selected by `Request.Purpose`.

`application.EmbeddingExecutor`, `EmbeddingSpec`, `EmbeddingInput`, `EmbeddingPurpose`, and `EmbeddingVector` are removed. `ProjectRuntimeOptions.Embedder` takes an `embed.Embedder` the way `LLM` takes the chat `Runner`; the application maps chunk IDs to positions in `Request.Texts` and reads the vector space from `Fingerprint()` itself. No compatibility layer: nothing consumes the public exports yet.

## Host composition

```go
pre := llm.Bounded(anthropicRunner("claude-sonnet"), 60*time.Second)
sum := llm.Bounded(ollamaRunner("glm-5.3-flash"), 30*time.Second)
runner := llm.Observed(
    llm.ByPurpose(map[llm.Purpose]llm.Runner{
        llm.PurposePreflight:    pre,
        llm.PurposeWritingGuide: pre,
        llm.PurposeSummarize:    sum,
    }, pre),
    tenantSink{db},                 // host's own RecordCall
)
embedder := embed.Observed(embed.Bounded(openaiEmbedder("text-embedding-3-small"), 30*time.Second), tenantSink{db})

app := application.New(..., application.ProjectRuntimeOptions{LLM: runner, Embedder: embedder})
```

## Local composition

`internal/llm/factory.New(cfg)` builds the adapter from the private config, then wraps `llm.RateLimited` and `llm.Bounded`; `cmd/sdd` wraps `llm.Observed` around `llmstats.FileSink`. `internal/llm/embed.New(cfg)` does the same with `embed.RateLimited`, `embed.Bounded`, and `embed.Observed`. One code path for the local binary and an external host.

## Stays internal

- `internal/llm/gollm` (adapter and its provider-aware retry) and `internal/llm/claude`
- `internal/llm/embed` provider adapters (OpenAI, Ollama) and templates
- `internal/llm/factory.New` and `internal/llm/embed.New` over `model.LLMConfig` / `model.EmbeddingConfig`
- `internal/llmstats.FileSink`, which owns the JSONL wire shape
