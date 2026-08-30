package application

import (
	"context"
	"encoding/json"
)

type LLMRequest struct {
	Purpose              string
	SystemPrompt         string
	Prompt               string
	OutputSchema         json.RawMessage
	RequiredCapabilities []string
	Routing              map[string]string
	PromptDigest         string
	IdempotencyKey       string
}

type LLMUsage struct {
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheCreateTokens int64
}

type LLMResult struct {
	Output              []byte
	ExecutorFingerprint string
	FinishReason        string
	// Model names the model that served the call, when the executor knows it.
	// Reported so hosts can attribute usage per model; the fingerprint
	// identifies the executor, not what ran inside it.
	Model string
	Usage LLMUsage
}

type LLMExecutor interface {
	Capabilities(context.Context) ([]string, error)
	Execute(context.Context, LLMRequest) (LLMResult, error)
}
