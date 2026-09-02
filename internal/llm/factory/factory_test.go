package factory

import (
	"strings"
	"testing"

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

func TestNew_InvalidTimeout(t *testing.T) {
	_, err := New(model.LLMConfig{Timeout: "soon"})
	if err == nil || !strings.Contains(err.Error(), "parsing timeout") {
		t.Fatalf("expected timeout parse error, got %v", err)
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

func TestCompose_RemoteProviderBuilds(t *testing.T) {
	// gollm validates the API key format at construction time: anthropic
	// keys need sk-ant- prefix and length > 20.
	r, err := compose(model.LLMConfig{
		Provider:     "anthropic",
		Model:        "claude-3-5-sonnet",
		APIKeys:      map[string]string{"anthropic": "sk-ant-testkey-aaaaaaaaaaaaaaaaaaaa"},
		RateLimitRPS: 4,
	})
	if err != nil {
		t.Fatalf("compose(anthropic): %v", err)
	}
	if r == nil {
		t.Fatal("compose returned nil runner")
	}
}

func TestEffectiveRPS(t *testing.T) {
	// Remote providers are always capped: an explicit value wins, otherwise
	// the per-model default applies so tier-1 users don't trip 429s.
	if got := effectiveRPS(model.LLMConfig{Provider: "anthropic", Model: "claude-opus-4-7"}); got != 0.5 {
		t.Errorf("default for opus = %v, want 0.5", got)
	}
	if got := effectiveRPS(model.LLMConfig{Provider: "anthropic", Model: "claude-opus-4-7", RateLimitRPS: 10}); got != 10 {
		t.Errorf("explicit RateLimitRPS = %v, want 10", got)
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
