package llm

import (
	"context"
	"time"

	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

// Bounded decorates a Runner with a per-call deadline. The pkg/llm contract
// makes bounding a call the instance's duty — the local factory always
// composes this, so a host that receives its runner gets bounded calls without
// application knowing the number.
func Bounded(r pkgllm.Runner, timeout time.Duration) pkgllm.Runner {
	return pkgllm.RunnerFunc(func(ctx context.Context, req pkgllm.Request) (pkgllm.Result, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return r.Run(ctx, req)
	})
}
