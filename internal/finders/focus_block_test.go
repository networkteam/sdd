package finders

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestExpandInvolvement_DerivesState(t *testing.T) {
	// Three targets with distinct expected states:
	//   driving       — actors set, score above threshold
	//   stalled       — actors set, score below threshold
	//   pull-available — explicit empty actors
	driving := entry("20260101-100000-d-tac-drv", withKind(model.KindDirective))
	stalled := entry("20260101-110000-d-tac-stl", withKind(model.KindDirective))
	pull := entry("20260101-120000-s-tac-pul", withKind(model.KindGap))

	focus := entry("20260101-130000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher"),
		withInvolvement(driving.ID, nil, false),    // inherits focus default → ["Christopher"]
		withInvolvement(stalled.ID, nil, false),    // inherits → ["Christopher"]
		withInvolvement(pull.ID, []string{}, true), // explicit empty → pull-available
	)
	g := model.NewGraph([]*model.Entry{driving, stalled, pull, focus})

	// Inject deterministic scores: driving above threshold, stalled below.
	scores := map[string]float64{
		driving.ID: 5.0,
		stalled.ID: 0.25, // below DefaultStalledThreshold (1.0)
		pull.ID:    0.0,  // not consulted (pull-available short-circuits)
	}
	scoreFn := func(e *model.Entry) float64 { return scores[e.ID] }

	block := expandInvolvement(g, []*model.Entry{focus}, scoreFn, DefaultStalledThreshold)

	if len(block.Focuses) != 1 {
		t.Fatalf("focuses: got %d, want 1", len(block.Focuses))
	}
	group := block.Focuses[0]
	if len(group.Targets) != 3 {
		t.Fatalf("targets: got %d, want 3", len(group.Targets))
	}

	checkTarget := func(t *testing.T, ft model.FocusTarget, wantState model.FocusState, wantScore float64) {
		t.Helper()
		if ft.State != wantState {
			t.Errorf("state for %s: got %q, want %q", ft.Target.ID, ft.State, wantState)
		}
		if ft.Score != wantScore {
			t.Errorf("score for %s: got %v, want %v", ft.Target.ID, ft.Score, wantScore)
		}
	}
	checkTarget(t, group.Targets[0], model.FocusStateDriving, 5.0)
	checkTarget(t, group.Targets[1], model.FocusStateStalled, 0.25)
	checkTarget(t, group.Targets[2], model.FocusStatePullAvailable, 0.0)
}

func TestExpandInvolvement_OmitsClosedAndSupersededTargets(t *testing.T) {
	open := entry("20260101-100000-d-tac-opn", withKind(model.KindDirective))
	closedDecision := entry("20260101-110000-d-tac-cld", withKind(model.KindDirective))
	closer := entry("20260101-120000-s-tac-clo", withKind(model.KindDone), withCloses(closedDecision.ID))
	supersededDecision := entry("20260101-130000-d-tac-sup", withKind(model.KindDirective))
	superseder := entry("20260101-140000-d-tac-spr", withKind(model.KindDirective), withSupersedes(supersededDecision.ID))

	focus := entry("20260101-150000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher"),
		withInvolvement(open.ID, nil, false),
		withInvolvement(closedDecision.ID, nil, false),
		withInvolvement(supersededDecision.ID, nil, false),
	)
	g := model.NewGraph([]*model.Entry{open, closedDecision, closer, supersededDecision, superseder, focus})

	scoreFn := func(*model.Entry) float64 { return 5.0 }
	block := expandInvolvement(g, []*model.Entry{focus}, scoreFn, DefaultStalledThreshold)

	group := block.Focuses[0]
	if len(group.Targets) != 1 {
		t.Fatalf("targets: got %d, want 1 (closed/superseded omitted)", len(group.Targets))
	}
	if group.Targets[0].Target.ID != open.ID {
		t.Errorf("only target should be the open one, got %s", group.Targets[0].Target.ID)
	}
}

func TestExpandInvolvement_ResolvesActors_PerInvolvementOverridesDefault(t *testing.T) {
	target := entry("20260101-100000-d-tac-tgt", withKind(model.KindDirective))
	focus := entry("20260101-110000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher", "Claude"),
		withInvolvement(target.ID, []string{"Alice"}, true),
	)
	g := model.NewGraph([]*model.Entry{target, focus})

	scoreFn := func(*model.Entry) float64 { return 5.0 }
	block := expandInvolvement(g, []*model.Entry{focus}, scoreFn, DefaultStalledThreshold)

	got := block.Focuses[0].Targets[0].ResolvedActors
	want := []string{"Alice"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("resolved actors: got %v, want %v", got, want)
	}
	if !block.Focuses[0].Targets[0].ActorsExplicit {
		t.Errorf("ActorsExplicit must be true when involvement set actors")
	}
}

func TestExpandInvolvement_ResolvesActors_InheritsFocusDefault(t *testing.T) {
	target := entry("20260101-100000-d-tac-tgt", withKind(model.KindDirective))
	focus := entry("20260101-110000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher", "Claude"),
		withInvolvement(target.ID, nil, false), // ActorsSet = false → inherit
	)
	g := model.NewGraph([]*model.Entry{target, focus})

	scoreFn := func(*model.Entry) float64 { return 5.0 }
	block := expandInvolvement(g, []*model.Entry{focus}, scoreFn, DefaultStalledThreshold)

	got := block.Focuses[0].Targets[0].ResolvedActors
	want := []string{"Christopher", "Claude"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("resolved actors: got %v, want %v", got, want)
	}
}

func TestExpandInvolvement_StalledThresholdConfigurable(t *testing.T) {
	// Same score, different threshold → different state. Verifies
	// stalled(value) actually flows through to classification.
	target := entry("20260101-100000-d-tac-tgt", withKind(model.KindDirective))
	focus := entry("20260101-110000-d-prc-foc",
		withKind(model.KindFocus),
		withFocusActors("Christopher"),
		withInvolvement(target.ID, nil, false),
	)
	g := model.NewGraph([]*model.Entry{target, focus})

	scoreFn := func(*model.Entry) float64 { return 0.5 }

	// Threshold 0.1 → 0.5 is above, driving.
	low := expandInvolvement(g, []*model.Entry{focus}, scoreFn, 0.1)
	if got := low.Focuses[0].Targets[0].State; got != model.FocusStateDriving {
		t.Errorf("threshold 0.1: got state %q, want driving", got)
	}

	// Threshold 1.0 → 0.5 is below, stalled.
	high := expandInvolvement(g, []*model.Entry{focus}, scoreFn, 1.0)
	if got := high.Focuses[0].Targets[0].State; got != model.FocusStateStalled {
		t.Errorf("threshold 1.0: got state %q, want stalled", got)
	}
}
