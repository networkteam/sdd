// Package gollm implements llm.Runner on top of github.com/teilomillet/gollm,
// providing a unified adapter for Anthropic API, OpenAI, Ollama, and other
// providers supported by gollm. This is the opt-in alternative to the
// claude-cli bridge; selected via the llm.provider config setting.
package gollm

import (
	"context"
	"fmt"
	"strings"
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
	model    string
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

	// Resolve the retry policy (provider-aware backoff lives in the gollm
	// fork). We previously forced SetMaxRetries(0) because gollm's stock
	// retry logged every attempt at WARN and flooded batch summarize; the
	// fork classifies transient vs permanent errors, honors server-provided
	// Retry-After, and logs attempts at Debug — so retries are now safe to
	// enable. gollm counts retries *after* the first attempt, hence -1.
	maxAttempts, baseDelay, maxDelay, err := cfg.Retry.Resolved()
	if err != nil {
		return nil, fmt.Errorf("gollm: %w", err)
	}

	opts := []upstream.ConfigOption{
		upstream.SetProvider(cfg.Provider),
		upstream.SetModel(cfg.Model),
		upstream.SetMaxRetries(maxAttempts - 1),
		upstream.SetRetryDelay(baseDelay),
		upstream.SetMaxRetryDelay(maxDelay),
		// Gollm's default MaxTokens (256) truncates longer summaries and
		// pre-flight JSON responses mid-sentence. 4096 covers summaries
		// (~150 tokens), pre-flight findings arrays, and has headroom for
		// richer prompts without being wasteful.
		upstream.SetMaxTokens(4096),
	}

	// API key is required for remote providers. Ollama uses a local endpoint
	// and does not need one.
	if needsAPIKey(cfg.Provider) {
		key := cfg.APIKeys[cfg.Provider]
		if key == "" {
			return nil, fmt.Errorf("gollm: api key missing for provider %q — run `sdd config set llm.api_keys.%s <key>`", cfg.Provider, cfg.Provider)
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
		// gollm validates ollama by probing the endpoint; on failure it
		// reports a generic apikey validation error. Rewrap as a targeted
		// "unreachable" message so users see the actionable hint.
		if cfg.Provider == "ollama" && strings.Contains(err.Error(), "apikey") {
			endpoint := cfg.OllamaEndpoint
			if endpoint == "" {
				endpoint = "http://localhost:11434"
			}
			return nil, fmt.Errorf("gollm ollama: provider unreachable at %s (is Ollama running?)", endpoint)
		}
		return nil, fmt.Errorf("gollm: creating client: %w", err)
	}

	return &Runner{
		client:   client,
		provider: cfg.Provider,
		model:    cfg.Model,
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

	resp, err := r.client.GenerateWithUsage(ctx, prompt)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("gollm %s: timed out", r.provider)
		}
		return nil, fmt.Errorf("gollm %s: %w", r.provider, err)
	}

	// The fork surfaces per-call usage (tokens + prompt-cache read/creation)
	// via GenerateWithUsage; map it into the agent-neutral LLMMetadata so the
	// gollm path stops returning nil Meta. Cost is not reported by the
	// provider APIs (it was a claude-cli extra), so it is left zero.
	return &llm.RunResult{Text: resp.Content, Meta: metaFromUsage(resp.Usage, r.model, r.provider)}, nil
}

// metaFromUsage maps a gollm Response.Usage into the agent-neutral
// llm.LLMMetadata. It prefers the Anthropic-style input/output counts and falls
// back to the OpenAI-style prompt/completion counts. Returns nil when the
// provider reported no usage (so logging and the stats sink skip the call).
func metaFromUsage(u *upstream.Usage, model, provider string) *llm.LLMMetadata {
	if u == nil {
		return nil
	}
	in := u.InputTokens
	if in == 0 {
		in = u.PromptTokens
	}
	out := u.OutputTokens
	if out == 0 {
		out = u.CompletionTokens
	}
	mu := llm.ModelUsage{
		InputTokens:       in,
		OutputTokens:      out,
		CacheReadTokens:   u.CacheReadInputTokens,
		CacheCreateTokens: u.CacheCreationInputTokens,
	}
	return &llm.LLMMetadata{
		Provider:          provider,
		InputTokens:       in,
		OutputTokens:      out,
		CacheReadTokens:   u.CacheReadInputTokens,
		CacheCreateTokens: u.CacheCreationInputTokens,
		Models:            map[string]llm.ModelUsage{model: mu},
	}
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
