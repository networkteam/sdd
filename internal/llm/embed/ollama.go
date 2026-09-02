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

const defaultOllamaEndpoint = "http://localhost:11434"

// ollamaEmbedder implements embed.Embedder against Ollama's batch-capable
// `POST {endpoint}/api/embed` API (newer endpoint with list-input support
// — the older `/api/embeddings` only accepts a single prompt). Local-only;
// the rate limiter is not applied (caller's disk and network handle
// backpressure).
type ollamaEmbedder struct {
	endpoint         string
	identity         llm.Identity
	queryTemplate    string
	documentTemplate string
	httpClient       *http.Client
}

func newOllama(cfg model.EmbeddingConfig) pkgembed.Embedder {
	endpoint := cfg.OllamaEndpoint
	if endpoint == "" {
		endpoint = defaultOllamaEndpoint
	}
	return &ollamaEmbedder{
		endpoint:         endpoint,
		identity:         llm.Identity{Provider: "ollama", Model: cfg.Model},
		queryTemplate:    cfg.QueryTemplate,
		documentTemplate: cfg.DocumentTemplate,
		httpClient:       &http.Client{},
	}
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	// PromptEvalCount is the token count Ollama reports for the batch on
	// /api/embed. Absent on older daemons — zero then.
	PromptEvalCount int    `json:"prompt_eval_count"`
	Error           string `json:"error,omitempty"`
}

func (e *ollamaEmbedder) Embed(ctx context.Context, req pkgembed.Request) (pkgembed.Result, error) {
	tmpl, err := templateFor(req.Purpose, e.documentTemplate, e.queryTemplate)
	if err != nil {
		return pkgembed.Result{}, err
	}
	return e.request(ctx, applyTemplate(tmpl, req.Texts))
}

// request sends the templated texts as one /api/embed call; see openaiEmbedder.request.
func (e *ollamaEmbedder) request(ctx context.Context, texts []string) (result pkgembed.Result, err error) {
	defer func() {
		if err != nil {
			err = &llm.Error{Identity: e.identity, Err: err}
		}
	}()
	body := ollamaEmbedRequest{Model: e.identity.Model, Input: texts}
	buf, err := json.Marshal(body)
	if err != nil {
		return pkgembed.Result{}, fmt.Errorf("marshal ollama request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/api/embed", bytes.NewReader(buf))
	if err != nil {
		return pkgembed.Result{}, fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return pkgembed.Result{}, fmt.Errorf("ollama embed request: %w", err)
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	var decoded ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return pkgembed.Result{}, fmt.Errorf("decode ollama response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || decoded.Error != "" {
		msg := decoded.Error
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return pkgembed.Result{}, fmt.Errorf("ollama embed error: %s", msg)
	}
	if len(decoded.Embeddings) != len(texts) {
		return pkgembed.Result{}, fmt.Errorf("ollama returned %d embeddings for %d inputs (model %q)",
			len(decoded.Embeddings), len(texts), e.identity.Model)
	}
	return pkgembed.Result{Vectors: decoded.Embeddings, Identity: e.identity, Usage: llm.Usage{InputTokens: decoded.PromptEvalCount}}, nil
}

// Fingerprint carries the model name only: Ollama models expose no
// truncation knob, so the name uniquely determines the vector dimension, and
// switching models changes the fingerprint and triggers the re-embed.
func (e *ollamaEmbedder) Fingerprint() string {
	return appendDocTemplateHash("ollama/"+e.identity.Model, e.documentTemplate)
}
