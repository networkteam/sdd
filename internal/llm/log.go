package llm

import (
	"context"
	"log/slog"
	"time"

	"github.com/networkteam/slogutils"
)

// logCallResult emits a single debug-level log line for an LLM call under a
// slog.Group("llm"). Per-model sub-groups are added when the runner provided
// metadata — both the claude-cli and gollm paths populate this now. The op +
// duration line fires regardless so -vv always shows progress across providers.
// When usage is present it also hands the call's metrics to a StatsSink (if one
// is on the context) for durable collection.
func logCallResult(ctx context.Context, meta *LLMMetadata, op string, elapsed time.Duration) {
	logger := slogutils.FromContext(ctx)

	llmAttrs := []any{
		slog.String("op", op),
		slog.Duration("duration", elapsed),
	}

	if meta != nil {
		if meta.Provider != "" {
			llmAttrs = append(llmAttrs, slog.String("provider", meta.Provider))
		}
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

	logger.Debug("llm call", slog.Group("llm", llmAttrs...))

	recordCallStat(ctx, meta, op, elapsed)
}

// recordCallStat hands the call's metrics to a StatsSink when one is set on the
// context and the provider reported usage. Best-effort: a nil sink, nil meta,
// or sink error never affects the caller.
func recordCallStat(ctx context.Context, meta *LLMMetadata, op string, elapsed time.Duration) {
	if meta == nil {
		return
	}
	sink := statsSinkFromContext(ctx)
	if sink == nil {
		return
	}
	sink.RecordCall(CallStat{
		Op:                op,
		Provider:          meta.Provider,
		Model:             primaryModel(meta),
		InputTokens:       meta.InputTokens,
		OutputTokens:      meta.OutputTokens,
		CacheReadTokens:   meta.CacheReadTokens,
		CacheCreateTokens: meta.CacheCreateTokens,
		DurationMS:        elapsed.Milliseconds(),
	})
}

// RecordEmbedCall logs one embedding batch at debug level and hands its
// metrics to the StatsSink on ctx (if any). It mirrors logCallResult for the
// embedding path, which carries no LLMMetadata: embeddings report input
// tokens and an item count, never output tokens or a cache breakdown. The
// caller supplies a fully-populated CallStat (op, provider, model, items,
// input tokens, duration). Best-effort — a nil sink is a no-op and a sink
// error never reaches the caller.
func RecordEmbedCall(ctx context.Context, stat CallStat) {
	logger := slogutils.FromContext(ctx)
	logger.Debug("llm call", slog.Group("llm",
		slog.String("op", stat.Op),
		slog.Duration("duration", time.Duration(stat.DurationMS)*time.Millisecond),
		slog.String("provider", stat.Provider),
		slog.Group(stat.Model,
			slog.Int("tokens.in", stat.InputTokens),
			slog.Int("items", stat.Items),
		),
	))

	if sink := statsSinkFromContext(ctx); sink != nil {
		sink.RecordCall(stat)
	}
}

// primaryModel returns a representative model name from the per-model usage map
// (one entry on the gollm path), or "" when none is recorded.
func primaryModel(meta *LLMMetadata) string {
	for name := range meta.Models {
		return name
	}
	return ""
}
