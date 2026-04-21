package llm

import (
	"context"
	"log/slog"
	"time"

	"github.com/networkteam/slogutils"
)

// logCallResult emits a single debug-level log line for an LLM call under a
// slog.Group("llm"). Per-model sub-groups are added only when the runner
// provided metadata (claude-cli populates this; the gollm adapter doesn't
// yet). The op + duration line fires regardless so -vv always shows
// progress across providers.
func logCallResult(ctx context.Context, meta *LLMMetadata, op string, elapsed time.Duration) {
	logger := slogutils.FromContext(ctx)

	llmAttrs := []any{
		slog.String("op", op),
		slog.Duration("duration", elapsed),
	}

	if meta != nil {
		for name, usage := range meta.Models {
			attrs := []any{
				slog.Int("tokens.in", usage.InputTokens),
				slog.Int("tokens.out", usage.OutputTokens),
			}
			if usage.CostUSD > 0 {
				attrs = append(attrs, slog.Float64("cost", usage.CostUSD))
			}
			llmAttrs = append(llmAttrs, slog.Group(name, attrs...))
		}
	}

	logger.Debug("llm call", slog.Group("llm", llmAttrs...))
}
