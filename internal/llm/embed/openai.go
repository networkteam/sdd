package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

const defaultOpenAIEndpoint = "https://api.openai.com"

// openaiEmbedder implements llm.Embedder against the
// `POST {endpoint}/v1/embeddings` API contract used by OpenAI and
// OpenAI-compatible self-hosted services.
type openaiEmbedder struct {
	endpoint   string
	apiKey     string
	model      string
	dimensions int // 0 means "use model native"
	batchSize  int
	httpClient *http.Client
}

func newOpenAI(cfg model.EmbeddingConfig, timeout time.Duration, batchSize int) (llm.Embedder, error) {
	apiKey := cfg.APIKeys["openai"]
	if apiKey == "" {
		return nil, fmt.Errorf("embedding.api_keys.openai is required for openai provider")
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultOpenAIEndpoint
	}
	return &openaiEmbedder{
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
		batchSize:  batchSize,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

type openaiEmbedRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	Dimensions     int      `json:"dimensions,omitempty"`
	EncodingFormat string   `json:"encoding_format"`
}

type openaiEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Embed splits a large input slice into capped sub-batches, sends each as
// a single /v1/embeddings call, and concatenates results in input order.
// The rate limiter wraps the whole call (one Wait per outer Embed) — the
// per-batch chunking sits inside.
func (e *openaiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += e.batchSize {
		end := start + e.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := e.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("openai batch [%d:%d]: %w", start, end, err)
		}
		out = append(out, batch...)
	}
	return out, nil
}

func (e *openaiEmbedder) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body := openaiEmbedRequest{
		Input:          texts,
		Model:          e.model,
		Dimensions:     e.dimensions,
		EncodingFormat: "float",
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/v1/embeddings", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed request: %w", err)
	}
	defer resp.Body.Close()

	var decoded openaiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode openai response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		if decoded.Error != nil {
			return nil, fmt.Errorf("openai embed error (status %d, %s/%s): %s",
				resp.StatusCode, decoded.Error.Type, decoded.Error.Code, decoded.Error.Message)
		}
		return nil, fmt.Errorf("openai embed status %d", resp.StatusCode)
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf("openai returned %d embeddings for %d inputs", len(decoded.Data), len(texts))
	}
	out := make([][]float32, len(decoded.Data))
	for i, d := range decoded.Data {
		out[i] = d.Embedding
	}
	return out, nil
}

func (e *openaiEmbedder) Dimensions() int {
	if e.dimensions > 0 {
		return e.dimensions
	}
	// Defaults for known OpenAI models (3-small=1536, 3-large=3072, ada-002=1536).
	// Returning 0 here would fail Index validation; pick a reasonable default
	// per common models, otherwise fall through to a sentinel that the
	// indexer learns from the first response.
	switch e.model {
	case "text-embedding-3-small", "text-embedding-ada-002":
		return 1536
	case "text-embedding-3-large":
		return 3072
	}
	return 0
}

func (e *openaiEmbedder) Fingerprint() string {
	if e.dimensions > 0 {
		return fmt.Sprintf("openai/%s/%d", e.model, e.dimensions)
	}
	return fmt.Sprintf("openai/%s", e.model)
}
