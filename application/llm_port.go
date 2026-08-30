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
	Output []byte
	// ExecutorFingerprint identifies the executor that served the call — the
	// host's own name for itself. It is not the provider: what ran inside the
	// executor is reported by LLMIdentity, and conflating the two puts a host
	// name in the provider column.
	ExecutorFingerprint string
	FinishReason        string
	Usage               LLMUsage
}

// LLMIdentity names the provider and model an executor routes to. Static
// configuration, not a property of any one response, so it attributes a failed
// call as readily as a successful one.
type LLMIdentity struct {
	Provider string
	Model    string
}

type LLMExecutor interface {
	Capabilities(context.Context) ([]string, error)
	// Identity reports what this executor talks to. Required: an executor that
	// cannot name its provider leaves every record it produces unattributable,
	// and no layer above it is in a position to supply the answer.
	Identity() LLMIdentity
	Execute(context.Context, LLMRequest) (LLMResult, error)
}
