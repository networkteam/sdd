// Package llm holds the local host's LLM client machinery around the public
// pkg/llm contract: the observing decorator, the timeout decorator, the
// CallStat/StatsSink recording types, and the embedding plumbing. Provider
// adapters live in the claude and gollm sub-packages; the factory composes
// them; the LLM operations themselves live in internal/llmops.
package llm

import (
	"context"

	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

// CallStat is one LLM call's metrics, handed to a StatsSink for durable
// collection. The timestamp is added by the sink implementation. The sink owns
// the wire shape; this is the in-process form.
type CallStat struct {
	// Purpose names what the call was for. Chat calls carry a pkg/llm Purpose
	// constant; embedding calls carry their own op names, which is why the
	// field is a plain string.
	Purpose  string
	Identity pkgllm.Identity
	Usage    pkgllm.Usage
	// Items is the number of inputs in this call. The embedding path sets it
	// (one batch = N texts) so throughput (items or tokens per second) is
	// derivable from DurationMS; chat calls are single-prompt and leave it 0.
	Items      int
	DurationMS int64
	// Error is the failure text when the call did not return a result, empty
	// on success. Failures are recorded because a call that times out or comes
	// back unparseable is exactly what the sink exists to make countable —
	// dropping it hides the brittleness it is evidence of. Such a row carries
	// no tokens, and provider and model only when the failure happened past
	// the point they were known.
	Error string
}

// StatsSink durably records per-call LLM metrics (e.g. to a local JSONL file).
// Implementations must be safe for concurrent use — batch operations like
// `sdd summarize --all` call them from multiple goroutines.
type StatsSink interface {
	RecordCall(CallStat)
}

type statsSinkKey struct{}

// WithStatsSink returns a context carrying the sink, retrieved by the
// embedding recording path. Chat calls record through the Observed decorator
// instead; the context carry remains until embeddings get the same treatment.
// A nil sink is permitted and makes recording a no-op.
func WithStatsSink(ctx context.Context, sink StatsSink) context.Context {
	return context.WithValue(ctx, statsSinkKey{}, sink)
}

// statsSinkFromContext returns the sink carried by ctx, or nil when none is set.
func statsSinkFromContext(ctx context.Context) StatsSink {
	s, _ := ctx.Value(statsSinkKey{}).(StatsSink)
	return s
}
