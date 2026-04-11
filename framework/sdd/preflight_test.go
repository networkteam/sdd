package sdd

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockRunner implements PreflightRunner for testing.
type mockRunner struct {
	response string
	err      error
	// captured prompt for inspection
	lastPrompt string
}

func (m *mockRunner) Run(_ context.Context, prompt string) (string, error) {
	m.lastPrompt = prompt
	return m.response, m.err
}

func TestSelectCheckType(t *testing.T) {
	// Base entries in the graph for reference
	signal := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	decision := entry("20260410-120000-d-tac-bbb", withContent("some decision"))
	plan := entry("20260410-120000-d-tac-ccc", withKind(KindPlan), withContent("some plan"))

	graph := NewGraph([]*Entry{signal, decision, plan})

	tests := []struct {
		name     string
		entry    *Entry
		expected CheckType
	}{
		{
			name:     "action closing decision",
			entry:    &Entry{Type: TypeAction, Closes: []string{decision.ID}},
			expected: CheckClosingAction,
		},
		{
			name:     "action closing plan",
			entry:    &Entry{Type: TypeAction, Closes: []string{plan.ID}},
			expected: CheckClosingAction,
		},
		{
			name:     "action closing signal",
			entry:    &Entry{Type: TypeAction, Closes: []string{signal.ID}},
			expected: CheckActionClosesSignals,
		},
		{
			name:     "action closing both decision and signal picks closing-action",
			entry:    &Entry{Type: TypeAction, Closes: []string{decision.ID, signal.ID}},
			expected: CheckClosingAction,
		},
		{
			name:     "decision closing signal",
			entry:    &Entry{Type: TypeDecision, Closes: []string{signal.ID}},
			expected: CheckClosingDecision,
		},
		{
			name:     "decision with refs only",
			entry:    &Entry{Type: TypeDecision, Refs: []string{signal.ID}},
			expected: CheckDecisionRefs,
		},
		{
			name:     "decision with no refs or closes",
			entry:    &Entry{Type: TypeDecision},
			expected: CheckDecisionRefs,
		},
		{
			name:     "signal",
			entry:    &Entry{Type: TypeSignal},
			expected: CheckSignalCapture,
		},
		{
			name:     "signal with refs",
			entry:    &Entry{Type: TypeSignal, Refs: []string{decision.ID}},
			expected: CheckSignalCapture,
		},
		{
			name:     "supersedes takes priority over closes",
			entry:    &Entry{Type: TypeDecision, Supersedes: []string{decision.ID}, Closes: []string{signal.ID}},
			expected: CheckSupersedes,
		},
		{
			name:     "supersedes on action",
			entry:    &Entry{Type: TypeAction, Supersedes: []string{decision.ID}},
			expected: CheckSupersedes,
		},
		{
			name:     "supersedes on signal",
			entry:    &Entry{Type: TypeSignal, Supersedes: []string{signal.ID}},
			expected: CheckSupersedes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectCheckType(tt.entry, graph)
			if got != tt.expected {
				t.Errorf("SelectCheckType() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestCheckTypeString(t *testing.T) {
	tests := []struct {
		ct   CheckType
		want string
	}{
		{CheckClosingAction, "closing-action"},
		{CheckClosingDecision, "closing-decision"},
		{CheckDecisionRefs, "decision-refs"},
		{CheckActionClosesSignals, "action-closes-signals"},
		{CheckSignalCapture, "signal-capture"},
		{CheckSupersedes, "supersedes"},
		{CheckType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.want {
			t.Errorf("CheckType(%d).String() = %q, want %q", int(tt.ct), got, tt.want)
		}
	}
}

func TestFormatEntryForPrompt(t *testing.T) {
	e := &Entry{
		ID:         "20260410-120000-d-tac-xyz",
		Type:       TypeDecision,
		Layer:      LayerTactical,
		Kind:       KindPlan,
		Refs:       []string{"20260410-110000-s-cpt-aaa"},
		Closes:     []string{"20260410-100000-s-stg-bbb"},
		Confidence: "high",
		Content:    "Build the validator.",
	}

	result := FormatEntryForPrompt(e)

	// Check all expected fields are present
	checks := []string{
		"ID: 20260410-120000-d-tac-xyz",
		"Type: decision",
		"Layer: tactical",
		"Kind: plan",
		"Refs: 20260410-110000-s-cpt-aaa",
		"Closes: 20260410-100000-s-stg-bbb",
		"Confidence: high",
		"Build the validator.",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("FormatEntryForPrompt() missing %q\nGot:\n%s", check, result)
		}
	}
}

func TestFormatEntryForPrompt_OmitsDefaultKind(t *testing.T) {
	e := &Entry{
		ID:      "20260410-120000-d-tac-xyz",
		Type:    TypeDecision,
		Layer:   LayerTactical,
		Kind:    KindDirective,
		Content: "Some directive.",
	}

	result := FormatEntryForPrompt(e)
	if strings.Contains(result, "Kind:") {
		t.Errorf("FormatEntryForPrompt() should omit Kind for directive, got:\n%s", result)
	}
}

func TestFormatEntryForPrompt_OmitsEmptyFields(t *testing.T) {
	e := &Entry{
		ID:      "20260410-120000-s-cpt-xyz",
		Type:    TypeSignal,
		Layer:   LayerConceptual,
		Content: "Observed something.",
	}

	result := FormatEntryForPrompt(e)
	for _, field := range []string{"Kind:", "Refs:", "Closes:", "Supersedes:", "Confidence:"} {
		if strings.Contains(result, field) {
			t.Errorf("FormatEntryForPrompt() should omit empty %s, got:\n%s", field, result)
		}
	}
}

func TestAssembleContext_BasicSignal(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("observed something"))
	graph := NewGraph([]*Entry{sig})

	proposed := &Entry{
		Type:    TypeSignal,
		Layer:   LayerConceptual,
		Content: "new signal",
	}

	pctx, err := AssembleContext(proposed, graph, CheckSignalCapture)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.ProposedEntry, "new signal") {
		t.Error("ProposedEntry should contain the signal content")
	}
	if pctx.ClosedEntries != "" {
		t.Error("ClosedEntries should be empty for signal capture")
	}
	if pctx.SupersededEntries != "" {
		t.Error("SupersededEntries should be empty")
	}
}

func TestAssembleContext_WithRefs(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("first signal"))
	dec := entry("20260410-130000-d-tac-bbb", withContent("decision content"), withRefs(sig.ID))
	graph := NewGraph([]*Entry{sig, dec})

	proposed := &Entry{
		Type:    TypeDecision,
		Layer:   LayerTactical,
		Refs:    []string{sig.ID},
		Content: "new decision",
	}

	pctx, err := AssembleContext(proposed, graph, CheckDecisionRefs)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.ReferencedEntries, "first signal") {
		t.Error("ReferencedEntries should contain the referenced signal content")
	}
}

func TestAssembleContext_WithCloses(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("signal to close"))
	graph := NewGraph([]*Entry{sig})

	proposed := &Entry{
		Type:    TypeDecision,
		Layer:   LayerConceptual,
		Closes:  []string{sig.ID},
		Content: "decision closing signal",
	}

	pctx, err := AssembleContext(proposed, graph, CheckClosingDecision)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.ClosedEntries, "signal to close") {
		t.Error("ClosedEntries should contain the closed signal content")
	}
}

func TestAssembleContext_WithContracts(t *testing.T) {
	contract := entry("20260410-120000-d-prc-aaa",
		withKind(KindContract),
		withContent("all entries must have refs"))
	graph := NewGraph([]*Entry{contract})

	proposed := &Entry{
		Type:    TypeSignal,
		Layer:   LayerConceptual,
		Content: "some signal",
	}

	pctx, err := AssembleContext(proposed, graph, CheckSignalCapture)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.ActiveContracts, "all entries must have refs") {
		t.Error("ActiveContracts should contain contract content")
	}
}

func TestAssembleContext_WithSupersedes(t *testing.T) {
	old := entry("20260410-120000-d-tac-aaa", withContent("old decision"))
	graph := NewGraph([]*Entry{old})

	proposed := &Entry{
		Type:       TypeDecision,
		Layer:      LayerTactical,
		Supersedes: []string{old.ID},
		Content:    "replacement decision",
	}

	pctx, err := AssembleContext(proposed, graph, CheckSupersedes)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.SupersededEntries, "old decision") {
		t.Error("SupersededEntries should contain the superseded entry content")
	}
}

func TestAssembleContext_OpenSignalsForDecisionRefs(t *testing.T) {
	sig1 := entry("20260410-120000-s-cpt-aaa", withContent("open signal one"))
	sig2 := entry("20260410-120100-s-tac-bbb", withContent("open signal two"))
	graph := NewGraph([]*Entry{sig1, sig2})

	proposed := &Entry{
		Type:    TypeDecision,
		Layer:   LayerConceptual,
		Refs:    []string{sig1.ID},
		Content: "new decision",
	}

	pctx, err := AssembleContext(proposed, graph, CheckDecisionRefs)
	if err != nil {
		t.Fatal(err)
	}

	if pctx.OpenSignals == "" {
		t.Error("OpenSignals should be populated for decision-refs check")
	}
	if !strings.Contains(pctx.OpenSignals, "open signal one") {
		t.Error("OpenSignals should contain first signal")
	}
	if !strings.Contains(pctx.OpenSignals, "open signal two") {
		t.Error("OpenSignals should contain second signal")
	}
}

func TestAssembleContext_OpenSignalsNotIncludedForOtherChecks(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("open signal"))
	graph := NewGraph([]*Entry{sig})

	proposed := &Entry{
		Type:    TypeSignal,
		Layer:   LayerConceptual,
		Content: "new signal",
	}

	pctx, err := AssembleContext(proposed, graph, CheckSignalCapture)
	if err != nil {
		t.Fatal(err)
	}

	if pctx.OpenSignals != "" {
		t.Error("OpenSignals should be empty for non-decision-refs checks")
	}
}

func TestRenderPrompt_AllCheckTypes(t *testing.T) {
	pctx := &PreflightContext{
		ProposedEntry:     "ID: test\nType: signal\n\nTest content",
		ReferencedEntries: "ID: ref\nType: signal\n\nRef content",
		ClosedEntries:     "ID: closed\nType: signal\n\nClosed content",
		SupersededEntries: "ID: old\nType: decision\n\nOld content",
		ActiveContracts:   "ID: contract\nType: decision\n\nContract content",
		PlanItems:         "### Attachment: plan.md\n\n1. Do thing\n2. Do other thing",
		OpenSignals:       "ID: open\nType: signal\n\nOpen signal",
	}

	for ct, tmplName := range checkTypeTemplates {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := RenderPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("RenderPrompt(%s) error: %v", ct, err)
			}
			if result == "" {
				t.Errorf("RenderPrompt(%s) returned empty string", ct)
			}
			// Every template should contain the proposed entry
			if !strings.Contains(result, "Test content") {
				t.Errorf("RenderPrompt(%s) missing proposed entry content", ct)
			}
			// Every template should mention output format
			if !strings.Contains(result, "PASS") || !strings.Contains(result, "FAIL") {
				t.Errorf("RenderPrompt(%s) missing output format instructions", ct)
			}
			_ = tmplName
		})
	}
}

func TestRenderPrompt_InvalidCheckType(t *testing.T) {
	pctx := &PreflightContext{ProposedEntry: "test"}
	_, err := RenderPrompt(CheckType(99), pctx)
	if err == nil {
		t.Error("RenderPrompt with invalid check type should return error")
	}
}

func TestRenderPrompt_ConditionalSections(t *testing.T) {
	// Empty optional fields should not produce section headers
	pctx := &PreflightContext{
		ProposedEntry: "ID: test\nType: signal\n\nTest content",
	}

	result, err := RenderPrompt(CheckSignalCapture, pctx)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "## Referenced entries") {
		t.Error("Should not include Referenced entries section when empty")
	}
	if strings.Contains(result, "## Active contracts") {
		t.Error("Should not include Active contracts section when empty")
	}
}

func TestParseResult(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPass bool
		wantGaps []string
		wantErr  bool
	}{
		{
			name:     "PASS",
			input:    "PASS",
			wantPass: true,
		},
		{
			name:     "PASS with trailing whitespace",
			input:    "  PASS  \n",
			wantPass: true,
		},
		{
			name:     "PASS case insensitive",
			input:    "Pass",
			wantPass: true,
		},
		{
			name:     "FAIL with gaps",
			input:    "FAIL\n- Missing coverage for plan item 1\n- Overclaiming on plan item 3",
			wantPass: false,
			wantGaps: []string{"Missing coverage for plan item 1", "Overclaiming on plan item 3"},
		},
		{
			name:     "FAIL with indented gaps",
			input:    "FAIL\n  - Gap one\n  - Gap two\n",
			wantPass: false,
			wantGaps: []string{"Gap one", "Gap two"},
		},
		{
			name:     "FAIL case insensitive",
			input:    "Fail\n- Some gap",
			wantPass: false,
			wantGaps: []string{"Some gap"},
		},
		{
			name:     "FAIL with no gaps",
			input:    "FAIL",
			wantPass: false,
			wantGaps: nil,
		},
		{
			name:     "FAIL with non-bullet gaps",
			input:    "FAIL\nSome gap without bullet",
			wantPass: false,
			wantGaps: []string{"Some gap without bullet"},
		},
		{
			name:    "empty response",
			input:   "",
			wantErr: true,
		},
		{
			name:    "unexpected format",
			input:   "The entry looks good to me!",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   \n  \n  ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseResult(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseResult() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseResult() unexpected error: %v", err)
			}
			if result.Pass != tt.wantPass {
				t.Errorf("ParseResult().Pass = %v, want %v", result.Pass, tt.wantPass)
			}
			if len(result.Gaps) != len(tt.wantGaps) {
				t.Fatalf("ParseResult().Gaps = %v (len %d), want %v (len %d)",
					result.Gaps, len(result.Gaps), tt.wantGaps, len(tt.wantGaps))
			}
			for i, gap := range result.Gaps {
				if gap != tt.wantGaps[i] {
					t.Errorf("ParseResult().Gaps[%d] = %q, want %q", i, gap, tt.wantGaps[i])
				}
			}
		})
	}
}

func TestRunPreflight_Pass(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := NewGraph([]*Entry{sig})

	proposed := &Entry{
		Type:    TypeSignal,
		Layer:   LayerConceptual,
		Content: "new observation",
	}

	runner := &mockRunner{response: "PASS"}
	result, err := RunPreflight(context.Background(), runner, proposed, graph)
	if err != nil {
		t.Fatalf("RunPreflight() error: %v", err)
	}
	if !result.Pass {
		t.Error("RunPreflight() expected pass")
	}
	if runner.lastPrompt == "" {
		t.Error("Runner should have been called with a prompt")
	}
}

func TestRunPreflight_Fail(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := NewGraph([]*Entry{sig})

	proposed := &Entry{
		Type:    TypeDecision,
		Layer:   LayerConceptual,
		Closes:  []string{sig.ID},
		Content: "decision closing signal",
	}

	runner := &mockRunner{response: "FAIL\n- Signal not genuinely addressed"}
	result, err := RunPreflight(context.Background(), runner, proposed, graph)
	if err != nil {
		t.Fatalf("RunPreflight() error: %v", err)
	}
	if result.Pass {
		t.Error("RunPreflight() expected fail")
	}
	if len(result.Gaps) != 1 || result.Gaps[0] != "Signal not genuinely addressed" {
		t.Errorf("RunPreflight().Gaps = %v, want [Signal not genuinely addressed]", result.Gaps)
	}
}

func TestRunPreflight_RunnerError(t *testing.T) {
	graph := NewGraph(nil)

	proposed := &Entry{
		Type:    TypeSignal,
		Layer:   LayerConceptual,
		Content: "some signal",
	}

	runner := &mockRunner{err: fmt.Errorf("claude CLI not found")}
	_, err := RunPreflight(context.Background(), runner, proposed, graph)
	if err == nil {
		t.Fatal("RunPreflight() expected error when runner fails")
	}
	if !strings.Contains(err.Error(), "running pre-flight validator") {
		t.Errorf("error should wrap runner failure, got: %v", err)
	}
}

func TestRunPreflight_ParseError(t *testing.T) {
	graph := NewGraph(nil)

	proposed := &Entry{
		Type:    TypeSignal,
		Layer:   LayerConceptual,
		Content: "some signal",
	}

	runner := &mockRunner{response: "I think this looks fine!"}
	_, err := RunPreflight(context.Background(), runner, proposed, graph)
	if err == nil {
		t.Fatal("RunPreflight() expected error when response is unparseable")
	}
	if !strings.Contains(err.Error(), "parsing pre-flight result") {
		t.Errorf("error should wrap parse failure, got: %v", err)
	}
}

func TestRunPreflight_CorrectCheckTypeSelection(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("signal"))
	dec := entry("20260410-130000-d-tac-bbb", withContent("decision"))
	graph := NewGraph([]*Entry{sig, dec})

	// Action closing decision should use closing-action template
	proposed := &Entry{
		Type:    TypeAction,
		Layer:   LayerTactical,
		Closes:  []string{dec.ID},
		Content: "implemented everything",
	}

	runner := &mockRunner{response: "PASS"}
	_, err := RunPreflight(context.Background(), runner, proposed, graph)
	if err != nil {
		t.Fatal(err)
	}

	// The prompt should contain closing-action-specific language
	if !strings.Contains(runner.lastPrompt, "close") {
		t.Error("Prompt should contain closing-action check language")
	}
	if !strings.Contains(runner.lastPrompt, "decision") {
		t.Error("Prompt should include the closed decision content")
	}
}
