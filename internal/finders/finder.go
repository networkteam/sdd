// Package finders processes domain query structs into results. Finders
// encapsulate the actual read logic and hold injected dependencies.
// Pure reads — no side effects of their own (though injected dependencies
// may perform IO, e.g. the LLM call behind the pre-flight runner).
package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/pkg/llm"
)

// Finder holds dependencies and config shared across query methods.
// Config is a snapshot taken at construction time — short-lived CLI means
// a single read at the composition root is sufficient. Nil means the
// finder was composed without Options.Config — a wiring fault, never a
// graph state — so the config accessors error rather than degrade
// (s-tac-uya). A composition root where absence is a legitimate state
// (fresh repo, read-only commands, tests) resolves it to an explicit
// empty PerRepoConfig instead of passing nil.
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

// localParticipant returns the canonical participant from config; "" means
// the key is unset.
func (f *Finder) localParticipant() (string, error) {
	if f.cfg == nil {
		return "", fmt.Errorf("local participant unavailable: no per-repo config")
	}
	return f.cfg.Participant, nil
}

// language returns the configured graph language locale code from config;
// "" means the key is unset (English default).
func (f *Finder) language() (string, error) {
	if f.cfg == nil {
		return "", fmt.Errorf("graph language unavailable: no per-repo config")
	}
	return f.cfg.Language, nil
}

// declaredDependencies returns the repo's committed cross-repo dependency
// declarations. Pre-flight's declared-dependency precondition reasons
// against this list.
func (f *Finder) declaredDependencies() ([]string, error) {
	if f.cfg == nil {
		return nil, fmt.Errorf("declared dependencies unavailable: no per-repo config")
	}
	return f.cfg.Dependencies, nil
}
