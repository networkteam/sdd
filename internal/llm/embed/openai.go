package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

const defaultOpenAIEndpoint = "https://api.openai.com"

// openaiEmbedder implements llm.Embedder against the
// `POST {endpoint}/v1/embeddings` API contract used by OpenAI and
// OpenAI-compatible self-hosted services.
type openaiEmbedder struct {
	endpoint         string
	apiKey           string
	model            string
	dimensions       int // 0 means "use model native"
	batchSize        int
	queryTemplate    string
	documentTemplate string
	httpClient       *http.Client
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
		endpoint:         endpoint,
		apiKey:           apiKey,
		model:            cfg.Model,
		dimensions:       cfg.Dimensions,
		batchSize:        batchSize,
		queryTemplate:    cfg.QueryTemplate,
		documentTemplate: cfg.DocumentTemplate,
		httpClient:       &http.Client{Timeout: timeout},
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
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// EmbedDocuments applies the configured document template before
// dispatching to the shared transport.
func (e *openaiEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, "embed-documents", applyTemplate(e.documentTemplate, texts))
}

// EmbedQueries applies the configured query template before dispatching
// to the shared transport.
func (e *openaiEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, "embed-queries", applyTemplate(e.queryTemplate, texts))
}

// embed splits the (already-templated) input slice into capped sub-batches,
// sends each as a single /v1/embeddings call, and concatenates results
// in input order. op labels the recorded stat (embed-documents vs
// embed-queries).
func (e *openaiEmbedder) embed(ctx context.Context, op string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += e.batchSize {
		end := start + e.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := e.embedBatch(ctx, op, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("openai batch [%d:%d]: %w", start, end, err)
		}
		out = append(out, batch...)
	}
	return out, nil
}

func (e *openaiEmbedder) embedBatch(ctx context.Context, op string, texts []string) ([][]float32, error) {
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

	start := time.Now()
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed request: %w", err)
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

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

	// One stat per HTTP call. Self-hosted OpenAI-compatible servers (omlx,
	// vLLM, LM Studio) report prompt_tokens in usage just as OpenAI does;
	// when a server omits it, tokens.in is 0 but items + duration still
	// give the throughput comparison.
	llm.RecordEmbedCall(ctx, llm.CallStat{
		Purpose:    op,
		Identity:   pkgllm.Identity{Provider: "openai", Model: e.model},
		Items:      len(texts),
		Usage:      pkgllm.Usage{InputTokens: decoded.Usage.PromptTokens},
		DurationMS: time.Since(start).Milliseconds(),
	})

	out := make([][]float32, len(decoded.Data))
	for i, d := range decoded.Data {
		out[i] = d.Embedding
	}
	return out, nil
}

func (e *openaiEmbedder) BatchSize() int { return e.batchSize }

func (e *openaiEmbedder) Dimensions() int {
	if e.dimensions > 0 {
		return e.dimensions
	}
	switch e.model {
	case "text-embedding-3-small", "text-embedding-ada-002":
		return 1536
	case "text-embedding-3-large":
		return 3072
	}
	return 0
}

func (e *openaiEmbedder) Fingerprint() string {
	base := "openai/" + e.model
	if e.dimensions > 0 {
		base = fmt.Sprintf("%s/%d", base, e.dimensions)
	}
	return appendDocTemplateHash(base, e.documentTemplate)
}
