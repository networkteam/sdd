// Package llm handles LLM-based generation tasks: prompt rendering, template
// embedding, runner invocation, and output parsing. Finders (read side) and
// handlers (write side) both consume this package — it owns the "call an LLM
// with a structured prompt" concern.
package llm

import (
	"context"
	"time"
)

// Runner executes a rendered prompt string against an LLM and returns the
// response with metadata. The implementation decides which model and transport
// to use. Injected so tests can substitute fakes.
type Runner interface {
	Run(ctx context.Context, prompt string) (*RunResult, error)
}

// RunResult holds the LLM response text and optional metadata.
type RunResult struct {
	Text string
	Meta *LLMMetadata
}

// LLMMetadata holds agent-neutral per-call metrics from the LLM provider.
type LLMMetadata struct {
	TotalCostUSD      float64
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	NumTurns          int
	Duration          time.Duration
	DurationAPI       time.Duration
	Models            map[string]ModelUsage
}

// ModelUsage holds per-model token and cost metrics.
type ModelUsage struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	CostUSD           float64
}
