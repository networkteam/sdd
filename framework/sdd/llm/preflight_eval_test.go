//go:build eval

// This file contains evaluation tests for pre-flight prompt template accuracy.
// Run manually when tuning templates:
//
//	go test -tags=eval -run TestPreflightEval ./sdd/llm/... -v

package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/resonance/framework/sdd/model"
)

// liveRunner implements Runner using the real claude CLI.
type liveRunner struct {
	model string
}

func (r *liveRunner) Run(ctx context.Context, prompt string) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", r.model)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("claude -p: %w", err)
	}
	return &RunResult{Text: string(out)}, nil
}

func TestPreflightEval_ClosingAction_MissingPlanItems(t *testing.T) {
	plan := &model.Entry{
		ID:      "20260410-120000-d-tac-pln",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindPlan,
		Content: "Implementation plan with 4 items.",
		Time:    time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Attachments: []string{
			"2026/04/10-120000-d-tac-pln/plan.md",
		},
	}

	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeAction,
		Layer:   model.LayerTactical,
		Closes:  []string{plan.ID},
		Content: "Implemented item 1 (database schema) and item 3 (API endpoints). Items 2 and 4 were not addressed.",
	}

	ct := selectCheckType(proposed, graph)
	pctx, err := assembleContext(proposed, graph, ct)
	if err != nil {
		t.Fatal(err)
	}
	pctx.PlanItems = `
### Attachment: plan.md

1. Create database schema for user accounts
2. Build authentication middleware
3. Implement API endpoints for CRUD operations
4. Write integration tests for all endpoints
`

	prompt, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	runResult, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := parsePreflightResult(runResult.Text)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", runResult.Text, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL, got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly identified gaps. Gaps reported: %v", result.Gaps)
	}
}

func TestPreflightEval_ClosingAction_FullCoverage(t *testing.T) {
	plan := &model.Entry{
		ID:      "20260410-120000-d-tac-pln",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindPlan,
		Content: "Implementation plan for user auth feature.",
		Time:    time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	}

	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:        model.TypeAction,
		Layer:       model.LayerTactical,
		Closes:      []string{plan.ID},
		Attachments: []string{"2026/04/10-130000-a-tac-xyz/implementation.md"},
		Content:     "Built the complete user authentication feature: added users table with email/password columns (bcrypt hashed), wrote Express middleware that validates JWT tokens on protected routes, created REST endpoints for all CRUD operations (create user via signup, read user profile, update user settings, delete user account), and added a full integration test suite covering happy paths and error cases for every endpoint.",
	}

	ct := selectCheckType(proposed, graph)
	pctx, err := assembleContext(proposed, graph, ct)
	if err != nil {
		t.Fatal(err)
	}
	pctx.PlanItems = `
### Attachment: plan.md

1. Create database schema for user accounts
2. Build authentication middleware
3. Implement API endpoints for CRUD operations
4. Write integration tests for all endpoints
`

	prompt, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	runResult, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := parsePreflightResult(runResult.Text)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", runResult.Text, err)
		return
	}

	if !result.Pass {
		t.Errorf("Expected PASS, got FAIL with gaps: %v\nRaw output:\n%s", result.Gaps, output)
	} else {
		t.Log("Correctly passed")
	}
}

func TestPreflightEval_SignalSmuggleDecision(t *testing.T) {
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Layer:      model.LayerTactical,
		Confidence: "high",
		Content:    "We must migrate the database to PostgreSQL by next sprint and deprecate the MongoDB adapter. The team should start immediately with the schema migration scripts.",
	}

	ct := selectCheckType(proposed, graph)
	pctx, err := assembleContext(proposed, graph, ct)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	runResult, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := parsePreflightResult(runResult.Text)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", runResult.Text, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL, got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly flagged. Gaps: %v", result.Gaps)
	}
}

func TestPreflightEval_ValidSignal(t *testing.T) {
	graph := model.NewGraph(nil)

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Layer:      model.LayerTactical,
		Confidence: "medium",
		Content:    "Three of the last five customer support tickets mention confusion about the billing page layout. The most common complaint is that the 'current plan' and 'upgrade options' sections look too similar, making it hard to tell which plan is currently active.",
	}

	ct := selectCheckType(proposed, graph)
	pctx, err := assembleContext(proposed, graph, ct)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	runResult, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := parsePreflightResult(runResult.Text)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", runResult.Text, err)
		return
	}

	if !result.Pass {
		t.Errorf("Expected PASS, got FAIL with gaps: %v\nRaw output:\n%s", result.Gaps, output)
	} else {
		t.Log("Correctly passed")
	}
}

func TestPreflightEval_RealGraphHistory_SilentScopeOut(t *testing.T) {
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
		Type:    model.TypeAction,
		Layer:   model.LayerTactical,
		Refs:    []string{decision.ID},
		Closes:  []string{decision.ID},
		Content: "Built sdd lint command with checks for dangling refs (non-existent entries), malformed IDs (short suffixes), type mismatches in closes (signal can't close, action can't be closed, decision can't close decision), and type mismatches in supersedes (must be same type). Warnings are populated during graph construction on the Entry struct so sdd show displays them inline. Running against the live graph found 4 issues in 3 entries. Does NOT yet cover broken or missing attachment references — that requirement from d-tac-kfo remains unimplemented.",
	}

	ct := selectCheckType(proposed, graph)
	pctx, err := assembleContext(proposed, graph, ct)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	runResult, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := parsePreflightResult(runResult.Text)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", runResult.Text, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL, got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly caught silent scope-out. Gaps: %v", result.Gaps)
	}
}

func TestPreflightEval_ContractViolation(t *testing.T) {
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

	ct := selectCheckType(proposed, graph)
	pctx, err := assembleContext(proposed, graph, ct)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	runResult, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := parsePreflightResult(runResult.Text)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", runResult.Text, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL, got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly flagged. Gaps: %v", result.Gaps)
	}
}
