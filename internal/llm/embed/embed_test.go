package embed_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	localembed "github.com/networkteam/sdd/internal/llm/embed"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

func documents(texts ...string) embed.Request {
	return embed.Request{Purpose: embed.PurposeDocument, Texts: texts}
}

func TestNew_NoProvider(t *testing.T) {
	t.Parallel()
	_, err := localembed.New(model.EmbeddingConfig{})
	if err == nil || !strings.Contains(err.Error(), "embedding provider") {
		t.Fatalf("expected provider-missing error, got %v", err)
	}
}

func TestNew_NoModel(t *testing.T) {
	t.Parallel()
	_, err := localembed.New(model.EmbeddingConfig{Provider: "openai", APIKeys: map[string]string{"openai": "sk-x"}})
	if err == nil || !strings.Contains(err.Error(), "embedding.model") {
		t.Fatalf("expected model-missing error, got %v", err)
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	t.Parallel()
	_, err := localembed.New(model.EmbeddingConfig{Provider: "cohere", Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "unknown embedding provider") {
		t.Fatalf("expected unknown-provider error, got %v", err)
	}
}

func TestBatchSize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cfg  model.EmbeddingConfig
		want int
	}{
		{model.EmbeddingConfig{Provider: "openai"}, 100},
		{model.EmbeddingConfig{Provider: "ollama"}, 64},
		{model.EmbeddingConfig{Provider: "ollama", BatchSize: 16}, 16},
		{model.EmbeddingConfig{Provider: "openai", BatchSize: -1}, 100},
	}
	for _, c := range cases {
		if got := localembed.BatchSize(c.cfg); got != c.want {
			t.Errorf("BatchSize(%+v) = %d, want %d", c.cfg, got, c.want)
		}
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
		var body struct {
			Input      []string `json:"input"`
			Model      string   `json:"model"`
			Dimensions int      `json:"dimensions"`
		}
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
			"usage": map[string]any{"prompt_tokens": 42},
		})
	}))
	defer srv.Close()

	emb, err := localembed.New(model.EmbeddingConfig{
		Provider:     "openai",
		Model:        "text-embedding-3-small",
		Endpoint:     srv.URL,
		APIKeys:      map[string]string{"openai": "test-key"},
		Dimensions:   256,
		RateLimitRPS: 1000, // effectively unlimited for the test
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := emb.Embed(context.Background(), documents("hello", "world"))
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	want := [][]float32{{0.1, 0.2}, {0.3, 0.4}}
	if !reflect.DeepEqual(got.Vectors, want) {
		t.Errorf("vectors: got %#v, want %#v", got.Vectors, want)
	}
	if got.Identity != (llm.Identity{Provider: "openai", Model: "text-embedding-3-small"}) {
		t.Errorf("identity: got %+v", got.Identity)
	}
	if got.Usage.InputTokens != 42 {
		t.Errorf("input tokens: got %d, want 42 (from usage.prompt_tokens)", got.Usage.InputTokens)
	}
	if emb.Fingerprint() != "openai/text-embedding-3-small/256" {
		t.Errorf("Fingerprint(): got %q", emb.Fingerprint())
	}
}

func TestOpenAIEmbedder_TemplateByPurpose(t *testing.T) {
	t.Parallel()

	var inputs [][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		inputs = append(inputs, body.Input)
		out := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			out[i] = map[string]any{"embedding": []float32{1}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": out})
	}))
	defer srv.Close()

	emb, err := localembed.New(model.EmbeddingConfig{
		Provider: "openai", Model: "m", Endpoint: srv.URL, APIKeys: map[string]string{"openai": "k"},
		DocumentTemplate: "passage: {text}", QueryTemplate: "query: {text}", RateLimitRPS: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := emb.Embed(context.Background(), documents("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := emb.Embed(context.Background(), embed.Request{Purpose: embed.PurposeQuery, Texts: []string{"b"}}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"passage: a"}, {"query: b"}}
	if !reflect.DeepEqual(inputs, want) {
		t.Errorf("templated inputs: got %#v, want %#v", inputs, want)
	}
	if _, err := emb.Embed(context.Background(), embed.Request{Purpose: "embed-other", Texts: []string{"c"}}); err == nil {
		t.Error("expected an unknown purpose to fail")
	}
}

func TestOpenAIEmbedder_HTTPErrorIsAttributed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

	emb, err := localembed.New(model.EmbeddingConfig{
		Provider:     "openai",
		Model:        "text-embedding-3-small",
		Endpoint:     srv.URL,
		APIKeys:      map[string]string{"openai": "bad"},
		RateLimitRPS: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = emb.Embed(context.Background(), documents("x"))
	if err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("expected error containing API message, got %v", err)
	}
	attributed, ok := errors.AsType[*llm.Error](err)
	if !ok || attributed.Identity.Provider != "openai" {
		t.Fatalf("expected an attributed *llm.Error, got %T %v", err, err)
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
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
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
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out, "prompt_eval_count": 17})
	}))
	defer srv.Close()

	emb, err := localembed.New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "nomic-embed-text",
		OllamaEndpoint: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := emb.Embed(context.Background(), documents("a", "b"))
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 batch HTTP call, got %d", calls)
	}
	if len(got.Vectors) != 2 || len(got.Vectors[0]) != 2 || len(got.Vectors[1]) != 2 {
		t.Fatalf("expected 2x2 embeddings, got %#v", got.Vectors)
	}
	if got.Identity != (llm.Identity{Provider: "ollama", Model: "nomic-embed-text"}) || got.Usage.InputTokens != 17 {
		t.Errorf("identity/usage: got %+v %+v", got.Identity, got.Usage)
	}
	if emb.Fingerprint() != "ollama/nomic-embed-text" {
		t.Errorf("Fingerprint(): got %q", emb.Fingerprint())
	}
}

// The adapter is a transport: one Embed is one request whatever the configured
// batch size, since splitting is composed around it (embed.Batched).
func TestOllamaEmbedder_OneRequestPerEmbed(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		out := make([][]float32, len(body.Input))
		for i, in := range body.Input {
			out[i] = []float32{float32(in[0]), float32(in[1])}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out, "prompt_eval_count": len(body.Input)})
	}))
	defer srv.Close()

	const batchSize = 16
	emb, err := localembed.New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "m",
		OllamaEndpoint: srv.URL,
		BatchSize:      batchSize,
	})
	if err != nil {
		t.Fatal(err)
	}

	n := batchSize + 5
	inputs := make([]string, n)
	for i := range inputs {
		inputs[i] = fmt.Sprintf("a%c", 'a'+byte(i%26))
	}
	got, err := emb.Embed(context.Background(), documents(inputs...))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Vectors) != n {
		t.Fatalf("got %d embeddings, want %d", len(got.Vectors), n)
	}
	if calls != 1 {
		t.Errorf("expected 1 request for %d inputs, got %d", n, calls)
	}
	if got.Usage.InputTokens != n {
		t.Errorf("tokens: got %d, want %d", got.Usage.InputTokens, n)
	}
}

func TestOllamaEmbedder_Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "model not found"})
	}))
	defer srv.Close()

	emb, err := localembed.New(model.EmbeddingConfig{
		Provider:       "ollama",
		Model:          "missing",
		OllamaEndpoint: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = emb.Embed(context.Background(), documents("x"))
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected error mentioning model not found, got %v", err)
	}
}
