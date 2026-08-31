package gollm

import (
	"strings"
	"testing"

	upstream "github.com/teilomillet/gollm"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

func TestNewRunner_MissingProvider(t *testing.T) {
	_, err := NewRunner(model.LLMConfig{})
	if err == nil {
		t.Fatal("expected error when provider is empty")
	}
	if !strings.Contains(err.Error(), "provider not configured") {
		t.Errorf("error should mention provider not configured, got %v", err)
	}
}

func TestNewRunner_MissingModel(t *testing.T) {
	_, err := NewRunner(model.LLMConfig{Provider: "anthropic"})
	if err == nil {
		t.Fatal("expected error when model is empty")
	}
	if !strings.Contains(err.Error(), "model not configured") {
		t.Errorf("error should mention model not configured, got %v", err)
	}
}

func TestNewRunner_AnthropicMissingAPIKey(t *testing.T) {
	_, err := NewRunner(model.LLMConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
	})
	if err == nil {
		t.Fatal("expected error when api key missing for anthropic")
	}
	if !strings.Contains(err.Error(), "api key missing") {
		t.Errorf("error should mention api key missing, got %v", err)
	}
}

func TestNewRunner_AnthropicEnablesCaching(t *testing.T) {
	// gollm enforces an API key format: anthropic keys need sk-ant- prefix
	// and length > 20. Use a syntactically valid fake key here.
	r, err := NewRunner(model.LLMConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
		APIKeys:  map[string]string{"anthropic": "sk-ant-testkey-aaaaaaaaaaaaaaaaaaaa"},
	})
	if err != nil {
		t.Fatalf("NewRunner(anthropic): %v", err)
	}
	if !r.useCache {
		t.Error("anthropic runner must have caching enabled for ephemeral system prompt")
	}
}

func TestNewRunner_BadTimeout(t *testing.T) {
	// Use anthropic with a valid-format key so the only error source is
	// the malformed timeout string.
	_, err := NewRunner(model.LLMConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
		APIKeys:  map[string]string{"anthropic": "sk-ant-testkey-aaaaaaaaaaaaaaaaaaaa"},
		Timeout:  "not-a-duration",
	})
	if err == nil {
		t.Fatal("expected error on unparseable timeout")
	}
	if !strings.Contains(err.Error(), "parsing timeout") {
		t.Errorf("error should mention timeout parsing, got %v", err)
	}
}

func TestUsageFromResponse(t *testing.T) {
	// No usage reported means no usage figures — never a fabricated one.
	// Attribution does not travel here at all; it comes from the Result's
	// Identity.
	if got := usageFromResponse(nil); got != (llm.Usage{}) {
		t.Errorf("nil usage should yield zero usage, got %+v", got)
	}

	// Anthropic-style fields, including the prompt-cache read breakdown.
	u := usageFromResponse(&upstream.Usage{
		InputTokens:          6341,
		OutputTokens:         8,
		CacheReadInputTokens: 6163,
	})
	if u.InputTokens != 6341 || u.OutputTokens != 8 || u.CacheReadTokens != 6163 {
		t.Errorf("anthropic mapping wrong: %+v", u)
	}

	// OpenAI-style counts fall back to input/output.
	u2 := usageFromResponse(&upstream.Usage{PromptTokens: 100, CompletionTokens: 20})
	if u2.InputTokens != 100 || u2.OutputTokens != 20 {
		t.Errorf("openai fallback wrong: %+v", u2)
	}
}

func TestCanonicalVariant(t *testing.T) {
	if got := canonicalVariant(nil); got != "" {
		t.Errorf("no params should yield no variant, got %q", got)
	}
	// Sorted by key so the same configuration always groups to the same row,
	// whatever order the config file listed it in.
	got := canonicalVariant(map[string]string{"verbosity": "low", "reasoning_effort": "high"})
	if got != "reasoning_effort=high,verbosity=low" {
		t.Errorf("variant = %q, want reasoning_effort=high,verbosity=low", got)
	}
}
