package llm

import (
	"context"
	"log/slog"
	"time"

	"github.com/networkteam/slogutils"
)

// logCallResult emits a single debug-level log line for an LLM call under a
// slog.Group("llm") and hands the call's metrics to the StatsSink on ctx.
// Identity comes from the runner, usage from the response — so a provider that
// reports no usage still records who served the call and how long it took.
func logCallResult(ctx context.Context, id Identity, meta *LLMMetadata, op string, elapsed time.Duration) {
	llmAttrs := []any{
		slog.String("op", op),
		slog.Duration("duration", elapsed),
		slog.String("provider", id.Provider),
		slog.String("model", id.String()),
	}
	if meta != nil {
		for name, usage := range meta.Models {
			attrs := []any{
				slog.Int("tokens.in", usage.InputTokens),
				slog.Int("tokens.out", usage.OutputTokens),
			}
			if usage.CacheReadTokens > 0 || usage.CacheCreateTokens > 0 {
				attrs = append(attrs,
					slog.Int("cache.read", usage.CacheReadTokens),
					slog.Int("cache.create", usage.CacheCreateTokens),
				)
			}
			if usage.CostUSD > 0 {
				attrs = append(attrs, slog.Float64("cost", usage.CostUSD))
			}
			llmAttrs = append(llmAttrs, slog.Group(name, attrs...))
		}
	}
	slogutils.FromContext(ctx).Debug("llm call", slog.Group("llm", llmAttrs...))

	stat := CallStat{
		Op:         op,
		Provider:   id.Provider,
		Model:      id.Model,
		Variant:    id.Variant,
		DurationMS: elapsed.Milliseconds(),
	}
	if meta != nil {
		stat.InputTokens = meta.InputTokens
		stat.OutputTokens = meta.OutputTokens
		stat.CacheReadTokens = meta.CacheReadTokens
		stat.CacheCreateTokens = meta.CacheCreateTokens
	}
	recordStat(ctx, stat)
}

// logCallFailure records a call that returned no result — the failure twin of
// logCallResult, attributed the same way, so a timeout is as countable and as
// traceable to its provider as a success.
func logCallFailure(ctx context.Context, id Identity, op string, elapsed time.Duration, cause error) {
	slogutils.FromContext(ctx).Debug("llm call failed", slog.Group("llm",
		slog.String("op", op),
		slog.Duration("duration", elapsed),
		slog.String("provider", id.Provider),
		slog.String("model", id.String()),
		slog.String("error", cause.Error()),
	))
	recordStat(ctx, CallStat{
		Op:         op,
		Provider:   id.Provider,
		Model:      id.Model,
		Variant:    id.Variant,
		DurationMS: elapsed.Milliseconds(),
		Error:      cause.Error(),
	})
}

// RecordEmbedCall logs one embedding batch at debug level and hands its
// metrics to the StatsSink on ctx (if any). It mirrors logCallResult for the
// embedding path, whose embedder carries its own identity: embeddings report
// input tokens and an item count, never output tokens or a cache breakdown.
// The caller supplies a fully-populated CallStat.
func RecordEmbedCall(ctx context.Context, stat CallStat) {
	slogutils.FromContext(ctx).Debug("llm call", slog.Group("llm",
		slog.String("op", stat.Op),
		slog.Duration("duration", time.Duration(stat.DurationMS)*time.Millisecond),
		slog.String("provider", stat.Provider),
		slog.Group(stat.Model,
			slog.Int("tokens.in", stat.InputTokens),
			slog.Int("items", stat.Items),
		),
	))
	recordStat(ctx, stat)
}

// recordStat is the single sink hand-off. Best-effort: a nil sink is a no-op
// and a sink error never reaches the caller.
func recordStat(ctx context.Context, stat CallStat) {
	if sink := statsSinkFromContext(ctx); sink != nil {
		sink.RecordCall(stat)
	}
}
