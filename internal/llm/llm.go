// Package llm handles LLM-based generation tasks: prompt rendering, template
// embedding, runner invocation, and output parsing. Finders (read side) and
// handlers (write side) both consume this package — it owns the "call an LLM
// with a structured prompt" concern.
package llm

import (
	"context"
	"time"
)

// Runner executes a structured LLM request and returns the response with
// metadata. The implementation decides which model and transport to use.
// Injected so tests can substitute fakes.
type Runner interface {
	Run(ctx context.Context, req Request) (*RunResult, error)
}

// Request carries the two-part prompt submitted to a Runner. SystemPrompt
// holds the stable portion (instructions, structural rules) — providers that
// support prompt caching treat this as the cacheable prefix. UserPrompt holds
// the per-call variable portion (entry content, refs). Runners that can't
// distinguish system from user concatenate them with SystemPrompt first.
type Request struct {
	SystemPrompt string
	UserPrompt   string
}

// Combined returns SystemPrompt followed by UserPrompt separated by a blank
// line when both are non-empty. Runners without native system-prompt support
// use this to flatten the Request into a single payload. The hash used for
// summary-skip detection is computed over this combined form so changes to
// either half invalidate the cached summary.
func (r Request) Combined() string {
	if r.SystemPrompt == "" {
		return r.UserPrompt
	}
	if r.UserPrompt == "" {
		return r.SystemPrompt
	}
	return r.SystemPrompt + "\n\n" + r.UserPrompt
}

// Run executes a pre-rendered Request against the Runner and emits the
// standard debug log entry. Callers that orchestrate prompt rendering
// themselves (e.g. the parallel summarize handler) use this instead of
// Runner.Run directly so logging stays uniform across call sites.
func Run(ctx context.Context, runner Runner, req Request, op string) (*RunResult, error) {
	start := time.Now()
	output, err := runner.Run(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		return nil, err
	}
	logCallResult(ctx, output.Meta, op, elapsed)
	return output, nil
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
