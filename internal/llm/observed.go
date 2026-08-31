package llm

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/networkteam/slogutils"

	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

// Observed decorates a Runner with the local host's observability: one
// debug-level log line and one CallStat row per call, success or failure.
// Everything it records travels in the port data — Purpose in the Request,
// Identity and Usage in the Result, failure attribution in the typed
// pkg/llm error — so recording is a composition convention, not a framework
// hook. A nil sink logs but records nothing.
func Observed(r pkgllm.Runner, sink StatsSink) pkgllm.Runner {
	return pkgllm.RunnerFunc(func(ctx context.Context, req pkgllm.Request) (pkgllm.Result, error) {
		start := time.Now()
		result, err := r.Run(ctx, req)
		elapsed := time.Since(start)
		logger := slogutils.FromContext(ctx)
		if err != nil {
			// A runner that knows what it routed to attributes its failure;
			// one that does not leaves the identity blank — visibly absent,
			// never invented.
			var identity pkgllm.Identity
			if attributed, ok := errors.AsType[*pkgllm.Error](err); ok {
				identity = attributed.Identity
			}
			logger.Debug("llm call failed", slog.Group("llm",
				slog.String("op", string(req.Purpose)),
				slog.Duration("duration", elapsed),
				slog.String("provider", identity.Provider),
				slog.String("model", identity.String()),
				slog.String("error", err.Error()),
			))
			record(sink, CallStat{
				Purpose:    string(req.Purpose),
				Identity:   identity,
				DurationMS: elapsed.Milliseconds(),
				Error:      err.Error(),
			})
			return result, err
		}
		attrs := []any{
			slog.String("op", string(req.Purpose)),
			slog.Duration("duration", elapsed),
			slog.String("provider", result.Identity.Provider),
			slog.String("model", result.Identity.String()),
			slog.Int("tokens.in", result.Usage.InputTokens),
			slog.Int("tokens.out", result.Usage.OutputTokens),
		}
		if result.Usage.CacheReadTokens > 0 || result.Usage.CacheCreateTokens > 0 {
			attrs = append(attrs,
				slog.Int("cache.read", result.Usage.CacheReadTokens),
				slog.Int("cache.create", result.Usage.CacheCreateTokens),
			)
		}
		if result.Usage.CostUSD > 0 {
			attrs = append(attrs, slog.Float64("cost", result.Usage.CostUSD))
		}
		logger.Debug("llm call", slog.Group("llm", attrs...))
		record(sink, CallStat{
			Purpose:    string(req.Purpose),
			Identity:   result.Identity,
			Usage:      result.Usage,
			DurationMS: elapsed.Milliseconds(),
		})
		return result, nil
	})
}

// record is the single sink hand-off for the chat path. Best-effort: a nil
// sink is a no-op.
func record(sink StatsSink, stat CallStat) {
	if sink != nil {
		sink.RecordCall(stat)
	}
}

// RecordEmbedCall logs one embedding batch at debug level and hands its
// metrics to the StatsSink on ctx (if any). The embedding twin of the Observed
// decorator's recording: embeddings report input tokens and an item count,
// never output tokens or a cache breakdown. The caller supplies a
// fully-populated CallStat.
func RecordEmbedCall(ctx context.Context, stat CallStat) {
	slogutils.FromContext(ctx).Debug("llm call", slog.Group("llm",
		slog.String("op", stat.Purpose),
		slog.Duration("duration", time.Duration(stat.DurationMS)*time.Millisecond),
		slog.String("provider", stat.Identity.Provider),
		slog.Group(stat.Identity.Model,
			slog.Int("tokens.in", stat.Usage.InputTokens),
			slog.Int("items", stat.Items),
		),
	))
	if sink := statsSinkFromContext(ctx); sink != nil {
		sink.RecordCall(stat)
	}
}
