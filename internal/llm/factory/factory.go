// Package factory is the local host's llm.Runner composition: it resolves
// model.LLMConfig — a host-private config schema that never goes public — into
// one composed pkg/llm Runner (provider adapter, rate-limit decorator, timeout
// decorator).
package factory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/time/rate"

	internalllm "github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/claude"
	gollmrunner "github.com/networkteam/sdd/internal/llm/gollm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

// defaultTimeout bounds each call when neither config nor caller sets one.
// The factory always composes a timeout: an unbounded runner turns a hung
// provider into a hung process.
const defaultTimeout = 2 * time.Minute

// New builds a composed llm.Runner from config. Provider and model fall back
// to model.DefaultLLMProvider / model.DefaultLLMModel when empty. Remote
// providers (anthropic, openai) get wrapped with a rate.Limiter: an
// explicit cfg.RateLimitRPS takes precedence, otherwise a conservative
// tier-1-safe default is selected per provider/model family (see
// providerDefaultRPS). Local providers (claude-cli, ollama) stay
// uncapped. Every runner is bounded by the configured cfg.Timeout (falling
// back to defaultTimeout) — deadlines are configuration, so they compose here
// rather than being defaulted by any consumer. Errors distinguish
// configuration problems from transport failures so the CLI can surface them
// distinctly.
func New(cfg model.LLMConfig) (llm.Runner, error) {
	runner, err := compose(cfg)
	if err != nil {
		return nil, err
	}

	timeout := defaultTimeout
	if cfg.Timeout != "" {
		d, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("llm: parsing timeout %q: %w", cfg.Timeout, err)
		}
		if d > 0 {
			timeout = d
		}
	}
	return internalllm.Bounded(runner, timeout), nil
}

// compose builds the provider adapter and its rate-limit wrap — everything
// but the timeout bound New always adds.
func compose(cfg model.LLMConfig) (llm.Runner, error) {
	provider := cfg.Provider
	if provider == "" {
		provider = model.DefaultLLMProvider
	}
	if cfg.Model == "" {
		cfg.Model = model.DefaultLLMModel
	}
	cfg.Provider = provider

	runner, err := buildProvider(cfg)
	if err != nil {
		return nil, err
	}

	if isRemote(provider) {
		rps := cfg.RateLimitRPS
		if rps == 0 {
			rps = providerDefaultRPS(provider, cfg.Model)
		}
		if rps > 0 {
			runner = newRateLimited(runner, rps)
		}
	}
	return runner, nil
}

// providerDefaultRPS returns a conservative, tier-1-safe RPS default for a
// given provider/model. Numbers bias below each tier-1 mathematical ceiling
// so bursty batch operations (e.g. sdd summarize --all) don't trip 429s
// out of the box. Returns 0 when no default applies (caller decides).
//
// Anthropic tier 1 (shared family limits): Opus 50 RPM, Sonnet 100 RPM,
// Haiku 200 RPM. OpenAI tier 1 varies per model; cheap mini/nano families
// get higher throughput than frontier models. Users on higher tiers
// override via the llm.rate_limit_rps config setting.
func providerDefaultRPS(provider, modelName string) float64 {
	name := strings.ToLower(modelName)
	switch provider {
	case "anthropic":
		switch {
		case strings.Contains(name, "opus"):
			return 0.5
		case strings.Contains(name, "haiku"):
			return 2.0
		default:
			// Sonnet or unknown — middle-ground default.
			return 1.0
		}
	case "openai":
		switch {
		case strings.Contains(name, "mini"), strings.Contains(name, "nano"):
			return 2.0
		default:
			return 1.0
		}
	default:
		return 0
	}
}

func buildProvider(cfg model.LLMConfig) (llm.Runner, error) {
	switch cfg.Provider {
	case "claude-cli":
		return claude.NewRunner(cfg.Model), nil
	case "anthropic", "openai", "ollama":
		return gollmrunner.NewRunner(cfg)
	default:
		return nil, fmt.Errorf("unknown llm provider %q (supported: claude-cli, anthropic, openai, ollama)", cfg.Provider)
	}
}

func isRemote(provider string) bool {
	switch provider {
	case "anthropic", "openai":
		return true
	default:
		return false
	}
}

// rateLimited wraps a Runner with a token-bucket limiter so parallel
// batch operations don't exceed provider rate limits.
type rateLimited struct {
	inner   llm.Runner
	limiter *rate.Limiter
}

func newRateLimited(inner llm.Runner, rps float64) llm.Runner {
	// Burst equals ceiling of 1s worth of requests — small bursts are
	// fine, sustained rate is the constraint.
	burst := int(rps)
	if burst < 1 {
		burst = 1
	}
	return &rateLimited{
		inner:   inner,
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

func (r *rateLimited) Run(ctx context.Context, req llm.Request) (llm.Result, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return llm.Result{}, fmt.Errorf("rate limiter: %w", err)
	}
	return r.inner.Run(ctx, req)
}
