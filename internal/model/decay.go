package model

import (
	"fmt"
	"math"
)

// DecayFunc maps an entry age in days to a recency weight in [0, 1].
// Used by ranking algorithms to weight an entry's incoming references by
// recency: a recent reference contributes more to "heat" than an old one.
type DecayFunc func(ageDays float64) float64

// DecayFunc resolves a decay name to its function per d-tac-uww §4.
// Returns an error for unknown names with the listed valid set.
//
// Vocabulary:
//
//	exp-{7,14,30}d    2^(-age_days/N)            — half-life every N days
//	linear-{7,14,30}d max(0, 1 - age_days/N)     — zero past N days
//	none              1                          — no age effect
//
// Exponential decay never reaches zero, so historical references retain
// vestigial weight. Linear decay reaches zero at the window edge, so
// references older than the window contribute nothing — useful for
// strict recency cutoffs.
func DecayByName(name string) (DecayFunc, error) {
	switch name {
	case "exp-7d":
		return decayExp(7), nil
	case "exp-14d":
		return decayExp(14), nil
	case "exp-30d":
		return decayExp(30), nil
	case "linear-7d":
		return decayLinear(7), nil
	case "linear-14d":
		return decayLinear(14), nil
	case "linear-30d":
		return decayLinear(30), nil
	case "none":
		return decayNone, nil
	default:
		return nil, fmt.Errorf("unknown decay %q (known: exp-7d, exp-14d, exp-30d, linear-7d, linear-14d, linear-30d, none)", name)
	}
}

// DefaultDecayName is the decay used when an algorithm is invoked without
// an explicit decay arg (e.g. `rank(heat)` resolves to heat(exp-14d)).
const DefaultDecayName = "exp-14d"

func decayExp(halfLife float64) DecayFunc {
	return func(d float64) float64 {
		if d < 0 {
			d = 0
		}
		return math.Pow(2, -d/halfLife)
	}
}

func decayLinear(window float64) DecayFunc {
	return func(d float64) float64 {
		if d < 0 {
			d = 0
		}
		v := 1 - d/window
		if v < 0 {
			return 0
		}
		return v
	}
}

func decayNone(_ float64) float64 { return 1 }
