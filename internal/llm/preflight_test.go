package llm

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func Test_selectCheckType(t *testing.T) {
	signal := entry("20260410-120000-s-cpt-aaa", withContent("some signal")) // default kind: empty, treated as gap
	gapSignal := entry("20260410-120100-s-cpt-gap", withKind(model.KindGap), withContent("gap signal"))
	questionSignal := entry("20260410-120200-s-cpt-qst", withKind(model.KindQuestion), withContent("question signal"))
	factSignal := entry("20260410-120300-s-cpt-fct", withKind(model.KindFact), withContent("fact signal"))
	decision := entry("20260410-120000-d-tac-bbb", withContent("some decision"))
	plan := entry("20260410-120000-d-tac-ccc", withKind(model.KindPlan), withContent("some plan"))
	contract := entry("20260410-120000-d-prc-ctr", withKind(model.KindContract), withContent("some contract"))
	aspiration := entry("20260410-120000-d-stg-asp", withKind(model.KindAspiration), withContent("some aspiration"))

	graph := model.NewGraph([]*model.Entry{signal, gapSignal, questionSignal, factSignal, decision, plan, contract, aspiration})

	tests := []struct {
		name     string
		entry    *model.Entry
		expected checkType
	}{
		{
			name:     "done signal closing decision",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{decision.ID}},
			expected: checkClosingDone,
		},
		{
			name:     "done signal closing plan",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{plan.ID}},
			expected: checkClosingDone,
		},
		{
			name:     "done signal closing gap signal (short-loop)",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{gapSignal.ID}},
			expected: checkShortLoop,
		},
		{
			name:     "done signal closing both decision and signal routes to closing-done",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{decision.ID, gapSignal.ID}},
			expected: checkClosingDone,
		},
		{
			name:     "legacy action closing decision routes to closing-done",
			entry:    &model.Entry{Type: model.TypeAction, Closes: []string{decision.ID}},
			expected: checkClosingDone,
		},
		{
			name:     "legacy action closing signal routes to short-loop",
			entry:    &model.Entry{Type: model.TypeAction, Closes: []string{signal.ID}},
			expected: checkShortLoop,
		},
		{
			name:     "fact signal closing question (dissolution)",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindFact, Closes: []string{questionSignal.ID}},
			expected: checkDissolution,
		},
		{
			name:     "insight signal closing question (dissolution)",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindInsight, Closes: []string{questionSignal.ID}},
			expected: checkDissolution,
		},
		{
			name:     "fact signal closing non-question routes to dissolution (unusual pattern)",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindFact, Closes: []string{factSignal.ID}},
			expected: checkDissolution,
		},
		{
			name:     "decision closing signal",
			entry:    &model.Entry{Type: model.TypeDecision, Closes: []string{signal.ID}},
			expected: checkClosingDecision,
		},
		{
			name:     "directive closing contract (retirement)",
			entry:    &model.Entry{Type: model.TypeDecision, Kind: model.KindDirective, Closes: []string{contract.ID}},
			expected: checkClosingDecision,
		},
		{
			name:     "aspiration decision with refs routes to aspiration-capture",
			entry:    &model.Entry{Type: model.TypeDecision, Kind: model.KindAspiration, Refs: []string{signal.ID}},
			expected: checkAspirationCapture,
		},
		{
			name:     "aspiration decision with no refs routes to aspiration-capture",
			entry:    &model.Entry{Type: model.TypeDecision, Kind: model.KindAspiration},
			expected: checkAspirationCapture,
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
		{checkClosingDone, "closing-done"},
		{checkClosingDecision, "closing-decision"},
		{checkDecisionRefs, "decision-refs"},
		{checkShortLoop, "short-loop"},
		{checkDissolution, "dissolution"},
		{checkAspirationCapture, "aspiration-capture"},
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

func Test_FormatEntryForPrompt_ShowsKindForSignals(t *testing.T) {
	// Regression for s-prc-y0e: pre-flight reported `missing-explicit-kind`
	// as a medium finding on insight signals that carried an explicit kind,
	// because FormatEntryForPrompt gated the Kind line on type == decision.
	// The LLM was telling the truth — the prompt genuinely didn't contain the
	// field. Under the two-type system (d-cpt-ydf), kind is structural on
	// signals too and must flow into the pre-flight prompt.
	e := &model.Entry{
		ID:      "20260420-163043-s-stg-ljq",
		Type:    model.TypeSignal,
		Layer:   model.LayerStrategic,
		Kind:    model.KindInsight,
		Content: "Some insight.",
	}

	result := FormatEntryForPrompt(e)
	if !strings.Contains(result, "Kind: insight") {
		t.Errorf("FormatEntryForPrompt() should show explicit Kind for signals, got:\n%s", result)
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

	pctx := assembleContext(proposed, graph, checkSignalCapture)

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

	pctx := assembleContext(proposed, graph, checkDecisionRefs)

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

	pctx := assembleContext(proposed, graph, checkClosingDecision)

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

	pctx := assembleContext(proposed, graph, checkSignalCapture)

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

	pctx := assembleContext(proposed, graph, checkSupersedes)

	if !strings.Contains(pctx.SupersededEntries, "old decision") {
		t.Error("SupersededEntries should contain the superseded entry content")
	}
}

func Test_assembleContext_OpenSignalsForDecisionRefs(t *testing.T) {
	sig1 := entry("20260410-120000-s-cpt-aaa", withKind(model.KindGap), withContent("open signal one"))
	sig2 := entry("20260410-120100-s-tac-bbb", withKind(model.KindGap), withContent("open signal two"))
	graph := model.NewGraph([]*model.Entry{sig1, sig2})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Refs:    []string{sig1.ID},
		Content: "new decision",
	}

	pctx := assembleContext(proposed, graph, checkDecisionRefs)

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

	pctx := assembleContext(proposed, graph, checkSignalCapture)

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

	pctx := assembleContext(proposed, graph, checkClosingDone)
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
			if result.Combined() == "" {
				t.Errorf("renderPreflightPrompt(%s) returned empty string", ct)
			}
			if !strings.Contains(result.Combined(), "Test content") {
				t.Errorf("renderPreflightPrompt(%s) missing proposed entry content", ct)
			}
			// Verdict partial must be embedded — the JSON output format and
			// severity semantics are the single source of truth for all checks.
			if !strings.Contains(result.Combined(), `"findings"`) {
				t.Errorf("renderPreflightPrompt(%s) missing JSON schema with findings key", ct)
			}
			if !strings.Contains(result.Combined(), `"severity"`) {
				t.Errorf("renderPreflightPrompt(%s) missing severity field in schema", ct)
			}
			// PASS/FAIL are the legacy binary verdict — must be gone.
			if strings.Contains(result.Combined(), "\"PASS\"") || strings.Contains(result.Combined(), "\"FAIL\"") {
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
	if !strings.Contains(result.Combined(), "Acceptance criteria") {
		t.Errorf("decision_refs template should name the acceptance criteria check")
	}
}

func Test_renderPreflightPrompt_ClosingDoneNamesACCheck(t *testing.T) {
	// closing_done.tmpl describes per-AC coverage when the closed entry is
	// a plan. The LLM reads the closed entry's kind and description.
	pctx := &preflightContext{
		ProposedEntry: "ID: s\nType: signal\nKind: done\n\nclosing the plan",
		ClosedEntries: "ID: p\nType: decision\nKind: plan\n\nplan description with AC section",
	}
	result, err := renderPreflightPrompt(checkClosingDone, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Acceptance criteria") {
		t.Errorf("closing_done template should name the acceptance criteria coverage check")
	}
}

func Test_renderPreflightPrompt_CompletionTemplatesIncludeDurabilityCheck(t *testing.T) {
	// Regression: the durability check must be present on every completion-record
	// template (closing_done, short_loop) so the LLM validates artifact durability
	// for any entry claiming completion.
	pctx := &preflightContext{
		ProposedEntry: "ID: s\nType: signal\nKind: done\n\nclosing something",
		ClosedEntries: "ID: x\nType: decision\n\nsome decision",
	}

	for _, ct := range []checkType{checkClosingDone, checkShortLoop} {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if !strings.Contains(result.Combined(), "Artifact durability") {
				t.Errorf("renderPreflightPrompt(%s) should include the durability check", ct)
			}
		})
	}
}

func Test_renderPreflightPrompt_CapturesIncludeUnrelatedRefsCheck(t *testing.T) {
	// Every capture template must invoke the shared unrelated_refs partial so
	// topically-disconnected refs get surfaced regardless of transaction type.
	pctx := &preflightContext{
		ProposedEntry:     "ID: test\n\nproposed",
		ReferencedEntries: "ID: ref\n\nreferenced entry",
		ClosedEntries:     "ID: closed\n\nclosed entry",
	}

	captureTypes := []checkType{
		checkSignalCapture,
		checkDecisionRefs,
		checkAspirationCapture,
		checkClosingDecision,
		checkClosingDone,
		checkShortLoop,
		checkDissolution,
	}
	for _, ct := range captureTypes {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if !strings.Contains(result.Combined(), "Unrelated references check") {
				t.Errorf("renderPreflightPrompt(%s) should include the unrelated_refs partial", ct)
			}
		})
	}
}

func Test_renderPreflightPrompt_CloseCarryingTemplatesIncludeUnusualClose(t *testing.T) {
	// Every close-carrying template must invoke the shared unusual_close partial.
	pctx := &preflightContext{
		ProposedEntry: "ID: test\n\nproposed",
		ClosedEntries: "ID: closed\n\nclosed entry",
	}

	closeTypes := []checkType{
		checkClosingDecision,
		checkClosingDone,
		checkShortLoop,
		checkDissolution,
	}
	for _, ct := range closeTypes {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if !strings.Contains(result.Combined(), "Unusual close-pattern check") {
				t.Errorf("renderPreflightPrompt(%s) should include the unusual_close partial", ct)
			}
		})
	}
}

func Test_renderPreflightPrompt_ClosingDecisionNamesRetirementRationale(t *testing.T) {
	// closing_decision.tmpl carries the retirement-rationale check for stable-kind targets.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: directive\n\nretiring the old contract",
		ClosedEntries: "ID: c\nType: decision\nKind: contract\n\ncontract to retire",
	}
	result, err := renderPreflightPrompt(checkClosingDecision, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Retirement-rationale calibration") {
		t.Errorf("closing_decision template should name the retirement-rationale calibration")
	}
}

func Test_renderPreflightPrompt_DecisionRefsNamesDirectiveShapeCheck(t *testing.T) {
	// decision_refs.tmpl flags directives at stg/cpt that read as aspirations.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: directive\nLayer: strategic\n\nperpetual pull toward X",
	}
	result, err := renderPreflightPrompt(checkDecisionRefs, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Directive-reads-aspiration-shaped") {
		t.Errorf("decision_refs template should name the directive-reads-aspiration-shaped calibration")
	}
}

func Test_renderPreflightPrompt_AspirationCaptureNeverHigh(t *testing.T) {
	// aspiration_capture.tmpl must state that findings are never high severity.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: aspiration\nLayer: strategic\n\nperpetual pull toward X",
	}
	result, err := renderPreflightPrompt(checkAspirationCapture, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Never `high`") && !strings.Contains(result.Combined(), "never `high`") {
		t.Errorf("aspiration_capture template should state findings are never high severity")
	}
}

func Test_renderPreflightPrompt_DissolutionNamesContextPresence(t *testing.T) {
	// dissolution.tmpl checks for dialogue-captured context connecting the closing entry
	// to the question, not the correctness of the reasoning.
	pctx := &preflightContext{
		ProposedEntry: "ID: s\nType: signal\nKind: fact\n\nfact resolving a question",
		ClosedEntries: "ID: q\nType: signal\nKind: question\n\nthe question being resolved",
	}
	result, err := renderPreflightPrompt(checkDissolution, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "dialogue-captured context") {
		t.Errorf("dissolution template should name dialogue-captured context as the test")
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

	if strings.Contains(result.Combined(), "## Referenced entries") {
		t.Error("Should not include Referenced entries section when empty")
	}
	if strings.Contains(result.Combined(), "## Active contracts") {
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
