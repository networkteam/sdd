// Package factory resolves an llm.Runner from model.LLMConfig. It lives in
// a sub-package to break the import cycle: internal/llm defines the Runner
// interface consumed by the claude and gollm sub-packages, and the factory
// sits above all three to dispatch and wrap.
package factory

import (
	"context"
	"fmt"

	"golang.org/x/time/rate"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/claude"
	gollmrunner "github.com/networkteam/sdd/internal/llm/gollm"
	"github.com/networkteam/sdd/internal/model"
)

// New builds an llm.Runner from config. Provider and model fall back to
// model.DefaultLLMProvider / model.DefaultLLMModel when empty. Remote
// providers (anthropic, openai) get wrapped with a rate.Limiter when
// cfg.RateLimitRPS is positive; local providers (claude-cli, ollama) are
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

	if isRemote(provider) && cfg.RateLimitRPS > 0 {
		runner = newRateLimited(runner, cfg.RateLimitRPS)
	}

	return runner, nil
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
