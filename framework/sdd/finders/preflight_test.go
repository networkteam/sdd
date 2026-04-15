package finders

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/networkteam/resonance/framework/sdd/llm"
	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// mockRunner implements llm.Runner for testing.
type mockRunner struct {
	response string
	err      error
	// captured prompt for inspection
	lastPrompt string
}

func (m *mockRunner) Run(_ context.Context, prompt string) (*llm.RunResult, error) {
	m.lastPrompt = prompt
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

	runner := &mockRunner{response: "No findings."}
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

	runner := &mockRunner{response: "- [high] signal-target-miss: signal not genuinely addressed"}
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

	runner := &mockRunner{response: "- [medium] plan-coverage-ambiguity: could be clearer\n- [low] opening-reference-dependent: stylistic"}
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

func TestRunPreflight_CorrectCheckTypeSelection(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("signal"))
	dec := entry("20260410-130000-d-tac-bbb", withContent("decision"))
	graph := model.NewGraph([]*model.Entry{sig, dec})

	proposed := &model.Entry{
		Type:    model.TypeAction,
		Layer:   model.LayerTactical,
		Closes:  []string{dec.ID},
		Content: "implemented everything",
	}

	runner := &mockRunner{response: "No findings."}
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
