package embed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/networkteam/sdd/pkg/llm"
)

// decorated wraps an Embedder's Embed while passing Fingerprint through.
type decorated struct {
	inner Embedder
	embed func(context.Context, Request) (Result, error)
}

func (d decorated) Embed(ctx context.Context, req Request) (Result, error) {
	return d.embed(ctx, req)
}

func (d decorated) Fingerprint() string {
	return d.inner.Fingerprint()
}

// Bounded decorates an Embedder with a per-call deadline over the whole
// Embed, however many transport round-trips the adapter makes for it.
func Bounded(e Embedder, timeout time.Duration) Embedder {
	return decorated{inner: e, embed: func(ctx context.Context, req Request) (Result, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return e.Embed(ctx, req)
	}}
}

// RateLimited decorates an Embedder with a token-bucket limiter of rps calls
// per second. The burst is one second's worth of calls, at least one.
func RateLimited(e Embedder, rps float64) Embedder {
	limiter := rate.NewLimiter(rate.Limit(rps), max(int(rps), 1))
	return decorated{inner: e, embed: func(ctx context.Context, req Request) (Result, error) {
		if err := limiter.Wait(ctx); err != nil {
			return Result{}, fmt.Errorf("rate limiter: %w", err)
		}
		return e.Embed(ctx, req)
	}}
}

// Observed decorates an Embedder so every call hands one llm.CallStat to sink,
// with Items set to the batch size and Usage taken from the result. Failure
// attribution and the nil-sink behaviour match llm.Observed.
func Observed(e Embedder, sink llm.StatsSink) Embedder {
	if sink == nil {
		return e
	}
	return decorated{inner: e, embed: func(ctx context.Context, req Request) (Result, error) {
		start := time.Now()
		result, err := e.Embed(ctx, req)
		stat := llm.CallStat{Purpose: string(req.Purpose), Items: len(req.Texts), Duration: time.Since(start)}
		if err != nil {
			if attributed, ok := errors.AsType[*llm.Error](err); ok {
				stat.Identity = attributed.Identity
			}
			stat.Error = err.Error()
		} else {
			stat.Identity = result.Identity
			stat.Usage = result.Usage
		}
		sink.RecordCall(ctx, stat)
		return result, err
	}}
}
