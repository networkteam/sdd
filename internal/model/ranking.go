package model

import (
	"math"
	"time"
)

// HeatScore computes the recency-weighted in-degree of an entry: each
// incoming reference contributes `decay(ageDays(ref_source))` to the
// score, where ageDays is the gap between `now` and the referencing
// entry's creation time. Default decay is exp-14d when callers don't
// override (see DefaultDecayName).
//
// Heat is the foundational rank signal: an entry referenced by many
// recent entries has high heat; an entry referenced only by old entries
// has low heat regardless of in-degree.
func HeatScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64 {
	if decay == nil {
		return 0
	}
	var sum float64
	for _, refID := range g.RefsTo[e.ID] {
		ref, ok := g.ByID[refID]
		if !ok {
			continue
		}
		ageDays := now.Sub(ref.Time).Hours() / 24
		sum += decay(ageDays)
	}
	return sum
}

// InDegreeScore returns the raw count of incoming references — purely
// structural centrality with no recency weighting. Equivalent to
// HeatScore with the `none` decay.
func InDegreeScore(g *Graph, e *Entry) float64 {
	return float64(len(g.RefsTo[e.ID]))
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
