package main

import (
	"slices"
	"testing"

	sdd "github.com/networkteam/sdd/application"
)

func TestRecoveryInteractiveSelectionReconcilesUnknownBeforeOfferingVerbs(t *testing.T) {
	unknown := sdd.RecoveryItem{State: sdd.RecoveryUnknown}
	if !recoveryNeedsReconciliation(unknown, "") {
		t.Fatal("interactive unknown recovery must reconcile before verb selection")
	}
	if recoveryNeedsReconciliation(unknown, "apply") {
		t.Fatal("an explicit non-interactive verb reconciles inside RecoverMutation")
	}
	if recoveryNeedsReconciliation(sdd.RecoveryItem{State: sdd.RecoveryUnknown, LegacyUnroutable: true}, "") {
		t.Fatal("legacy recovery must bind its target before reconciliation")
	}
}

func TestRecoveryVerbMenusCoverEveryActionableProjection(t *testing.T) {
	tests := []struct {
		name string
		item sdd.RecoveryItem
		want []sdd.RecoveryVerb
	}{
		{name: "not applied", item: sdd.RecoveryItem{State: sdd.RecoveryNotAppliedAwaitingDecision}, want: []sdd.RecoveryVerb{sdd.RecoveryApply, sdd.RecoveryDiscard}},
		{name: "applied", item: sdd.RecoveryItem{State: sdd.RecoveryAppliedFinalizationPending}, want: []sdd.RecoveryVerb{sdd.RecoveryFinalizeRetry}},
		{name: "unknown evidence", item: sdd.RecoveryItem{State: sdd.RecoveryUnknown}, want: []sdd.RecoveryVerb{sdd.RecoveryAbandonUnknown}},
		{name: "legacy", item: sdd.RecoveryItem{State: sdd.RecoveryUnknown, LegacyUnroutable: true}, want: []sdd.RecoveryVerb{sdd.RecoveryBindTarget}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := recoveryVerbs(test.item); !slices.Equal(got, test.want) {
				t.Fatalf("verbs = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseRecoveryVerbCoversEveryPublicVerb(t *testing.T) {
	for _, verb := range []sdd.RecoveryVerb{
		sdd.RecoveryApply, sdd.RecoveryDiscard, sdd.RecoveryFinalizeRetry, sdd.RecoveryAbandonUnknown, sdd.RecoveryBindTarget,
	} {
		if got, err := parseRecoveryVerb(string(verb)); err != nil || got != verb {
			t.Fatalf("parse %q = %q, %v", verb, got, err)
		}
	}
	if _, err := parseRecoveryVerb(string(sdd.RecoveryReconcile)); err == nil {
		t.Fatal("reconcile-only refresh must not be exposed as a user recovery verb")
	}
}
