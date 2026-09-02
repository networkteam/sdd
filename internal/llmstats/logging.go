package llmstats

import (
	"context"
	"log/slog"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/pkg/llm"
)

// Logged decorates a sink so every call also emits one debug-level log line
// through the context's logger before being handed on; a nil next only logs.
func Logged(next llm.StatsSink) llm.StatsSink {
	return loggingSink{next: next}
}

type loggingSink struct {
	next llm.StatsSink
}

func (s loggingSink) RecordCall(ctx context.Context, stat llm.CallStat) {
	logger := slogutils.FromContext(ctx)
	attrs := []any{
		slog.String("op", stat.Purpose),
		slog.Duration("duration", stat.Duration),
		slog.String("provider", stat.Identity.Provider),
		slog.String("model", stat.Identity.String()),
	}
	if stat.Error != "" {
		logger.Debug("llm call failed", slog.Group("llm", append(attrs, slog.String("error", stat.Error))...))
	} else {
		attrs = append(attrs,
			slog.Int("tokens.in", stat.Usage.InputTokens),
			slog.Int("tokens.out", stat.Usage.OutputTokens),
		)
		if stat.Items > 0 {
			attrs = append(attrs, slog.Int("items", stat.Items))
		}
		if stat.Usage.CacheReadTokens > 0 || stat.Usage.CacheCreateTokens > 0 {
			attrs = append(attrs,
				slog.Int("cache.read", stat.Usage.CacheReadTokens),
				slog.Int("cache.create", stat.Usage.CacheCreateTokens),
			)
		}
		if stat.Usage.CostUSD > 0 {
			attrs = append(attrs, slog.Float64("cost", stat.Usage.CostUSD))
		}
		logger.Debug("llm call", slog.Group("llm", attrs...))
	}
	if s.next != nil {
		s.next.RecordCall(ctx, stat)
	}
}
