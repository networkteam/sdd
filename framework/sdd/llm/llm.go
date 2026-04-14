// Package llm handles LLM-based generation tasks: prompt rendering, template
// embedding, runner invocation, and output parsing. Finders (read side) and
// handlers (write side) both consume this package — it owns the "call an LLM
// with a structured prompt" concern.
package llm

import "context"

// Runner executes a rendered prompt string against an LLM and returns the raw
// response. The implementation decides which model and transport to use.
// Injected so tests can substitute fakes.
type Runner interface {
	Run(ctx context.Context, prompt string) (string, error)
}
