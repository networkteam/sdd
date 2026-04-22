package finders

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// mockRunner implements llm.Runner for testing.
type mockRunner struct {
	response string
	err      error
	// captured prompt for inspection (combined System + User).
	lastPrompt string
}

func (m *mockRunner) Run(_ context.Context, req llm.Request) (*llm.RunResult, error) {
	m.lastPrompt = req.Combined()
	if m.err != nil {
		return nil, m.err
	}
	return &llm.RunResult{Text: m.response}, nil
}

func TestRunPreflight_NoFindings(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new observation",
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatalf("Preflight() error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("Preflight() expected no findings, got %+v", result.Findings)
	}
	if result.HasBlocking() {
		t.Error("Preflight() should not block when no findings")
	}
	if runner.lastPrompt == "" {
		t.Error("Runner should have been called with a prompt")
	}
}

func TestRunPreflight_BlockingFinding(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Closes:  []string{sig.ID},
		Content: "decision closing signal",
	}

	runner := &mockRunner{response: `{"findings": [{"severity": "high", "category": "signal-target-miss", "observation": "signal not genuinely addressed"}]}`}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatalf("Preflight() error: %v", err)
	}
	if !result.HasBlocking() {
		t.Error("Preflight() expected blocking finding")
	}
	if len(result.Findings) != 1 {
		t.Fatalf("Preflight().Findings len = %d, want 1", len(result.Findings))
	}
	got := result.Findings[0]
	if got.Severity != query.SeverityHigh {
		t.Errorf("Finding severity = %q, want high", got.Severity)
	}
	if got.Category != "signal-target-miss" {
		t.Errorf("Finding category = %q, want signal-target-miss", got.Category)
	}
	if got.Observation != "signal not genuinely addressed" {
		t.Errorf("Finding observation = %q, want %q", got.Observation, "signal not genuinely addressed")
	}
}

func TestRunPreflight_NonBlockingFindings(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new observation",
	}

	runner := &mockRunner{response: `{"findings": [{"severity": "medium", "category": "plan-coverage-ambiguity", "observation": "could be clearer"}, {"severity": "low", "category": "opening-reference-dependent", "observation": "stylistic"}]}`}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatalf("Preflight() error: %v", err)
	}
	if result.HasBlocking() {
		t.Error("Preflight() should not block on medium/low findings")
	}
	if len(result.Findings) != 2 {
		t.Fatalf("Preflight().Findings len = %d, want 2", len(result.Findings))
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

func TestRunPreflight_ParticipantDrift_MatchedSkipped(t *testing.T) {
	prior := entry("20260410-120000-s-cpt-aaa", withContent("signal"))
	prior.Participants = []string{"Christopher", "Claude"}
	graph := model.NewGraph([]*model.Entry{prior})

	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Layer:        model.LayerConceptual,
		Content:      "new observation",
		Participants: []string{"Christopher"},
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range result.Findings {
		if fd.Category == "participant-drift" {
			t.Errorf("drift finding should not fire for an established name, got %+v", fd)
		}
	}
}

func TestRunPreflight_ParticipantDrift_UnknownNameFlagged(t *testing.T) {
	prior := entry("20260410-120000-s-cpt-aaa", withContent("signal"))
	prior.Participants = []string{"Christopher", "Claude"}
	graph := model.NewGraph([]*model.Entry{prior})

	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Layer:        model.LayerConceptual,
		Content:      "new observation",
		Participants: []string{"Chris"}, // near-miss typo
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("Findings len = %d, want 1; got %+v", len(result.Findings), result.Findings)
	}
	fd := result.Findings[0]
	if fd.Category != "participant-drift" || fd.Severity != query.SeverityMedium {
		t.Errorf("finding = %+v, want category=participant-drift severity=medium", fd)
	}
	// Observation must list the established names so the author can judge typo vs new voice.
	for _, name := range []string{"Christopher", "Claude"} {
		if !strings.Contains(fd.Observation, name) {
			t.Errorf("observation missing established name %q: %s", name, fd.Observation)
		}
	}
	if !strings.Contains(fd.Observation, `"Chris"`) {
		t.Errorf("observation should mention the drifting name %q: %s", "Chris", fd.Observation)
	}
}

func TestRunPreflight_ParticipantDrift_PerNameValidation(t *testing.T) {
	prior := entry("20260410-120000-s-cpt-aaa", withContent("signal"))
	prior.Participants = []string{"Christopher", "Claude"}
	graph := model.NewGraph([]*model.Entry{prior})

	proposed := &model.Entry{
		Type:  model.TypeSignal,
		Layer: model.LayerConceptual,
		// Mixed: one known, one unknown. Per-name (AC 10): exactly one
		// finding, targeting only the drifting name.
		Participants: []string{"Christopher", "Bob"},
		Content:      "new observation",
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	var drift []query.Finding
	for _, fd := range result.Findings {
		if fd.Category == "participant-drift" {
			drift = append(drift, fd)
		}
	}
	if len(drift) != 1 {
		t.Fatalf("expected exactly 1 drift finding (per-name), got %d: %+v", len(drift), drift)
	}
	if !strings.Contains(drift[0].Observation, `"Bob"`) {
		t.Errorf("drift finding should target Bob, got: %s", drift[0].Observation)
	}
	if strings.Contains(drift[0].Observation, `"Christopher"`) &&
		strings.Contains(drift[0].Observation, `participant "Christopher"`) {
		t.Errorf("drift finding should not target the known name: %s", drift[0].Observation)
	}
}

func TestRunPreflight_ParticipantDrift_EmptyGraphBootstrap(t *testing.T) {
	// Fresh graph: no established participants. The drift check cannot
	// make any judgment — no findings.
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Layer:        model.LayerConceptual,
		Content:      "first ever entry",
		Participants: []string{"Christopher"},
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(runner)
	result, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range result.Findings {
		if fd.Category == "participant-drift" {
			t.Errorf("drift finding should not fire on an empty graph, got %+v", fd)
		}
	}
}

func TestRunPreflight_CorrectCheckTypeSelection(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("signal"))
	dec := entry("20260410-130000-d-tac-bbb", withContent("decision"))
	graph := model.NewGraph([]*model.Entry{sig, dec})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{dec.ID},
		Content: "implemented everything",
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(runner)
	_, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(runner.lastPrompt, "close") {
		t.Error("Prompt should contain closing-action check language")
	}
	if !strings.Contains(runner.lastPrompt, "decision") {
		t.Error("Prompt should include the closed decision content")
	}
}
