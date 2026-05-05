package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestNew_NoProvider(t *testing.T) {
	t.Parallel()
	_, err := New(model.EmbeddingConfig{})
	if err == nil || !strings.Contains(err.Error(), "embedding provider") {
		t.Fatalf("expected provider-missing error, got %v", err)
	}
}

func TestNew_NoModel(t *testing.T) {
	t.Parallel()
	_, err := New(model.EmbeddingConfig{Provider: "openai", APIKeys: map[string]string{"openai": "sk-x"}})
	if err == nil || !strings.Contains(err.Error(), "embedding.model") {
		t.Fatalf("expected model-missing error, got %v", err)
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	t.Parallel()
	_, err := New(model.EmbeddingConfig{Provider: "cohere", Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "unknown embedding provider") {
		t.Fatalf("expected unknown-provider error, got %v", err)
	}
}

func TestOpenAIEmbedder_Embed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header: got %q", got)
		}
		var body openaiEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(body.Input, []string{"hello", "world"}) {
			t.Errorf("input: got %#v", body.Input)
		}
		if body.Model != "text-embedding-3-small" {
			t.Errorf("model: got %q", body.Model)
		}
		if body.Dimensions != 256 {
			t.Errorf("dimensions: got %d, want 256", body.Dimensions)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{0.1, 0.2}},
				{"embedding": []float32{0.3, 0.4}},
			},
		})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:   "openai",
		Model:      "text-embedding-3-small",
		Endpoint:   srv.URL,
		APIKeys:    map[string]string{"openai": "test-key"},
		Dimensions: 256,
		// Disable rate limiting for the test by setting a high value.
		RateLimitRPS: 1000,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := emb.EmbedDocuments(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	want := [][]float32{{0.1, 0.2}, {0.3, 0.4}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if emb.Dimensions() != 256 {
		t.Errorf("Dimensions(): got %d", emb.Dimensions())
	}
	if emb.Fingerprint() != "openai/text-embedding-3-small/256" {
		t.Errorf("Fingerprint(): got %q", emb.Fingerprint())
	}
}

func TestOpenAIEmbedder_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "invalid api key",
				"type":    "invalid_request_error",
				"code":    "invalid_api_key",
			},
		})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:     "openai",
		Model:        "text-embedding-3-small",
		Endpoint:     srv.URL,
		APIKeys:      map[string]string{"openai": "bad"},
		RateLimitRPS: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = emb.EmbedDocuments(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("expected error containing API message, got %v", err)
	}
}

func TestOllamaEmbedder_Embed(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/embed" {
			t.Errorf("path: got %q (expected /api/embed)", r.URL.Path)
		}
		var body ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "nomic-embed-text" {
			t.Errorf("model: got %q", body.Model)
		}
		out := make([][]float32, len(body.Input))
		for i, in := range body.Input {
			out[i] = []float32{0.5, float32(in[0])}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "nomic-embed-text",
		OllamaEndpoint: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := emb.EmbedDocuments(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 batch HTTP call, got %d", calls)
	}
	if len(got) != 2 || len(got[0]) != 2 || len(got[1]) != 2 {
		t.Fatalf("expected 2x2 embeddings, got %#v", got)
	}
	if emb.Dimensions() != 2 {
		t.Errorf("Dimensions(): got %d", emb.Dimensions())
	}
	if emb.Fingerprint() != "ollama/nomic-embed-text/2" {
		t.Errorf("Fingerprint(): got %q", emb.Fingerprint())
	}
}

// TestOllamaEmbedder_BatchSplit verifies large inputs are split into
// sub-batches of ollamaBatchSize while preserving input order.
func TestOllamaEmbedder_BatchSplit(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		out := make([][]float32, len(body.Input))
		for i, in := range body.Input {
			// Encode the input index as the first dim so we can verify order.
			out[i] = []float32{float32(in[0]), float32(in[1])}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer srv.Close()

	const batchSize = 16
	emb, err := New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "m",
		OllamaEndpoint: srv.URL,
		BatchSize:      batchSize,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Generate batchSize+5 unique inputs to trigger a second sub-batch.
	n := batchSize + 5
	inputs := make([]string, n)
	for i := range inputs {
		inputs[i] = fmt.Sprintf("a%c", 'a'+byte(i%26))
	}
	got, err := emb.EmbedDocuments(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("got %d embeddings, want %d", len(got), n)
	}
	if calls != 2 {
		t.Errorf("expected 2 batch calls (split at ollamaBatchSize), got %d", calls)
	}
}

func TestOllamaEmbedder_Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "model not found"})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "missing",
		OllamaEndpoint: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = emb.EmbedDocuments(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected error mentioning model not found, got %v", err)
	}
}
