package gollm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"

	gollmrunner "github.com/networkteam/sdd/internal/llm/gollm"
)

// What actually reaches the provider is not inferable from the adapter code —
// gollm copies its option map into the request body verbatim, so a key we set
// may land under a name the provider ignores. This drives a real request at a
// local listener and asserts on the bytes.
func TestOllamaRequestWire(t *testing.T) {
	var mu sync.Mutex
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(raw, &body)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"model":"m","response":"{}","done":true,"prompt_eval_count":11,"eval_count":22}`))
	}))
	defer srv.Close()

	runner, err := gollmrunner.NewRunner(model.LLMConfig{
		Provider: "ollama", Model: "m", OllamaEndpoint: srv.URL,
		Params: map[string]string{"think": "high"},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	res, err := runner.Run(context.Background(), llm.Request{
		SystemPrompt: "SYSTEM-BLOCK-MARKER",
		UserPrompt:   "USER-BLOCK-MARKER",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	t.Logf("request keys: %v", keysOf(body))
	t.Logf("full body: %+v", body)

	if body["think"] != "high" {
		t.Errorf("think = %v, want high", body["think"])
	}
	prompt, _ := body["prompt"].(string)
	if !strings.Contains(prompt, "SYSTEM-BLOCK-MARKER") {
		t.Errorf("system block missing from the flattened prompt: %q", prompt)
	}
	// Ollama flattens through Prompt.String(), which used to render the input
	// both bare and again under a Messages header — every call paid twice.
	if n := strings.Count(prompt, "USER-BLOCK-MARKER"); n != 1 {
		t.Errorf("user block sent %d times, want 1: %q", n, prompt)
	}
	if res.Usage.InputTokens != 11 || res.Usage.OutputTokens != 22 {
		t.Errorf("usage not parsed: %+v", res.Usage)
	}
	if res.Identity.Variant != "think=high" {
		t.Errorf("variant = %q, want think=high", res.Identity.Variant)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
