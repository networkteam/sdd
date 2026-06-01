package model

import (
	"testing"
	"time"
)

func TestMergeConfig_EmptyOverlayPreservesBase(t *testing.T) {
	base := &Config{
		GraphDir: ".sdd/graph",
		LLM: LLMConfig{
			Provider:    "claude-cli",
			Model:       "claude-haiku-4-5-20251001",
			Concurrency: 4,
		},
	}
	got := MergeConfig(base, &Config{})
	if got.GraphDir != ".sdd/graph" || got.LLM.Provider != "claude-cli" || got.LLM.Concurrency != 4 {
		t.Errorf("empty overlay should preserve base, got %+v", got)
	}
}

func TestMergeConfig_NonEmptyOverlayOverrides(t *testing.T) {
	base := &Config{
		LLM: LLMConfig{Provider: "claude-cli", Model: "claude-haiku-4-5-20251001", Concurrency: 4},
	}
	overlay := &Config{
		LLM: LLMConfig{Provider: "ollama", Model: "llama3.1:70b"},
	}
	got := MergeConfig(base, overlay)
	if got.LLM.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", got.LLM.Provider)
	}
	if got.LLM.Model != "llama3.1:70b" {
		t.Errorf("Model = %q, want llama3.1:70b", got.LLM.Model)
	}
	if got.LLM.Concurrency != 4 {
		t.Errorf("Concurrency inherited from base = %d, want 4", got.LLM.Concurrency)
	}
}

func TestMergeConfig_APIKeysMerge(t *testing.T) {
	base := &Config{
		LLM: LLMConfig{APIKeys: map[string]string{"anthropic": "base-key", "openai": "base-openai"}},
	}
	overlay := &Config{
		LLM: LLMConfig{APIKeys: map[string]string{"anthropic": "local-key", "ollama": "local-ollama"}},
	}
	got := MergeConfig(base, overlay)
	if got.LLM.APIKeys["anthropic"] != "local-key" {
		t.Errorf("overlay should override anthropic key, got %q", got.LLM.APIKeys["anthropic"])
	}
	if got.LLM.APIKeys["openai"] != "base-openai" {
		t.Errorf("base openai key should be preserved, got %q", got.LLM.APIKeys["openai"])
	}
	if got.LLM.APIKeys["ollama"] != "local-ollama" {
		t.Errorf("overlay should add ollama key, got %q", got.LLM.APIKeys["ollama"])
	}
	// Base must not be mutated.
	if len(base.LLM.APIKeys) != 2 || base.LLM.APIKeys["anthropic"] != "base-key" {
		t.Errorf("MergeConfig mutated base APIKeys: %+v", base.LLM.APIKeys)
	}
}

func TestMergeConfig_NilOverlay(t *testing.T) {
	base := &Config{LLM: LLMConfig{Provider: "claude-cli"}}
	got := MergeConfig(base, nil)
	if got.LLM.Provider != "claude-cli" {
		t.Errorf("nil overlay should return copy of base, got %+v", got)
	}
	// Ensure it's a copy, not the same pointer.
	got.LLM.Provider = "mutated"
	if base.LLM.Provider != "claude-cli" {
		t.Error("MergeConfig returned a pointer to base; must return a copy")
	}
}

func TestMergeConfig_NilBase(t *testing.T) {
	got := MergeConfig(nil, &Config{LLM: LLMConfig{Provider: "ollama"}})
	if got == nil || got.LLM.Provider != "ollama" {
		t.Errorf("nil base with overlay should produce overlay values, got %+v", got)
	}
}

func TestMergeConfig_ParticipantOverlay(t *testing.T) {
	base := &Config{}
	overlay := &Config{Participant: "Christopher"}
	got := MergeConfig(base, overlay)
	if got.Participant != "Christopher" {
		t.Errorf("Participant = %q, want Christopher", got.Participant)
	}
}

func TestMergeConfig_SyncOverlay(t *testing.T) {
	base := &Config{Sync: SyncConfig{Cooldown: "5m"}}
	overlay := &Config{Sync: SyncConfig{Cooldown: "1h"}}
	got := MergeConfig(base, overlay)
	if got.Sync.Cooldown != "1h" {
		t.Errorf("Sync.Cooldown = %q, want 1h", got.Sync.Cooldown)
	}

	// Empty overlay preserves base.
	got = MergeConfig(base, &Config{})
	if got.Sync.Cooldown != "5m" {
		t.Errorf("empty overlay should preserve base Sync.Cooldown = %q", got.Sync.Cooldown)
	}
}

func TestRetryConfig_Resolved(t *testing.T) {
	// Empty config yields the baked defaults.
	maxAttempts, baseD, maxD, err := RetryConfig{}.Resolved()
	if err != nil {
		t.Fatalf("default resolve errored: %v", err)
	}
	if maxAttempts != DefaultRetryMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", maxAttempts, DefaultRetryMaxAttempts)
	}
	if baseD != 2*time.Second || maxD != 60*time.Second {
		t.Errorf("default delays = (%v, %v), want (2s, 60s)", baseD, maxD)
	}

	// Explicit values are honored.
	maxAttempts, baseD, maxD, err = RetryConfig{MaxAttempts: 3, BaseDelay: "500ms", MaxDelay: "10s"}.Resolved()
	if err != nil {
		t.Fatalf("explicit resolve errored: %v", err)
	}
	if maxAttempts != 3 || baseD != 500*time.Millisecond || maxD != 10*time.Second {
		t.Errorf("explicit resolve = (%d, %v, %v), want (3, 500ms, 10s)", maxAttempts, baseD, maxD)
	}

	// A non-positive MaxAttempts falls back to the default.
	if got, _, _, _ := (RetryConfig{MaxAttempts: -1}).Resolved(); got != DefaultRetryMaxAttempts {
		t.Errorf("negative MaxAttempts = %d, want default %d", got, DefaultRetryMaxAttempts)
	}

	// Invalid durations surface an error rather than degrading silently.
	if _, _, _, err := (RetryConfig{BaseDelay: "soon"}).Resolved(); err == nil {
		t.Error("invalid base_delay should error")
	}
	if _, _, _, err := (RetryConfig{MaxDelay: "later"}).Resolved(); err == nil {
		t.Error("invalid max_delay should error")
	}
}

func TestMergeConfig_RetryOverlay(t *testing.T) {
	base := &Config{LLM: LLMConfig{Retry: RetryConfig{MaxAttempts: 5, BaseDelay: "2s", MaxDelay: "60s"}}}

	// Overlay overrides only the fields it sets.
	overlay := &Config{LLM: LLMConfig{Retry: RetryConfig{MaxAttempts: 3}}}
	got := MergeConfig(base, overlay)
	if got.LLM.Retry.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3 (overridden)", got.LLM.Retry.MaxAttempts)
	}
	if got.LLM.Retry.BaseDelay != "2s" || got.LLM.Retry.MaxDelay != "60s" {
		t.Errorf("unset overlay fields should inherit base, got %+v", got.LLM.Retry)
	}

	// Empty overlay preserves base.
	got = MergeConfig(base, &Config{})
	if got.LLM.Retry.MaxAttempts != 5 || got.LLM.Retry.BaseDelay != "2s" {
		t.Errorf("empty overlay should preserve base retry, got %+v", got.LLM.Retry)
	}
}

func TestResolveSyncCooldown(t *testing.T) {
	cases := []struct {
		name   string
		cfg    *Config
		wantMS int64
	}{
		{"nil config uses default", nil, 15 * 60 * 1000},
		{"empty value uses default", &Config{}, 15 * 60 * 1000},
		{"valid override", &Config{Sync: SyncConfig{Cooldown: "2m"}}, 2 * 60 * 1000},
		{"garbage falls back to default", &Config{Sync: SyncConfig{Cooldown: "not-a-duration"}}, 15 * 60 * 1000},
		{"zero falls back to default", &Config{Sync: SyncConfig{Cooldown: "0s"}}, 15 * 60 * 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSyncCooldown(tc.cfg).Milliseconds()
			if got != tc.wantMS {
				t.Errorf("got %d ms, want %d ms", got, tc.wantMS)
			}
		})
	}
}
