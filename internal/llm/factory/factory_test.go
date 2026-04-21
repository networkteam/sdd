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

func TestNew_RemoteProviderWithoutRateLimit(t *testing.T) {
	// Same valid config but no rate limit — should not be wrapped.
	r, err := New(model.LLMConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
		APIKeys:  map[string]string{"anthropic": "sk-ant-testkey-aaaaaaaaaaaaaaaaaaaa"},
	})
	if err != nil {
		t.Fatalf("New(anthropic): %v", err)
	}
	if _, ok := r.(*rateLimited); ok {
		t.Error("remote runner with RateLimitRPS=0 must not be wrapped")
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
