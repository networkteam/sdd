package llm

import (
	"context"
	"log/slog"
	"time"

	"github.com/networkteam/slogutils"
)

// logCallResult emits a single debug-level log line for an LLM call under a
// slog.Group("llm") with per-model sub-groups. Call sites (Preflight,
// Summarize) invoke this after runner.Run returns.
func logCallResult(ctx context.Context, meta *LLMMetadata, op string, elapsed time.Duration) {
	if meta == nil {
		return
	}

	logger := slogutils.FromContext(ctx)

	// Build per-model attrs as sub-groups.
	var modelAttrs []any
	for name, usage := range meta.Models {
		attrs := []any{
			slog.Int("tokens.in", usage.InputTokens),
			slog.Int("tokens.out", usage.OutputTokens),
		}
		if usage.CostUSD > 0 {
			attrs = append(attrs, slog.Float64("cost", usage.CostUSD))
		}
		modelAttrs = append(modelAttrs, slog.Group(name, attrs...))
	}

	llmAttrs := []any{
		slog.String("op", op),
		slog.Duration("duration", elapsed),
	}
	llmAttrs = append(llmAttrs, modelAttrs...)

	logger.Debug("llm call", slog.Group("llm", llmAttrs...))
}
