// Package llm is the public LLM boundary of the SDD framework
// (20260830-234501-d-cpt-q6n): pure vocabulary plus one one-method interface,
// and the composition pieces every host needs around it — Bounded, RateLimited,
// Observed with its CallStat and StatsSink, ByPurpose (20260902-114838-d-tac-cov).
// No provider adapters, no configuration, no I/O: those stay at each host's
// composition site.
//
// The guiding analogy is net/http: Runner is our RoundTripper. Attribution is
// response-carried (Result.Identity) rather than a method on the interface,
// because implementations may route.
package llm

import "context"

// Purpose names what a call is for: a routing key for implementations and an
// observability dimension, the same value for both so they cannot drift. The
// set is closed: purposes are minted only by application operations. Hosts
// route on these constants and never invent values.
type Purpose string

const (
	PurposePreflight    Purpose = "preflight"
	PurposeSummarize    Purpose = "summarize"
	PurposeWritingGuide Purpose = "writing-guide"
)

// Request carries everything an implementation may route on: ctx (who) and
// Purpose (what for), plus the two-part prompt.
type Request struct {
	Purpose Purpose
	// SystemPrompt is the stable prefix, cacheable by providers that can.
	SystemPrompt string
	// UserPrompt is the per-call variable part.
	UserPrompt string
}

// Combined returns SystemPrompt followed by UserPrompt separated by a blank
// line when both are non-empty. Runners without native system-prompt support
// use this to flatten the Request into a single payload.
func (r Request) Combined() string {
	if r.SystemPrompt == "" {
		return r.UserPrompt
	}
	if r.UserPrompt == "" {
		return r.SystemPrompt
	}
	return r.SystemPrompt + "\n\n" + r.UserPrompt
}

// Identity names what served a call, reported per call on Result. It is never
// a static property of an implementation, because implementations may route.
type Identity struct {
	Provider string
	Model    string
	// Variant is the behaviour-affecting configuration the model ran under —
	// a reasoning effort, a thinking budget — carried so calls at different
	// settings measure apart. Canonical form is comma-separated key=value in
	// sorted key order ("reasoning_effort=high"); empty at model defaults.
	//
	// The boundary is what the request carries, not what the setting means:
	// a value sent as its own field is a variant, a value inside the model
	// identifier is the model.
	Variant string
}

// String renders the identity for display: model, then variant in parentheses.
func (i Identity) String() string {
	if i.Variant == "" {
		return i.Model
	}
	return i.Model + " (" + i.Variant + ")"
}

// Usage is the common format for provider-reported consumption. A field the
// provider does not report stays zero: reported, never reconstructed.
type Usage struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	CostUSD           float64
}

// Result reports what a call produced and what served it.
type Result struct {
	Text string
	// Identity is required on success: what actually served this call.
	Identity Identity
	Usage    Usage
}

// Runner is the single port. Contract, stated RoundTripper-style: fill
// Result.Identity on success; report Usage, never invent it; no internal
// retries (retry is caller policy); bound your own calls, because deadlines
// are configuration and configuration is host-private; ctx carries
// request-scoped facts a routing implementation may use (tenant, logger).
type Runner interface {
	Run(ctx context.Context, req Request) (Result, error)
}

// RunnerFunc adapts a function to Runner (test doubles, error stubs).
type RunnerFunc func(context.Context, Request) (Result, error)

func (f RunnerFunc) Run(ctx context.Context, req Request) (Result, error) {
	return f(ctx, req)
}

// Error optionally attributes a failed call. An implementation that knows what
// it routed to wraps its error so failures measure like successes.
type Error struct {
	Identity Identity
	Err      error
}

func (e *Error) Error() string {
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	return e.Err
}
