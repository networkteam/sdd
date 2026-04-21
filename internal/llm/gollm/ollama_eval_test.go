//go:build ollama_eval

// Ollama end-to-end evaluation. Requires a local Ollama running at the
// configured endpoint (default http://localhost:11434) with the configured
// model pulled. Run manually:
//
//	go test -tags=ollama_eval -run TestOllamaEval ./internal/llm/gollm/... -v
//
// The build tag keeps this test out of the default CI run so the suite
// doesn't depend on Ollama being installed.

package gollm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

func TestOllamaEval_RoundTrip(t *testing.T) {
	endpoint := os.Getenv("OLLAMA_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		t.Skip("set OLLAMA_MODEL to run (e.g. llama3.1:8b)")
	}

	r, err := NewRunner(modelConfigForOllama(endpoint, model))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := llm.Request{
		SystemPrompt: "Respond with a single word.",
		UserPrompt:   "What color is the sky on a clear day? One word.",
	}
	out, err := r.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(out.Text) == "" {
		t.Fatal("expected non-empty response from Ollama")
	}
	t.Logf("Ollama response: %q", out.Text)
}

func modelConfigForOllama(endpoint, modelName string) model.LLMConfig {
	return model.LLMConfig{
		Provider:       "ollama",
		Model:          modelName,
		OllamaEndpoint: endpoint,
		Timeout:        "30s",
	}
}
