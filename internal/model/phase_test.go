package model_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestPhaseCounted(t *testing.T) {
	for phase, want := range map[model.Phase]bool{
		model.PhaseIndexing: true, model.PhaseSummarizing: true,
		model.PhaseSyncing: false, model.PhaseConnecting: false, model.PhaseCloning: false,
	} {
		if got := phase.Counted(); got != want {
			t.Errorf("%s: only phases working through a known number of units are counted, got %v", phase, got)
		}
	}
}
