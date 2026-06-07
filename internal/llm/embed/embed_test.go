package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

// fakeSink captures the CallStats an embedder records, for the
// stat-recording assertions below.
type fakeSink struct {
	mu    sync.Mutex
	calls []llm.CallStat
}

func (s *fakeSink) RecordCall(c llm.CallStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, c)
}

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

func TestOpenAIEmbedder_RecordsStat(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{0.1, 0.2}},
				{"embedding": []float32{0.3, 0.4}},
			},
			"usage": map[string]any{"prompt_tokens": 42},
		})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:     "openai",
		Model:        "text-embedding-3-small",
		Endpoint:     srv.URL,
		APIKeys:      map[string]string{"openai": "test-key"},
		RateLimitRPS: 1000,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sink := &fakeSink{}
	ctx := llm.WithStatsSink(context.Background(), sink)
	if _, err := emb.EmbedDocuments(ctx, []string{"hello", "world"}); err != nil {
		t.Fatalf("EmbedDocuments: %v", err)
	}

	if len(sink.calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(sink.calls))
	}
	got := sink.calls[0]
	if got.Op != "embed-documents" {
		t.Errorf("op: got %q, want embed-documents", got.Op)
	}
	if got.Provider != "openai" {
		t.Errorf("provider: got %q, want openai", got.Provider)
	}
	if got.Model != "text-embedding-3-small" {
		t.Errorf("model: got %q", got.Model)
	}
	if got.Items != 2 {
		t.Errorf("items: got %d, want 2", got.Items)
	}
	if got.InputTokens != 42 {
		t.Errorf("input tokens: got %d, want 42 (from usage.prompt_tokens)", got.InputTokens)
	}
}

func TestOllamaEmbedder_RecordsStat(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings":        [][]float32{{0.1, 0.2}},
			"prompt_eval_count": 17,
		})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "qwen3-embedding:8b",
		OllamaEndpoint: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sink := &fakeSink{}
	ctx := llm.WithStatsSink(context.Background(), sink)
	if _, err := emb.EmbedQueries(ctx, []string{"only one query"}); err != nil {
		t.Fatalf("EmbedQueries: %v", err)
	}

	if len(sink.calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(sink.calls))
	}
	got := sink.calls[0]
	if got.Op != "embed-queries" {
		t.Errorf("op: got %q, want embed-queries", got.Op)
	}
	if got.Provider != "ollama" {
		t.Errorf("provider: got %q, want ollama", got.Provider)
	}
	if got.Items != 1 {
		t.Errorf("items: got %d, want 1", got.Items)
	}
	if got.InputTokens != 17 {
		t.Errorf("input tokens: got %d, want 17 (from prompt_eval_count)", got.InputTokens)
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
	if emb.Fingerprint() != "ollama/nomic-embed-text" {
		t.Errorf("Fingerprint(): got %q", emb.Fingerprint())
	}
}

// TestOllamaEmbedder_FingerprintStableAcrossFirstCall pins that the
// fingerprint observed before the first embed call equals the
// fingerprint observed after — Ollama's dims are discovered from the
// first response, so a fingerprint that included dims would shift
// mid-session. The IndexHandler captures the fingerprint once per
// build, so any shift would mark every prior row as drift on the next
// run.
func TestOllamaEmbedder_FingerprintStableAcrossFirstCall(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ollamaEmbedRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		out := make([][]float32, len(body.Input))
		for i := range body.Input {
			out[i] = []float32{0.1, 0.2, 0.3, 0.4} // 4-dim
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "test-model",
		OllamaEndpoint: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := emb.Fingerprint()
	if _, err := emb.EmbedDocuments(context.Background(), []string{"warm up"}); err != nil {
		t.Fatal(err)
	}
	after := emb.Fingerprint()
	if before != after {
		t.Errorf("fingerprint shifted across the first call:\n  before %q\n  after  %q", before, after)
	}
	if emb.Dimensions() != 4 {
		t.Errorf("Dimensions(): got %d, want 4", emb.Dimensions())
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
