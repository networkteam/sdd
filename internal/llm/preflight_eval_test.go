//go:build eval

// This file contains evaluation tests for pre-flight prompt template accuracy.
// Run manually when tuning templates (costs real claude API calls):
//
//	go test -tags=eval -run TestPreflightEval ./sdd/llm/... -v
//
// Expectations match the severity-scored output format: HasBlocking() == true
// means at least one `high` finding was reported (the blocking threshold).

package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// liveRunner implements Runner using the real claude CLI.
type liveRunner struct {
	model string
}

func (r *liveRunner) Run(ctx context.Context, req Request) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", r.model)
	cmd.Stdin = strings.NewReader(req.Combined())
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("claude -p: %w", err)
	}
	return &RunResult{Text: string(out)}, nil
}

// runEval runs pre-flight against the proposed entry and returns the parsed
// result plus raw output for logging on failure.
func runEval(t *testing.T, graph *model.Graph, proposed *model.Entry) (*PreflightResult, string) {
	t.Helper()
	ct := selectCheckType(proposed, graph)
	pctx := assembleContext(proposed, graph, ct)
	req, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	runResult, err := runner.Run(ctx, req)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := parsePreflightResult(runResult.Text)
	if err != nil {
		t.Fatalf("Parse error (raw output: %q): %v", runResult.Text, err)
	}
	return result, runResult.Text
}

// planWithACs returns a plan decision whose description embeds an AC section.
// Matches the new design: plan items live in the description, not an attachment.
func planWithACs(id string, body string, acItems ...string) *model.Entry {
	var sb strings.Builder
	sb.WriteString(body)
	sb.WriteString("\n\n## Acceptance criteria\n")
	for _, item := range acItems {
		fmt.Fprintf(&sb, "- [ ] %s\n", item)
	}
	return &model.Entry{
		ID:      id,
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindPlan,
		Content: sb.String(),
		Time:    time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	}
}

func TestPreflightEval_ClosingAction_SilentlyOmittedACs(t *testing.T) {
	// Action silently omits two of four AC items. Expected: high.
	plan := planWithACs("20260410-120000-d-tac-pln",
		"Implementation plan with four items.",
		"Create database schema for user accounts",
		"Build authentication middleware",
		"Implement API endpoints for CRUD operations",
		"Write integration tests for all endpoints",
	)
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{plan.ID},
		Content: "Created the users table with email (unique) and bcrypt-hashed password columns via a new migration. Wired up a /users/:id GET endpoint returning JSON.",
	}

	result, raw := runEval(t, graph, proposed)
	if !result.HasBlocking() {
		t.Errorf("Expected at least one high finding (silent AC omission), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly identified silent AC omission. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ClosingAction_NamedButNoReasoning(t *testing.T) {
	// Action names the omitted items explicitly but offers NO reasoning.
	// Per the clarified calibration: uncovered-without-reasoning is high,
	// whether silent or explicit. Reasoning presence is what gates.
	plan := planWithACs("20260410-120000-d-tac-pln",
		"Implementation plan with four items.",
		"Create database schema for user accounts",
		"Build authentication middleware",
		"Implement API endpoints for CRUD operations",
		"Write integration tests for all endpoints",
	)
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{plan.ID},
		Content: "Implemented item 1 (database schema) and item 3 (API endpoints). Items 2 and 4 were not addressed.",
	}

	result, raw := runEval(t, graph, proposed)
	if !result.HasBlocking() {
		t.Errorf("Expected at least one high finding (named but no reasoning), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged as high. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ClosingAction_DeviationWithReasoning(t *testing.T) {
	// Action omits items but supplies brief reasoning for each. Per the
	// clarified calibration: reasoning presence (not quality) is what
	// matters — expected: no high finding.
	plan := planWithACs("20260410-120000-d-tac-pln",
		"Implementation plan with four items.",
		"Create database schema for user accounts",
		"Build authentication middleware",
		"Implement API endpoints for CRUD operations",
		"Write integration tests for all endpoints",
	)
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{plan.ID},
		Content: "Implemented item 1 (database schema with users table and bcrypt passwords) and item 3 (full CRUD endpoints at /users). Deviation: authentication middleware (item 2) deferred — dialogued that we'd adopt an existing Passport.js library in a follow-up rather than build from scratch. Deviation: integration tests (item 4) deferred to a follow-up action — agreed during implementation that the schema/endpoint work needed smoke testing first, with the full suite as a separate closure.",
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high finding (reasoning is present for each deviation), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly treated deviation-with-reasoning as non-blocking. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ClosingAction_FullCoverage(t *testing.T) {
	// Action covers every AC item with specific evidence. Expected: no high.
	plan := planWithACs("20260410-120000-d-tac-pln",
		"Implementation plan for user auth feature.",
		"Create database schema for user accounts",
		"Build authentication middleware",
		"Implement API endpoints for CRUD operations",
		"Write integration tests for all endpoints",
	)
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:        model.TypeSignal,
		Kind:        model.KindDone,
		Layer:       model.LayerTactical,
		Closes:      []string{plan.ID},
		Attachments: []string{"2026/04/10-130000-s-tac-xyz/implementation.md"},
		Content:     "Built the complete user authentication feature: added users table with email/password columns (bcrypt hashed), wrote Express middleware that validates JWT tokens on protected routes, created REST endpoints for all CRUD operations (create user via signup, read user profile, update user settings, delete user account), and added a full integration test suite covering happy paths and error cases for every endpoint.",
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high findings, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly passed. Non-blocking findings: %+v", result.Findings)
	}
}

func TestPreflightEval_SignalSmuggleDecision(t *testing.T) {
	// Signal reads as a committed decision with imperative + timeline +
	// ownership and no observational content. Expected: high finding.
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Layer:      model.LayerTactical,
		Confidence: "high",
		Content:    "We must migrate the database to PostgreSQL by next sprint and deprecate the MongoDB adapter. The team should start immediately with the schema migration scripts.",
	}

	result, raw := runEval(t, graph, proposed)
	if !result.HasBlocking() {
		t.Errorf("Expected at least one high finding (committed decision framed as signal), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ValidSignal(t *testing.T) {
	// Observational signal with evidence and specific framing. Expected: no high.
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Layer:      model.LayerTactical,
		Confidence: "medium",
		Content:    "Three of the last five customer support tickets mention confusion about the billing page layout. The most common complaint is that the 'current plan' and 'upgrade options' sections look too similar, making it hard to tell which plan is currently active.",
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high findings, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly passed. Non-blocking findings: %+v", result.Findings)
	}
}

func TestPreflightEval_RealGraphHistory_SilentScopeOut(t *testing.T) {
	// Action silently omits a requirement from the decision it claims to close,
	// though in this variant the action DOES acknowledge the omission ("Does
	// NOT yet cover broken or missing attachment references"). Expected per
	// new calibration: explicit acknowledgment is no finding or low; only a
	// silent omission is high. This case walks the boundary — the
	// acknowledgment counts as a deviation note, so no high finding expected.
	decision := &model.Entry{
		ID:      "20260410-122858-d-tac-kfo",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Content: "Build a sdd lint command for graph integrity checks. Checks: dangling refs (pointing at non-existent entries), short/malformed IDs in refs/closes/supersedes, type mismatches (e.g. closes pointing at an action), broken or missing attachment references. LoadGraph collects validation errors per entry as a custom structured error type on the Entry struct. sdd lint formats the full report. sdd show displays warnings per entry (including entries in the ref chain). Structured errors enable good formatting across contexts.",
		Time:    time.Date(2026, 4, 10, 12, 28, 58, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{decision})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Refs:    []string{decision.ID},
		Closes:  []string{decision.ID},
		Content: "Built sdd lint command with checks for dangling refs (non-existent entries), malformed IDs (short suffixes), type mismatches in closes (signal can't close, action can't be closed, decision can't close decision), and type mismatches in supersedes (must be same type). Warnings are populated during graph construction on the Entry struct so sdd show displays them inline. Running against the live graph found 4 issues in 3 entries. Does NOT yet cover broken or missing attachment references — that requirement from d-tac-kfo remains unimplemented.",
	}

	result, raw := runEval(t, graph, proposed)
	// Explicit deviation acknowledgment — per new calibration, this is no finding.
	if result.HasBlocking() {
		t.Errorf("Expected no high findings (deviation explicitly acknowledged), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly treated acknowledged deviation as non-blocking. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ActionClosesSignal_NoDurableArtifact(t *testing.T) {
	// Action closes a signal claiming work was done but references no durable
	// artifact — no commit, no attachment, no upstream attachment. This is the
	// regression case: the durability check was missing from
	// action_closes_signals.tmpl. Expected: high.
	signal := &model.Entry{
		ID:         "20260416-120000-s-prc-aaa",
		Type:       model.TypeSignal,
		Layer:      model.LayerProcess,
		Confidence: "high",
		Content:    "Catch-up should treat WIP markers as informational context, not continuation suggestions.",
		Time:       time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{signal})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerProcess,
		Closes:  []string{signal.ID},
		Content: "Updated the catch-up playbook and catch-up sub-skill to treat WIP markers as informational context. Fresh sessions no longer suggest picking up WIP work.",
	}

	result, raw := runEval(t, graph, proposed)
	if !result.HasBlocking() {
		t.Errorf("Expected at least one high finding (no durable artifact referenced), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged missing durability. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ActionClosesSignal_WithCommitRef(t *testing.T) {
	// Same action but references a commit. Expected: no high.
	signal := &model.Entry{
		ID:         "20260416-120000-s-prc-aaa",
		Type:       model.TypeSignal,
		Layer:      model.LayerProcess,
		Confidence: "high",
		Content:    "Catch-up should treat WIP markers as informational context, not continuation suggestions.",
		Time:       time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{signal})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerProcess,
		Closes:  []string{signal.ID},
		Content: "Updated the catch-up playbook and catch-up sub-skill to treat WIP markers as informational context. Fresh sessions no longer suggest picking up WIP work. Commit adebd7e.",
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high findings (commit reference provides durability), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly passed with commit reference. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ContractViolation(t *testing.T) {
	// Decision at tactical layer with no refs, while an active contract
	// requires refs on all tactical-or-below decisions. Per the new
	// calibration, a clear contract violation should still be high.
	contract := &model.Entry{
		ID:      "20260408-120000-d-prc-ccc",
		Type:    model.TypeDecision,
		Layer:   model.LayerProcess,
		Kind:    model.KindContract,
		Content: "All decisions at the tactical layer or below must include refs to the signals or decisions that motivated them. No decision may be created without explicit grounding.",
		Time:    time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{contract})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Content: "Switch the logging framework from log4j to slog for better structured logging support.",
	}

	result, raw := runEval(t, graph, proposed)
	if !result.HasBlocking() {
		t.Errorf("Expected at least one high finding (contract violation), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged. Findings: %+v", result.Findings)
	}
}
