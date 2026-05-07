package finders

import (
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// DefaultStalledThreshold is the score below which a focus target is
// classified as stalled when actors are assigned. Default 1.0 is "fewer
// than one fresh ref-equivalent post-14d-decay" — under exp-14d, a single
// ref two weeks old contributes 0.5, so a target needs more than two
// weeks of inactivity (or fewer than ~two recent refs) to drop below.
// Configurable via the `stalled(value)` modifier per d-tac-uww §6.
//
// The default landed during slice 7 implementation as a starting point;
// the slice 8 closing findings attachment compares this default against
// alternatives on the live graph.
const DefaultStalledThreshold = 1.0

// expandInvolvement transforms a slice of focus entries into a FocusBlock
// by resolving each focus's involvement triples into target rows. Targets
// whose graph status is closed or superseded are omitted per design §6.
// Closed targets stay in the focus's involvement: list (immutability) but
// don't surface in the focus-block — the block is a "what's still in
// scope and not yet done" view, not a historical record.
//
// scoreFn returns the heat score for a target under the section's
// configured rank; nil scoreFn produces zero scores and skips state
// classification (state defaults to FocusStateDriving when actors set,
// pull-available when empty). The stalledThreshold is applied only when
// scoreFn is non-nil.
func expandInvolvement(g *model.Graph, focuses []*model.Entry, scoreFn func(*model.Entry) float64, stalledThreshold float64) model.FocusBlock {
	closed := closedSet(g)
	superseded := supersededSet(g)

	block := model.FocusBlock{}
	for _, focus := range focuses {
		group := model.FocusGroup{
			Focus:  focus,
			Actors: focus.FocusActors,
			When:   focus.FocusWhen,
		}
		for _, inv := range focus.Involvement {
			target := g.ByID[inv.Target]
			if target == nil {
				continue // dangling reference (lint surfaces it elsewhere)
			}
			if _, dropped := closed[target.ID]; dropped {
				continue
			}
			if _, dropped := superseded[target.ID]; dropped {
				continue
			}

			row := model.FocusTarget{
				Target:         target,
				ResolvedActors: focus.ResolveActors(inv),
				ActorsExplicit: inv.ActorsSet,
				ResolvedWhen:   focus.ResolveWhen(inv),
			}
			if scoreFn != nil {
				row.Score = scoreFn(target)
			}
			row.State = deriveFocusState(row.ResolvedActors, row.Score, scoreFn != nil, stalledThreshold)
			group.Targets = append(group.Targets, row)
		}
		block.Focuses = append(block.Focuses, group)
	}
	return block
}

// deriveFocusState applies the design §6 algorithm:
//
//	resolved_actors empty           → pull-available
//	heat < stalled_threshold        → stalled (only when ranked)
//	otherwise                       → driving
//
// "Resolved actors empty" includes both the explicit `actors: []` case
// and the focus-level default-empty case — both produce nil/zero-length
// after ResolveActors, and both signify "in scope, awaiting pickup."
//
// hasScore is the toggle for stalled classification: an unranked
// section can't compute heat, so it can't classify stalled. With actors
// set and no score, treat as driving (the optimistic default — better
// to show "engaged" than to mark stalled in the absence of evidence).
func deriveFocusState(actors []string, score float64, hasScore bool, threshold float64) model.FocusState {
	if len(actors) == 0 {
		return model.FocusStatePullAvailable
	}
	if hasScore && score < threshold {
		return model.FocusStateStalled
	}
	return model.FocusStateDriving
}

// closedSet/supersededSet replicate the Graph helpers (which are in the
// model package's focus.go) for the focus-block expansion. The model
// helpers are private; surface a finder-side equivalent so we don't
// widen the model API for one expansion path. Both walk the graph once
// and cache; expandInvolvement uses them per call so a Finder reused
// across many ViewQuery calls doesn't share stale state.

func closedSet(g *model.Graph) map[string]struct{} {
	out := make(map[string]struct{})
	for _, e := range g.Entries {
		for _, id := range e.Closes {
			out[id] = struct{}{}
		}
	}
	return out
}

func supersededSet(g *model.Graph) map[string]struct{} {
	out := make(map[string]struct{})
	for _, e := range g.Entries {
		for _, id := range e.Supersedes {
			out[id] = struct{}{}
		}
	}
	return out
}

// focusBlockScorer returns the per-entry heat scorer used for focus-block
// state derivation. Slice 7 hard-codes heat(exp-14d) — design §6's
// default rank — rather than threading section-level rank() through to
// state classification. rank() is mutually exclusive with as-focus-block
// in this slice; the only knob over state classification is the
// stalled(value) threshold modifier.
//
// Returning a closure scoped to the graph + clock keeps the call site
// simple: expandInvolvement consumes a `func(*Entry) float64` agnostic of
// where the score comes from, which keeps tests deterministic when they
// inject a different scoring function.
func focusBlockScorer(g *model.Graph, now time.Time) func(*model.Entry) float64 {
	decay, _ := model.DecayByName("exp-14d")
	return func(e *model.Entry) float64 {
		return model.HeatScore(g, e, decay, now)
	}
}
