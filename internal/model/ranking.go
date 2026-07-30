package model

import (
	"math"
	"time"
)

// SupersedeHopFactor damps a reference's heat contribution once per supersede
// step between the entry it named and the live head that inherits it. At 0.8 a
// single correction keeps four fifths of its standing and three hops fall to
// about half — deliberately provisional, a calibration guess to be revised
// against comparative findings on the live graph (d-cpt-x6z).
const SupersedeHopFactor = 0.8

// HeatScore computes the recency-weighted in-degree of an entry: each
// incoming reference contributes
// `decay(ageDays(ref_source)) * SupersedeHopFactor^hops` to the score,
// where ageDays is the gap between `now` and the referencing entry's
// creation time and hops is the supersede distance the reference
// travelled to reach this entry. Default decay is exp-14d when callers
// don't override (see DefaultDecayName).
//
// Heat is the foundational rank signal: an entry referenced by many
// recent entries has high heat; an entry referenced only by old entries
// has low heat regardless of in-degree. Reading the resolved index means
// a superseding entry inherits the attention its predecessor earned,
// damped per hop rather than reset to zero.
func HeatScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64 {
	if decay == nil {
		return 0
	}
	var sum float64
	for _, in := range g.InboundRefs[e.ID] {
		ref, ok := g.ByID[in.Source]
		if !ok {
			continue
		}
		ageDays := now.Sub(ref.Time).Hours() / 24
		sum += decay(ageDays) * math.Pow(SupersedeHopFactor, float64(in.Hops))
	}
	return sum
}

// InDegreeScore returns the raw count of incoming references, resolved
// through supersession so a successor counts what pointed at its
// predecessor. The hop factor deliberately does not apply: a fractional
// count has no meaning (d-cpt-x6z).
func InDegreeScore(g *Graph, e *Entry) float64 {
	return float64(len(g.InboundRefs[e.ID]))
}

// MultScore is heat × in-degree. Entries that are both recent (high
// heat) and structurally central (high in-degree) rise to the top;
// either dimension at zero zeroes the product.
func MultScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64 {
	return HeatScore(g, e, decay, now) * InDegreeScore(g, e)
}

// AddScore is heat + in-degree. Entries that are recent OR structurally
// central both rise; the dimensions contribute additively.
//
// The d-tac-uww spec calls for `normalized(heat + in-degree)` without
// pinning down the normalization strategy. Slice 3 uses raw sum as the
// simplest defensible choice; the comparative findings attached to the
// closing done signal will inform whether to scale either dimension.
func AddScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64 {
	return HeatScore(g, e, decay, now) + InDegreeScore(g, e)
}

// LogScore is heat × log(1 + in-degree). Diminishing returns on
// in-degree softens the bias toward heavily-referenced entries while
// still rewarding recency.
func LogScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64 {
	return HeatScore(g, e, decay, now) * math.Log(1+InDegreeScore(g, e))
}

// ColdnessScore is heat's inverse: it ranks unacted-on entries highest.
// Where heat decays each incoming reference's age, coldness decays the
// entry's own creation age and divides by in-degree, so a fresh entry
// with no incoming refs scores 1 and every reference demotes it toward
// the hot lane. The 1/(1+in_degree) form makes that hand-off gradual —
// one ref halves the score, two thirds it — rather than a hard cutoff at
// the first ref. Default decay is exp-30d (DefaultColdnessDecayName) when
// callers don't override: undone work should fade slowly, not in weeks.
func ColdnessScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64 {
	if decay == nil {
		return 0
	}
	entryAgeDays := now.Sub(e.Time).Hours() / 24
	return decay(entryAgeDays) / (1 + InDegreeScore(g, e))
}
