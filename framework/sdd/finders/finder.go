// Package finders processes domain query structs into results. Finders
// encapsulate the actual read logic and hold injected dependencies.
// Pure reads — no side effects of their own (though injected dependencies
// may perform IO, e.g. the LLM call behind the pre-flight runner).
package finders

import "github.com/networkteam/resonance/framework/sdd/llm"

// Finder holds dependencies and config shared across query methods.
type Finder struct {
	preflightRunner llm.Runner
}

// New constructs a Finder with the given dependencies.
func New(preflightRunner llm.Runner) *Finder {
	return &Finder{
		preflightRunner: preflightRunner,
	}
}
