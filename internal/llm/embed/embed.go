// Package embed is the local host's embed.Embedder composition: it resolves
// model.EmbeddingConfig — a host-private schema that never goes public — into
// one pkg/llm/embed Embedder (provider adapter plus rate-limit decorator),
// paralleling the chat-runner factory.
package embed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
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

// New constructs an Embedder from cfg. Returns an error when Provider is
// unrecognised or required fields (Model, API key for remote providers) are
// missing. Remote providers ("openai") are wrapped with embed.RateLimited
// using cfg.RateLimitRPS or a conservative provider default.
//
// The per-round-trip deadline (cfg.Timeout) lives in the adapters' HTTP
// client rather than in embed.Bounded: one Embed spans as many round-trips as
// its batch needs, and a cold index reconcile hands thousands of chunks to a
// single call, so a whole-call bound would fail exactly the calls that take
// longest legitimately.
func New(cfg model.EmbeddingConfig) (pkgembed.Embedder, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("no embedding provider configured — run `sdd config set embedding.provider <provider>`")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("embedding.model is required")
	}

	timeout := resolveTimeout(cfg.Timeout)
	batchSize := BatchSize(cfg)

	var inner pkgembed.Embedder
	var err error
	switch cfg.Provider {
	case "openai":
		inner, err = newOpenAI(cfg, timeout, batchSize)
	case "ollama":
		inner = newOllama(cfg, timeout, batchSize)
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
			inner = pkgembed.RateLimited(inner, rps)
		}
	}
	return inner, nil
}

// BatchSize resolves the number of inputs one transport round-trip carries:
// cfg.BatchSize when positive, else the provider default. The adapters split
// larger requests by it internally; the CLI indexer buckets its work by it so
// progress advances once per round-trip.
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

// batched runs call over texts in batchSize-sized slices — one transport
// round-trip each — concatenating vectors in input order and summing the
// reported input tokens. A failure is attributed to identity so the observing
// decorator can record it.
func batched(ctx context.Context, identity llm.Identity, texts []string, batchSize int,
	call func(ctx context.Context, texts []string) ([][]float32, int, error)) (pkgembed.Result, error) {
	result := pkgembed.Result{Identity: identity}
	if len(texts) == 0 {
		return result, nil
	}
	result.Vectors = make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := min(start+batchSize, len(texts))
		vectors, tokens, err := call(ctx, texts[start:end])
		if err != nil {
			return pkgembed.Result{}, &llm.Error{Identity: identity, Err: fmt.Errorf("%s batch [%d:%d]: %w", identity.Provider, start, end, err)}
		}
		result.Vectors = append(result.Vectors, vectors...)
		result.Usage.InputTokens += tokens
	}
	return result, nil
}
