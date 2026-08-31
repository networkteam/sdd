// Package finders processes domain query structs into results. Finders
// encapsulate the actual read logic and hold injected dependencies.
// Pure reads — no side effects of their own (though injected dependencies
// may perform IO, e.g. the LLM call behind the pre-flight runner).
package finders

import (
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/pkg/llm"
)

// Finder holds dependencies and config shared across query methods.
// Config is a snapshot taken at construction time — short-lived CLI means
// a single read at the composition root is sufficient. Nil means no config
// is available (fresh repo, read-only commands, tests); finder methods
// degrade gracefully.
type Finder struct {
	preflightRunner    llm.Runner
	writingGuideRunner llm.Runner
	cfg                *model.PerRepoConfig
	gitSyncer          GitSyncer
	repos              *repos.Registry
	procedureRegistry  *engine.Registry
}

// Options configures a new Finder. Zero-valued fields mean "not available"
// — methods that need a dependency fall back to empty/nil behaviour rather
// than failing.
type Options struct {
	PreflightRunner    llm.Runner
	WritingGuideRunner llm.Runner
	Config             *model.PerRepoConfig
	GitSyncer          GitSyncer
	// Repos is the pure read surface over the connected repos — the only
	// cross-repo capability a finder holds (no clone, no pull). Nil means no
	// connected-repos support: cross-repo refs stay unresolved.
	Repos *repos.Registry
	// ProcedureRegistry is the engine registration procedure specs load and
	// size against (d-tac-rzi): pre-flight's serve-budget arithmetic and
	// lint's procedure-runtime provider. Nil skips both checks.
	ProcedureRegistry *engine.Registry
}

// New constructs a Finder with the given options.
func New(opts Options) *Finder {
	return &Finder{
		preflightRunner:    opts.PreflightRunner,
		writingGuideRunner: opts.WritingGuideRunner,
		cfg:                opts.Config,
		gitSyncer:          opts.GitSyncer,
		repos:              opts.Repos,
		procedureRegistry:  opts.ProcedureRegistry,
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

// language returns the configured graph language locale code from config, or
// "" when no config is available or the key is unset (English default).
// Shared helper for Preflight (LLM input) and Status (render data).
func (f *Finder) language() string {
	if f.cfg == nil {
		return ""
	}
	return f.cfg.Language
}

// declaredDependencies returns the repo's committed cross-repo dependency
// declarations, or nil when no config is available. Pre-flight's
// declared-dependency precondition reasons against this list.
func (f *Finder) declaredDependencies() []string {
	if f.cfg == nil {
		return nil
	}
	return f.cfg.Dependencies
}
