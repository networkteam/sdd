package model

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// fixedNow is the reference clock for ranking tests — a Wednesday at
// noon, deliberately unrelated to today's date so test results don't
// drift with calendar time.
var fixedNow = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// daysAgo returns a time `d` days before fixedNow, useful for building
// reference-source entries with deterministic ages.
func daysAgo(d float64) time.Time {
	return fixedNow.Add(-time.Duration(d * 24 * float64(time.Hour)))
}

// minimalEntry builds the smallest model.Entry sufficient for ranking
// computations. Only ID, Type, Layer, and Time are read by the algorithms.
func minimalEntry(id string, t time.Time) *Entry {
	return &Entry{
		ID:    id,
		Type:  TypeDecision,
		Layer: LayerTactical,
		Kind:  KindDirective,
		Time:  t,
	}
}

func TestHeatScore_NoIncomingRefs(t *testing.T) {
	target := minimalEntry("20260501-120000-d-tac-trg", fixedNow)
	g := NewGraph([]*Entry{target})

	decay, _ := DecayByName(DefaultDecayName)
	got := HeatScore(g, target, decay, fixedNow)
	if got != 0 {
		t.Errorf("heat with no refs: got %v, want 0", got)
	}
}

func TestHeatScore_OneFreshRef(t *testing.T) {
	// A single reference made today (age 0) should contribute 1 to heat
	// regardless of decay function — every decay returns 1 at age 0.
	target := minimalEntry("20260501-100000-d-tac-trg", daysAgo(30))
	ref := minimalEntry("20260501-120000-d-tac-ref", fixedNow)
	ref.Refs = refsOf(target.ID)

	g := NewGraph([]*Entry{target, ref})

	for _, decayName := range []string{"exp-14d", "linear-14d", "none"} {
		t.Run(decayName, func(t *testing.T) {
			decay, _ := DecayByName(decayName)
			got := HeatScore(g, target, decay, fixedNow)
			if math.Abs(got-1) > epsilon {
				t.Errorf("%s heat with one fresh ref: got %v, want 1", decayName, got)
			}
		})
	}
}

func TestHeatScore_OldRefDecays(t *testing.T) {
	// Reference made 14 days ago: exp-14d weight is 0.5, linear-14d is 0,
	// none is 1.
	target := minimalEntry("20260101-100000-d-tac-trg", daysAgo(60))
	ref := minimalEntry("20260101-120000-d-tac-ref", daysAgo(14))
	ref.Refs = refsOf(target.ID)

	g := NewGraph([]*Entry{target, ref})

	cases := []struct {
		decay string
		want  float64
	}{
		{"exp-14d", 0.5},
		{"linear-14d", 0},
		{"none", 1},
	}
	for _, tc := range cases {
		t.Run(tc.decay, func(t *testing.T) {
			decay, _ := DecayByName(tc.decay)
			got := HeatScore(g, target, decay, fixedNow)
			if math.Abs(got-tc.want) > epsilon {
				t.Errorf("%s heat: got %v, want %v", tc.decay, got, tc.want)
			}
		})
	}
}

func TestHeatScore_MultipleRefsSum(t *testing.T) {
	// Heat is a sum over incoming refs. Two fresh refs → 2.0 (with any decay).
	target := minimalEntry("20260101-100000-d-tac-trg", daysAgo(30))
	ref1 := minimalEntry("20260101-120000-d-tac-rf1", fixedNow)
	ref1.Refs = refsOf(target.ID)
	ref2 := minimalEntry("20260101-130000-d-tac-rf2", fixedNow)
	ref2.Refs = refsOf(target.ID)

	g := NewGraph([]*Entry{target, ref1, ref2})

	decay, _ := DecayByName("exp-14d")
	got := HeatScore(g, target, decay, fixedNow)
	if math.Abs(got-2) > epsilon {
		t.Errorf("heat with two fresh refs: got %v, want 2", got)
	}
}

func TestInDegreeScore(t *testing.T) {
	target := minimalEntry("20260101-100000-d-tac-trg", daysAgo(30))
	ref1 := minimalEntry("20260101-120000-d-tac-rf1", fixedNow)
	ref1.Refs = refsOf(target.ID)
	ref2 := minimalEntry("20260101-130000-d-tac-rf2", fixedNow)
	ref2.Refs = refsOf(target.ID)
	unrelated := minimalEntry("20260101-140000-d-tac-unr", fixedNow)

	g := NewGraph([]*Entry{target, ref1, ref2, unrelated})

	if got := InDegreeScore(g, target); got != 2 {
		t.Errorf("in-degree of target with 2 refs: got %v, want 2", got)
	}
	if got := InDegreeScore(g, unrelated); got != 0 {
		t.Errorf("in-degree of unreferenced entry: got %v, want 0", got)
	}
}

func TestMultScore(t *testing.T) {
	// Two fresh refs: heat(exp-14d)=2, in-degree=2, mult=4.
	target := minimalEntry("20260101-100000-d-tac-trg", daysAgo(30))
	ref1 := minimalEntry("20260101-120000-d-tac-rf1", fixedNow)
	ref1.Refs = refsOf(target.ID)
	ref2 := minimalEntry("20260101-130000-d-tac-rf2", fixedNow)
	ref2.Refs = refsOf(target.ID)

	g := NewGraph([]*Entry{target, ref1, ref2})

	decay, _ := DecayByName("exp-14d")
	got := MultScore(g, target, decay, fixedNow)
	if math.Abs(got-4) > epsilon {
		t.Errorf("mult: got %v, want 4 (heat=2 × indeg=2)", got)
	}
}

func TestAddScore(t *testing.T) {
	// Two fresh refs: heat=2, in-degree=2, add=4.
	target := minimalEntry("20260101-100000-d-tac-trg", daysAgo(30))
	ref1 := minimalEntry("20260101-120000-d-tac-rf1", fixedNow)
	ref1.Refs = refsOf(target.ID)
	ref2 := minimalEntry("20260101-130000-d-tac-rf2", fixedNow)
	ref2.Refs = refsOf(target.ID)

	g := NewGraph([]*Entry{target, ref1, ref2})

	decay, _ := DecayByName("exp-14d")
	got := AddScore(g, target, decay, fixedNow)
	if math.Abs(got-4) > epsilon {
		t.Errorf("add: got %v, want 4 (heat=2 + indeg=2)", got)
	}
}

func TestLogScore(t *testing.T) {
	// One fresh ref: heat=1, in-degree=1, log=heat × log(1 + indeg) = 1 × log(2).
	target := minimalEntry("20260101-100000-d-tac-trg", daysAgo(30))
	ref := minimalEntry("20260101-120000-d-tac-ref", fixedNow)
	ref.Refs = refsOf(target.ID)

	g := NewGraph([]*Entry{target, ref})

	decay, _ := DecayByName("exp-14d")
	got := LogScore(g, target, decay, fixedNow)
	want := math.Log(2)
	if math.Abs(got-want) > epsilon {
		t.Errorf("log: got %v, want %v (heat=1 × log(2))", got, want)
	}
}

func TestColdnessScore(t *testing.T) {
	// Coldness = decay(entry's own age) / (1 + in-degree). Built on a
	// fixed clock so every value is exact. exp-30d is coldness's own
	// half-life (DefaultColdnessDecayName), so age 30d halves the score
	// and age 120d is four half-lives (1/16).
	decay30, _ := DecayByName("exp-30d")

	cases := []struct {
		name     string
		ageDays  float64
		inDegree int
		decay    DecayFunc
		want     float64
	}{
		{"fresh, unacted", 0, 0, decay30, 1.0},
		{"acted once (gradual hand-off)", 0, 1, decay30, 0.5},
		{"acted twice", 0, 2, decay30, 1.0 / 3.0},
		{"aged, unacted (one half-life)", 30, 0, decay30, 0.5},
		{"ancient, unacted", 120, 0, decay30, 0.0625},
		{"nil decay guard", 0, 0, nil, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := minimalEntry("20260501-120000-d-tac-trg", daysAgo(tc.ageDays))
			entries := []*Entry{target}
			for i := 0; i < tc.inDegree; i++ {
				ref := minimalEntry(fmt.Sprintf("20260501-1200%02d-d-tac-rf%d", i, i), fixedNow)
				ref.Refs = refsOf(target.ID)
				entries = append(entries, ref)
			}
			g := NewGraph(entries)

			got := ColdnessScore(g, target, tc.decay, fixedNow)
			if math.Abs(got-tc.want) > epsilon {
				t.Errorf("coldness(age=%vd, in-degree=%d): got %v, want %v", tc.ageDays, tc.inDegree, got, tc.want)
			}
		})
	}
}

// supersedeChain builds `n` successive entries each superseding the previous,
// returning them oldest-first. All are fresh at fixedNow — only hop distance
// matters to the assertions built on it.
func supersedeChain(origin *Entry, n int) []*Entry {
	chain := make([]*Entry, 0, n)
	prev := origin
	for i := 0; i < n; i++ {
		e := minimalEntry(fmt.Sprintf("20260501-1300%02d-d-tac-sc%d", i, i), fixedNow)
		e.Supersedes = []string{prev.ID}
		chain = append(chain, e)
		prev = e
	}
	return chain
}

func TestHeatScore_InheritsAcrossSupersede(t *testing.T) {
	// A fresh reference to an entry that has since been superseded lands on
	// the live head, damped one hop: 1.0 × 0.8. The predecessor keeps nothing —
	// the reference resolved away from it rather than being counted twice.
	origin := minimalEntry("20260501-100000-d-tac-org", daysAgo(30))
	successor := supersedeChain(origin, 1)[0]
	ref := minimalEntry("20260501-120000-d-tac-ref", fixedNow)
	ref.Refs = refsOf(origin.ID)

	g := NewGraph([]*Entry{origin, successor, ref})
	decay, _ := DecayByName("exp-14d")

	if got := HeatScore(g, successor, decay, fixedNow); math.Abs(got-SupersedeHopFactor) > epsilon {
		t.Errorf("successor heat: got %v, want %v (one hop)", got, SupersedeHopFactor)
	}
	if got := HeatScore(g, origin, decay, fixedNow); got != 0 {
		t.Errorf("superseded origin heat: got %v, want 0", got)
	}
}

func TestHeatScore_HopsCompound(t *testing.T) {
	// hopFactor^hops by construction: each additional supersede step multiplies
	// the same reference's contribution by another factor.
	origin := minimalEntry("20260501-100000-d-tac-org", daysAgo(30))
	ref := minimalEntry("20260501-120000-d-tac-ref", fixedNow)
	ref.Refs = refsOf(origin.ID)
	decay, _ := DecayByName("exp-14d")

	for hops := 0; hops <= 3; hops++ {
		t.Run(fmt.Sprintf("%d-hops", hops), func(t *testing.T) {
			chain := supersedeChain(origin, hops)
			g := NewGraph(append([]*Entry{origin, ref}, chain...))

			head := origin
			if hops > 0 {
				head = chain[hops-1]
			}
			want := math.Pow(SupersedeHopFactor, float64(hops))
			if got := HeatScore(g, head, decay, fixedNow); math.Abs(got-want) > epsilon {
				t.Errorf("heat at head after %d hops: got %v, want %v", hops, got, want)
			}
		})
	}
}

func TestInDegreeScore_ResolvesButDoesNotDamp(t *testing.T) {
	// In-degree inherits the resolution — a successor counts what pointed at
	// its predecessor — but stays an integer count: a fractional count has no
	// meaning, so the hop factor never touches it.
	origin := minimalEntry("20260501-100000-d-tac-org", daysAgo(30))
	chain := supersedeChain(origin, 2)
	ref1 := minimalEntry("20260501-120000-d-tac-rf1", fixedNow)
	ref1.Refs = refsOf(origin.ID)
	ref2 := minimalEntry("20260501-121000-d-tac-rf2", fixedNow)
	ref2.Refs = refsOf(chain[0].ID)

	g := NewGraph(append([]*Entry{origin, ref1, ref2}, chain...))

	if got := InDegreeScore(g, chain[1]); got != 2 {
		t.Errorf("head in-degree: got %v, want 2 (undamped count)", got)
	}
	if got := InDegreeScore(g, origin); got != 0 {
		t.Errorf("superseded origin in-degree: got %v, want 0", got)
	}
}

func TestColdnessScore_SuccessorLeavesTheColdLane(t *testing.T) {
	// The point of resolution reaching in-degree: a fresh successor inheriting
	// a reference is no longer maximally cold, so it stops surfacing in the
	// open-loops lane as work nobody has touched.
	origin := minimalEntry("20260501-100000-d-tac-org", daysAgo(30))
	successor := supersedeChain(origin, 1)[0]
	ref := minimalEntry("20260501-120000-d-tac-ref", fixedNow)
	ref.Refs = refsOf(origin.ID)

	g := NewGraph([]*Entry{origin, successor, ref})
	decay, _ := DecayByName("exp-30d")

	// Successor is fresh (age 0), so decay(entryAge) = 1 and the inherited
	// in-degree of 1 halves it.
	if got := ColdnessScore(g, successor, decay, fixedNow); math.Abs(got-0.5) > epsilon {
		t.Errorf("successor coldness: got %v, want 0.5 (in-degree 1 inherited)", got)
	}
}

func TestCompositeScores_InheritSupersedeResolution(t *testing.T) {
	// mult/add/log read the same resolved index, so they inherit without
	// carrying rules of their own.
	origin := minimalEntry("20260501-100000-d-tac-org", daysAgo(30))
	successor := supersedeChain(origin, 1)[0]
	ref := minimalEntry("20260501-120000-d-tac-ref", fixedNow)
	ref.Refs = refsOf(origin.ID)

	g := NewGraph([]*Entry{origin, successor, ref})
	decay, _ := DecayByName("exp-14d")

	// heat = 0.8 (one hop), in-degree = 1 (undamped).
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"mult", MultScore(g, successor, decay, fixedNow), SupersedeHopFactor},
		{"add", AddScore(g, successor, decay, fixedNow), SupersedeHopFactor + 1},
		{"log", LogScore(g, successor, decay, fixedNow), SupersedeHopFactor * math.Log(2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if math.Abs(tc.got-tc.want) > epsilon {
				t.Errorf("%s on successor: got %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestHeatScore_ClosureDoesNotInherit(t *testing.T) {
	// Scope is supersession only. A closing entry earns its own references;
	// nothing flows across `closes`, and the closed entry keeps its heat.
	gap := minimalEntry("20260501-100000-s-tac-gap", daysAgo(30))
	closer := minimalEntry("20260501-130000-s-tac-cls", fixedNow)
	closer.Closes = []string{gap.ID}
	ref := minimalEntry("20260501-120000-d-tac-ref", fixedNow)
	ref.Refs = refsOf(gap.ID)

	g := NewGraph([]*Entry{gap, closer, ref})
	decay, _ := DecayByName("exp-14d")

	if got := HeatScore(g, gap, decay, fixedNow); math.Abs(got-1) > epsilon {
		t.Errorf("closed entry heat: got %v, want 1 (retirement is not supersession)", got)
	}
	if got := HeatScore(g, closer, decay, fixedNow); got != 0 {
		t.Errorf("closing entry heat: got %v, want 0", got)
	}
}

func TestHeatScore_NilDecaySafe(t *testing.T) {
	// Defensive: nil decay (programmer error from caller) returns 0
	// rather than panicking.
	target := minimalEntry("20260101-100000-d-tac-trg", fixedNow)
	g := NewGraph([]*Entry{target})

	if got := HeatScore(g, target, nil, fixedNow); got != 0 {
		t.Errorf("heat with nil decay: got %v, want 0", got)
	}
}
