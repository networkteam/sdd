package model

import (
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

func TestHeatScore_NilDecaySafe(t *testing.T) {
	// Defensive: nil decay (programmer error from caller) returns 0
	// rather than panicking.
	target := minimalEntry("20260101-100000-d-tac-trg", fixedNow)
	g := NewGraph([]*Entry{target})

	if got := HeatScore(g, target, nil, fixedNow); got != 0 {
		t.Errorf("heat with nil decay: got %v, want 0", got)
	}
}
