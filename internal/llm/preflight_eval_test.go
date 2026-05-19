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
	// English default — the language-drift rubric only fires for non-empty locales.
	pctx := assembleContext(proposed, graph, ct, "")
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
		Refs:    []model.Ref{{ID: decision.ID, Kind: model.RefKindAddresses}},
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

func TestPreflightEval_AugmentingDirective_CleanRefinement(t *testing.T) {
	// Positive case: a tactical directive refs an active plan and refines
	// one of its acceptance criteria. No supersede, no close, no replacement
	// of a core commitment — just sharpening. The augment-plan pattern
	// (per d-prc-9ti) covers this exactly. Expected: no high finding tied to
	// the augment-plan pattern (no demand for a backward ref on the plan, no
	// "supersede the AC" remedy, no "scope smuggling", no "dangling commitment"
	// on the lifecycle).
	plan := planWithACs("20260415-100000-d-tac-pln",
		"Migrate the user-search index from postgres trigrams to elasticsearch for better fuzzy matching.",
		"Index ingestion runs nightly via a cron job",
		"Search latency at p95 stays under 200ms for indexed corpora",
		"Stale-index warnings surface in the operator dashboard",
	)
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Refs:    []model.Ref{{ID: plan.ID, Kind: model.RefKindBuildsOn}},
		Content: "The 200ms p95 latency target in d-tac-pln applies to query corpora up to 50k documents — beyond that we accept up to 350ms in the first iteration. The plan's AC stands for the bulk of expected production traffic; this directive sharpens the boundary so the closing done signal can address both regimes explicitly. Plan stays active; this directive is closed by the plan's done signal alongside the plan.",
		Time:    time.Date(2026, 4, 15, 14, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high findings (clean augmenting directive — refines an AC without superseding the plan), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly accepted augmenting directive. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_AugmentingDirective_GenuineSupersessionFlagged(t *testing.T) {
	// Negative case: a directive refs an active plan but rather than refining
	// an AC, it asserts that one of the plan's commitments is wrong and should
	// not be carried forward. This is replacement-shaped — it belongs in a
	// supersedes operation on the plan, not an augmentation. The augment-aware
	// template explicitly distinguishes refinement from replacement; this case
	// must still be flagged.
	//
	// Expected: at least one finding mentioning supersede / replacement /
	// AC drop. Severity may be `medium` (the directive is structurally valid;
	// supersession is the better-fit shape) or `high` — both are correct
	// outcomes. The regression we guard against is *no finding at all* —
	// silently accepting a replacement-shaped directive as augmentation.
	plan := planWithACs("20260415-100000-d-tac-pln",
		"Migrate the user-search index from postgres trigrams to elasticsearch for better fuzzy matching.",
		"Index ingestion runs nightly via a cron job",
		"Search latency at p95 stays under 200ms for indexed corpora",
		"Stale-index warnings surface in the operator dashboard",
	)
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Refs:    []model.Ref{{ID: plan.ID, Kind: model.RefKindBuildsOn}},
		Content: "The nightly cron-job AC in d-tac-pln is wrong. We will not run nightly ingestion at all — instead, ingestion will be event-driven from the source-of-truth write stream, with no scheduled runs. The dashboard stale-index warning AC also no longer applies because there is no scheduled cadence to fall behind. The plan's overall direction (move to elasticsearch) holds; the operational shape changes substantially.",
		Time:    time.Date(2026, 4, 15, 14, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if !mentionsSupersession(result.Findings) {
		t.Errorf("Expected at least one finding mentioning supersession / replacement / AC drop (directive replaces a core AC commitment — should be supersession of the plan, not augmentation), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly surfaced replacement-shape concern. Findings: %+v", result.Findings)
	}
}

// mentionsSupersession reports whether any finding's category or observation
// names supersession, replacement, or AC drop — the signals that the validator
// caught a replacement-shaped directive that should belong in a supersedes
// operation rather than an augmentation. Used by the augmenting-directive
// negative case to allow medium-or-high severity calibration.
func mentionsSupersession(findings []Finding) bool {
	for _, f := range findings {
		blob := strings.ToLower(f.Category + " " + f.Observation)
		if strings.Contains(blob, "supersede") ||
			strings.Contains(blob, "supersession") ||
			strings.Contains(blob, "replacement") ||
			strings.Contains(blob, "replaces") {
			return true
		}
	}
	return false
}

func TestPreflightEval_AugmentingDirective_TopicFilterReconstruction(t *testing.T) {
	// Reconstruction of the original blocked case from s-prc-vko: a tactical
	// directive resolves topic-filter ownership across two active plans
	// (Plan 1 ships the `topic(L)` primitive; Plan 2 consumes it). Two
	// capture attempts at this directive were blocked at high severity by
	// pre-flight; capture went through via --skip-preflight. With augment-
	// aware decision_refs.tmpl, this case should produce no high findings
	// tied to the augment-plan pattern.
	plan1 := planWithACs("20260506-151849-d-tac-gvn",
		"Plans the type-system expansion to 7+7 kinds by adding `kind: annotation` signal for topic clustering and `kind: focus` decision for involvement-driven planning, including CLI capture, display rendering, and pre-flight validation.",
		"`kind: annotation` validates frontmatter shape (target, label) at capture time",
		"`kind: focus` validates frontmatter shape (target, actors, optional date range) at capture time",
		"`sdd list --kind annotation|focus` filters on the new kinds",
		"`sdd list --topic <label>` filters via prefix-match on topic-path components, case-insensitive; reuses `topic(L)` filter primitive from Plan 2's shared internals",
		"Pre-flight validates participant canonicals against active actor chain heads",
	)
	plan2 := planWithACs("20260506-151345-d-tac-uww",
		"Plans implementation of `sdd view`, a new CLI command with a composable pipeline of query primitives (source, filter, transform, aggregate, rank, page, render).",
		"Pipeline primitives in shared internal packages, callable from other CLI commands (`sdd list --topic` consumes `topic(L)` filter)",
		"`source` primitive accepts a graph reference and emits an entry stream",
		"`filter` primitive accepts a predicate and removes non-matching entries",
		"`render` primitive emits a textual rendering of the streamed entries",
	)
	graph := model.NewGraph([]*model.Entry{plan1, plan2})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			{ID: plan1.ID, Kind: model.RefKindBuildsOn},
			{ID: plan2.ID, Kind: model.RefKindBuildsOn},
		},
		Content: "Plan 1 (d-tac-gvn) ships the `topic(L)` filter primitive in shared internal packages as part of its `sdd list --topic` AC. Plan 2 (d-tac-uww) consumes the existing primitive rather than re-implementing it. This resolves an ambiguity in Plan 1's AC text — which described the primitive as living in Plan 2's shared internals — by clarifying ownership in Plan 1's favor, since Plan 1 is the first plan to need the primitive. Plan 1 stays active; this directive is scoped to Plan 1's AC contract and is closed by Plan 1's done signal alongside the plan.",
		Time:    time.Date(2026, 5, 6, 15, 47, 59, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high findings (reconstructed topic-filter ownership case — clean augmenting directive across two active plans), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly accepted reconstructed augmenting directive. Findings: %+v", result.Findings)
	}
}

// mentionsRefMeta reports whether any finding's category or observation
// mentions ref / kind / desc concerns — used by the ref-metadata consistency
// tests to detect that the partial fired without binding to a specific
// category string the template doesn't pin down.
func mentionsRefMeta(findings []Finding) bool {
	for _, f := range findings {
		blob := strings.ToLower(f.Category + " " + f.Observation)
		if strings.Contains(blob, "desc") ||
			strings.Contains(blob, "kind") ||
			strings.Contains(blob, "ref ") ||
			strings.Contains(blob, "reference") {
			return true
		}
	}
	return false
}

// hasFindingAtSeverity reports whether any finding matches the predicate at
// the given severity.
func hasFindingAtSeverity(findings []Finding, sev Severity, predicate func(Finding) bool) bool {
	for _, f := range findings {
		if f.Severity != sev {
			continue
		}
		if predicate == nil || predicate(f) {
			return true
		}
	}
	return false
}

// TestPreflightEval_RefMeta_DescConsistent_NoFinding is the positive case:
// desc names the relationship the body actually carries. No ref-metadata
// finding should fire.
func TestPreflightEval_RefMeta_DescConsistent_NoFinding(t *testing.T) {
	gap := &model.Entry{
		ID:      "20260507-171914-s-cpt-zsd",
		Type:    model.TypeSignal,
		Kind:    model.KindGap,
		Layer:   model.LayerConceptual,
		Content: "Heat algorithm weighting in `sdd view` conflates grounding citations with extension or closure refs, causing standing entries to rank artificially high. Differentiating ref types via per-kind weights would let grounding citations decay and resolution citations weight fully.",
		Time:    time.Date(2026, 5, 7, 17, 19, 14, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{gap})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			{ID: gap.ID, Kind: model.RefKindAddresses, Desc: "responds to the heat-conflation gap"},
		},
		Content: "Implement label-aware heat weighting in `sdd view`'s rank algorithm — multiply each reference's contribution by a per-kind weight (grounds 0.25, related 0.5, active engagement 1.0) before decay and summing, so grounding citations no longer dominate. This addresses the heat-conflation gap raised in s-cpt-zsd. Closure will include comparative findings validating or revising the starting weight values.",
		Time:    time.Date(2026, 5, 17, 18, 1, 49, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	// No high finding expected; the metadata is consistent with the body.
	if result.HasBlocking() {
		t.Errorf("Expected no high findings (desc/kind consistent with body), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly accepted consistent ref metadata. Findings: %+v", result.Findings)
	}
}

// TestPreflightEval_RefMeta_DescContradicts_High exercises the high-severity
// branch: the desc characterizes the relationship one way; the body
// affirmatively refutes it. The validator must flag this as `high` with a
// finding that names the ref.
func TestPreflightEval_RefMeta_DescContradicts_High(t *testing.T) {
	contract := &model.Entry{
		ID:      "20260408-120000-d-prc-iom",
		Type:    model.TypeDecision,
		Kind:    model.KindContract,
		Layer:   model.LayerProcess,
		Content: "Documents in the graph are never modified after creation. Updates land as new entries that supersede or close the prior one. This immutability is the precondition for the graph as durable record.",
		Time:    time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{contract})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerProcess,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			{ID: contract.ID, Kind: model.RefKindGrounds, Desc: "extends the immutability contract"},
		},
		Content: "This directive retires the immutability contract d-prc-iom. We will allow in-place edits to entry bodies when the change is purely editorial (typo, link repair) and recorded in a `revision_history` field on the entry. The original immutability framing was correct at the time but proved too rigid for low-stakes edits that don't change semantic content.",
		Time:    time.Date(2026, 5, 19, 22, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	// Direct contradiction (desc "extends" vs body "retires") should be high.
	if !hasFindingAtSeverity(result.Findings, SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected a high finding mentioning the contradicting desc, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged desc contradiction as high. Findings: %+v", result.Findings)
	}
}

// TestPreflightEval_RefMeta_WrongKind_High exercises the wrong-kind branch
// of the high-severity rubric: the ref's kind misrepresents the relationship
// the body uses; another kind in the closed set names it correctly.
func TestPreflightEval_RefMeta_WrongKind_High(t *testing.T) {
	gap := &model.Entry{
		ID:      "20260505-100000-s-cpt-blur",
		Type:    model.TypeSignal,
		Kind:    model.KindGap,
		Layer:   model.LayerConceptual,
		Content: "Readers of catch-up output frequently miss state transitions in referenced entries because list rendering hides the derived status. The proposal: render a per-ref expansion that surfaces each reference's current status alongside its semantic kind, so a glance at any entry's outgoing refs makes lifecycle changes immediately visible without drilling.",
		Time:    time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{gap})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindPlan,
		Refs: []model.Ref{
			// Wrong kind: gap signals are addressed, not grounded against.
			// `grounds` is for anchoring to standing structure (contracts,
			// aspirations, standing directives).
			{ID: gap.ID, Kind: model.RefKindGrounds},
		},
		Content: `Plan an ` + "`expand(refs)`" + ` render modifier for ` + "`sdd view`" + ` list outputs that displays each entry's outgoing references as indented sub-lines carrying derived status and semantic relationship kind. This addresses the temporal-blur gap (s-cpt-blur) where readers miss state changes in referenced entries.

## Acceptance criteria

- [ ] ` + "`expand(refs)`" + ` renders one sub-line per outgoing ref
- [ ] Each sub-line shows derived status and ref kind
`,
		Time: time.Date(2026, 5, 19, 22, 5, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if !hasFindingAtSeverity(result.Findings, SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected a high finding mentioning the wrong kind (grounds on a gap signal), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged wrong kind as high. Findings: %+v", result.Findings)
	}
}

// TestPreflightEval_RefMeta_TopicalDrift_NotHigh exercises the softer
// divergence band: desc emphasizes an aspect the body never mentions, while
// the body still genuinely grounds in the referenced entry. No affirmative
// refutation, so the rubric must not fire a `high` ref-metadata finding.
// Whether the validator surfaces a low/medium observation or judges the
// drift acceptable is left to its judgment — both are valid calibration
// outcomes for the softer band.
func TestPreflightEval_RefMeta_TopicalDrift_NotHigh(t *testing.T) {
	aspiration := &model.Entry{
		ID:      "20260422-122136-d-stg-beb",
		Type:    model.TypeDecision,
		Kind:    model.KindAspiration,
		Layer:   model.LayerStrategic,
		Content: "Decisions emerge from multi-party engagement. All tooling serves dialogue rather than replacing reasoning. Reasoning-first is a consequence of dialogue shaping decisions, not a separate aspiration.",
		Time:    time.Date(2026, 4, 22, 12, 21, 36, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{aspiration})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerConceptual,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			// Desc names "dialogue-first" — accurate at the aspiration level.
			// The body grounds in the aspiration but frames the connection
			// through "multi-author review" rather than naming dialogue-first.
			// Topical drift (same axis, different focal length), not
			// contradiction.
			{ID: aspiration.ID, Kind: model.RefKindGrounds, Desc: "anchors to dialogue-first principle"},
		},
		Content: "Architecture-changing decisions in the SDD codebase must carry at least two distinct participants in `participants:` before the closing done signal lands. Per d-stg-beb (the aspiration the SDD project pulls toward), reasoning that shapes the framework should not emerge from a single author in isolation — multi-author review is the operational form that aspiration takes for non-trivial decisions. This applies to plan and directive decisions at the strategic and conceptual layers; tactical execution work is exempt because its decisions are typically scoped to a single workstream and reviewed at closing time against the parent plan.",
		Time:    time.Date(2026, 5, 19, 22, 10, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	// Direct contradiction is high; topical drift sits below the
	// contradiction threshold. The validator must NOT block on metadata
	// alone here.
	if hasFindingAtSeverity(result.Findings, SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected no high ref-metadata finding for topical drift (no affirmative refutation), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		// Whether a low/medium ref-meta finding fires is left to the
		// validator's judgment; log either way for visibility.
		t.Logf("Topical-drift case did not produce a high ref-metadata finding. Findings: %+v", result.Findings)
	}
}

// mentionsRefMetaPredicate is the predicate form of mentionsRefMeta for use
// with hasFindingAtSeverity.
func mentionsRefMetaPredicate(f Finding) bool {
	return mentionsRefMeta([]Finding{f})
}
