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

// writeFindings runs a built entry through the construction boundary the way
// the handler does — the per-kind rules moved there, so command tests assert
// against the delegated path rather than a command-local copy.
func writeFindings(t *testing.T, cmd *command.NewEntryCmd, id string) []model.Finding {
	t.Helper()
	entry, err := cmd.BuildEntry(id)
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	construction, findings := model.ConstructFromEntry(entry)
	_, writeFindings := construction.ValidateForWrite(model.NewGraph(nil))
	return append(findings, writeFindings...)
}

func hasFindingOn(findings []model.Finding, field string) bool {
	for _, f := range findings {
		if f.Field == field {
			return true
		}
	}
	return false
}

func TestWritePath_RejectsInvalidKindForType(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:  model.TypeSignal,
		Layer: model.LayerTactical,
		Kind:  model.KindContract, // decision kind on a signal
	}
	if !hasFindingOn(writeFindings(t, cmd, "20260414-120000-s-tac-abc"), "kind") {
		t.Error("want a kind finding for decision kind on signal")
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

func TestWritePath_RequiresIntentForDirective(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Kind:        model.KindDirective,
		Description: "a directive without intent",
	}
	if !hasFindingOn(writeFindings(t, cmd, "20260414-120000-d-tac-abc"), "intent") {
		t.Error("want an intent finding for directive without intent")
	}
}

func TestWritePath_RequiresIntentForDefaultedDirective(t *testing.T) {
	// A decision with no explicit kind defaults to directive, so it must still
	// supply an intent — the requirement keys off the effective kind.
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Description: "a defaulted directive without intent",
	}
	if !hasFindingOn(writeFindings(t, cmd, "20260414-120000-d-tac-abc"), "intent") {
		t.Error("want an intent finding for defaulted directive without intent")
	}
}

func TestWritePath_AcceptsValidIntentOnDirective(t *testing.T) {
	for _, in := range []string{"pending", "guiding", "settled"} {
		cmd := &command.NewEntryCmd{
			Type:        model.TypeDecision,
			Layer:       model.LayerTactical,
			Kind:        model.KindDirective,
			Intent:      in,
			Description: "a directive with intent",
		}
		if findings := writeFindings(t, cmd, "20260414-120000-d-tac-abc"); len(findings) > 0 {
			t.Errorf("intent %q: findings = %+v, want none", in, findings)
		}
	}
}

func TestWritePath_RejectsInvalidIntent(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Kind:        model.KindDirective,
		Intent:      "tentative",
		Description: "a directive with a made-up intent",
	}
	if !hasFindingOn(writeFindings(t, cmd, "20260414-120000-d-tac-abc"), "intent") {
		t.Error("want an intent finding for invalid intent value")
	}
}

func TestWritePath_RejectsIntentOnNonDirective(t *testing.T) {
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Kind:        model.KindPlan,
		Intent:      "guiding",
		Description: "a plan\n\n## Acceptance criteria\n\n- [ ] one",
	}
	if !hasFindingOn(writeFindings(t, cmd, "20260414-120000-d-tac-abc"), "intent") {
		t.Error("want an intent finding for intent on a non-directive decision")
	}
}

func TestWritePath_AllowsNoIntentOnNonDirective(t *testing.T) {
	// Only directives require intent; other decision kinds carry none.
	cmd := &command.NewEntryCmd{
		Type:        model.TypeDecision,
		Layer:       model.LayerTactical,
		Kind:        model.KindContract,
		Description: "a contract without intent",
	}
	if findings := writeFindings(t, cmd, "20260414-120000-d-tac-abc"); len(findings) > 0 {
		t.Errorf("findings = %+v, want none for a contract without intent", findings)
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

func TestNewEntryCmdFactIndex(t *testing.T) {
	index, err := model.NewFactIndex("How to compose graph views", "cli/view")
	if err != nil {
		t.Fatal(err)
	}
	cmd := &command.NewEntryCmd{
		Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindFact,
		TopicLabels: []string{"cli/view"}, Index: index,
		Description: "a fact with an index",
	}
	if findings := writeFindings(t, cmd, "20260719-120000-s-tac-idx"); len(findings) > 0 {
		t.Fatalf("findings = %+v, want none for a valid indexed fact", findings)
	}
	entry, err := cmd.BuildEntry("20260719-120000-s-tac-idx")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Index != index || entry.Index.Title != "How to compose graph views" {
		t.Fatalf("entry.Index = %+v", entry.Index)
	}

	cmd.Kind = model.KindInsight
	if !hasFindingOn(writeFindings(t, cmd, "20260719-120000-s-tac-idx"), "index") {
		t.Fatal("want an index finding for index on non-fact")
	}
	cmd.Kind = model.KindFact
	cmd.TopicLabels = []string{"agent/ux"}
	if !hasFindingOn(writeFindings(t, cmd, "20260719-120000-s-tac-idx"), "index") {
		t.Fatal("want an index finding for index topic absent from topics")
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
