//go:build eval

// This file contains evaluation tests for pre-flight prompt template accuracy.
// These are NOT deterministic correctness tests — they measure how well the LLM
// produces expected pass/fail results for known scenarios.
//
// Run manually when tuning templates:
//
//	go test -tags=eval -run TestPreflightEval ./sdd/... -v
//
// Failures mean "template may need tuning" or "LLM was non-deterministic, re-run to confirm".
// Uses t.Errorf (not t.Fatal) so all cases run even if some fail.

package sdd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// liveRunner implements PreflightRunner using the real claude CLI.
type liveRunner struct {
	model string
}

func (r *liveRunner) Run(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", r.model)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude -p: %w", err)
	}
	return string(out), nil
}

func TestPreflightEval_ClosingAction_MissingPlanItems(t *testing.T) {
	// True positive: action claims to close a 4-item plan but only covers 2 items.
	// Expected: FAIL with missing items listed.

	plan := &Entry{
		ID:      "20260410-120000-d-tac-pln",
		Type:    TypeDecision,
		Layer:   LayerTactical,
		Kind:    KindPlan,
		Content: "Implementation plan with 4 items.",
		Time:    time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Attachments: []string{
			"2026/04/10-120000-d-tac-pln/plan.md",
		},
	}

	graph := NewGraph([]*Entry{plan})

	proposed := &Entry{
		Type:    TypeAction,
		Layer:   LayerTactical,
		Closes:  []string{plan.ID},
		Content: "Implemented item 1 (database schema) and item 3 (API endpoints). Items 2 and 4 were not addressed.",
	}

	// We can't read the plan attachment from disk in this test, so inject it directly
	checkType := SelectCheckType(proposed, graph)
	pctx, err := AssembleContext(proposed, graph, checkType)
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

	prompt, err := RenderPrompt(checkType, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	output, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := ParseResult(output)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", output, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL (action missing 2 of 4 plan items), got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly identified gaps. Gaps reported: %v", result.Gaps)
	}
}

func TestPreflightEval_ClosingAction_FullCoverage(t *testing.T) {
	// True negative: action legitimately covers all plan items, even with different wording.
	// Expected: PASS.

	plan := &Entry{
		ID:      "20260410-120000-d-tac-pln",
		Type:    TypeDecision,
		Layer:   LayerTactical,
		Kind:    KindPlan,
		Content: "Implementation plan for user auth feature.",
		Time:    time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	}

	graph := NewGraph([]*Entry{plan})

	proposed := &Entry{
		Type:    TypeAction,
		Layer:   LayerTactical,
		Closes:  []string{plan.ID},
		Content: "Built the complete user authentication feature: added users table with email/password columns (bcrypt hashed), wrote Express middleware that validates JWT tokens on protected routes, created REST endpoints for all CRUD operations (create user via signup, read user profile, update user settings, delete user account), and added a full integration test suite covering happy paths and error cases for every endpoint.",
	}

	checkType := SelectCheckType(proposed, graph)
	pctx, err := AssembleContext(proposed, graph, checkType)
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

	prompt, err := RenderPrompt(checkType, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	output, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := ParseResult(output)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", output, err)
		return
	}

	if !result.Pass {
		t.Errorf("Expected PASS (all plan items covered with different wording), got FAIL with gaps: %v\nRaw output:\n%s", result.Gaps, output)
	} else {
		t.Log("Correctly passed — all items covered despite different wording")
	}
}

func TestPreflightEval_SignalSmuggleDecision(t *testing.T) {
	// True positive: something labeled as a signal actually prescribes action.
	// Expected: FAIL.

	graph := NewGraph(nil)

	proposed := &Entry{
		Type:       TypeSignal,
		Layer:      LayerTactical,
		Confidence: "high",
		Content:    "We must migrate the database to PostgreSQL by next sprint and deprecate the MongoDB adapter. The team should start immediately with the schema migration scripts.",
	}

	checkType := SelectCheckType(proposed, graph)
	pctx, err := AssembleContext(proposed, graph, checkType)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := RenderPrompt(checkType, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	output, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := ParseResult(output)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", output, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL (signal contains prescriptive action, should be a decision), got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly flagged smuggled decision. Gaps: %v", result.Gaps)
	}
}

func TestPreflightEval_ValidSignal(t *testing.T) {
	// True negative: a well-formed signal that is genuinely an observation.
	// Expected: PASS.

	graph := NewGraph(nil)

	proposed := &Entry{
		Type:       TypeSignal,
		Layer:      LayerConceptual,
		Confidence: "medium",
		Content:    "Three of the last five customer support tickets mention confusion about the billing page layout. The most common complaint is that the 'current plan' and 'upgrade options' sections look too similar, making it hard to tell which plan is currently active.",
	}

	checkType := SelectCheckType(proposed, graph)
	pctx, err := AssembleContext(proposed, graph, checkType)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := RenderPrompt(checkType, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	output, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := ParseResult(output)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", output, err)
		return
	}

	if !result.Pass {
		t.Errorf("Expected PASS (valid observation signal), got FAIL with gaps: %v\nRaw output:\n%s", result.Gaps, output)
	} else {
		t.Log("Correctly passed valid signal")
	}
}

func TestPreflightEval_RealGraphHistory_SilentScopeOut(t *testing.T) {
	// True positive based on real graph history: action a-tac-tsd claimed to close
	// decision d-tac-kfo but silently dropped the attachment validation requirement.
	// The decision listed 4 checks including "broken or missing attachment references"
	// but the action only implemented 3 and noted the omission at the end.
	// Expected: FAIL — the action should not close the decision with incomplete coverage.

	decision := &Entry{
		ID:      "20260410-122858-d-tac-kfo",
		Type:    TypeDecision,
		Layer:   LayerTactical,
		Kind:    KindDirective,
		Content: "Build a sdd lint command for graph integrity checks. Checks: dangling refs (pointing at non-existent entries), short/malformed IDs in refs/closes/supersedes, type mismatches (e.g. closes pointing at an action), broken or missing attachment references. LoadGraph collects validation errors per entry as a custom structured error type on the Entry struct. sdd lint formats the full report. sdd show displays warnings per entry (including entries in the ref chain). Structured errors enable good formatting across contexts.",
		Time:    time.Date(2026, 4, 10, 12, 28, 58, 0, time.UTC),
	}

	graph := NewGraph([]*Entry{decision})

	proposed := &Entry{
		Type:    TypeAction,
		Layer:   LayerTactical,
		Refs:    []string{decision.ID},
		Closes:  []string{decision.ID},
		Content: "Built sdd lint command with checks for dangling refs (non-existent entries), malformed IDs (short suffixes), type mismatches in closes (signal can't close, action can't be closed, decision can't close decision), and type mismatches in supersedes (must be same type). Warnings are populated during graph construction on the Entry struct so sdd show displays them inline. Running against the live graph found 4 issues in 3 entries. Does NOT yet cover broken or missing attachment references — that requirement from d-tac-kfo remains unimplemented.",
	}

	checkType := SelectCheckType(proposed, graph)
	pctx, err := AssembleContext(proposed, graph, checkType)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := RenderPrompt(checkType, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	output, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := ParseResult(output)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", output, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL (action admits missing attachment validation requirement), got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly caught silent scope-out. Gaps: %v", result.Gaps)
	}
}

func TestPreflightEval_ContractViolation(t *testing.T) {
	// True positive: entry violates an active contract.
	// Expected: FAIL with contract violation noted.

	contract := &Entry{
		ID:      "20260408-120000-d-prc-ccc",
		Type:    TypeDecision,
		Layer:   LayerProcess,
		Kind:    KindContract,
		Content: "All decisions at the tactical layer or below must include refs to the signals or decisions that motivated them. No decision may be created without explicit grounding.",
		Time:    time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
	}

	graph := NewGraph([]*Entry{contract})

	proposed := &Entry{
		Type:    TypeDecision,
		Layer:   LayerTactical,
		Content: "Switch the logging framework from log4j to slog for better structured logging support.",
	}

	checkType := SelectCheckType(proposed, graph)
	pctx, err := AssembleContext(proposed, graph, checkType)
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := RenderPrompt(checkType, pctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := &liveRunner{model: "claude-haiku-4-5-20251001"}
	output, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Runner error: %v", err)
	}

	result, err := ParseResult(output)
	if err != nil {
		t.Errorf("Parse error (raw output: %q): %v", output, err)
		return
	}

	if result.Pass {
		t.Errorf("Expected FAIL (decision has no refs, violating contract), got PASS. Raw output:\n%s", output)
	} else {
		t.Logf("Correctly flagged contract violation. Gaps: %v", result.Gaps)
	}
}
