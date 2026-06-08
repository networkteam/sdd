package model_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// mkDec builds a tactical directive decision, optionally superseding others.
func mkDec(id string, supersedes ...string) *model.Entry {
	return &model.Entry{
		ID:         id,
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Kind:       model.KindDirective,
		Supersedes: supersedes,
	}
}

func entryHasWarning(e *model.Entry, substr string) bool {
	for _, w := range e.Warnings {
		if strings.Contains(w.Message, substr) {
			return true
		}
	}
	return false
}

func TestResolveRef(t *testing.T) {
	a := mkDec("20260101-100000-d-tac-aaa")
	b := mkDec("20260101-100100-d-tac-bbb", a.ID)
	c := mkDec("20260101-100200-d-tac-ccc", b.ID)
	d := mkDec("20260101-100300-d-tac-ddd") // standalone, never superseded
	g := model.NewGraph([]*model.Entry{a, b, c, d})

	tests := []struct {
		name      string
		id        string
		wantHead  string
		wantStale bool
		wantPath  []string
	}{
		{"active_target", d.ID, d.ID, false, []string{d.ID}},
		{"single_hop", b.ID, c.ID, true, []string{b.ID, c.ID}},
		{"multi_hop_from_root", a.ID, c.ID, true, []string{a.ID, b.ID, c.ID}},
		{"head_resolves_to_itself", c.ID, c.ID, false, []string{c.ID}},
		{"unknown_id_resolves_to_itself", "20260101-099999-d-tac-zzz", "20260101-099999-d-tac-zzz", false, []string{"20260101-099999-d-tac-zzz"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := g.ResolveRef(tt.id)
			if rr.Head() != tt.wantHead {
				t.Errorf("Head() = %q, want %q", rr.Head(), tt.wantHead)
			}
			if rr.Origin() != tt.id {
				t.Errorf("Origin() = %q, want %q", rr.Origin(), tt.id)
			}
			if rr.IsStale() != tt.wantStale {
				t.Errorf("IsStale() = %v, want %v", rr.IsStale(), tt.wantStale)
			}
			if got := rr.Path(); !slices.Equal(got, tt.wantPath) {
				t.Errorf("Path() = %v, want %v", got, tt.wantPath)
			}
		})
	}
}

// TestResolveRef_PathIsCopy guards the unexported-slice contract: mutating the
// returned Path must not corrupt the resolution for a later caller.
func TestResolveRef_PathIsCopy(t *testing.T) {
	a := mkDec("20260101-100000-d-tac-aaa")
	b := mkDec("20260101-100100-d-tac-bbb", a.ID)
	g := model.NewGraph([]*model.Entry{a, b})

	p := g.ResolveRef(a.ID).Path()
	p[0] = "mutated"
	if got := g.ResolveRef(a.ID); got.Origin() != a.ID || got.Head() != b.ID {
		t.Errorf("Path() mutation leaked into resolution: origin=%q head=%q", got.Origin(), got.Head())
	}
}

// TestDerivedStatus_ResolvesToLiveHeadOnMultiHopSupersede covers s-tac-5p5 at
// the status layer: a multiply-superseded entry reports the live head, not the
// immediate (itself-superseded) successor.
func TestDerivedStatus_ResolvesToLiveHeadOnMultiHopSupersede(t *testing.T) {
	a := mkDec("20260101-100000-d-tac-aaa")
	b := mkDec("20260101-100100-d-tac-bbb", a.ID)
	c := mkDec("20260101-100200-d-tac-ccc", b.ID)
	g := model.NewGraph([]*model.Entry{a, b, c})

	got := g.DerivedStatus(a)
	want := model.Status{Kind: model.StatusSupersededBy, By: c.ID}
	if got != want {
		t.Errorf("DerivedStatus(a) = %+v, want %+v (live head, not immediate %s)", got, want, b.ID)
	}
}

// TestDerivedStatus_SkipsSupersededCloser covers s-tac-ohl: an entry closed by
// two dones where the first-inserted closer is later superseded must report the
// active closer, not the stale one. Resolving the chosen closer to its live
// head yields the active replacement (which also re-closes the target).
func TestDerivedStatus_SkipsSupersededCloser(t *testing.T) {
	target := mkDec("20260101-100000-d-tac-tgt")
	done1 := &model.Entry{ID: "20260101-110000-s-tac-do1", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindDone, Closes: []string{target.ID}}
	done2 := &model.Entry{ID: "20260101-120000-s-tac-do2", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindDone, Closes: []string{target.ID}, Supersedes: []string{done1.ID}}
	g := model.NewGraph([]*model.Entry{target, done1, done2})

	got := g.DerivedStatus(target)
	want := model.Status{Kind: model.StatusClosedBy, By: done2.ID}
	if got != want {
		t.Errorf("DerivedStatus(target) = %+v, want %+v (active closer, not superseded %s)", got, want, done1.ID)
	}
}

// TestValidateSupersedeForks covers s-tac-5p5's lint half (AC5): an entry
// superseded by more than one entry is a fork anomaly, flagged with both
// superseders named.
func TestValidateSupersedeForks(t *testing.T) {
	x := mkDec("20260101-100000-d-tac-xxx")
	y := mkDec("20260101-110000-d-tac-yyy", x.ID)
	z := mkDec("20260101-120000-d-tac-zzz", x.ID) // also supersedes x — fork
	g := model.NewGraph([]*model.Entry{x, y, z})
	_ = g

	if !entryHasWarning(x, "superseded by 2 entries") {
		t.Fatalf("expected fork warning on %s, got %+v", x.ID, x.Warnings)
	}
	if !entryHasWarning(x, y.ID) || !entryHasWarning(x, z.ID) {
		t.Errorf("fork warning should name both superseders %s and %s, got %+v", y.ID, z.ID, x.Warnings)
	}
}

func TestValidateSupersedeForks_LinearChainNoWarning(t *testing.T) {
	a := mkDec("20260101-100000-d-tac-aaa")
	b := mkDec("20260101-110000-d-tac-bbb", a.ID)
	g := model.NewGraph([]*model.Entry{a, b})
	_ = g

	if entryHasWarning(a, "superseded by") {
		t.Errorf("linear chain should not raise a fork warning: %+v", a.Warnings)
	}
}
