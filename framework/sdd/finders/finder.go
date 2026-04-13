// Package finders processes domain query structs into results. Finders
// encapsulate the actual read logic and hold injected dependencies.
// Pure reads — no side effects of their own (though injected dependencies
// may perform IO, e.g. the LLM call behind the pre-flight runner).
package finders

import "context"

// PreflightRunner executes a rendered prompt and returns the model's raw
// response. Injected into Finder so production uses the claude CLI and
// tests can substitute a fake.
type PreflightRunner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

// Finder holds dependencies and config shared across query methods.
type Finder struct {
	preflightRunner PreflightRunner
}

// New constructs a Finder with the given dependencies.
func New(preflightRunner PreflightRunner) *Finder {
	return &Finder{
		preflightRunner: preflightRunner,
	}
}
