package factory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

func TestNew_ClaudeCLIDefault(t *testing.T) {
	// Empty config → claude-cli default, always constructs without error.
	r, err := New(model.LLMConfig{})
	if err != nil {
		t.Fatalf("New(empty): %v", err)
	}
	if r == nil {
		t.Fatal("New(empty) returned nil runner")
	}
}

func TestNew_ClaudeCLIExplicit(t *testing.T) {
	r, err := New(model.LLMConfig{Provider: "claude-cli", Model: "custom-model"})
	if err != nil {
		t.Fatalf("New(claude-cli): %v", err)
	}
	if r == nil {
		t.Fatal("claude-cli runner must not be nil")
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	_, err := New(model.LLMConfig{Provider: "made-up"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown llm provider") {
		t.Errorf("error should mention unknown provider, got %v", err)
	}
}

func TestNew_RemoteProviderMissingAPIKey(t *testing.T) {
	for _, provider := range []string{"anthropic", "openai"} {
		t.Run(provider, func(t *testing.T) {
			_, err := New(model.LLMConfig{Provider: provider, Model: "m"})
			if err == nil {
				t.Fatalf("expected error when api key missing for %s", provider)
			}
			if !strings.Contains(err.Error(), "api key missing") {
				t.Errorf("error should mention missing api key, got %v", err)
			}
		})
	}
}

func TestNew_RemoteProviderWithRateLimit(t *testing.T) {
	// Remote provider with rate_limit_rps set → wrapped in rateLimited.
	// gollm validates the API key format at construction time: anthropic
	// keys need sk-ant- prefix and length > 20.
	r, err := New(model.LLMConfig{
		Provider:     "anthropic",
		Model:        "claude-3-5-sonnet",
		APIKeys:      map[string]string{"anthropic": "sk-ant-testkey-aaaaaaaaaaaaaaaaaaaa"},
		RateLimitRPS: 4,
	})
	if err != nil {
		t.Fatalf("New(anthropic): %v", err)
	}
	if _, ok := r.(*rateLimited); !ok {
		t.Errorf("expected *rateLimited wrapper for remote provider with RateLimitRPS > 0, got %T", r)
	}
}

func TestNew_RemoteProviderAppliesDefaultRateLimit(t *testing.T) {
	// No explicit RateLimitRPS → conservative per-model default should apply
	// so tier-1 users on Anthropic/OpenAI don't immediately hit 429s.
	r, err := New(model.LLMConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
		APIKeys:  map[string]string{"anthropic": "sk-ant-testkey-aaaaaaaaaaaaaaaaaaaa"},
	})
	if err != nil {
		t.Fatalf("New(anthropic): %v", err)
	}
	if _, ok := r.(*rateLimited); !ok {
		t.Errorf("remote runner with RateLimitRPS=0 must be wrapped with provider default, got %T", r)
	}
}

func TestProviderDefaultRPS(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		want     float64
	}{
		// Anthropic families — shared limits per family, so name match
		// drives the default regardless of minor version.
		{"anthropic", "claude-opus-4-7", 0.5},
		{"anthropic", "claude-3-opus-latest", 0.5},
		{"anthropic", "claude-sonnet-4-6", 1.0},
		{"anthropic", "claude-3-5-sonnet-20241022", 1.0},
		{"anthropic", "claude-haiku-4-5-20251001", 2.0},
		// Unknown Anthropic model → safe middle-ground (Sonnet equivalent).
		{"anthropic", "claude-future-edition", 1.0},

		// OpenAI — mini/nano families get higher throughput, frontier is 1.0.
		{"openai", "gpt-5", 1.0},
		{"openai", "gpt-5-mini", 2.0},
		{"openai", "gpt-5-nano", 2.0},
		{"openai", "o3", 1.0},

		// Local / unknown → zero (no wrap).
		{"claude-cli", "anything", 0},
		{"ollama", "llama3", 0},
		{"made-up", "x", 0},
	}

	for _, c := range cases {
		got := providerDefaultRPS(c.provider, c.model)
		if got != c.want {
			t.Errorf("providerDefaultRPS(%q, %q) = %v; want %v", c.provider, c.model, got, c.want)
		}
	}
}

func TestNew_RemoteProviderExplicitOverridesDefault(t *testing.T) {
	// Explicit RateLimitRPS takes precedence over the per-model default —
	// higher-tier users can dial up throughput.
	r, err := New(model.LLMConfig{
		Provider:     "anthropic",
		Model:        "claude-opus-4-7", // default would be 0.5
		APIKeys:      map[string]string{"anthropic": "sk-ant-testkey-aaaaaaaaaaaaaaaaaaaa"},
		RateLimitRPS: 10,
	})
	if err != nil {
		t.Fatalf("New(anthropic): %v", err)
	}
	wrapped, ok := r.(*rateLimited)
	if !ok {
		t.Fatalf("expected *rateLimited wrapper, got %T", r)
	}
	// rate.Limiter doesn't expose its rate directly, but the burst size is
	// derived from the RPS, so we can use it as an indirect signal that
	// the explicit value (10) won rather than the default (0.5).
	if wrapped.limiter.Burst() != 10 {
		t.Errorf("expected burst 10 from explicit RateLimitRPS, got %d (default would be 1)", wrapped.limiter.Burst())
	}
}

// fakeRunner is a minimal llm.Runner for testing the decorator pipeline.
type fakeRunner struct {
	calls int
}

func (f *fakeRunner) Run(_ context.Context, _ llm.Request) (*llm.RunResult, error) {
	f.calls++
	return &llm.RunResult{Text: "ok"}, nil
}

func TestRateLimited_CallsInner(t *testing.T) {
	inner := &fakeRunner{}
	wrapped := newRateLimited(inner, 100) // generous rate; test passes instantly
	out, err := wrapped.Run(context.Background(), llm.Request{UserPrompt: "hi"})
	if err != nil {
		t.Fatalf("rateLimited Run: %v", err)
	}
	if out.Text != "ok" {
		t.Errorf("expected inner runner output, got %q", out.Text)
	}
	if inner.calls != 1 {
		t.Errorf("expected inner runner called once, got %d", inner.calls)
	}
}

func TestRateLimited_RespectsCancelledContext(t *testing.T) {
	inner := &fakeRunner{}
	// Very slow rate so Wait would block — we cancel before any slot opens.
	wrapped := newRateLimited(inner, 0.01)
	// Drain the initial burst slot so the next Run must wait.
	_, _ = wrapped.Run(context.Background(), llm.Request{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := wrapped.Run(ctx, llm.Request{})
	if err == nil {
		t.Fatal("expected error when context is cancelled before rate slot opens")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rate limiter") {
		t.Errorf("error should signal rate limiter / cancellation, got %v", err)
	}
}
