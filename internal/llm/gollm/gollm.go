// Package gollm implements llm.Runner on top of github.com/teilomillet/gollm,
// providing a unified adapter for Anthropic API, OpenAI, Ollama, and other
// providers supported by gollm. This is the opt-in alternative to the
// claude-cli bridge; selected via .sdd/config.local.yaml llm.provider.
package gollm

import (
	"context"
	"fmt"
	"time"

	upstream "github.com/teilomillet/gollm"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

// Runner wraps a gollm client and adapts it to llm.Runner. When the provider
// is Anthropic, the SystemPrompt is passed with ephemeral cache control so
// the stable prefix is server-cached across calls in the same 5-minute
// window — a significant speedup for batch operations like
// sdd summarize --all.
type Runner struct {
	client   upstream.LLM
	provider string
	useCache bool
}

// NewRunner constructs a gollm-backed Runner from an LLMConfig. Provider
// must be one of the gollm-supported providers (e.g. "anthropic", "openai",
// "ollama"). Returns a typed error when required config is missing (API
// key for remote providers).
func NewRunner(cfg model.LLMConfig) (*Runner, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("gollm: provider not configured")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("gollm: model not configured for provider %q", cfg.Provider)
	}

	opts := []upstream.ConfigOption{
		upstream.SetProvider(cfg.Provider),
		upstream.SetModel(cfg.Model),
	}

	// API key is required for remote providers. Ollama uses a local endpoint
	// and does not need one.
	if needsAPIKey(cfg.Provider) {
		key := cfg.APIKeys[cfg.Provider]
		if key == "" {
			return nil, fmt.Errorf("gollm: api key missing for provider %q (set llm.api_keys.%s in .sdd/config.local.yaml)", cfg.Provider, cfg.Provider)
		}
		opts = append(opts, upstream.SetAPIKey(key))
	}

	// Ollama endpoint override.
	if cfg.Provider == "ollama" && cfg.OllamaEndpoint != "" {
		opts = append(opts, upstream.SetOllamaEndpoint(cfg.OllamaEndpoint))
	}

	// Enable Anthropic prompt caching — sends the anthropic-beta header so
	// cache_control blocks on system prompts are honored server-side.
	useCache := cfg.Provider == "anthropic"
	if useCache {
		opts = append(opts, upstream.SetEnableCaching(true))
	}

	// Timeout flows into gollm's HTTP client.
	if cfg.Timeout != "" {
		d, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("gollm: parsing timeout %q: %w", cfg.Timeout, err)
		}
		opts = append(opts, upstream.SetTimeout(d))
	}

	client, err := upstream.NewLLM(opts...)
	if err != nil {
		return nil, fmt.Errorf("gollm: creating client: %w", err)
	}

	return &Runner{
		client:   client,
		provider: cfg.Provider,
		useCache: useCache,
	}, nil
}

// Run sends the request to the underlying gollm client. When caching is
// enabled (Anthropic), the SystemPrompt is tagged ephemeral so the server
// caches the prefix. Other providers ignore the cache hint but still see
// the split system/user message pair.
func (r *Runner) Run(ctx context.Context, req llm.Request) (*llm.RunResult, error) {
	var opts []upstream.PromptOption
	if req.SystemPrompt != "" {
		cacheType := upstream.CacheType("")
		if r.useCache {
			cacheType = upstream.CacheTypeEphemeral
		}
		opts = append(opts, upstream.WithSystemPrompt(req.SystemPrompt, cacheType))
	}

	prompt := upstream.NewPrompt(req.UserPrompt, opts...)

	text, err := r.client.Generate(ctx, prompt)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("gollm %s: timed out", r.provider)
		}
		return nil, fmt.Errorf("gollm %s: %w", r.provider, err)
	}

	// gollm's Generate returns text only; Meta stays nil — token/cost
	// metrics are not surfaced through this path. logCallResult handles
	// nil Meta gracefully.
	return &llm.RunResult{Text: text}, nil
}

// needsAPIKey reports whether the provider requires an API key configured
// in APIKeys. Local providers (ollama) do not.
func needsAPIKey(provider string) bool {
	switch provider {
	case "ollama":
		return false
	default:
		return true
	}
}
