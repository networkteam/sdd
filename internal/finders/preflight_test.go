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
	f := New(Options{PreflightRunner: runner})
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
	f := New(Options{PreflightRunner: runner})
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
	f := New(Options{PreflightRunner: runner})
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
	f := New(Options{PreflightRunner: runner})
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
	f := New(Options{PreflightRunner: runner})
	_, err := f.Preflight(context.Background(), query.PreflightQuery{Entry: proposed, Graph: graph})
	if err == nil {
		t.Fatal("Preflight() expected error when response is unparseable")
	}
	if !strings.Contains(err.Error(), "parsing pre-flight result") {
		t.Errorf("error should wrap parse failure, got: %v", err)
	}
}

// Mechanical pre-flight checks per plan d-cpt-d34 replace the retired
// LLM-judged participant-drift rubric. The tests below cover participant
// coverage (AC 6), actor write-once invariant (AC 5), and the role
// mechanical check (AC 7). LLM-side rubrics are tested in internal/llm.

func TestMechanical_ParticipantCoverage_ActiveActorMatches(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Layer:        model.LayerConceptual,
		Content:      "new observation",
		Participants: []string{"Christopher"},
	}

	got := mechanicalPreflight(proposed, graph)
	if len(got) != 0 {
		t.Fatalf("expected no findings for matching canonical, got %+v", got)
	}
}

func TestMechanical_ParticipantCoverage_UnknownCanonicalBlocks(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Layer:        model.LayerConceptual,
		Content:      "new observation",
		Participants: []string{"Claude"},
	}

	got := mechanicalPreflight(proposed, graph)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding for unknown canonical, got %d", len(got))
	}
	if got[0].Severity != query.SeverityHigh {
		t.Errorf("severity = %q, want high", got[0].Severity)
	}
	if got[0].Category != "participant-drift" {
		t.Errorf("category = %q, want participant-drift", got[0].Category)
	}
}

func TestMechanical_ParticipantCoverage_GraceModeWhenNoActors(t *testing.T) {
	// Graph has no actor signals — grace mode skips the check entirely.
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Layer:        model.LayerConceptual,
		Content:      "first entry",
		Participants: []string{"Christopher"},
	}

	got := mechanicalPreflight(proposed, graph)
	for _, f := range got {
		if f.Category == "participant-drift" {
			t.Errorf("grace mode should skip participant check, got %+v", f)
		}
	}
}

func TestMechanical_ParticipantCoverage_AliasDoesNotMatch(t *testing.T) {
	// Aliases are read-side convenience only — never resolve to canonical
	// at capture time.
	actor := actorEntry("Christopher", []string{"Chris", "CH"})
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Layer:        model.LayerConceptual,
		Content:      "observation",
		Participants: []string{"Chris"},
	}

	got := mechanicalPreflight(proposed, graph)
	found := false
	for _, f := range got {
		if f.Category == "participant-drift" {
			found = true
		}
	}
	if !found {
		t.Error("expected participant-drift finding for alias used in participants")
	}
}

func TestMechanical_ActorWriteOnce_NewChainAllowed(t *testing.T) {
	// First actor for canonical "Christopher" — nothing existing to collide.
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:      model.TypeSignal,
		Kind:      model.KindActor,
		Layer:     model.LayerProcess,
		Canonical: "Christopher",
		Content:   "joining the project",
	}

	got := mechanicalPreflight(proposed, graph)
	for _, f := range got {
		if f.Category == "actor-canonical-reused" {
			t.Errorf("new chain should not trigger reuse finding, got %+v", f)
		}
	}
}

func TestMechanical_ActorWriteOnce_ExtendingSameChainAllowed(t *testing.T) {
	// A supersession within the same chain must not trip the write-once check,
	// even when the canonical is unchanged.
	existing := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{existing})

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Kind:       model.KindActor,
		Layer:      model.LayerProcess,
		Canonical:  "Christopher", // same canonical
		Supersedes: []string{existing.ID},
		Content:    "typo correction in aliases",
	}

	got := mechanicalPreflight(proposed, graph)
	for _, f := range got {
		if f.Category == "actor-canonical-reused" {
			t.Errorf("within-chain reuse should be allowed, got %+v", f)
		}
	}
}

func TestMechanical_ActorWriteOnce_CrossChainReuseBlocks(t *testing.T) {
	// A different chain already carries "Christopher". A new chain cannot
	// reuse the canonical.
	existing := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{existing})

	proposed := &model.Entry{
		Type:      model.TypeSignal,
		Kind:      model.KindActor,
		Layer:     model.LayerProcess,
		Canonical: "Christopher",
		// No supersedes — starts a new chain.
		Content: "a different person who happens to share the name",
	}

	got := mechanicalPreflight(proposed, graph)
	found := false
	for _, f := range got {
		if f.Category == "actor-canonical-reused" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Error("expected actor-canonical-reused finding for cross-chain reuse")
	}
}

func TestMechanical_RoleCanonicalMismatch_Blocks(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindRole,
		Layer:   model.LayerProcess,
		Actor:   "Claude", // no chain carries this canonical
		Refs:    []string{actor.ID},
		Content: "contribution pattern",
	}

	got := mechanicalPreflight(proposed, graph)
	found := false
	for _, f := range got {
		if f.Category == "role-canonical-mismatch" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Errorf("expected role-canonical-mismatch finding, got %+v", got)
	}
}

func TestMechanical_RoleRefsMissingHead_Blocks(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	other := entry("20260410-130000-s-cpt-oth", withContent("unrelated"))
	graph := model.NewGraph([]*model.Entry{actor, other})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindRole,
		Layer:   model.LayerProcess,
		Actor:   "Christopher",
		Refs:    []string{other.ID}, // missing actor head
		Content: "contribution pattern",
	}

	got := mechanicalPreflight(proposed, graph)
	found := false
	for _, f := range got {
		if f.Category == "role-refs-missing-head" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Errorf("expected role-refs-missing-head finding, got %+v", got)
	}
}

func TestMechanical_RoleValid_NoFindings(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:         model.TypeDecision,
		Kind:         model.KindRole,
		Layer:        model.LayerProcess,
		Actor:        "Christopher",
		Refs:         []string{actor.ID},
		Participants: []string{"Christopher"},
		Content:      "contribution pattern",
	}

	got := mechanicalPreflight(proposed, graph)
	for _, f := range got {
		if f.Severity == query.SeverityHigh {
			t.Errorf("unexpected high finding: %+v", f)
		}
	}
}

// actorEntry is a test helper that builds a kind: actor signal.
//
//nolint:unparam // canonical is intentionally parameterized for future test cases
func actorEntry(canonical string, aliases []string) *model.Entry {
	const defaultActorID = "20260410-120000-s-prc-act"
	e := entry(defaultActorID, withContent("actor: "+canonical))
	e.Type = model.TypeSignal
	e.Kind = model.KindActor
	e.Layer = model.LayerProcess
	e.Canonical = canonical
	e.Aliases = aliases
	return e
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
	f := New(Options{PreflightRunner: runner})
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
