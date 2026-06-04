package gollm

import (
	"strings"
	"testing"

	upstream "github.com/teilomillet/gollm"

	"github.com/networkteam/sdd/internal/model"
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

func TestMetaFromUsage(t *testing.T) {
	if metaFromUsage(nil, "m", "anthropic") != nil {
		t.Error("nil usage should yield nil meta")
	}

	// Anthropic-style fields, including the prompt-cache read breakdown.
	m := metaFromUsage(&upstream.Usage{
		InputTokens:          6341,
		OutputTokens:         8,
		CacheReadInputTokens: 6163,
	}, "claude-sonnet-4-6", "anthropic")
	if m == nil {
		t.Fatal("expected non-nil meta")
	}
	if m.InputTokens != 6341 || m.OutputTokens != 8 || m.CacheReadTokens != 6163 {
		t.Errorf("anthropic mapping wrong: %+v", m)
	}
	if m.Provider != "anthropic" {
		t.Errorf("provider not propagated: %+v", m)
	}
	if mu, ok := m.Models["claude-sonnet-4-6"]; !ok || mu.CacheReadTokens != 6163 {
		t.Errorf("per-model usage wrong: %+v", m.Models)
	}

	// OpenAI-style counts fall back to input/output.
	m2 := metaFromUsage(&upstream.Usage{PromptTokens: 100, CompletionTokens: 20}, "gpt", "openai")
	if m2.InputTokens != 100 || m2.OutputTokens != 20 {
		t.Errorf("openai fallback wrong: %+v", m2)
	}
}
