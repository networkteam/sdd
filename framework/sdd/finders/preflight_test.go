package finders

import (
	"context"
	"fmt"
	"strings"
	"testing"

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

func (m *mockRunner) Run(_ context.Context, prompt string) (string, error) {
	m.lastPrompt = prompt
	return m.response, m.err
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

	if !strings.Contains(runner.lastPrompt, "close") {
		t.Error("Prompt should contain closing-action check language")
	}
	if !strings.Contains(runner.lastPrompt, "decision") {
		t.Error("Prompt should include the closed decision content")
	}
}
