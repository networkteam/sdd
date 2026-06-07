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
)

const defaultOllamaEndpoint = "http://localhost:11434"

// ollamaEmbedder implements llm.Embedder against Ollama's batch-capable
// `POST {endpoint}/api/embed` API (newer endpoint with list-input support
// — the older `/api/embeddings` only accepts a single prompt). Local-only;
// the rate limiter is not applied (caller's disk and network handle
// backpressure).
type ollamaEmbedder struct {
	endpoint         string
	model            string
	dims             int // discovered from first response, cached for Dimensions()
	batchSize        int
	queryTemplate    string
	documentTemplate string
	httpClient       *http.Client
}

func newOllama(cfg model.EmbeddingConfig, timeout time.Duration, batchSize int) llm.Embedder {
	endpoint := cfg.OllamaEndpoint
	if endpoint == "" {
		endpoint = defaultOllamaEndpoint
	}
	return &ollamaEmbedder{
		endpoint:         endpoint,
		model:            cfg.Model,
		dims:             cfg.Dimensions,
		batchSize:        batchSize,
		queryTemplate:    cfg.QueryTemplate,
		documentTemplate: cfg.DocumentTemplate,
		httpClient:       &http.Client{Timeout: timeout},
	}
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	// PromptEvalCount is the token count Ollama reports for the batch on
	// /api/embed. Absent on older daemons — zero then, which the stat
	// records as tokens.in 0 (items + duration still capture throughput).
	PromptEvalCount int    `json:"prompt_eval_count"`
	Error           string `json:"error,omitempty"`
}

// EmbedDocuments applies the configured document template before
// dispatching to the shared transport.
func (e *ollamaEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, "embed-documents", applyTemplate(e.documentTemplate, texts))
}

// EmbedQueries applies the configured query template before dispatching
// to the shared transport.
func (e *ollamaEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, "embed-queries", applyTemplate(e.queryTemplate, texts))
}

// embed splits a (already-templated) input slice into capped sub-batches,
// sends each as a single /api/embed request, and concatenates the
// results in input order. op labels the recorded stat (embed-documents vs
// embed-queries).
func (e *ollamaEmbedder) embed(ctx context.Context, op string, texts []string) ([][]float32, error) {
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
			return nil, fmt.Errorf("ollama batch [%d:%d]: %w", start, end, err)
		}
		out = append(out, batch...)
		if e.dims == 0 && len(batch) > 0 {
			e.dims = len(batch[0])
		}
	}
	return out, nil
}

func (e *ollamaEmbedder) embedBatch(ctx context.Context, op string, texts []string) (vec [][]float32, err error) {
	body := ollamaEmbedRequest{Model: e.model, Input: texts}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/api/embed", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request: %w", err)
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	var decoded ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode ollama response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || decoded.Error != "" {
		msg := decoded.Error
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("ollama embed error: %s", msg)
	}
	if len(decoded.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama returned %d embeddings for %d inputs (model %q)",
			len(decoded.Embeddings), len(texts), e.model)
	}

	// One stat per HTTP call, mirroring the openai path.
	llm.RecordEmbedCall(ctx, llm.CallStat{
		Op:          op,
		Provider:    "ollama",
		Model:       e.model,
		Items:       len(texts),
		InputTokens: decoded.PromptEvalCount,
		DurationMS:  time.Since(start).Milliseconds(),
	})

	return decoded.Embeddings, nil
}

func (e *ollamaEmbedder) BatchSize() int { return e.batchSize }

func (e *ollamaEmbedder) Dimensions() int { return e.dims }

// Fingerprint deliberately excludes dims even though they're observable
// after the first call. Ollama models don't expose a matryoshka-style
// truncation knob — the model name uniquely determines the vector
// dimension — so dims are redundant in the fingerprint AND including
// them creates a stability bug: dims is 0 at construction and only
// becomes populated after the first response. Capturing the fingerprint
// before vs after the first call would otherwise yield different values
// and mark every previously-indexed row as drift on the next session.
//
// If you ever switch to a model with a different dim, the model-name
// component of the fingerprint changes and triggers re-embed — which
// is the correct behavior.
func (e *ollamaEmbedder) Fingerprint() string {
	return appendDocTemplateHash("ollama/"+e.model, e.documentTemplate)
}
