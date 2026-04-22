package model_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestDerivedStatus(t *testing.T) {
	signal := &model.Entry{ID: "20260410-100000-s-tac-sig", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap}
	decision := &model.Entry{ID: "20260410-100100-d-tac-dec", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective}
	plan := &model.Entry{ID: "20260410-100200-d-tac-plan", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindPlan}
	contract := &model.Entry{ID: "20260410-100300-d-tac-con", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindContract}

	closer := &model.Entry{
		ID:     "20260410-110000-s-tac-cls",
		Type:   model.TypeSignal,
		Layer:  model.LayerTactical,
		Kind:   model.KindDone,
		Closes: []string{signal.ID},
	}
	superseder := &model.Entry{
		ID:         "20260410-110100-d-tac-sup",
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Kind:       model.KindDirective,
		Supersedes: []string{decision.ID},
	}

	g := model.NewGraph([]*model.Entry{signal, decision, plan, contract, closer, superseder})

	tests := []struct {
		name  string
		entry *model.Entry
		want  model.Status
	}{
		{"open_signal", signal, model.Status{Kind: model.StatusClosedBy, By: closer.ID}},
		{"active_decision", decision, model.Status{Kind: model.StatusSupersededBy, By: superseder.ID}},
		{"active_plan", plan, model.Status{Kind: model.StatusActive}},
		{"active_contract", contract, model.Status{Kind: model.StatusActive}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.DerivedStatus(tt.entry)
			if got != tt.want {
				t.Errorf("DerivedStatus(%s) = %+v, want %+v", tt.entry.ID, got, tt.want)
			}
		})
	}
}

func TestDerivedStatus_UnaffectedSignalAndDecision(t *testing.T) {
	signal := &model.Entry{ID: "20260410-100000-s-tac-sig", Type: model.TypeSignal, Layer: model.LayerTactical}
	decision := &model.Entry{ID: "20260410-100100-d-tac-dec", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective}

	g := model.NewGraph([]*model.Entry{signal, decision})

	if got := g.DerivedStatus(signal); got.Kind != model.StatusOpen {
		t.Errorf("signal status = %v, want %v", got, model.StatusOpen)
	}
	if got := g.DerivedStatus(decision); got.Kind != model.StatusActive {
		t.Errorf("decision status = %v, want %v", got, model.StatusActive)
	}
}
