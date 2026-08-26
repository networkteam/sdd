package textpatch_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/textpatch"
)

func TestApplyInOrder(t *testing.T) {
	got, err := textpatch.Apply("alpha beta gamma", []textpatch.Pair{
		{Old: "beta", New: "BETA"},
		{Old: "alpha BETA", New: "start"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "start gamma" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyEmptyPairsIsIdentity(t *testing.T) {
	got, err := textpatch.Apply("unchanged", nil)
	if err != nil || got != "unchanged" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestApplyRefusalsNameThePair(t *testing.T) {
	cases := []struct {
		name  string
		pairs []textpatch.Pair
		want  string
	}{
		{"not found", []textpatch.Pair{{Old: "alpha", New: "a"}, {Old: "missing", New: "x"}}, "pair 2: old text not found"},
		{"ambiguous", []textpatch.Pair{{Old: "a", New: "x"}}, "pair 1: old text matches 5 times"},
		{"empty old", []textpatch.Pair{{Old: "", New: "x"}}, "pair 1: old text is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := textpatch.Apply("alpha beta gamma", tc.pairs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestApplyAtomicOnLateFailure: the contract is all-or-nothing — a failure on
// a later pair returns an error and the caller keeps its original text.
func TestApplyAtomicOnLateFailure(t *testing.T) {
	text := "one two three"
	_, err := textpatch.Apply(text, []textpatch.Pair{
		{Old: "one", New: "1"},
		{Old: "absent", New: "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "pair 2") {
		t.Fatalf("err = %v", err)
	}
	if text != "one two three" {
		t.Fatal("input mutated")
	}
}
