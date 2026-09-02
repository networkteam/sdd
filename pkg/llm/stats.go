package llm

import (
	"context"
	"time"
)

// CallStat is one call's record, handed to a StatsSink by the observing
// decorators (Observed here, and its embedding twin in pkg/llm/embed). The
// sink owns the durable shape and adds the timestamp; this is the in-process
// form.
type CallStat struct {
	// Purpose names what the call was for: a chat Purpose or an embed.Purpose,
	// which is why the field is a plain string.
	Purpose  string
	Identity Identity
	Usage    Usage
	// Items is the number of inputs in the call. Embedding batches set it so
	// throughput per item is derivable from Duration; chat calls leave it 0.
	Items    int
	Duration time.Duration
	// Error is the failure text when the call returned no result, empty on
	// success. Failures are recorded because a call that times out or errors
	// is exactly what a sink exists to make countable. Such a row carries no
	// tokens, and an identity only when the failure was attributed.
	Error string
}

// StatsSink durably records per-call metrics. ctx is the call's context, so a
// sink can read request-scoped facts (a tenant, a logger) the way a routing
// Runner does. Implementations must be safe for concurrent use.
type StatsSink interface {
	RecordCall(ctx context.Context, stat CallStat)
}
