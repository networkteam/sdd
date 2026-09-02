package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
	pkgembed "github.com/networkteam/sdd/pkg/llm/embed"
)

const defaultOpenAIEndpoint = "https://api.openai.com"

// openaiEmbedder implements embed.Embedder against the
// `POST {endpoint}/v1/embeddings` API contract used by OpenAI and
// OpenAI-compatible self-hosted services.
type openaiEmbedder struct {
	endpoint         string
	apiKey           string
	identity         llm.Identity
	dimensions       int // 0 means "use model native"
	queryTemplate    string
	documentTemplate string
	httpClient       *http.Client
}

func newOpenAI(cfg model.EmbeddingConfig) (pkgembed.Embedder, error) {
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
		identity:         llm.Identity{Provider: "openai", Model: cfg.Model},
		dimensions:       cfg.Dimensions,
		queryTemplate:    cfg.QueryTemplate,
		documentTemplate: cfg.DocumentTemplate,
		httpClient:       &http.Client{},
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
	// Self-hosted OpenAI-compatible servers (omlx, vLLM, LM Studio) report
	// prompt_tokens as OpenAI does; a server that omits it yields 0 tokens.
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (e *openaiEmbedder) Embed(ctx context.Context, req pkgembed.Request) (pkgembed.Result, error) {
	tmpl, err := templateFor(req.Purpose, e.documentTemplate, e.queryTemplate)
	if err != nil {
		return pkgembed.Result{}, err
	}
	return e.request(ctx, applyTemplate(tmpl, req.Texts))
}

// request sends the templated texts as one /v1/embeddings call; the deadline
// and any batching arrive composed around the adapter.
func (e *openaiEmbedder) request(ctx context.Context, texts []string) (result pkgembed.Result, err error) {
	defer func() {
		if err != nil {
			err = &llm.Error{Identity: e.identity, Err: err}
		}
	}()
	body := openaiEmbedRequest{
		Input:          texts,
		Model:          e.identity.Model,
		Dimensions:     e.dimensions,
		EncodingFormat: "float",
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return pkgembed.Result{}, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/v1/embeddings", bytes.NewReader(buf))
	if err != nil {
		return pkgembed.Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return pkgembed.Result{}, fmt.Errorf("openai embed request: %w", err)
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	var decoded openaiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return pkgembed.Result{}, fmt.Errorf("decode openai response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		if decoded.Error != nil {
			return pkgembed.Result{}, fmt.Errorf("openai embed error (status %d, %s/%s): %s",
				resp.StatusCode, decoded.Error.Type, decoded.Error.Code, decoded.Error.Message)
		}
		return pkgembed.Result{}, fmt.Errorf("openai embed status %d", resp.StatusCode)
	}
	if len(decoded.Data) != len(texts) {
		return pkgembed.Result{}, fmt.Errorf("openai returned %d embeddings for %d inputs", len(decoded.Data), len(texts))
	}

	vectors := make([][]float32, len(decoded.Data))
	for i, d := range decoded.Data {
		vectors[i] = d.Embedding
	}
	return pkgembed.Result{Vectors: vectors, Identity: e.identity, Usage: llm.Usage{InputTokens: decoded.Usage.PromptTokens}}, nil
}

func (e *openaiEmbedder) Fingerprint() string {
	base := "openai/" + e.identity.Model
	if e.dimensions > 0 {
		base = fmt.Sprintf("%s/%d", base, e.dimensions)
	}
	return appendDocTemplateHash(base, e.documentTemplate)
}
