package model

import (
	"testing"
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
