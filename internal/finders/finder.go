// Package finders processes domain query structs into results. Finders
// encapsulate the actual read logic and hold injected dependencies.
// Pure reads — no side effects of their own (though injected dependencies
// may perform IO, e.g. the LLM call behind the pre-flight runner).
package finders

import (
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

// Finder holds dependencies and config shared across query methods.
// Config is a snapshot taken at construction time — short-lived CLI means
// a single read at the composition root is sufficient. Nil means no config
// is available (fresh repo, read-only commands, tests); finder methods
// degrade gracefully.
type Finder struct {
	preflightRunner llm.Runner
	cfg             *model.Config
}

// Options configures a new Finder. Zero-valued fields mean "not available"
// — methods that need a dependency fall back to empty/nil behaviour rather
// than failing.
type Options struct {
	PreflightRunner llm.Runner
	Config          *model.Config
}

// New constructs a Finder with the given options.
func New(opts Options) *Finder {
	return &Finder{
		preflightRunner: opts.PreflightRunner,
		cfg:             opts.Config,
	}
}

// localParticipant returns the canonical participant from config, or ""
// when no config is available. Shared helper for Preflight (LLM input)
// and Status (render data).
func (f *Finder) localParticipant() string {
	if f.cfg == nil {
		return ""
	}
	return f.cfg.Participant
}
