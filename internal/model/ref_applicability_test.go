package model

import (
	"testing"
	"time"
)

func Test_ClassifyRefTarget(t *testing.T) {
	tests := []struct {
		name   string
		target *Entry
		status Status
		want   RefTargetClass
	}{
		{
			name:   "active decision",
			target: &Entry{Type: TypeDecision, Kind: KindDirective},
			status: Status{Kind: StatusActive},
			want:   TargetLiveDecision,
		},
		{
			name:   "open signal",
			target: &Entry{Type: TypeSignal, Kind: KindGap},
			status: Status{Kind: StatusOpen},
			want:   TargetLiveSignal,
		},
		{
			name:   "terminal done",
			target: &Entry{Type: TypeSignal, Kind: KindDone},
			status: Status{Kind: StatusNone},
			want:   TargetTerminalDone,
		},
		{
			name:   "closed signal",
			target: &Entry{Type: TypeSignal, Kind: KindGap},
			status: Status{Kind: StatusClosedBy, By: "x"},
			want:   TargetRetired,
		},
		{
			name:   "superseded decision",
			target: &Entry{Type: TypeDecision, Kind: KindPlan},
			status: Status{Kind: StatusSupersededBy, By: "x"},
			want:   TargetRetired,
		},
		{
			// Status outranks kind: a done closed by a corrective done is
			// retired, not terminal.
			name:   "corrective-done-closed done",
			target: &Entry{Type: TypeSignal, Kind: KindDone},
			status: Status{Kind: StatusClosedBy, By: "x"},
			want:   TargetRetired,
		},
		{
			name:   "cascade-closed role",
			target: &Entry{Type: TypeDecision, Kind: KindRole},
			status: Status{Kind: StatusCascadeClosedBy, By: "x"},
			want:   TargetRetired,
		},
		{
			name:   "orphan role",
			target: &Entry{Type: TypeDecision, Kind: KindRole},
			status: Status{Kind: StatusCascadeOrphan},
			want:   TargetRetired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyRefTarget(tt.target, tt.status); got != tt.want {
				t.Errorf("ClassifyRefTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_RefKindMatrix_Complete(t *testing.T) {
	// Every capturable kind must declare every target class, and every cell
	// must carry a note — the prompt fragment and the mechanical finding both
	// render it.
	classes := []RefTargetClass{TargetLiveDecision, TargetLiveSignal, TargetTerminalDone, TargetRetired}
	for _, k := range RefKindValues() {
		for _, c := range classes {
			cell, ok := RefKindApplicability(k, c)
			if !ok {
				t.Errorf("matrix missing cell (%s, %s)", k, c)
				continue
			}
			if cell.Note == "" {
				t.Errorf("matrix cell (%s, %s) has empty note", k, c)
			}
		}
	}
}

func Test_RefKindMatrix_InapplicableCells(t *testing.T) {
	// The inapplicable set is exactly the vocabulary's documented-impossible
	// combinations — everything else is applicable (permissive at the edges).
	// Changing this set is a calibration decision, not a refactor: it widens
	// or narrows the deterministic blocking surface.
	wantInapplicable := map[RefKind][]RefTargetClass{
		RefKindRefines:   {TargetTerminalDone, TargetRetired},
		RefKindAddresses: {TargetTerminalDone},
	}
	classes := []RefTargetClass{TargetLiveDecision, TargetLiveSignal, TargetTerminalDone, TargetRetired}
	for _, k := range RefKindValues() {
		inapplicable := map[RefTargetClass]bool{}
		for _, c := range wantInapplicable[k] {
			inapplicable[c] = true
		}
		for _, c := range classes {
			cell, _ := RefKindApplicability(k, c)
			if cell.Applicable == inapplicable[c] {
				t.Errorf("cell (%s, %s): Applicable = %v, want %v", k, c, cell.Applicable, !inapplicable[c])
			}
		}
	}
}

func Test_RefKindApplicability_NonCapturable(t *testing.T) {
	for _, k := range []RefKind{RefKindUnknown, RefKindGrounds, RefKindEvidence, RefKind("bogus")} {
		if _, ok := RefKindApplicability(k, TargetLiveDecision); ok {
			t.Errorf("RefKindApplicability(%q) should not classify non-capturable kinds", k)
		}
	}
}

func Test_AdmissibleRefKinds(t *testing.T) {
	// Terminal done excludes refines and addresses; retired excludes refines.
	got := AdmissibleRefKinds(TargetTerminalDone)
	for _, k := range got {
		if k == RefKindRefines || k == RefKindAddresses {
			t.Errorf("AdmissibleRefKinds(terminal-done) must not contain %s", k)
		}
	}
	if len(got) != len(RefKindValues())-2 {
		t.Errorf("AdmissibleRefKinds(terminal-done) len = %d, want %d", len(got), len(RefKindValues())-2)
	}

	if len(AdmissibleRefKinds(TargetLiveDecision)) != len(RefKindValues()) {
		t.Error("AdmissibleRefKinds(live-decision) should be the full capturable vocabulary")
	}
}

func Test_AdmissibleRefKinds_AgreesWithGraphStatus(t *testing.T) {
	// End to end through a real graph: classification keyed off DerivedStatus
	// must agree with the matrix on the canonical leak shapes.
	plan := &Entry{ID: "p1", Type: TypeDecision, Kind: KindPlan, Content: "plan", Time: time.Now()}
	done := &Entry{ID: "d1", Type: TypeSignal, Kind: KindDone, Closes: []string{"p1"}, Content: "done", Time: time.Now()}
	g := NewGraph([]*Entry{plan, done})

	// Closed plan: refines inapplicable, builds-on applicable.
	planClass := ClassifyRefTarget(plan, g.DerivedStatus(plan))
	if planClass != TargetRetired {
		t.Fatalf("closed plan class = %q, want retired", planClass)
	}
	if cell, _ := RefKindApplicability(RefKindRefines, planClass); cell.Applicable {
		t.Error("refines on a closed plan must be inapplicable")
	}

	// Terminal done: addresses inapplicable, builds-on and grounded-in applicable.
	doneClass := ClassifyRefTarget(done, g.DerivedStatus(done))
	if doneClass != TargetTerminalDone {
		t.Fatalf("done class = %q, want terminal-done", doneClass)
	}
	if cell, _ := RefKindApplicability(RefKindAddresses, doneClass); cell.Applicable {
		t.Error("addresses on a terminal done must be inapplicable")
	}
	for _, k := range []RefKind{RefKindBuildsOn, RefKindGroundedIn} {
		if cell, _ := RefKindApplicability(k, doneClass); !cell.Applicable {
			t.Errorf("%s on a terminal done must be applicable (the documented tie-break)", k)
		}
	}
}
