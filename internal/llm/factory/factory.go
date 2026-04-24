// Package factory resolves an llm.Runner from model.LLMConfig. It lives in
// a sub-package to break the import cycle: internal/llm defines the Runner
// interface consumed by the claude and gollm sub-packages, and the factory
// sits above all three to dispatch and wrap.
package factory

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/time/rate"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/claude"
	gollmrunner "github.com/networkteam/sdd/internal/llm/gollm"
	"github.com/networkteam/sdd/internal/model"
)

// New builds an llm.Runner from config. Provider and model fall back to
// model.DefaultLLMProvider / model.DefaultLLMModel when empty. Remote
// providers (anthropic, openai) get wrapped with a rate.Limiter: an
// explicit cfg.RateLimitRPS takes precedence, otherwise a conservative
// tier-1-safe default is selected per provider/model family (see
// providerDefaultRPS). Local providers (claude-cli, ollama) stay
// uncapped. Errors distinguish configuration problems from transport
// failures so the CLI can surface them distinctly.
func New(cfg model.LLMConfig) (llm.Runner, error) {
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
// override via llm.rate_limit_rps in .sdd/config.local.yaml.
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

func (r *rateLimited) Run(ctx context.Context, req llm.Request) (*llm.RunResult, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}
	return r.inner.Run(ctx, req)
}
