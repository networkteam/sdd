package model

import (
	"strings"
	"testing"
)

func TestSetYAMLField_EmptyInputBuildsFreshDocument(t *testing.T) {
	out, err := SetYAMLField(nil, "participant", "Christopher")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "participant: Christopher") {
		t.Errorf("expected participant on a fresh document, got:\n%s", out)
	}
	cfg, err := ParseConfig(out)
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if cfg.Participant != "Christopher" {
		t.Errorf("Participant = %q, want Christopher", cfg.Participant)
	}
}

func TestSetYAMLField_AppendPreservesUnknownSiblings(t *testing.T) {
	// Real-world regression: config.local.yaml already carries the llm
	// block from d-tac-bes when d-tac-q5p adds the participant key. The
	// llm block and every nested value must survive.
	existing := []byte(`llm:
  provider: anthropic
  model: claude-haiku-4-5-20251001
  api_keys:
    anthropic: sk-ant-xxx
`)
	out, err := SetYAMLField(existing, "participant", "Christopher")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"participant: Christopher",
		"provider: anthropic",
		"model: claude-haiku-4-5-20251001",
		"sk-ant-xxx",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	cfg, err := ParseConfig(out)
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if cfg.Participant != "Christopher" {
		t.Errorf("Participant lost, got %q", cfg.Participant)
	}
	if cfg.LLM.Provider != "anthropic" || cfg.LLM.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("LLM block lost: %+v", cfg.LLM)
	}
	if cfg.LLM.APIKeys["anthropic"] != "sk-ant-xxx" {
		t.Errorf("api_keys.anthropic lost, got %q", cfg.LLM.APIKeys["anthropic"])
	}
}

func TestSetYAMLField_UpdatesExistingScalarInPlace(t *testing.T) {
	existing := []byte(`participant: OldName
llm:
  provider: ollama
`)
	out, err := SetYAMLField(existing, "participant", "NewName")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "OldName") {
		t.Errorf("old value not replaced:\n%s", out)
	}
	if !strings.Contains(string(out), "NewName") {
		t.Errorf("new value missing:\n%s", out)
	}
	if !strings.Contains(string(out), "provider: ollama") {
		t.Errorf("sibling llm block dropped:\n%s", out)
	}
}

func TestSetYAMLField_DottedPathCreatesIntermediateMapping(t *testing.T) {
	// Writing llm.provider into an empty file creates the llm mapping
	// and the provider scalar — both missing ancestors at once.
	out, err := SetYAMLField(nil, "llm.provider", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig(out)
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("llm.provider = %q, want anthropic:\n%s", cfg.LLM.Provider, out)
	}
}

func TestSetYAMLField_DottedPathUpdatesNestedLeafAndKeepsSiblings(t *testing.T) {
	existing := []byte(`llm:
  provider: ollama
  model: llama3.1:70b
participant: Christopher
`)
	out, err := SetYAMLField(existing, "llm.provider", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig(out)
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("llm.provider = %q, want anthropic", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "llama3.1:70b" {
		t.Errorf("sibling llm.model lost, got %q", cfg.LLM.Model)
	}
	if cfg.Participant != "Christopher" {
		t.Errorf("top-level participant lost, got %q", cfg.Participant)
	}
}

func TestSetYAMLField_PreservesHeadComments(t *testing.T) {
	existing := []byte(`# committed header comment
llm:
  # how we talk to the provider
  provider: ollama
`)
	out, err := SetYAMLField(existing, "llm.provider", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# committed header comment",
		"# how we talk to the provider",
		"provider: anthropic",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("comment or value missing %q:\n%s", want, out)
		}
	}
}

func TestSetYAMLField_EmptyPathRejected(t *testing.T) {
	if _, err := SetYAMLField(nil, "", "x"); err == nil {
		t.Error("empty path should be rejected")
	}
}

func TestSetYAMLField_EmptySegmentRejected(t *testing.T) {
	for _, bad := range []string{"llm.", ".llm", "a..b"} {
		if _, err := SetYAMLField(nil, bad, "x"); err == nil {
			t.Errorf("path %q should be rejected (empty segment)", bad)
		}
	}
}

func TestSetYAMLField_RejectsNonScalarLeaf(t *testing.T) {
	existing := []byte(`llm:
  provider: anthropic
`)
	// Targeting the mapping `llm` directly with a scalar value is not
	// supported — caller should target a leaf.
	if _, err := SetYAMLField(existing, "llm", "bogus"); err == nil {
		t.Error("writing a scalar over a mapping should be rejected")
	}
}

func TestSetYAMLField_Indentation2Spaces(t *testing.T) {
	// The existing committed config uses 2-space indent; the patcher
	// must preserve that so re-writes don't produce diffs of pure style.
	out, err := SetYAMLField(nil, "llm.provider", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "\n  provider: anthropic") {
		t.Errorf("expected 2-space indent under llm, got:\n%s", out)
	}
}
