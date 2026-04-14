package finders

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
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

func Test_selectCheckType(t *testing.T) {
	// Base entries in the graph for reference
	signal := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	decision := entry("20260410-120000-d-tac-bbb", withContent("some decision"))
	plan := entry("20260410-120000-d-tac-ccc", withKind(model.KindPlan), withContent("some plan"))

	graph := model.NewGraph([]*model.Entry{signal, decision, plan})

	tests := []struct {
		name     string
		entry    *model.Entry
		expected checkType
	}{
		{
			name:     "action closing decision",
			entry:    &model.Entry{Type: model.TypeAction, Closes: []string{decision.ID}},
			expected: checkClosingAction,
		},
		{
			name:     "action closing plan",
			entry:    &model.Entry{Type: model.TypeAction, Closes: []string{plan.ID}},
			expected: checkClosingAction,
		},
		{
			name:     "action closing signal",
			entry:    &model.Entry{Type: model.TypeAction, Closes: []string{signal.ID}},
			expected: checkActionClosesSignals,
		},
		{
			name:     "action closing both decision and signal picks closing-action",
			entry:    &model.Entry{Type: model.TypeAction, Closes: []string{decision.ID, signal.ID}},
			expected: checkClosingAction,
		},
		{
			name:     "decision closing signal",
			entry:    &model.Entry{Type: model.TypeDecision, Closes: []string{signal.ID}},
			expected: checkClosingDecision,
		},
		{
			name:     "decision with refs only",
			entry:    &model.Entry{Type: model.TypeDecision, Refs: []string{signal.ID}},
			expected: checkDecisionRefs,
		},
		{
			name:     "decision with no refs or closes",
			entry:    &model.Entry{Type: model.TypeDecision},
			expected: checkDecisionRefs,
		},
		{
			name:     "signal",
			entry:    &model.Entry{Type: model.TypeSignal},
			expected: checkSignalCapture,
		},
		{
			name:     "signal with refs",
			entry:    &model.Entry{Type: model.TypeSignal, Refs: []string{decision.ID}},
			expected: checkSignalCapture,
		},
		{
			name:     "supersedes takes priority over closes",
			entry:    &model.Entry{Type: model.TypeDecision, Supersedes: []string{decision.ID}, Closes: []string{signal.ID}},
			expected: checkSupersedes,
		},
		{
			name:     "supersedes on action",
			entry:    &model.Entry{Type: model.TypeAction, Supersedes: []string{decision.ID}},
			expected: checkSupersedes,
		},
		{
			name:     "supersedes on signal",
			entry:    &model.Entry{Type: model.TypeSignal, Supersedes: []string{signal.ID}},
			expected: checkSupersedes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectCheckType(tt.entry, graph)
			if got != tt.expected {
				t.Errorf("selectCheckType() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func Test_checkTypeString(t *testing.T) {
	tests := []struct {
		ct   checkType
		want string
	}{
		{checkClosingAction, "closing-action"},
		{checkClosingDecision, "closing-decision"},
		{checkDecisionRefs, "decision-refs"},
		{checkActionClosesSignals, "action-closes-signals"},
		{checkSignalCapture, "signal-capture"},
		{checkSupersedes, "supersedes"},
		{checkType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.want {
			t.Errorf("checkType(%d).String() = %q, want %q", int(tt.ct), got, tt.want)
		}
	}
}

func Test_formatEntryForPrompt(t *testing.T) {
	e := &model.Entry{
		ID:         "20260410-120000-d-tac-xyz",
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Kind:       model.KindPlan,
		Refs:       []string{"20260410-110000-s-cpt-aaa"},
		Closes:     []string{"20260410-100000-s-stg-bbb"},
		Confidence: "high",
		Content:    "Build the validator.",
	}

	result := formatEntryForPrompt(e)

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
			t.Errorf("formatEntryForPrompt() missing %q\nGot:\n%s", check, result)
		}
	}
}

func Test_formatEntryForPrompt_ShowsKindForDecisions(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-d-tac-xyz",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Content: "Some directive.",
	}

	result := formatEntryForPrompt(e)
	if !strings.Contains(result, "Kind: directive") {
		t.Errorf("formatEntryForPrompt() should show Kind for decisions, got:\n%s", result)
	}
}

func Test_formatEntryForPrompt_OmitsKindForSignals(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-s-tac-xyz",
		Type:    model.TypeSignal,
		Layer:   model.LayerTactical,
		Content: "Observed something.",
	}

	result := formatEntryForPrompt(e)
	if strings.Contains(result, "Kind:") {
		t.Errorf("formatEntryForPrompt() should omit Kind for signals, got:\n%s", result)
	}
}

func Test_formatEntryForPrompt_OmitsEmptyFields(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-s-cpt-xyz",
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "Observed something.",
	}

	result := formatEntryForPrompt(e)
	for _, field := range []string{"Kind:", "Refs:", "Closes:", "Supersedes:", "Confidence:"} {
		if strings.Contains(result, field) {
			t.Errorf("formatEntryForPrompt() should omit empty %s, got:\n%s", field, result)
		}
	}
}

func Test_assembleContext_BasicSignal(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("observed something"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new signal",
	}

	pctx, err := assembleContext(proposed, graph, checkSignalCapture)
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

func Test_assembleContext_WithRefs(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("first signal"))
	dec := entry("20260410-130000-d-tac-bbb", withContent("decision content"), withRefs(sig.ID))
	graph := model.NewGraph([]*model.Entry{sig, dec})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Refs:    []string{sig.ID},
		Content: "new decision",
	}

	pctx, err := assembleContext(proposed, graph, checkDecisionRefs)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.ReferencedEntries, "first signal") {
		t.Error("ReferencedEntries should contain the referenced signal content")
	}
}

func Test_assembleContext_WithCloses(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("signal to close"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Closes:  []string{sig.ID},
		Content: "decision closing signal",
	}

	pctx, err := assembleContext(proposed, graph, checkClosingDecision)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.ClosedEntries, "signal to close") {
		t.Error("ClosedEntries should contain the closed signal content")
	}
}

func Test_assembleContext_WithContracts(t *testing.T) {
	contract := entry("20260410-120000-d-prc-aaa",
		withKind(model.KindContract),
		withContent("all entries must have refs"))
	graph := model.NewGraph([]*model.Entry{contract})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "some signal",
	}

	pctx, err := assembleContext(proposed, graph, checkSignalCapture)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.ActiveContracts, "all entries must have refs") {
		t.Error("ActiveContracts should contain contract content")
	}
}

func Test_assembleContext_WithSupersedes(t *testing.T) {
	old := entry("20260410-120000-d-tac-aaa", withContent("old decision"))
	graph := model.NewGraph([]*model.Entry{old})

	proposed := &model.Entry{
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Supersedes: []string{old.ID},
		Content:    "replacement decision",
	}

	pctx, err := assembleContext(proposed, graph, checkSupersedes)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(pctx.SupersededEntries, "old decision") {
		t.Error("SupersededEntries should contain the superseded entry content")
	}
}

func Test_assembleContext_OpenSignalsForDecisionRefs(t *testing.T) {
	sig1 := entry("20260410-120000-s-cpt-aaa", withContent("open signal one"))
	sig2 := entry("20260410-120100-s-tac-bbb", withContent("open signal two"))
	graph := model.NewGraph([]*model.Entry{sig1, sig2})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Refs:    []string{sig1.ID},
		Content: "new decision",
	}

	pctx, err := assembleContext(proposed, graph, checkDecisionRefs)
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

func Test_assembleContext_OpenSignalsNotIncludedForOtherChecks(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("open signal"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new signal",
	}

	pctx, err := assembleContext(proposed, graph, checkSignalCapture)
	if err != nil {
		t.Fatal(err)
	}

	if pctx.OpenSignals != "" {
		t.Error("OpenSignals should be empty for non-decision-refs checks")
	}
}

func Test_renderPrompt_AllCheckTypes(t *testing.T) {
	pctx := &preflightContext{
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
			result, err := renderPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPrompt(%s) error: %v", ct, err)
			}
			if result == "" {
				t.Errorf("renderPrompt(%s) returned empty string", ct)
			}
			// Every template should contain the proposed entry
			if !strings.Contains(result, "Test content") {
				t.Errorf("renderPrompt(%s) missing proposed entry content", ct)
			}
			// Every template should mention output format
			if !strings.Contains(result, "PASS") || !strings.Contains(result, "FAIL") {
				t.Errorf("renderPrompt(%s) missing output format instructions", ct)
			}
			_ = tmplName
		})
	}
}

func Test_renderPrompt_InvalidCheckType(t *testing.T) {
	pctx := &preflightContext{ProposedEntry: "test"}
	_, err := renderPrompt(checkType(99), pctx)
	if err == nil {
		t.Error("renderPrompt with invalid check type should return error")
	}
}

func Test_renderPrompt_ConditionalSections(t *testing.T) {
	// Empty optional fields should not produce section headers
	pctx := &preflightContext{
		ProposedEntry: "ID: test\nType: signal\n\nTest content",
	}

	result, err := renderPrompt(checkSignalCapture, pctx)
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

func Test_parseResult(t *testing.T) {
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
			result, err := parseResult(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("parseResult() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseResult() unexpected error: %v", err)
			}
			if result.Pass != tt.wantPass {
				t.Errorf("parseResult().Pass = %v, want %v", result.Pass, tt.wantPass)
			}
			if len(result.Gaps) != len(tt.wantGaps) {
				t.Fatalf("parseResult().Gaps = %v (len %d), want %v (len %d)",
					result.Gaps, len(result.Gaps), tt.wantGaps, len(tt.wantGaps))
			}
			for i, gap := range result.Gaps {
				if gap != tt.wantGaps[i] {
					t.Errorf("parseResult().Gaps[%d] = %q, want %q", i, gap, tt.wantGaps[i])
				}
			}
		})
	}
}

func TestRunPreflight_Pass(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new observation",
	}

	runner := &mockRunner{response: "PASS"}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatalf("Preflight() error: %v", err)
	}
	if !result.Pass {
		t.Error("Preflight() expected pass")
	}
	if runner.lastPrompt == "" {
		t.Error("Runner should have been called with a prompt")
	}
}

func TestRunPreflight_Fail(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Closes:  []string{sig.ID},
		Content: "decision closing signal",
	}

	runner := &mockRunner{response: "FAIL\n- Signal not genuinely addressed"}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatalf("Preflight() error: %v", err)
	}
	if result.Pass {
		t.Error("Preflight() expected fail")
	}
	if len(result.Gaps) != 1 || result.Gaps[0] != "Signal not genuinely addressed" {
		t.Errorf("Preflight().Gaps = %v, want [Signal not genuinely addressed]", result.Gaps)
	}
}

func TestRunPreflight_RunnerError(t *testing.T) {
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "some signal",
	}

	runner := &mockRunner{err: fmt.Errorf("claude CLI not found")}
	f := New(runner)
	_, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err == nil {
		t.Fatal("Preflight() expected error when runner fails")
	}
	if !strings.Contains(err.Error(), "running pre-flight validator") {
		t.Errorf("error should wrap runner failure, got: %v", err)
	}
}

func TestRunPreflight_ParseError(t *testing.T) {
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "some signal",
	}

	runner := &mockRunner{response: "I think this looks fine!"}
	f := New(runner)
	_, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err == nil {
		t.Fatal("Preflight() expected error when response is unparseable")
	}
	if !strings.Contains(err.Error(), "parsing pre-flight result") {
		t.Errorf("error should wrap parse failure, got: %v", err)
	}
}

func TestRunPreflight_CorrectCheckTypeSelection(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("signal"))
	dec := entry("20260410-130000-d-tac-bbb", withContent("decision"))
	graph := model.NewGraph([]*model.Entry{sig, dec})

	// Action closing decision should use closing-action template
	proposed := &model.Entry{
		Type:    model.TypeAction,
		Layer:   model.LayerTactical,
		Closes:  []string{dec.ID},
		Content: "implemented everything",
	}

	runner := &mockRunner{response: "PASS"}
	f := New(runner)
	_, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
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
