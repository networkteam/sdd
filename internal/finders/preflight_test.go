package finders

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/llm"
)

// mockRunner implements llm.Runner for testing.
type mockRunner struct {
	response string
	err      error
	// captured prompt for inspection (combined System + User).
	lastPrompt string
}

func (m *mockRunner) Run(_ context.Context, req llm.Request) (llm.Result, error) {
	m.lastPrompt = req.Combined()
	if m.err != nil {
		return llm.Result{}, m.err
	}
	return llm.Result{Text: m.response, Identity: llm.Identity{Provider: "test", Model: "test-model"}}, nil
}

// graphWithRefKindFact builds a test graph carrying the ref-kind vocabulary
// fact every real graph inherits from the base-fact merge — llm.Preflight
// fails loud without it.
func graphWithRefKindFact(entries ...*model.Entry) *model.Graph {
	fact := entry(basefacts.RefKindsFactID, withKind(model.KindFact), withContent("ref-kind vocabulary stub"))
	return model.NewGraph(append(entries, fact))
}

func TestRunPreflight_NoFindings(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("some signal"))
	graph := graphWithRefKindFact(sig)

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new observation",
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(Options{PreflightRunner: runner, Config: &model.PerRepoConfig{}})
	result, err := f.Preflight(context.Background(), graph, query.PreflightQuery{Entry: proposed})
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
	graph := graphWithRefKindFact(sig)

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Closes:  []string{sig.ID},
		Content: "decision closing signal",
	}

	runner := &mockRunner{response: `{"findings": [{"severity": "high", "category": "signal-target-miss", "observation": "signal not genuinely addressed"}]}`}
	f := New(Options{PreflightRunner: runner, Config: &model.PerRepoConfig{}})
	result, err := f.Preflight(context.Background(), graph, query.PreflightQuery{Entry: proposed})
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
	graph := graphWithRefKindFact(sig)

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new observation",
	}

	runner := &mockRunner{response: `{"findings": [{"severity": "medium", "category": "plan-coverage-ambiguity", "observation": "could be clearer"}, {"severity": "low", "category": "opening-reference-dependent", "observation": "stylistic"}]}`}
	f := New(Options{PreflightRunner: runner, Config: &model.PerRepoConfig{}})
	result, err := f.Preflight(context.Background(), graph, query.PreflightQuery{Entry: proposed})
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
	graph := graphWithRefKindFact()

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "some signal",
	}

	runner := &mockRunner{err: fmt.Errorf("claude CLI not found")}
	f := New(Options{PreflightRunner: runner, Config: &model.PerRepoConfig{}})
	_, err := f.Preflight(context.Background(), graph, query.PreflightQuery{Entry: proposed})
	if err == nil {
		t.Fatal("Preflight() expected error when runner fails")
	}
	if !strings.Contains(err.Error(), "running pre-flight validator") {
		t.Errorf("error should wrap runner failure, got: %v", err)
	}
}

func TestRunPreflight_ParseError(t *testing.T) {
	graph := graphWithRefKindFact()

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "some signal",
	}

	runner := &mockRunner{response: "I think this looks fine!"}
	f := New(Options{PreflightRunner: runner, Config: &model.PerRepoConfig{}})
	_, err := f.Preflight(context.Background(), graph, query.PreflightQuery{Entry: proposed})
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

	got := mechanicalPreflight(proposed, graph, nil, nil)
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

	got := mechanicalPreflight(proposed, graph, nil, nil)
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

	got := mechanicalPreflight(proposed, graph, nil, nil)
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

	got := mechanicalPreflight(proposed, graph, nil, nil)
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

func TestMechanical_FocusActorCoverage_ValidActorsPass(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:        model.TypeDecision,
		Kind:        model.KindFocus,
		Layer:       model.LayerTactical,
		Content:     "focus body",
		FocusActors: []string{"Christopher"},
		Involvement: []model.Involvement{
			{Target: actor.ID, Actors: []string{"Christopher"}},
		},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	for _, f := range got {
		if f.Category == "focus-actor-drift" {
			t.Errorf("valid focus actors should not draw a finding, got %+v", f)
		}
	}
}

func TestMechanical_FocusActorCoverage_UnknownFocusActorBlocks(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:        model.TypeDecision,
		Kind:        model.KindFocus,
		Layer:       model.LayerTactical,
		Content:     "focus body",
		FocusActors: []string{"Claude"},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "focus-actor-drift" && f.Severity == query.SeverityHigh {
			found = true
			if !strings.Contains(f.Observation, "actors[0]") || !strings.Contains(f.Observation, "Claude") {
				t.Errorf("finding should name actors[0] and the drifting name, got %q", f.Observation)
			}
		}
	}
	if !found {
		t.Errorf("expected focus-actor-drift finding for unknown focus-level actor, got %+v", got)
	}
}

func TestMechanical_FocusActorCoverage_UnknownInvolvementActorBlocks(t *testing.T) {
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:        model.TypeDecision,
		Kind:        model.KindFocus,
		Layer:       model.LayerTactical,
		Content:     "focus body",
		FocusActors: []string{"Christopher"},
		Involvement: []model.Involvement{
			{Target: actor.ID, Actors: []string{"Christopher"}},
			{Target: actor.ID, Actors: []string{"Christopher", "Claude"}},
		},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "focus-actor-drift" && f.Severity == query.SeverityHigh {
			found = true
			if !strings.Contains(f.Observation, "involvement[1].actors[1]") || !strings.Contains(f.Observation, "Claude") {
				t.Errorf("finding should name involvement[1].actors[1] and the drifting name, got %q", f.Observation)
			}
		}
	}
	if !found {
		t.Errorf("expected focus-actor-drift finding for unknown involvement actor, got %+v", got)
	}
}

func TestMechanical_FocusActorCoverage_GraceModeWhenNoActors(t *testing.T) {
	// No active actor signals — grace mode skips the check entirely.
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:        model.TypeDecision,
		Kind:        model.KindFocus,
		Layer:       model.LayerTactical,
		Content:     "focus body",
		FocusActors: []string{"Christopher"},
		Involvement: []model.Involvement{
			{Target: "20260410-120000-s-cpt-tgt", Actors: []string{"Claude"}},
		},
	}

	for _, f := range mechanicalPreflight(proposed, graph, nil, nil) {
		if f.Category == "focus-actor-drift" {
			t.Errorf("grace mode should skip focus actor check, got %+v", f)
		}
	}
}

func TestMechanical_FocusActorCoverage_NonFocusUnaffected(t *testing.T) {
	// A non-focus entry carrying FocusActors/Involvement (defensively) must
	// not draw the focus check — it is gated on kind: focus.
	actor := actorEntry("Christopher", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:        model.TypeDecision,
		Kind:        model.KindDirective,
		Layer:       model.LayerTactical,
		Content:     "directive body",
		FocusActors: []string{"Claude"},
	}

	for _, f := range mechanicalPreflight(proposed, graph, nil, nil) {
		if f.Category == "focus-actor-drift" {
			t.Errorf("non-focus entry must not draw focus-actor-drift, got %+v", f)
		}
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

	got := mechanicalPreflight(proposed, graph, nil, nil)
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

	got := mechanicalPreflight(proposed, graph, nil, nil)
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

	got := mechanicalPreflight(proposed, graph, nil, nil)
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
		Refs:    refsOf(actor.ID),
		Content: "contribution pattern",
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
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
		Refs:    refsOf(other.ID), // missing actor head
		Content: "contribution pattern",
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
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
		Refs:         refsOf(actor.ID),
		Participants: []string{"Christopher"},
		Content:      "contribution pattern",
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	for _, f := range got {
		if f.Severity == query.SeverityHigh {
			t.Errorf("unexpected high finding: %+v", f)
		}
	}
}

func TestMechanical_RefKindMissing_Blocks(t *testing.T) {
	target := entry("20260410-120000-s-cpt-tgt", withContent("target"))
	graph := model.NewGraph([]*model.Entry{target})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "decision body",
		Refs:    []model.Ref{{ID: target.ID}}, // kind omitted
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "ref-kind-missing" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ref-kind-missing finding, got %+v", got)
	}
}

func TestMechanical_RefKindUnknown_BlocksAtCapture(t *testing.T) {
	target := entry("20260410-120000-s-cpt-tgt", withContent("target"))
	graph := model.NewGraph([]*model.Entry{target})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "decision body",
		Refs:    []model.Ref{{ID: target.ID, Kind: model.RefKindUnknown}},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "ref-kind-invalid" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ref-kind-invalid finding for unknown kind, got %+v", got)
	}
}

func TestMechanical_RefKindInvalid_Blocks(t *testing.T) {
	target := entry("20260410-120000-s-cpt-tgt", withContent("target"))
	graph := model.NewGraph([]*model.Entry{target})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "decision body",
		Refs:    []model.Ref{{ID: target.ID, Kind: model.RefKind("bogus")}},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "ref-kind-invalid" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ref-kind-invalid finding for bogus kind, got %+v", got)
	}
}

func TestMechanical_RefKindValid_NoFindings(t *testing.T) {
	target := entry("20260410-120000-s-cpt-tgt", withContent("target"))
	graph := model.NewGraph([]*model.Entry{target})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "decision body",
		Refs:    []model.Ref{{ID: target.ID, Kind: model.RefKindAddresses, Desc: "addresses gap"}},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	for _, f := range got {
		if f.Category == "ref-kind-missing" || f.Category == "ref-kind-invalid" {
			t.Errorf("unexpected ref-kind finding on valid entry: %+v", f)
		}
	}
}

// --- ref-kind applicability matrix checks (plan d-tac-tph AC 4) ---

func TestMechanical_RefKindInapplicable_AddressesTerminalDone_Blocks(t *testing.T) {
	// The s-prc-2lm leak class, now deterministic: a terminal done cannot be
	// addressed. The finding must name admissible alternatives.
	done := entry("20260410-120000-s-ops-don", withContent("created the ticket"))
	done.Type = model.TypeSignal
	done.Kind = model.KindDone
	graph := model.NewGraph([]*model.Entry{done})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindFact,
		Layer:   model.LayerConceptual,
		Content: "fact body",
		Refs:    []model.Ref{{ID: done.ID, Kind: model.RefKindAddresses}},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "ref-kind-inapplicable" && f.Severity == query.SeverityHigh {
			found = true
			if !strings.Contains(f.Observation, "builds-on") || !strings.Contains(f.Observation, "grounded-in") {
				t.Errorf("finding should name admissible alternatives, got %q", f.Observation)
			}
		}
	}
	if !found {
		t.Errorf("expected ref-kind-inapplicable finding for addresses on terminal done, got %+v", got)
	}
}

func TestMechanical_RefKindInapplicable_RefinesClosedTarget_Blocks(t *testing.T) {
	target := entry("20260410-120000-d-tac-pln", withContent("plan body"))
	target.Type = model.TypeDecision
	target.Kind = model.KindPlan
	closer := entry("20260411-120000-s-tac-don", withContent("done"))
	closer.Type = model.TypeSignal
	closer.Kind = model.KindDone
	closer.Closes = []string{target.ID}
	graph := model.NewGraph([]*model.Entry{target, closer})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "directive body",
		Refs:    []model.Ref{{ID: target.ID, Kind: model.RefKindRefines}},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "ref-kind-inapplicable" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ref-kind-inapplicable finding for refines on closed target, got %+v", got)
	}
}

func TestMechanical_RefKindInapplicable_RefinesSuperseded_PointsAtHead(t *testing.T) {
	// A refines stranded on a superseded target usually means the author
	// wanted the live head — the finding should say so explicitly.
	old := entry("20260410-120000-d-tac-old", withContent("old plan"))
	old.Type = model.TypeDecision
	old.Kind = model.KindPlan
	head := entry("20260411-120000-d-tac-new", withContent("new plan"))
	head.Type = model.TypeDecision
	head.Kind = model.KindPlan
	head.Supersedes = []string{old.ID}
	graph := model.NewGraph([]*model.Entry{old, head})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "directive body",
		Refs:    []model.Ref{{ID: old.ID, Kind: model.RefKindRefines}},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "ref-kind-inapplicable" && f.Severity == query.SeverityHigh {
			found = true
			if !strings.Contains(f.Observation, head.ID) {
				t.Errorf("finding should point at the live head %s, got %q", head.ID, f.Observation)
			}
		}
	}
	if !found {
		t.Errorf("expected ref-kind-inapplicable finding for refines on superseded target, got %+v", got)
	}
}

func TestMechanical_RefKindApplicable_NoFindings(t *testing.T) {
	// The applicable cells the LLM kept escalating must never draw a
	// mechanical finding: builds-on / grounded-in on a terminal done (the
	// documented tie-break), addresses on an open gap (s-prc-l2d), and
	// builds-on on an active plan (soft cell, accepted live-graph usage).
	done := entry("20260410-120000-s-ops-don", withContent("recovery done"))
	done.Type = model.TypeSignal
	done.Kind = model.KindDone
	gap := entry("20260410-130000-s-cpt-gap", withContent("open gap"))
	gap.Type = model.TypeSignal
	gap.Kind = model.KindGap
	plan := entry("20260410-140000-d-tac-pln", withContent("active plan"))
	plan.Type = model.TypeDecision
	plan.Kind = model.KindPlan
	graph := model.NewGraph([]*model.Entry{done, gap, plan})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "directive body",
		Refs: []model.Ref{
			{ID: done.ID, Kind: model.RefKindBuildsOn},
			{ID: done.ID, Kind: model.RefKindGroundedIn},
			{ID: gap.ID, Kind: model.RefKindAddresses},
			{ID: plan.ID, Kind: model.RefKindBuildsOn},
		},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	for _, f := range got {
		if f.Category == "ref-kind-inapplicable" {
			t.Errorf("unexpected ref-kind-inapplicable finding on applicable kind: %+v", f)
		}
	}
}

func TestMechanical_RefKindApplicability_DanglingTargetSkipped(t *testing.T) {
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "directive body",
		Refs:    []model.Ref{{ID: "20260410-120000-s-cpt-xxx", Kind: model.RefKindRefines}},
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	for _, f := range got {
		if f.Category == "ref-kind-inapplicable" {
			t.Errorf("dangling target must be skipped (ref resolution reports it), got %+v", f)
		}
	}
}

// actorEntry is a test helper that builds a kind: actor signal.
//
//nolint:unparam // canonical is intentionally parameterized for future test cases
func TestMechanical_ProcedureWriteOnce_NewChainAllowed(t *testing.T) {
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:      model.TypeDecision,
		Kind:      model.KindProcedure,
		Layer:     model.LayerProcess,
		Canonical: "capture",
		Content:   "the capture move",
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected no findings for fresh procedure canonical, got %+v", got)
	}
}

func TestMechanical_ProcedureWriteOnce_ReuseBlocked(t *testing.T) {
	existing := procedureEntry("20260702-120000-d-prc-cap", "capture")
	graph := model.NewGraph([]*model.Entry{existing})

	proposed := &model.Entry{
		Type:      model.TypeDecision,
		Kind:      model.KindProcedure,
		Layer:     model.LayerProcess,
		Canonical: "capture",
		Content:   "a second, unrelated move claiming the same canonical",
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding for reused procedure canonical, got %+v", got)
	}
	if got[0].Category != "procedure-canonical-reused" || got[0].Severity != query.SeverityHigh {
		t.Errorf("finding = %+v, want high procedure-canonical-reused", got[0])
	}
}

func TestMechanical_ProcedureWriteOnce_SupersedeSameChainAllowed(t *testing.T) {
	existing := procedureEntry("20260702-120000-d-prc-cap", "capture")
	graph := model.NewGraph([]*model.Entry{existing})

	proposed := &model.Entry{
		Type:       model.TypeDecision,
		Kind:       model.KindProcedure,
		Layer:      model.LayerProcess,
		Canonical:  "capture",
		Supersedes: []string{existing.ID},
		Content:    "project override of the capture move",
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	for _, f := range got {
		if f.Category == "procedure-canonical-reused" {
			t.Errorf("supersede within the chain must not trip write-once, got %+v", f)
		}
	}
}

func TestMechanical_ProcedureCanonical_ActorNamespaceSeparate(t *testing.T) {
	// An actor already holds the canonical string — the procedure namespace
	// is separate, so a procedure may use it without a finding.
	actor := actorEntry("capture", nil)
	graph := model.NewGraph([]*model.Entry{actor})

	proposed := &model.Entry{
		Type:      model.TypeDecision,
		Kind:      model.KindProcedure,
		Layer:     model.LayerProcess,
		Canonical: "capture",
		Content:   "the capture move",
	}

	got := mechanicalPreflight(proposed, graph, nil, nil)
	for _, f := range got {
		if f.Category == "procedure-canonical-reused" {
			t.Errorf("actor canonical must not block procedure canonical, got %+v", f)
		}
	}
}

func TestMechanical_ServeBudgetFinding(t *testing.T) {
	specEntry := func(extra string) *model.Entry {
		content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: hugeproc\n" + extra +
			"state:\n    report: {type: text, desc: x}\n" +
			"steps:\n    - id: draft\n      collect: [report]\n      inject:\n" +
			"          - {fn: wide, maxBytes: 50000}\n" +
			"      transitions:\n          - when: hasBody\n            to: end(completed)\n" +
			"---\n\n## unit: draft\n\nGuidance.\n"
		e, err := model.ParseEntry("20260831-121000-d-prc-hug.md", content)
		if err != nil {
			t.Fatalf("fixture entry: %v", err)
		}
		return e
	}
	reg := engine.NewRegistry()
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "wide", Doc: "t"},
		Fn:  func(*engine.Context, map[string]any) (any, error) { return "", nil },
	}); err != nil {
		t.Fatal(err)
	}
	graph := model.NewGraph(nil)

	budgetFindings := func(entry *model.Entry, resolver engine.QueryResolver) []query.Finding {
		var out []query.Finding
		for _, f := range mechanicalPreflight(entry, graph, nil, resolver) {
			if f.Category == "serve-budget" {
				out = append(out, f)
			}
		}
		return out
	}

	got := budgetFindings(specEntry(""), reg)
	if len(got) != 1 || got[0].Severity != query.SeverityMedium {
		t.Fatalf("findings = %+v, want one medium serve-budget finding", got)
	}
	if !strings.Contains(got[0].Observation, "draft") || !strings.Contains(got[0].Observation, "serveBudget") {
		t.Errorf("observation should name the step and the silencer, got %q", got[0].Observation)
	}
	if got := budgetFindings(specEntry("serveBudget: 60000\n"), reg); len(got) != 0 {
		t.Errorf("declared serveBudget must silence the finding, got %+v", got)
	}
	if got := budgetFindings(specEntry(""), nil); len(got) != 0 {
		t.Errorf("nil resolver must skip the check, got %+v", got)
	}
}

func procedureEntry(id, canonical string) *model.Entry {
	e := entry(id, withContent("procedure: "+canonical))
	e.Type = model.TypeDecision
	e.Kind = model.KindProcedure
	e.Layer = model.LayerProcess
	e.Canonical = canonical
	return e
}

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
	graph := graphWithRefKindFact(sig, dec)

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{dec.ID},
		Content: "implemented everything",
	}

	runner := &mockRunner{response: `{"findings": []}`}
	f := New(Options{PreflightRunner: runner, Config: &model.PerRepoConfig{}})
	_, err := f.Preflight(context.Background(), graph, query.PreflightQuery{Entry: proposed})
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

func TestMechanical_SupersedeNonHead_Blocks(t *testing.T) {
	// target is already superseded by an active successor; superseding target
	// again would create a fork — the author should supersede the live head.
	target := &model.Entry{ID: "20260101-100000-d-tac-tgt", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective}
	succ := &model.Entry{ID: "20260101-110000-d-tac-suc", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective, Supersedes: []string{target.ID}}
	graph := model.NewGraph([]*model.Entry{target, succ})

	proposed := &model.Entry{
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Kind:       model.KindDirective,
		Supersedes: []string{target.ID},
		Content:    "second successor — would fork the chain",
	}
	got := mechanicalPreflight(proposed, graph, nil, nil)
	found := false
	for _, f := range got {
		if f.Category == "supersede-non-head" && f.Severity == query.SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Errorf("expected supersede-non-head high finding, got %+v", got)
	}
}

func TestMechanical_SupersedeLiveHead_Allowed(t *testing.T) {
	// target is a live head (nothing supersedes it) — superseding it is linear.
	target := &model.Entry{ID: "20260101-100000-d-tac-tgt", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective}
	graph := model.NewGraph([]*model.Entry{target})

	proposed := &model.Entry{
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Kind:       model.KindDirective,
		Supersedes: []string{target.ID},
		Content:    "linear successor",
	}
	for _, f := range mechanicalPreflight(proposed, graph, nil, nil) {
		if f.Category == "supersede-non-head" {
			t.Errorf("superseding a live head should not fork, got %+v", f)
		}
	}
}

func TestMechanical_SupersedeSettledBranch_Allowed(t *testing.T) {
	// target's only successor has since closed — the branch is settled, so
	// reviving it as the sole active successor is not a fork.
	target := &model.Entry{ID: "20260101-100000-d-tac-tgt", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective}
	succ := &model.Entry{ID: "20260101-110000-d-tac-suc", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective, Supersedes: []string{target.ID}}
	closer := &model.Entry{ID: "20260101-120000-s-tac-cls", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindDone, Closes: []string{succ.ID}}
	graph := model.NewGraph([]*model.Entry{target, succ, closer})

	proposed := &model.Entry{
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Kind:       model.KindDirective,
		Supersedes: []string{target.ID},
		Content:    "reviving a settled chain — single active successor, not a fork",
	}
	for _, f := range mechanicalPreflight(proposed, graph, nil, nil) {
		if f.Category == "supersede-non-head" {
			t.Errorf("superseding a target whose successor is closed should not fork, got %+v", f)
		}
	}
}

// Cross-repo capture preconditions: the declared-dependency rule
// (d-cpt-6cq) blocks any cross-repo ref into a repo this graph does not
// declare, and resolve-or-block (d-cpt-uh0) requires a backward-class ref
// into a declared repo to resolve in its cached graph; forward-class kinds
// are exempt from resolution only.

func TestMechanical_CrossRepoRef_UndeclaredDependencyBlocks(t *testing.T) {
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerTactical,
		Kind:    model.KindGap,
		Content: "observation grounded in a remote entry",
		Refs: []model.Ref{
			{ID: "github.com/networkteam/other:20260601-120000-s-tac-abc", Kind: model.RefKindGroundedIn},
			// Forward-class refs are exempt from resolution, but not from
			// the dependency rule — one-way holds for every ref direction.
			{ID: "github.com/networkteam/other:20260601-130000-d-tac-def", Kind: model.RefKindSurfaces},
		},
	}
	got := mechanicalPreflight(proposed, graph, nil, nil)
	undeclared := 0
	for _, f := range got {
		if f.Category == "cross-repo-dep-undeclared" && f.Severity == query.SeverityHigh {
			undeclared++
		}
		if f.Category == "cross-repo-ref-unresolved" {
			t.Errorf("undeclared repo must block on the dependency rule, not double-report resolution: %+v", f)
		}
	}
	if undeclared != 2 {
		t.Errorf("expected 2 cross-repo-dep-undeclared findings (backward and forward), got %+v", got)
	}
}

func TestMechanical_CrossRepoRef_DeclaredButUnconnectedBlocks(t *testing.T) {
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerTactical,
		Kind:    model.KindGap,
		Content: "observation grounded in a remote entry",
		Refs: []model.Ref{
			{ID: "github.com/networkteam/other:20260601-120000-s-tac-abc", Kind: model.RefKindGroundedIn},
		},
	}
	got := mechanicalPreflight(proposed, graph, []string{"github.com/networkteam/other"}, nil)
	found := false
	for _, f := range got {
		if f.Category == "cross-repo-ref-unresolved" && f.Severity == query.SeverityHigh {
			found = true
		}
		if f.Category == "cross-repo-dep-undeclared" {
			t.Errorf("declared dependency must not trigger the dependency finding: %+v", f)
		}
	}
	if !found {
		t.Errorf("expected cross-repo-ref-unresolved high finding, got %+v", got)
	}
}

func TestMechanical_CrossRepoRef_ForwardClassExemptFromResolution(t *testing.T) {
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerTactical,
		Kind:    model.KindGap,
		Content: "work that surfaced a remote entry",
		Refs: []model.Ref{
			{ID: "github.com/networkteam/other:20260601-120000-s-tac-abc", Kind: model.RefKindSurfaces},
			{ID: "github.com/networkteam/other:20260601-130000-d-tac-def", Kind: model.RefKindRequiredBy},
		},
	}
	for _, f := range mechanicalPreflight(proposed, graph, []string{"github.com/networkteam/other"}, nil) {
		if f.Category == "cross-repo-ref-unresolved" || f.Category == "cross-repo-dep-undeclared" {
			t.Errorf("declared forward-class cross-repo refs must pass, got %+v", f)
		}
	}
}

func TestMechanical_CrossRepoRef_ResolverOutcomes(t *testing.T) {
	entry := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerTactical,
		Kind:    model.KindGap,
		Content: "observation",
		Refs: []model.Ref{
			{ID: "github.com/networkteam/other:20260601-120000-s-tac-abc", Kind: model.RefKindGroundedIn},
		},
	}

	resolved := func(repoID, entryID string) crossRepoRefResolution {
		if repoID != "github.com/networkteam/other" || entryID != "20260601-120000-s-tac-abc" {
			t.Errorf("resolver got (%q, %q)", repoID, entryID)
		}
		return crossRepoEntryResolved
	}
	if got := crossRepoResolutionFindings(entry, resolved, []string{"github.com/networkteam/other"}); len(got) != 0 {
		t.Errorf("resolved target must produce no findings, got %+v", got)
	}

	missing := func(string, string) crossRepoRefResolution { return crossRepoEntryMissing }
	got := crossRepoResolutionFindings(entry, missing, []string{"github.com/networkteam/other"})
	if len(got) != 1 || got[0].Severity != query.SeverityHigh {
		t.Fatalf("missing entry must produce one high finding, got %+v", got)
	}
	if !strings.Contains(got[0].Observation, "absent from repo") {
		t.Errorf("missing-entry finding should name the absence, got %q", got[0].Observation)
	}

	unavailable := func(string, string) crossRepoRefResolution { return crossRepoRepoUnavailable }
	got = crossRepoResolutionFindings(entry, unavailable, []string{"github.com/networkteam/other"})
	if len(got) != 1 || got[0].Severity != query.SeverityHigh {
		t.Fatalf("unavailable repo must produce one high finding, got %+v", got)
	}
	if !strings.Contains(got[0].Observation, "not connected") {
		t.Errorf("unavailable-repo finding should name the unconnected repo, got %q", got[0].Observation)
	}
}
