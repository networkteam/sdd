package model

import (
	"math"
	"testing"
)

const epsilon = 1e-9

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestDecayByName_Unknown(t *testing.T) {
	_, err := DecayByName("not-a-decay")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestDecayByName_AllNames(t *testing.T) {
	names := []string{
		"exp-7d", "exp-14d", "exp-30d",
		"linear-7d", "linear-14d", "linear-30d",
		"none",
	}
	for _, n := range names {
		t.Run(n, func(t *testing.T) {
			fn, err := DecayByName(n)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fn == nil {
				t.Fatalf("nil decay function for %q", n)
			}
		})
	}
}

func TestDecay_AgeZero(t *testing.T) {
	// Every decay returns 1 at age 0 — that's the canonical "fresh
	// reference contributes its full weight" anchor.
	for _, n := range []string{
		"exp-7d", "exp-14d", "exp-30d",
		"linear-7d", "linear-14d", "linear-30d",
		"none",
	} {
		t.Run(n, func(t *testing.T) {
			fn, _ := DecayByName(n)
			if got := fn(0); !approxEqual(got, 1) {
				t.Errorf("%s(0) = %v, want 1", n, got)
			}
		})
	}
}

func TestDecayExponential_HalfLife(t *testing.T) {
	cases := []struct {
		name     string
		halfLife float64
	}{
		{"exp-7d", 7},
		{"exp-14d", 14},
		{"exp-30d", 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, _ := DecayByName(tc.name)
			// At one half-life, weight is 0.5.
			if got := fn(tc.halfLife); !approxEqual(got, 0.5) {
				t.Errorf("%s(%v) = %v, want 0.5", tc.name, tc.halfLife, got)
			}
			// At two half-lives, weight is 0.25.
			if got := fn(tc.halfLife * 2); !approxEqual(got, 0.25) {
				t.Errorf("%s(%v) = %v, want 0.25", tc.name, tc.halfLife*2, got)
			}
			// At ten half-lives, weight is small but nonzero (exp never reaches 0).
			if got := fn(tc.halfLife * 10); got <= 0 {
				t.Errorf("%s(%v) = %v, want > 0 (exp never reaches 0)", tc.name, tc.halfLife*10, got)
			}
		})
	}
}

func TestDecayLinear_Window(t *testing.T) {
	cases := []struct {
		name   string
		window float64
	}{
		{"linear-7d", 7},
		{"linear-14d", 14},
		{"linear-30d", 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, _ := DecayByName(tc.name)
			// At half the window, weight is 0.5.
			if got := fn(tc.window / 2); !approxEqual(got, 0.5) {
				t.Errorf("%s(%v) = %v, want 0.5", tc.name, tc.window/2, got)
			}
			// At the window edge, weight is 0.
			if got := fn(tc.window); !approxEqual(got, 0) {
				t.Errorf("%s(%v) = %v, want 0", tc.name, tc.window, got)
			}
			// Past the window, weight stays 0 (no negative weights).
			if got := fn(tc.window * 2); !approxEqual(got, 0) {
				t.Errorf("%s(%v) = %v, want 0", tc.name, tc.window*2, got)
			}
		})
	}
}

func TestDecayNone_AlwaysOne(t *testing.T) {
	fn, _ := DecayByName("none")
	for _, age := range []float64{0, 1, 7, 30, 1000} {
		if got := fn(age); !approxEqual(got, 1) {
			t.Errorf("none(%v) = %v, want 1", age, got)
		}
	}
}

func TestDecay_NegativeAgeClamped(t *testing.T) {
	// Negative age is treated as 0 — defensive against clock skew where
	// a reference's source appears slightly newer than now. Each decay
	// returns 1 in that case.
	for _, n := range []string{"exp-14d", "linear-14d", "none"} {
		t.Run(n, func(t *testing.T) {
			fn, _ := DecayByName(n)
			if got := fn(-5); !approxEqual(got, 1) {
				t.Errorf("%s(-5) = %v, want 1", n, got)
			}
		})
	}
}
