package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// Bounded decorates a Runner with a per-call deadline. Bounding a call is the
// instance's duty under the Runner contract, so a host composes this at its
// site with its own configured timeout.
func Bounded(r Runner, timeout time.Duration) Runner {
	return RunnerFunc(func(ctx context.Context, req Request) (Result, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return r.Run(ctx, req)
	})
}

// RateLimited decorates a Runner with a token-bucket limiter of rps requests
// per second, so parallel batch operations stay under provider limits. The
// burst is one second's worth of requests, at least one.
func RateLimited(r Runner, rps float64) Runner {
	limiter := rate.NewLimiter(rate.Limit(rps), max(int(rps), 1))
	return RunnerFunc(func(ctx context.Context, req Request) (Result, error) {
		if err := limiter.Wait(ctx); err != nil {
			return Result{}, fmt.Errorf("rate limiter: %w", err)
		}
		return r.Run(ctx, req)
	})
}

// Observed decorates a Runner so every call, success or failure, hands one
// CallStat to sink. Everything recorded travels in the port data: Purpose in
// the Request, Identity and Usage in the Result, attribution of a failure in
// the typed Error. A runner that does not attribute its failure leaves the
// identity blank, visibly absent rather than invented. A nil sink returns r
// unwrapped: recording is a composition convention, and skipping it shows as
// an empty record, never as a broken call.
func Observed(r Runner, sink StatsSink) Runner {
	if sink == nil {
		return r
	}
	return RunnerFunc(func(ctx context.Context, req Request) (Result, error) {
		start := time.Now()
		result, err := r.Run(ctx, req)
		stat := CallStat{Purpose: string(req.Purpose), Duration: time.Since(start)}
		if err != nil {
			if attributed, ok := errors.AsType[*Error](err); ok {
				stat.Identity = attributed.Identity
			}
			stat.Error = err.Error()
		} else {
			stat.Identity = result.Identity
			stat.Usage = result.Usage
		}
		sink.RecordCall(ctx, stat)
		return result, err
	})
}

// ByPurpose dispatches each call to the runner registered for its Purpose,
// and to fallback for purposes not in routes. A call whose purpose has no
// route and no fallback fails; the framework mints purposes, so an unrouted
// one is a composition gap the host should see.
func ByPurpose(routes map[Purpose]Runner, fallback Runner) Runner {
	return RunnerFunc(func(ctx context.Context, req Request) (Result, error) {
		if r, ok := routes[req.Purpose]; ok {
			return r.Run(ctx, req)
		}
		if fallback == nil {
			return Result{}, fmt.Errorf("llm: no runner routed for purpose %q", req.Purpose)
		}
		return fallback.Run(ctx, req)
	})
}
