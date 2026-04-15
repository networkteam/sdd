package llm

import (
	"strings"
	"testing"

	"github.com/networkteam/resonance/framework/sdd/model"
)

func Test_selectCheckType(t *testing.T) {
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

func Test_FormatEntryForPrompt(t *testing.T) {
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

	result := FormatEntryForPrompt(e)

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

func Test_FormatEntryForPrompt_ShowsKindForDecisions(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-d-tac-xyz",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Content: "Some directive.",
	}

	result := FormatEntryForPrompt(e)
	if !strings.Contains(result, "Kind: directive") {
		t.Errorf("FormatEntryForPrompt() should show Kind for decisions, got:\n%s", result)
	}
}

func Test_FormatEntryForPrompt_OmitsKindForSignals(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-s-tac-xyz",
		Type:    model.TypeSignal,
		Layer:   model.LayerTactical,
		Content: "Observed something.",
	}

	result := FormatEntryForPrompt(e)
	if strings.Contains(result, "Kind:") {
		t.Errorf("FormatEntryForPrompt() should omit Kind for signals, got:\n%s", result)
	}
}

func Test_FormatEntryForPrompt_OmitsEmptyFields(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-s-cpt-xyz",
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "Observed something.",
	}

	result := FormatEntryForPrompt(e)
	for _, field := range []string{"Kind:", "Refs:", "Closes:", "Supersedes:", "Confidence:"} {
		if strings.Contains(result, field) {
			t.Errorf("FormatEntryForPrompt() should omit empty %s, got:\n%s", field, result)
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

func Test_assembleContext_ClosedPlanDescriptionFlowsThrough(t *testing.T) {
	// Plans carry their AC section inline in the description; closing an
	// action against a plan flows that description through ClosedEntries
	// via FormatEntryForPrompt — no attachment extraction needed.
	plan := entry("20260410-120000-d-tac-pln",
		withKind(model.KindPlan),
		withContent("Plan body.\n\n## Acceptance criteria\n- [ ] finish X\n- [ ] finish Y\n"))
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeAction,
		Layer:   model.LayerTactical,
		Closes:  []string{plan.ID},
		Content: "action closing the plan",
	}

	pctx, err := assembleContext(proposed, graph, checkClosingAction)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pctx.ClosedEntries, "## Acceptance criteria") {
		t.Errorf("ClosedEntries should contain the plan's AC heading inline, got %q", pctx.ClosedEntries)
	}
	if !strings.Contains(pctx.ClosedEntries, "finish X") || !strings.Contains(pctx.ClosedEntries, "finish Y") {
		t.Errorf("ClosedEntries should contain checklist items from the plan description")
	}
}

func Test_renderPreflightPrompt_AllCheckTypes(t *testing.T) {
	pctx := &preflightContext{
		ProposedEntry:     "ID: test\nType: signal\n\nTest content",
		ReferencedEntries: "ID: ref\nType: signal\n\nRef content",
		ClosedEntries:     "ID: closed\nType: signal\n\nClosed content",
		SupersededEntries: "ID: old\nType: decision\n\nOld content",
		ActiveContracts:   "ID: contract\nType: decision\n\nContract content",
		OpenSignals:       "ID: open\nType: signal\n\nOpen signal",
	}

	for ct, tmplName := range checkTypeTemplates {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if result == "" {
				t.Errorf("renderPreflightPrompt(%s) returned empty string", ct)
			}
			if !strings.Contains(result, "Test content") {
				t.Errorf("renderPreflightPrompt(%s) missing proposed entry content", ct)
			}
			// Verdict partial must be embedded — the JSON output format and
			// severity semantics are the single source of truth for all checks.
			if !strings.Contains(result, `"findings"`) {
				t.Errorf("renderPreflightPrompt(%s) missing JSON schema with findings key", ct)
			}
			if !strings.Contains(result, `"severity"`) {
				t.Errorf("renderPreflightPrompt(%s) missing severity field in schema", ct)
			}
			// PASS/FAIL are the legacy binary verdict — must be gone.
			if strings.Contains(result, "\"PASS\"") || strings.Contains(result, "\"FAIL\"") {
				t.Errorf("renderPreflightPrompt(%s) still contains legacy PASS/FAIL output", ct)
			}
			_ = tmplName
		})
	}
}

func Test_renderPreflightPrompt_DecisionRefsNamesACCheck(t *testing.T) {
	// decision_refs.tmpl describes the AC-presence check for plan decisions.
	// The LLM reads kind from .ProposedEntry and applies the check contextually;
	// the check text is always rendered.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: plan\n\nplan body",
	}
	result, err := renderPreflightPrompt(checkDecisionRefs, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Acceptance criteria") {
		t.Errorf("decision_refs template should name the acceptance criteria check")
	}
}

func Test_renderPreflightPrompt_ClosingActionNamesACCheck(t *testing.T) {
	// closing_action.tmpl describes per-AC coverage when the closed entry is
	// a plan. The LLM reads the closed entry's kind and description.
	pctx := &preflightContext{
		ProposedEntry: "ID: a\nType: action\n\nclosing the plan",
		ClosedEntries: "ID: p\nType: decision\nKind: plan\n\nplan description with AC section",
	}
	result, err := renderPreflightPrompt(checkClosingAction, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Acceptance criteria") {
		t.Errorf("closing_action template should name the acceptance criteria coverage check")
	}
}

func Test_renderPreflightPrompt_InvalidCheckType(t *testing.T) {
	pctx := &preflightContext{ProposedEntry: "test"}
	_, err := renderPreflightPrompt(checkType(99), pctx)
	if err == nil {
		t.Error("renderPreflightPrompt with invalid check type should return error")
	}
}

func Test_renderPreflightPrompt_ConditionalSections(t *testing.T) {
	pctx := &preflightContext{
		ProposedEntry: "ID: test\nType: signal\n\nTest content",
	}

	result, err := renderPreflightPrompt(checkSignalCapture, pctx)
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

func Test_parsePreflightResult(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantFindings []Finding
		wantErr      bool
	}{
		{
			name:         "empty findings array",
			input:        `{"findings": []}`,
			wantFindings: nil,
		},
		{
			name:  "single high finding",
			input: `{"findings": [{"severity": "high", "category": "type-mismatch", "observation": "signal contains imperative commitment"}]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "type-mismatch", Observation: "signal contains imperative commitment"},
			},
		},
		{
			name: "mixed severities",
			input: `{"findings": [
				{"severity": "high", "category": "missing-ref", "observation": "directly-answered signal not referenced"},
				{"severity": "medium", "category": "plan-coverage-ambiguity", "observation": "test behavior unstated"},
				{"severity": "low", "category": "opening-reference-dependent", "observation": "first sentence relies on ref"}
			]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "missing-ref", Observation: "directly-answered signal not referenced"},
				{Severity: SeverityMedium, Category: "plan-coverage-ambiguity", Observation: "test behavior unstated"},
				{Severity: SeverityLow, Category: "opening-reference-dependent", Observation: "first sentence relies on ref"},
			},
		},
		{
			name:  "severity case insensitive",
			input: `{"findings": [{"severity": "HIGH", "category": "t", "observation": "x"}]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "t", Observation: "x"},
			},
		},
		{
			name:  "JSON wrapped in code fence",
			input: "```json\n{\"findings\": [{\"severity\": \"high\", \"category\": \"a\", \"observation\": \"one\"}]}\n```",
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "a", Observation: "one"},
			},
		},
		{
			name:         "JSON with preamble and postamble prose",
			input:        "Let me validate this entry:\n\n{\"findings\": []}\n\nThat's my assessment.",
			wantFindings: nil,
		},
		{
			name:  "braces inside observation string do not confuse balance",
			input: `{"findings": [{"severity": "high", "category": "a", "observation": "The entry says {x: y} which is odd"}]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "a", Observation: "The entry says {x: y} which is odd"},
			},
		},
		{
			name:    "unknown severity",
			input:   `{"findings": [{"severity": "critical", "category": "t", "observation": "x"}]}`,
			wantErr: true,
		},
		{
			name:    "missing category",
			input:   `{"findings": [{"severity": "high", "observation": "x"}]}`,
			wantErr: true,
		},
		{
			name:    "missing observation",
			input:   `{"findings": [{"severity": "high", "category": "t"}]}`,
			wantErr: true,
		},
		{
			name:    "no JSON object",
			input:   "PASS\nSome prose without JSON.",
			wantErr: true,
		},
		{
			name:    "unbalanced braces",
			input:   `{"findings": [{"severity": "high"`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			input:   `{"findings": [{severity: high}]}`,
			wantErr: true,
		},
		{
			name:    "empty response",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePreflightResult(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("parsePreflightResult() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePreflightResult() unexpected error: %v", err)
			}
			if len(result.Findings) != len(tt.wantFindings) {
				t.Fatalf("parsePreflightResult().Findings len = %d, want %d\ngot: %+v",
					len(result.Findings), len(tt.wantFindings), result.Findings)
			}
			for i, f := range result.Findings {
				want := tt.wantFindings[i]
				if f.Severity != want.Severity {
					t.Errorf("finding[%d].Severity = %q, want %q", i, f.Severity, want.Severity)
				}
				if f.Category != want.Category {
					t.Errorf("finding[%d].Category = %q, want %q", i, f.Category, want.Category)
				}
				if f.Observation != want.Observation {
					t.Errorf("finding[%d].Observation = %q, want %q", i, f.Observation, want.Observation)
				}
			}
		})
	}
}

func Test_PreflightResult_HasBlocking(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     bool
	}{
		{"empty", nil, false},
		{"only medium", []Finding{{Severity: SeverityMedium}}, false},
		{"only low", []Finding{{Severity: SeverityLow}}, false},
		{"medium and low", []Finding{{Severity: SeverityMedium}, {Severity: SeverityLow}}, false},
		{"single high", []Finding{{Severity: SeverityHigh}}, true},
		{"high among others", []Finding{{Severity: SeverityLow}, {Severity: SeverityHigh}, {Severity: SeverityMedium}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &PreflightResult{Findings: tt.findings}
			if got := r.HasBlocking(); got != tt.want {
				t.Errorf("HasBlocking() = %v, want %v", got, tt.want)
			}
		})
	}
}
