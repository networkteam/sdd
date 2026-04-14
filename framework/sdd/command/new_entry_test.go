package command_test

import (
	"testing"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/model"
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

func TestBuildEntry_NoKindForSignals(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "some signal",
	}

	entry, err := cmd.BuildEntry("20260414-120000-s-tac-abc")
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if entry.Kind != "" {
		t.Errorf("Kind = %q, want empty for signals", entry.Kind)
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
