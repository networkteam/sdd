// Package embed is the local host's embed.Embedder composition: it resolves
// model.EmbeddingConfig — a host-private schema that never goes public — into
// one pkg/llm/embed Embedder (provider adapter, timeout, rate limit),
// paralleling the chat-runner factory.
package embed

import (
	"fmt"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/model"
	pkgembed "github.com/networkteam/sdd/pkg/llm/embed"
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

// New constructs the per-round-trip embedder from cfg: the provider adapter,
// which sends one request per Embed, bounded by cfg.Timeout and, for remote
// providers ("openai"), rate-limited by cfg.RateLimitRPS or a conservative
// provider default. The caller composes embed.Batched around it with
// BatchSize(cfg) (20260902-154750-d-tac-o1s), so deadline and limiter apply
// to each request. Returns an error when Provider is unrecognised or required
// fields (Model, API key for remote providers) are missing.
func New(cfg model.EmbeddingConfig) (pkgembed.Embedder, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("no embedding provider configured — run `sdd config set embedding.provider <provider>`")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("embedding.model is required")
	}

	var adapter pkgembed.Embedder
	var err error
	switch cfg.Provider {
	case "openai":
		adapter, err = newOpenAI(cfg)
	case "ollama":
		adapter = newOllama(cfg)
	default:
		return nil, fmt.Errorf("unknown embedding provider %q (supported: openai, ollama)", cfg.Provider)
	}
	if err != nil {
		return nil, err
	}

	embedder := pkgembed.Bounded(adapter, resolveTimeout(cfg.Timeout))
	if isRemote(cfg.Provider) {
		rps := cfg.RateLimitRPS
		if rps == 0 {
			rps = providerDefaultRPS(cfg.Provider, cfg.Model)
		}
		if rps > 0 {
			embedder = pkgembed.RateLimited(embedder, rps)
		}
	}
	return embedder, nil
}

// BatchSize resolves the number of texts one request carries: cfg.BatchSize
// when positive, else the provider default. The composition site hands it to
// embed.Batched, and the CLI indexer buckets its work by it so a bucket is at
// most one request.
func BatchSize(cfg model.EmbeddingConfig) int {
	if cfg.BatchSize > 0 {
		return cfg.BatchSize
	}
	switch cfg.Provider {
	case "ollama":
		return defaultOllamaBatchSize
	default:
		return defaultOpenAIBatchSize
	}
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

// templateFor selects the configured template for the request's purpose.
func templateFor(purpose pkgembed.Purpose, document, query string) (string, error) {
	switch purpose {
	case pkgembed.PurposeDocument:
		return document, nil
	case pkgembed.PurposeQuery:
		return query, nil
	default:
		return "", fmt.Errorf("unknown embedding purpose %q", purpose)
	}
}
