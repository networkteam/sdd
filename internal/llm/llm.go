// Package llm handles LLM-based generation tasks: prompt rendering, template
// embedding, runner invocation, and output parsing. Finders (read side) and
// handlers (write side) both consume this package — it owns the "call an LLM
// with a structured prompt" concern.
package llm

import (
	"context"
	"time"
)

// Identity names what serves a call. It is fixed at construction from
// configuration, never derived from a response — a call that fails returns no
// response at all, and a provider that reports no usage still has a name.
type Identity struct {
	Provider string
	Model    string
	// Variant distinguishes configurations of the same model that behave
	// differently enough to measure apart — a reasoning effort, a thinking
	// budget. Such a setting changes latency and token usage, so folding those
	// calls into one row averages two populations into a number describing
	// neither. Canonical form is comma-separated key=value in sorted key order
	// ("reasoning_effort=high"); empty when the model runs at its defaults.
	//
	// The boundary is what the request carries, not what the setting means:
	// a value sent as its own field is a variant, a value inside the model
	// identifier is the model. So an Ollama tag ("glm-5.3-flash:cloud") stays
	// part of Model — it already separates its own rows, and splitting it back
	// out would mean parsing structure into a string we receive opaquely.
	Variant string
}

// String renders the identity for display: model, then variant in parentheses.
func (i Identity) String() string {
	if i.Variant == "" {
		return i.Model
	}
	return i.Model + " (" + i.Variant + ")"
}

// Runner executes a structured LLM request and returns the response with
// usage metadata. The implementation decides which model and transport to use.
// Injected so tests can substitute fakes.
//
// Identity is part of the contract because the runner is the only layer that
// knows what it talks to. Every layer below it — the call recorder, the stats
// sink, the report — must take that answer rather than reconstruct one, and a
// layer that cannot know a value must require it instead of defaulting it.
type Runner interface {
	Identity() Identity
	Run(ctx context.Context, req Request) (*RunResult, error)
}

// Request carries the two-part prompt submitted to a Runner. SystemPrompt
// holds the stable portion (instructions, structural rules) — providers that
// support prompt caching treat this as the cacheable prefix. UserPrompt holds
// the per-call variable portion (entry content, refs). Runners that can't
// distinguish system from user concatenate them with SystemPrompt first.
type Request struct {
	SystemPrompt string
	UserPrompt   string
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

// Run executes a pre-rendered Request against the Runner, emits the standard
// debug log entry, and records the call. Every chat call goes through here —
// the CLI paths directly, the engine path via the host's executor adapter — so
// that logging and stats collection have exactly one site.
func Run(ctx context.Context, runner Runner, req Request, op string) (*RunResult, error) {
	id := runner.Identity()
	start := time.Now()
	output, err := runner.Run(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		logCallFailure(ctx, id, op, elapsed, err)
		return nil, err
	}
	logCallResult(ctx, id, output.Meta, op, elapsed)
	return output, nil
}

// RunResult holds the LLM response text and optional metadata.
type RunResult struct {
	Text string
	Meta *LLMMetadata
}

// LLMMetadata holds agent-neutral per-call usage reported by the provider.
// It carries no provider or model name: identity comes from the Runner, so a
// provider that reports nothing here still leaves an attributed record.
type LLMMetadata struct {
	TotalCostUSD      float64
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	NumTurns          int
	Duration          time.Duration
	DurationAPI       time.Duration
	Models            map[string]ModelUsage
}

// ModelUsage holds per-model token and cost metrics.
type ModelUsage struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	CostUSD           float64
}
