// Package claude implements llm.Runner by invoking the Claude CLI.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/llm"
)

// Runner invokes the claude CLI with --output-format json and parses the
// response into agent-neutral LLMMetadata.
type Runner struct {
	model string
}

// NewRunner returns an llm.Runner backed by the claude CLI.
func NewRunner(model string) llm.Runner {
	return &Runner{model: model}
}

// Run executes claude -p --output-format json and parses the JSON response.
func (r *Runner) Run(ctx context.Context, prompt string) (*llm.RunResult, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", r.model, "--output-format", "json")
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude -p timed out (increase with --preflight-timeout)")
		}
		return nil, fmt.Errorf("claude -p: %w", err)
	}

	var resp claudeResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parsing claude JSON response: %w", err)
	}

	meta := &llm.LLMMetadata{
		TotalCostUSD:      resp.TotalCostUSD,
		InputTokens:       resp.Usage.InputTokens,
		OutputTokens:      resp.Usage.OutputTokens,
		CacheReadTokens:   resp.Usage.CacheReadInputTokens,
		CacheCreateTokens: resp.Usage.CacheCreationInputTokens,
		NumTurns:          resp.NumTurns,
		Duration:          time.Duration(resp.DurationMs) * time.Millisecond,
		DurationAPI:       time.Duration(resp.DurationAPIMs) * time.Millisecond,
		Models:            make(map[string]llm.ModelUsage, len(resp.ModelUsage)),
	}

	for name, mu := range resp.ModelUsage {
		meta.Models[name] = llm.ModelUsage{
			InputTokens:       mu.InputTokens,
			OutputTokens:      mu.OutputTokens,
			CacheReadTokens:   mu.CacheReadInputTokens,
			CacheCreateTokens: mu.CacheCreationInputTokens,
			CostUSD:           mu.CostUSD,
		}
	}

	return &llm.RunResult{
		Text: resp.Result,
		Meta: meta,
	}, nil
}

// claudeResponse maps the JSON output of claude -p --output-format json.
type claudeResponse struct {
	Result        string                      `json:"result"`
	TotalCostUSD  float64                     `json:"total_cost_usd"`
	DurationMs    int64                       `json:"duration_ms"`
	DurationAPIMs int64                       `json:"duration_api_ms"`
	NumTurns      int                         `json:"num_turns"`
	IsError       bool                        `json:"is_error"`
	Usage         claudeUsage                 `json:"usage"`
	ModelUsage    map[string]claudeModelUsage `json:"modelUsage"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type claudeModelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	CostUSD                  float64 `json:"costUSD"`
}
