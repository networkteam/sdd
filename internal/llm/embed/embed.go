// Package embed provides Embedder implementations for OpenAI-compatible
// (`/v1/embeddings`) and Ollama (`/api/embeddings`) endpoints, plus a
// factory that dispatches by configured provider and wraps remote
// providers with a rate.Limiter (paralleling the chat-runner factory).
package embed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

// Per-provider default batch sizes when EmbeddingConfig.BatchSize is zero.
// Values are chosen to bound per-call payload size (OpenAI's hard limit is
// 2048 inputs but ~100 keeps payloads small enough that token-cap errors
// don't fail the whole batch) and to keep timeouts predictable for
// local Ollama.
const (
	defaultOpenAIBatchSize = 100
	defaultOllamaBatchSize = 64
)

// New constructs an Embedder from cfg. Returns an error when Provider is
// unrecognised or required fields (Model, API key for remote providers)
// are missing. Remote providers ("openai") are wrapped with a rate.Limiter
// using cfg.RateLimitRPS or a conservative provider default. The batch
// size flows from cfg.BatchSize (zero → provider default) so command-line
// flags / config overrides reach the transport without env-var sniffing
// in the package.
func New(cfg model.EmbeddingConfig) (llm.Embedder, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("no embedding provider configured — run `sdd config set embedding.provider <provider>` (user-global, so every project and cross-repo index shares one embedding space)")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("embedding.model is required")
	}

	timeout := resolveTimeout(cfg.Timeout)

	var inner llm.Embedder
	var err error
	switch cfg.Provider {
	case "openai":
		inner, err = newOpenAI(cfg, timeout, resolveBatchSize(cfg.BatchSize, defaultOpenAIBatchSize))
	case "ollama":
		inner = newOllama(cfg, timeout, resolveBatchSize(cfg.BatchSize, defaultOllamaBatchSize))
	default:
		return nil, fmt.Errorf("unknown embedding provider %q (supported: openai, ollama)", cfg.Provider)
	}
	if err != nil {
		return nil, err
	}

	if isRemote(cfg.Provider) {
		rps := cfg.RateLimitRPS
		if rps == 0 {
			rps = providerDefaultRPS(cfg.Provider, cfg.Model)
		}
		if rps > 0 {
			inner = newRateLimited(inner, rps)
		}
	}
	return inner, nil
}

func isRemote(provider string) bool {
	return provider == "openai"
}

// providerDefaultRPS biases below tier-1 ceilings so bursty index builds
// don't trip 429s out of the box. Users on higher tiers override via
// embedding.rate_limit_rps.
func providerDefaultRPS(provider, modelName string) float64 {
	switch provider {
	case "openai":
		// OpenAI tier-1 embedding limits are higher than chat — but still
		// rate-limit to be safe under bursty index builds.
		switch {
		case strings.Contains(strings.ToLower(modelName), "text-embedding-3"):
			return 5.0
		default:
			return 3.0
		}
	default:
		return 0
	}
}

// resolveBatchSize returns cfg.BatchSize if positive, otherwise the
// provider-specific fallback. Negative values fall back too (treat as
// "unset") rather than erroring — the CLI surface validates earlier.
func resolveBatchSize(cfg, fallback int) int {
	if cfg > 0 {
		return cfg
	}
	return fallback
}

// resolveTimeout parses the configured Go duration string, falling back to
// 2m when empty or unparseable. Embedding calls are batch-shaped — a
// single call over the default 64-input batch can take ~90s on a local
// 8b parameter model — so the default errs on the generous side. Users
// running fast remote providers (OpenAI text-embedding-3) typically
// finish in well under a second per batch and won't notice the higher
// default; users on slow local models won't lose work to a too-short
// default they didn't know to override.
func resolveTimeout(raw string) time.Duration {
	if raw == "" {
		return 2 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 2 * time.Minute
	}
	return d
}

// rateLimited wraps an Embedder with a token-bucket limiter so parallel
// batch operations don't exceed provider rate limits.
type rateLimited struct {
	inner   llm.Embedder
	limiter *rate.Limiter
}

func newRateLimited(inner llm.Embedder, rps float64) llm.Embedder {
	burst := int(rps)
	if burst < 1 {
		burst = 1
	}
	return &rateLimited{
		inner:   inner,
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

func (r *rateLimited) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}
	return r.inner.EmbedDocuments(ctx, texts)
}

func (r *rateLimited) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}
	return r.inner.EmbedQueries(ctx, texts)
}

func (r *rateLimited) Dimensions() int     { return r.inner.Dimensions() }
func (r *rateLimited) Fingerprint() string { return r.inner.Fingerprint() }
func (r *rateLimited) BatchSize() int      { return r.inner.BatchSize() }
