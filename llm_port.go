package sdd

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
	InputTokens  int64
	OutputTokens int64
}

type LLMResult struct {
	Output              []byte
	ExecutorFingerprint string
	FinishReason        string
	Usage               LLMUsage
}

type LLMExecutor interface {
	Capabilities(context.Context) ([]string, error)
	Execute(context.Context, LLMRequest) (LLMResult, error)
}
