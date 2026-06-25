package command_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
)

func TestBuildEntry_DefaultsKindForDecisions(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Description: "some decision",
	}

	entry, err := cmd.BuildEntry("20260414-120000-d-tac-abc")
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if entry.Kind != model.KindDirective {
		t.Errorf("Kind = %q, want %q", entry.Kind, model.KindDirective)
	}
}

func TestBuildEntry_PreservesExplicitKind(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Kind:        model.KindContract,
		Description: "some contract",
	}

	entry, err := cmd.BuildEntry("20260414-120000-d-tac-abc")
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if entry.Kind != model.KindContract {
		t.Errorf("Kind = %q, want %q", entry.Kind, model.KindContract)
	}
}

func TestBuildEntry_DefaultsKindForSignals(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "some signal",
	}

	entry, err := cmd.BuildEntry("20260414-120000-s-tac-abc")
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if entry.Kind != model.KindGap {
		t.Errorf("Kind = %q, want %q", entry.Kind, model.KindGap)
	}
}

func TestValidate_RejectsInvalidKindForType(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:  model.TypeSignal,
		Layer: model.LayerTactical,
		Kind:  model.KindContract, // decision kind on a signal
	}
	if err := cmd.Validate(); err == nil {
		t.Error("Validate() = nil, want error for decision kind on signal")
	}
}

func TestValidate_AcceptsSignalKind(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:  model.TypeSignal,
		Layer: model.LayerTactical,
		Kind:  model.KindInsight,
	}
	if err := cmd.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_RequiresIntentForDirective(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
	}
	if err := cmd.Validate(); err == nil {
		t.Error("Validate() = nil, want error for directive without intent")
	}
}

func TestValidate_RequiresIntentForDefaultedDirective(t *testing.T) {
	// A decision with no explicit kind defaults to directive, so it must still
	// supply an intent — the requirement keys off the effective kind.
	cmd := &command.NewEntryCmd{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
	}
	if err := cmd.Validate(); err == nil {
		t.Error("Validate() = nil, want error for defaulted directive without intent")
	}
}

func TestValidate_AcceptsValidIntentOnDirective(t *testing.T) {
	for _, in := range []string{"pending", "guiding", "settled"} {
		cmd := &command.NewEntryCmd{
			Type:   model.TypeDecision,
			Layer:  model.LayerTactical,
			Kind:   model.KindDirective,
			Intent: in,
		}
		if err := cmd.Validate(); err != nil {
			t.Errorf("Validate() with intent %q = %v, want nil", in, err)
		}
	}
}

func TestValidate_RejectsInvalidIntent(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:   model.TypeDecision,
		Layer:  model.LayerTactical,
		Kind:   model.KindDirective,
		Intent: "tentative",
	}
	if err := cmd.Validate(); err == nil {
		t.Error("Validate() = nil, want error for invalid intent value")
	}
}

func TestValidate_RejectsIntentOnNonDirective(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:   model.TypeDecision,
		Layer:  model.LayerTactical,
		Kind:   model.KindPlan,
		Intent: "guiding",
	}
	if err := cmd.Validate(); err == nil {
		t.Error("Validate() = nil, want error for intent on a non-directive decision")
	}
}

func TestValidate_AllowsNoIntentOnNonDirective(t *testing.T) {
	// Only directives require intent; other decision kinds carry none.
	cmd := &command.NewEntryCmd{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindContract,
	}
	if err := cmd.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for a contract without intent", err)
	}
}

func TestBuildEntry_MapsIntent(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Kind:        model.KindDirective,
		Intent:      "settled",
		Description: "a born-terminal directive",
	}
	entry, err := cmd.BuildEntry("20260414-120000-d-tac-abc")
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if entry.Intent != model.IntentSettled {
		t.Errorf("Intent = %q, want %q", entry.Intent, model.IntentSettled)
	}
}

func TestBuildEntry_ResolvesAttachmentPaths(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Description: "see [plan]({{attachments}}/plan.md)",
		Attachments: []command.Attachment{
			{Source: "/tmp/plan.md", Target: "plan.md"},
		},
	}

	entry, err := cmd.BuildEntry("20260414-120000-d-tac-abc")
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if len(entry.Attachments) != 1 {
		t.Fatalf("Attachments = %d, want 1", len(entry.Attachments))
	}
	// Content should have {{attachments}} resolved
	if entry.Content == cmd.Description {
		t.Error("Content should have attachment links resolved")
	}
}
