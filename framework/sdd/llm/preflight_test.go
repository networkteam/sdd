package llm

import (
	"os"
	"path/filepath"
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

	pctx, err := assembleContext(proposed, graph, checkSignalCapture, nil)
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

	pctx, err := assembleContext(proposed, graph, checkDecisionRefs, nil)
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

	pctx, err := assembleContext(proposed, graph, checkClosingDecision, nil)
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

	pctx, err := assembleContext(proposed, graph, checkSignalCapture, nil)
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

	pctx, err := assembleContext(proposed, graph, checkSupersedes, nil)
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

	pctx, err := assembleContext(proposed, graph, checkDecisionRefs, nil)
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

	pctx, err := assembleContext(proposed, graph, checkSignalCapture, nil)
	if err != nil {
		t.Fatal(err)
	}

	if pctx.OpenSignals != "" {
		t.Error("OpenSignals should be empty for non-decision-refs checks")
	}
}

func Test_extractACSection(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no heading",
			in:   "# Plan\n\nSome body.\n",
			want: "",
		},
		{
			name: "heading followed by checklist to EOF",
			in:   "# Plan\n\n## Acceptance criteria\n- [ ] one\n- [ ] two\n",
			want: "- [ ] one\n- [ ] two",
		},
		{
			name: "heading followed by next level-2 section",
			in:   "## Acceptance criteria\n- [ ] item\n\n## Out of scope\n- detail\n",
			want: "- [ ] item",
		},
		{
			name: "heading followed by next level-1 section",
			in:   "## Acceptance criteria\n- [ ] item\n\n# New Part\n",
			want: "- [ ] item",
		},
		{
			name: "heading mid-document",
			in:   "# Plan\n\n## Overview\nbody\n\n## Acceptance criteria\n- [ ] thing\n",
			want: "- [ ] thing",
		},
		{
			name: "heading case-insensitive",
			in:   "## ACCEPTANCE criteria\n- [ ] x\n",
			want: "- [ ] x",
		},
		{
			name: "heading with trailing colon",
			in:   "## Acceptance criteria:\n- [ ] y\n",
			want: "- [ ] y",
		},
		{
			name: "level-3 subheading does not terminate",
			in:   "## Acceptance criteria\n### Group\n- [ ] a\n",
			want: "### Group\n- [ ] a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractACSection(tt.in)
			if got != tt.want {
				t.Errorf("extractACSection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_assembleContext_ProposedPlanWithAC(t *testing.T) {
	graph := model.NewGraph(nil)
	attPath := "2026/04/15/15-113612-d-prc-xyz/plan.md"
	proposed := &model.Entry{
		Type:        model.TypeDecision,
		Layer:       model.LayerProcess,
		Kind:        model.KindPlan,
		Content:     "plan for the thing",
		Attachments: []string{attPath},
	}
	planBody := "# Plan\n\n## Overview\nstuff\n\n## Acceptance criteria\n- [ ] deliver X\n- [ ] deliver Y\n"

	pctx, err := assembleContext(proposed, graph, checkDecisionRefs, map[string][]byte{
		attPath: []byte(planBody),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pctx.ProposedIsPlan {
		t.Error("ProposedIsPlan should be true for a kind:plan decision")
	}
	if !strings.Contains(pctx.ProposedAC, "deliver X") || !strings.Contains(pctx.ProposedAC, "deliver Y") {
		t.Errorf("ProposedAC missing checklist items, got %q", pctx.ProposedAC)
	}
}

func Test_assembleContext_ProposedPlanMissingAC(t *testing.T) {
	graph := model.NewGraph(nil)
	attPath := "2026/04/15/15-113612-d-prc-xyz/plan.md"
	proposed := &model.Entry{
		Type:        model.TypeDecision,
		Layer:       model.LayerProcess,
		Kind:        model.KindPlan,
		Content:     "plan for the thing",
		Attachments: []string{attPath},
	}
	planBody := "# Plan\n\n## Overview\nstuff without an AC section\n"

	pctx, err := assembleContext(proposed, graph, checkDecisionRefs, map[string][]byte{
		attPath: []byte(planBody),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pctx.ProposedIsPlan {
		t.Error("ProposedIsPlan should be true even when AC section is absent")
	}
	if pctx.ProposedAC != "" {
		t.Errorf("ProposedAC should be empty when AC section is missing, got %q", pctx.ProposedAC)
	}
}

func Test_assembleContext_ClosedPlanACExtracted(t *testing.T) {
	// Write a plan entry with an attachment on disk, then verify that
	// closing it populates both PlanItems (full text) and AcceptanceCriteria
	// (extracted section) in the context.
	dir := t.TempDir()

	plan := entry("20260410-120000-d-tac-pln",
		withKind(model.KindPlan),
		withContent("plan body"),
		withAttachments("2026/04/10/12-0000-d-tac-pln/plan.md"))
	attRel := plan.Attachments[0]
	attAbs := filepath.Join(dir, attRel)
	if err := os.MkdirAll(filepath.Dir(attAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	planBody := "# Plan\n\n## Overview\nstuff\n\n## Acceptance criteria\n- [ ] finish X\n- [ ] finish Y\n"
	if err := os.WriteFile(attAbs, []byte(planBody), 0o644); err != nil {
		t.Fatal(err)
	}

	graph := model.NewGraph([]*model.Entry{plan})
	graph.SetGraphDir(dir)

	proposed := &model.Entry{
		Type:    model.TypeAction,
		Layer:   model.LayerTactical,
		Closes:  []string{plan.ID},
		Content: "action closing the plan",
	}

	pctx, err := assembleContext(proposed, graph, checkClosingAction, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pctx.PlanItems, "finish X") {
		t.Errorf("PlanItems should contain full plan body, got %q", pctx.PlanItems)
	}
	if !strings.Contains(pctx.AcceptanceCriteria, "finish X") ||
		!strings.Contains(pctx.AcceptanceCriteria, "finish Y") {
		t.Errorf("AcceptanceCriteria should contain checklist items, got %q", pctx.AcceptanceCriteria)
	}
	if strings.Contains(pctx.AcceptanceCriteria, "Overview") {
		t.Errorf("AcceptanceCriteria should not contain other sections, got %q", pctx.AcceptanceCriteria)
	}
}

func Test_assembleContext_NonPlanDecisionHasNoProposedAC(t *testing.T) {
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Kind:    model.KindDirective,
		Content: "a directive",
	}

	pctx, err := assembleContext(proposed, graph, checkDecisionRefs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pctx.ProposedIsPlan {
		t.Error("ProposedIsPlan should be false for a non-plan decision")
	}
	if pctx.ProposedAC != "" {
		t.Errorf("ProposedAC should be empty for non-plan decision, got %q", pctx.ProposedAC)
	}
}

func Test_renderPreflightPrompt_AllCheckTypes(t *testing.T) {
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
			if !strings.Contains(result, "PASS") || !strings.Contains(result, "FAIL") {
				t.Errorf("renderPreflightPrompt(%s) missing output format instructions", ct)
			}
			_ = tmplName
		})
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
			name:         "No findings sentinel",
			input:        "No findings.",
			wantFindings: nil,
		},
		{
			name:         "No findings without period",
			input:        "No findings",
			wantFindings: nil,
		},
		{
			name:         "No findings case insensitive",
			input:        "no FINDINGS.",
			wantFindings: nil,
		},
		{
			name:         "No findings with surrounding whitespace",
			input:        "  No findings.  \n",
			wantFindings: nil,
		},
		{
			name:  "single high finding",
			input: "- [high] type-mismatch: signal contains imperative commitment",
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "type-mismatch", Observation: "signal contains imperative commitment"},
			},
		},
		{
			name:  "mixed severities",
			input: "- [high] missing-ref: directly-answered signal not referenced\n- [medium] plan-coverage-ambiguity: test behavior unstated\n- [low] opening-reference-dependent: first sentence relies on ref",
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "missing-ref", Observation: "directly-answered signal not referenced"},
				{Severity: SeverityMedium, Category: "plan-coverage-ambiguity", Observation: "test behavior unstated"},
				{Severity: SeverityLow, Category: "opening-reference-dependent", Observation: "first sentence relies on ref"},
			},
		},
		{
			name:  "severity case insensitive",
			input: "- [HIGH] type-mismatch: something\n- [Medium] other: thing",
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "type-mismatch", Observation: "something"},
				{Severity: SeverityMedium, Category: "other", Observation: "thing"},
			},
		},
		{
			name:  "bullet prefix optional",
			input: "[high] type-mismatch: no leading dash",
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "type-mismatch", Observation: "no leading dash"},
			},
		},
		{
			name:  "indented bullet",
			input: "  - [medium] ac-unaddressed: AC 3 silently omitted",
			wantFindings: []Finding{
				{Severity: SeverityMedium, Category: "ac-unaddressed", Observation: "AC 3 silently omitted"},
			},
		},
		{
			name:  "blank lines between findings",
			input: "- [high] a: one\n\n- [low] b: two\n",
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "a", Observation: "one"},
				{Severity: SeverityLow, Category: "b", Observation: "two"},
			},
		},
		{
			name:    "unknown severity",
			input:   "- [critical] type-mismatch: something",
			wantErr: true,
		},
		{
			name:    "malformed line",
			input:   "- this is not a finding",
			wantErr: true,
		},
		{
			name:    "missing category",
			input:   "- [high]: observation text",
			wantErr: true,
		},
		{
			name:    "missing observation",
			input:   "- [high] type-mismatch:",
			wantErr: true,
		},
		{
			name:    "empty response",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   \n  \n  ",
			wantErr: true,
		},
		{
			name:    "legacy PASS output rejected",
			input:   "PASS",
			wantErr: true,
		},
		{
			name:    "legacy FAIL output rejected",
			input:   "FAIL\n- Some gap",
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
