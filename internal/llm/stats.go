package llm

import "context"

// CallStat is one LLM call's metrics, handed to a StatsSink for durable
// collection. The timestamp is added by the sink implementation. Cost is
// deliberately omitted: the provider APIs on the active (gollm) path report
// tokens and the prompt-cache breakdown but not a dollar cost, and we do not
// maintain a pricing table (see d-tac-zis).
type CallStat struct {
	Op       string
	Provider string
	Model    string
	// Items is the number of inputs in this call. The embedding path sets it
	// (one batch = N texts) so throughput (items or tokens per second) is
	// derivable from DurationMS; chat calls are single-prompt and leave it 0.
	Items             int
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	DurationMS        int64
}

// StatsSink durably records per-call LLM metrics (e.g. to a local JSONL file).
// Implementations must be safe for concurrent use — batch operations like
// `sdd summarize --all` call them from multiple goroutines.
type StatsSink interface {
	RecordCall(CallStat)
}

type statsSinkKey struct{}

// WithStatsSink returns a context carrying the sink, retrieved later by the
// call-logging path. A nil sink is permitted and makes recording a no-op.
func WithStatsSink(ctx context.Context, sink StatsSink) context.Context {
	return context.WithValue(ctx, statsSinkKey{}, sink)
}

// statsSinkFromContext returns the sink carried by ctx, or nil when none is set.
func statsSinkFromContext(ctx context.Context) StatsSink {
	s, _ := ctx.Value(statsSinkKey{}).(StatsSink)
	return s
}
