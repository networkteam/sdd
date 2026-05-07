package model

import (
	"strings"
	"testing"
	"time"
)

// referenceNow is a Friday at noon UTC, picked deliberately so day-,
// week-, month-, and year-arithmetic each have unambiguous expected
// outcomes when subtracted.
var referenceNow = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func TestResolveSinceSpec_ISODate(t *testing.T) {
	got, err := ResolveSinceSpec("2026-04-15", referenceNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveSinceSpec_DurationDays(t *testing.T) {
	got, err := ResolveSinceSpec("7d", referenceNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := referenceNow.Add(-7 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveSinceSpec_DurationWeeks(t *testing.T) {
	got, err := ResolveSinceSpec("2w", referenceNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := referenceNow.Add(-14 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveSinceSpec_DurationMonthsCalendar(t *testing.T) {
	// Calendar arithmetic: 1 month before May 1 is April 1, regardless
	// of February's short length etc. This is the AC-21 calendar-correct
	// guarantee from d-tac-uww.
	got, err := ResolveSinceSpec("1m", referenceNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveSinceSpec_DurationYearsCalendar(t *testing.T) {
	// 1 year before 2026-05-01 is 2025-05-01 — same date, prior year.
	got, err := ResolveSinceSpec("1y", referenceNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2025, 5, 1, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveSinceSpec_ZeroDuration(t *testing.T) {
	// 0d cutoff equals now (no entries excluded by time other than
	// future-dated, which shouldn't exist in practice).
	got, err := ResolveSinceSpec("0d", referenceNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(referenceNow) {
		t.Errorf("got %v, want %v (zero duration = now)", got, referenceNow)
	}
}

func TestResolveSinceSpec_Invalid(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		errContains string
	}{
		{"empty", "", "empty spec"},
		{"unknown unit", "7x", "unrecognized spec"},
		{"non-numeric duration prefix", "abcd", "invalid duration"},
		{"malformed date", "2026/04/15", "unrecognized spec"},
		{"date with bad month", "2026-13-01", "invalid ISO date"},
		{"negative duration", "-7d", "non-negative"},
		{"just a unit", "d", "unrecognized spec"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveSinceSpec(tc.input, referenceNow)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
			}
		})
	}
}
