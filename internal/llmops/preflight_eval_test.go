//go:build eval

// This file contains evaluation tests for pre-flight prompt template accuracy.
// Run manually when tuning templates (costs real LLM calls):
//
//	go test -tags=eval -run TestPreflightEval ./internal/llmops/... -v
//
// The suite is an EXTERNAL test package (llm_test) so it drives the real
// pre-flight pipeline through the exported llm.Preflight orchestrator with a
// production-built Runner — no bespoke runner, and the eval exercises the same
// path production does. The candidate configuration (provider, model, params)
// is resolved by evalConfig below; per-call usage records through the sinks in
// eval_stats_test.go.
//
// Expectations match the severity-scored output format: HasBlocking() == true
// means at least one `high` finding was reported (the blocking threshold).

package llmops_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/finders"
	internalllm "github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/factory"
	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/pkg/llm"
)

// capturingRunner wraps a real llm.Runner and records the last raw response.
// llm.Preflight returns only the parsed result, so the wrapper preserves the
// raw model output for failure logging while the eval still runs the real
// orchestrator end to end.
type capturingRunner struct {
	inner    llm.Runner
	lastText string
}

func (r *capturingRunner) Run(ctx context.Context, req llm.Request) (llm.Result, error) {
	res, err := r.inner.Run(ctx, req)
	if res.Text != "" {
		r.lastText = res.Text
	}
	return res, err
}

// evalRunner builds the production Runner the eval validates against, wrapped
// to capture raw output and to record per-call usage (eval_stats_test.go).
// The candidate configuration comes from evalConfig.
func evalRunner(t *testing.T) *capturingRunner {
	t.Helper()
	runner, err := factory.New(evalConfig(t))
	if err != nil {
		t.Fatalf("building eval runner: %v", err)
	}
	return &capturingRunner{inner: internalllm.Observed(runner, multiSink{evalFileSink(t), tLogSink{t}})}
}

// evalConfig resolves the candidate configuration for this run. The user-global
// config (~/.config/sdd/config.yaml) supplies the base — API keys, Ollama
// endpoint, timeout, rate limits — so candidates run under the same conditions
// production would give them. SDD_EVAL_PROVIDER / SDD_EVAL_MODEL /
// SDD_EVAL_PARAMS select the candidate identity on top; provider and model are
// always set here, never inherited from the global config, so the incumbent
// cannot leak in as a silent default. Provider defaults to the direct Anthropic
// API when a key is reachable (env or global config) — much faster per call
// than the claude CLI transport, which matters now that pass-rate cases
// multiply the call count — else the keyless claude-cli. Both defaults target
// the Sonnet class — the model the ref-meta non-determinism was reported on
// (s-prc-uh3).
func evalConfig(t *testing.T) model.LLMConfig {
	t.Helper()
	cfg := globalLLMConfig(t)
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		if cfg.APIKeys == nil {
			cfg.APIKeys = map[string]string{}
		}
		cfg.APIKeys["anthropic"] = key
	}
	cfg.Provider = getenvOr("SDD_EVAL_PROVIDER", defaultEvalProvider(cfg))
	cfg.Model = getenvOr("SDD_EVAL_MODEL", defaultEvalModel(cfg.Provider))
	cfg.Params = parseEvalParams(t, os.Getenv("SDD_EVAL_PARAMS"))
	return cfg
}

// globalLLMConfig reads the llm section of the user-global config. A missing
// file yields the zero config; a malformed one fails the run — proceeding
// keyless would surface as a confusing provider error later.
func globalLLMConfig(t *testing.T) model.LLMConfig {
	t.Helper()
	loc, err := repos.DefaultLocations()
	if err != nil {
		t.Fatalf("resolving global config location: %v", err)
	}
	gc, err := repos.LoadConfigFrom(loc.ConfigPath)
	if err != nil {
		t.Fatalf("reading global config %s: %v", loc.ConfigPath, err)
	}
	return gc.LLM
}

// parseEvalParams parses SDD_EVAL_PARAMS ("key=value,key=value") into the
// config's Params map — behaviour-affecting provider settings such as a
// reasoning effort, recorded as the call's variant. Empty input yields nil.
func parseEvalParams(t *testing.T, raw string) map[string]string {
	t.Helper()
	if raw == "" {
		return nil
	}
	params := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || key == "" {
			t.Fatalf("malformed SDD_EVAL_PARAMS entry %q (want key=value,key=value)", pair)
		}
		params[key] = value
	}
	return params
}

func defaultEvalProvider(cfg model.LLMConfig) string {
	if cfg.APIKeys["anthropic"] != "" {
		return "anthropic"
	}
	return "claude-cli"
}

// defaultEvalModel maps the eval's Sonnet-class default to each provider's
// model naming: the claude CLI resolves the "sonnet" alias itself; the
// Anthropic API needs the full model ID.
func defaultEvalModel(provider string) string {
	if provider == "anthropic" {
		return "claude-sonnet-4-6"
	}
	return "sonnet"
}

func getenvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// runEval runs the real pre-flight pipeline (llm.Preflight) against the proposed
// entry and returns the parsed result plus the raw model output for logging on
// failure.
func runEval(t *testing.T, graph *model.Graph, proposed *model.Entry) (*llmops.PreflightResult, string) {
	t.Helper()
	result, raw, err := runEvalOnce(t, graph, proposed)
	if err != nil {
		t.Fatalf("Preflight error (raw output: %q): %v", raw, err)
	}
	return result, raw
}

// runEvalOnce is the non-fatal variant of runEval for pass-rate cases: an
// infrastructure error (runner failure, malformed JSON) is returned instead of
// aborting the test, so a single flaky response counts as a failed run rather
// than killing the whole measurement (s-prc-vvd's run 5 was exactly that).
func runEvalOnce(t *testing.T, graph *model.Graph, proposed *model.Entry) (*llmops.PreflightResult, string, error) {
	t.Helper()
	runner := evalRunner(t)

	// 240s: the claude-cli transport slows down late in a long sequential
	// suite — two cases hit the previous 120s cap on calls that complete in
	// ~90s when run alone.
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	// English default — the language-drift rubric only fires for non-empty locales.
	result, err := llmops.Preflight(ctx, runner, proposed, evalGraphWithBase(t, graph), "")
	return result, runner.lastText, err
}

// evalGraphWithBase mirrors the production load paths (finders.LoadGraph,
// application.BuildSnapshot): the case's entries plus the embedded base
// entries, disk-wins, so prompt inputs pulled from graph facts resolve.
func evalGraphWithBase(t *testing.T, graph *model.Graph) *model.Graph {
	t.Helper()
	base, err := finders.BaseEntries()
	if err != nil {
		t.Fatalf("loading base entries: %v", err)
	}
	onDisk := make(map[string]bool, len(graph.Entries))
	for _, e := range graph.Entries {
		onDisk[e.ID] = true
	}
	return model.NewGraph(model.MergeEmbedded(graph.Entries, onDisk, base))
}

// --- N-run pass-rate support (plan d-tac-tph, AC 2) ---

// passRate is a per-case pass-rate requirement: at least MinPasses of Runs
// must satisfy the case's check. Single-shot assertions can't see flakiness;
// running each pinned case N times turns the validator's non-determinism into
// a measured number. Blocking-tier cases (a spurious high blocks capture and
// trains users to bypass the validator) pin stricter rates than advisory-tier
// ones (a spurious medium costs a read-and-respond cycle).
type passRate struct {
	Runs      int
	MinPasses int
}

var (
	blockingTier = passRate{Runs: 3, MinPasses: 3}
	advisoryTier = passRate{Runs: 3, MinPasses: 2}
)

// withRunsOverride applies SDD_EVAL_RUNS: when set, it replaces Runs and
// rescales MinPasses to keep the case's required ratio (rounding up), so
// `SDD_EVAL_RUNS=10` measures the same bar over a larger sample.
func (p passRate) withRunsOverride() passRate {
	v := os.Getenv("SDD_EVAL_RUNS")
	if v == "" {
		return p
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return p
	}
	return passRate{Runs: n, MinPasses: (p.MinPasses*n + p.Runs - 1) / p.Runs}
}

// runEvalPassRate runs the real pre-flight pipeline Runs times and requires at
// least MinPasses runs where check returns nil. Infrastructure errors count as
// failed runs (logged), not aborts.
func runEvalPassRate(t *testing.T, graph *model.Graph, proposed *model.Entry, rate passRate, check func(*llmops.PreflightResult) error) {
	t.Helper()
	rate = rate.withRunsOverride()
	passes := 0
	for i := 1; i <= rate.Runs; i++ {
		result, raw, err := runEvalOnce(t, graph, proposed)
		if err != nil {
			t.Logf("run %d/%d FAIL (infrastructure): %v\nRaw output:\n%s", i, rate.Runs, err, raw)
			continue
		}
		if checkErr := check(result); checkErr != nil {
			t.Logf("run %d/%d FAIL: %v\nFindings: %+v\nRaw output:\n%s", i, rate.Runs, checkErr, result.Findings, raw)
			continue
		}
		passes++
		t.Logf("run %d/%d pass. Findings: %+v", i, rate.Runs, result.Findings)
	}
	if passes < rate.MinPasses {
		t.Errorf("pass rate %d/%d below required %d/%d", passes, rate.Runs, rate.MinPasses, rate.Runs)
	} else {
		t.Logf("pass rate %d/%d (required %d/%d)", passes, rate.Runs, rate.MinPasses, rate.Runs)
	}
}

// noHighRefMeta fails when a high-severity ref-meta finding fired — the
// blocking-tier leak the applicable-never-high rule (d-prc-v0h) forbids.
func noHighRefMeta(result *llmops.PreflightResult) error {
	if hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsRefMetaPredicate) {
		return fmt.Errorf("high ref-meta finding fired on an applicable kind")
	}
	return nil
}

// noMediumOrHighRefMeta fails when any ref-meta finding above low fired — the
// advisory-precision bar: a defensible kind with no body support for a
// different admissible kind stays at low.
func noMediumOrHighRefMeta(result *llmops.PreflightResult) error {
	if hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsRefMetaPredicate) {
		return fmt.Errorf("high ref-meta finding fired on a defensible kind")
	}
	if hasFindingAtSeverity(result.Findings, llmops.SeverityMedium, mentionsRefMetaPredicate) {
		return fmt.Errorf("medium ref-meta finding fired on a defensible kind without body evidence")
	}
	return nil
}

// noBlocking fails when any high finding fired, regardless of category.
func noBlocking(result *llmops.PreflightResult) error {
	if result.HasBlocking() {
		return fmt.Errorf("blocking high finding fired")
	}
	return nil
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
	//
	// The done cites the commit that landed the code — the durable artifact a
	// repo change requires per d-prc-kqx — so the durability check passes and the
	// AC-reasoning behavior is isolated; without it, durability correctly fires
	// high on the missing commit and masks what we're testing. (The attachment is
	// supplementary implementation notes, not the durability artifact.)
	plan := planWithACs("20260410-120000-d-tac-pln",
		"Implementation plan with four items.",
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
		Attachments: []string{"2026/04/10-130000-s-tac-def/implementation.md"},
		Content:     "Implemented item 1 (database schema with users table and bcrypt passwords) and item 3 (full CRUD endpoints at /users); details in the attached implementation notes. Deviation: authentication middleware (item 2) deferred — dialogued that we'd adopt an existing Passport.js library in a follow-up rather than build from scratch. Deviation: integration tests (item 4) deferred to a follow-up action — agreed during implementation that the schema/endpoint work needed smoke testing first, with the full suite as a separate closure. Commit 9d3f1ac.",
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
		Content:     "Built the complete user authentication feature: added users table with email/password columns (bcrypt hashed), wrote Express middleware that validates JWT tokens on protected routes, created REST endpoints for all CRUD operations (create user via signup, read user profile, update user settings, delete user account), and added a full integration test suite covering happy paths and error cases for every endpoint. Commit 4b8e2f1.",
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
	// The done cites its commit so durability stays quiet (d-prc-kqx).
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
		Content: "Built sdd lint command with checks for dangling refs (non-existent entries), malformed IDs (short suffixes), type mismatches in closes (signal can't close, action can't be closed, decision can't close decision), and type mismatches in supersedes (must be same type). Warnings are populated during graph construction on the Entry struct so sdd show displays them inline. Running against the live graph found 4 issues in 3 entries. Does NOT yet cover broken or missing attachment references — that requirement from d-tac-kfo remains unimplemented. Commit 2e7a9c4.",
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

func TestPreflightEval_ActionVerification_HumanAttestation(t *testing.T) {
	// New durability path (d-prc-kqx): an act that changed nothing — a manual,
	// human-attested verification — needs no commit, URL, or attachment. The
	// participant's attestation in the body is itself the durable trace, with the
	// playback-and-confirm step as the safeguard. Expected: no high.
	signal := &model.Entry{
		ID:         "20260612-090000-s-tac-vrf",
		Type:       model.TypeSignal,
		Layer:      model.LayerTactical,
		Confidence: "medium",
		Content:    "Open question whether the @AGENTS.md import actually expands and the rendered skills load in a fresh agent session — never confirmed against a real session.",
		Time:       time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{signal})

	proposed := &model.Entry{
		Type:         model.TypeSignal,
		Kind:         model.KindDone,
		Layer:        model.LayerTactical,
		Participants: []string{"Christopher", "Claude"},
		Closes:       []string{signal.ID},
		Content:      "Confirmed in a fresh agent session that the @AGENTS.md import expands and the rendered skills load correctly. No code or files changed — this records a manual verification, attested by Christopher in the session. There is no commit or attachment because the act produced nothing beyond the confirmation itself.",
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high finding (a human-attested verification that changed nothing needs no artifact, per d-prc-kqx), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly accepted human-attested verification. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ActionClosesSignal_WithExternalURL(t *testing.T) {
	// New durability path (d-prc-kqx): work whose result lives outside this repo
	// is durable via a URL (a deploy, a merged PR, a CI run), not only a local
	// commit. The act changed nothing in this repo, so a URL is the honest
	// artifact. Expected: no high.
	signal := &model.Entry{
		ID:         "20260612-093000-s-ops-dep",
		Type:       model.TypeSignal,
		Layer:      model.LayerOperational,
		Confidence: "high",
		Content:    "The staging environment needs to be stood up so the team can review the release candidate before the next tag.",
		Time:       time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{signal})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerOperational,
		Closes:  []string{signal.ID},
		Content: "Deployed the release candidate to staging; it is live at https://staging.example.com and the deploy pipeline run is green at https://github.com/example/repo/actions/runs/991234. Nothing changed in this repository — the artifact is the deploy itself.",
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high finding (external work is durable via a URL, per d-prc-kqx), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly accepted external URL as durable. Findings: %+v", result.Findings)
	}
}

func TestPreflightEval_ActionResearch_AttachmentIsDeliverable(t *testing.T) {
	// Durability calibration (d-prc-kqx, narrowed): a done whose deliverable IS an
	// attached document — research / synthesis / evaluation — is durable via that
	// attachment. No code or source changed, so no commit is required and none
	// should be demanded. Guards against overshoot: the hard commit requirement is
	// scoped to code/source changes, not to attachment-backed knowledge work.
	// Expected: no high.
	gap := &model.Entry{
		ID:         "20260613-090000-s-cpt-rsr",
		Type:       model.TypeSignal,
		Layer:      model.LayerConceptual,
		Confidence: "medium",
		Content:    "We need a comparison of the candidate embedding providers before committing to one for the hosted index.",
		Time:       time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{gap})

	proposed := &model.Entry{
		Type:        model.TypeSignal,
		Kind:        model.KindDone,
		Layer:       model.LayerConceptual,
		Closes:      []string{gap.ID},
		Attachments: []string{"2026/06/13-100000-s-cpt-cmp/comparison.md"},
		Content:     "Completed the embedding-provider comparison across cost, latency, recall, and operational burden; the full analysis and recommendation are in the attached write-up. No code changed — the deliverable is the analysis itself.",
	}

	result, raw := runEval(t, graph, proposed)
	if result.HasBlocking() {
		t.Errorf("Expected no high finding (a research done's deliverable is its attachment; no code changed, so no commit is required, per d-prc-kqx), got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly accepted attachment-as-deliverable research done. Findings: %+v", result.Findings)
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
		Refs:    []model.Ref{{ID: plan.ID, Kind: model.RefKindRefines}},
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
func mentionsSupersession(findings []llmops.Finding) bool {
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
			{ID: plan1.ID, Kind: model.RefKindRefines},
			{ID: plan2.ID, Kind: model.RefKindRefines},
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

// mentionsRefMeta reports whether any finding is a ref-metadata consistency
// finding, keyed on the finding Category. The ref_meta_consistency rubric emits
// ref-kind-* / ref-meta-* / desc-* categories; matching the category rather
// than scanning the observation prose avoids false positives when an unrelated
// finding's prose happens to mention "ref" or "kind" — e.g. a low ac-specificity
// note about "each ref by a per-kind weight" is not a ref-meta finding.
func mentionsRefMeta(findings []llmops.Finding) bool {
	for _, f := range findings {
		cat := strings.ToLower(f.Category)
		if strings.Contains(cat, "ref-kind") ||
			strings.Contains(cat, "ref-meta") ||
			strings.Contains(cat, "ref-desc") ||
			strings.HasPrefix(cat, "desc-") {
			return true
		}
	}
	return false
}

// hasFindingAtSeverity reports whether any finding matches the predicate at
// the given severity.
func hasFindingAtSeverity(findings []llmops.Finding, sev llmops.Severity, predicate func(llmops.Finding) bool) bool {
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
			{ID: contract.ID, Kind: model.RefKindGroundedIn, Desc: "extends the immutability contract"},
		},
		Content: "This directive retires the immutability contract d-prc-iom. We will allow in-place edits to entry bodies when the change is purely editorial (typo, link repair) and recorded in a `revision_history` field on the entry. The original immutability framing was correct at the time but proved too rigid for low-stakes edits that don't change semantic content.",
		Time:    time.Date(2026, 5, 19, 22, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	// Direct contradiction (desc "extends" vs body "retires") should be high.
	if !hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected a high finding mentioning the contradicting desc, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged desc contradiction as high. Findings: %+v", result.Findings)
	}
}

// TestPreflightEval_RefMeta_WrongKind_EvidenceMedium exercises the
// evidence-backed kind-mismatch branch: the ref's kind misrepresents the
// relationship and the body itself supplies the quotable evidence ("This
// addresses the temporal-blur gap"). grounded-in on an open gap is
// *applicable* (a gap can be a basis), so this is no longer a high — the
// matrix reserves high for precondition violations, which are mechanical.
// The validator must still catch the mismatch at medium with the body
// quote, and must not block.
func TestPreflightEval_RefMeta_WrongKind_EvidenceMedium(t *testing.T) {
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
			// `grounded-in` is for a basis the source rests on (contract,
			// aspiration, fact, prior decision) — not a gap the entry acts on.
			{ID: gap.ID, Kind: model.RefKindGroundedIn},
		},
		Content: `Plan an ` + "`expand(refs)`" + ` render modifier for ` + "`sdd view`" + ` list outputs that displays each entry's outgoing references as indented sub-lines carrying derived status and semantic relationship kind. This addresses the temporal-blur gap (s-cpt-blur) where readers miss state changes in referenced entries.

## Acceptance criteria

- [ ] ` + "`expand(refs)`" + ` renders one sub-line per outgoing ref
- [ ] Each sub-line shows derived status and ref kind
`,
		Time: time.Date(2026, 5, 19, 22, 5, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected no high (kind questions are never high — applicability is mechanical), got: %+v\nRaw output:\n%s", result.Findings, raw)
	}
	if !hasFindingAtSeverity(result.Findings, llmops.SeverityMedium, mentionsRefMetaPredicate) &&
		!hasFindingAtSeverity(result.Findings, llmops.SeverityLow, mentionsRefMetaPredicate) {
		t.Errorf("Expected a medium (evidence-backed) or low ref-meta finding for the kind mismatch, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Caught the kind mismatch without blocking. Findings: %+v", result.Findings)
	}
}

// TestPreflightEval_RefMeta_TopicalDrift_NotHigh exercises the softer
// divergence band: the desc emphasizes a narrower facet than the body argues
// through, while the KIND is genuinely correct. No affirmative refutation and
// no wrong kind, so the rubric must not fire a `high` ref-metadata finding;
// a low "a sharper desc would help" observation is an acceptable outcome.
//
// The target is a `fact` so `grounded-in` is unambiguously correct — a fact is
// a basis you reason from, never something you "realize" (the prior version of
// this scenario grounded-in an aspiration the body *operationalized*, which the
// principle vocabulary correctly reclassifies as `addresses`, confounding the
// topical-drift band this case is meant to probe).
func TestPreflightEval_RefMeta_TopicalDrift_NotHigh(t *testing.T) {
	fact := &model.Entry{
		ID:      "20260520-090000-s-prc-noise",
		Type:    model.TypeSignal,
		Kind:    model.KindFact,
		Layer:   model.LayerProcess,
		Content: "Across 12 sampled `related` refs, ~5 were correct siblings and ~7 were defensible-but-underspecified; zero were body contradictions. Medium-severity ref-kind findings caught no real errors in the sample.",
		Time:    time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{fact})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerProcess,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			// Correct kind: the directive reasons from the fact as its basis.
			// The desc names "the zero-error finding" while the body argues
			// through the broader "signal-to-noise" framing — topical drift
			// (same fact, different focal length), not a wrong kind.
			{ID: fact.ID, Kind: model.RefKindGroundedIn, Desc: "rests on the zero-error finding for medium ref-kind checks"},
		},
		Content: "Demote pre-flight's medium-severity ref-kind band to low for defensible-but-sharper choices. The measured signal-to-noise (s-prc-noise) is poor: the medium band costs read-and-respond cycles without catching genuine errors, so a defensible choice should not read as a blocking-adjacent finding. High stays for true contradictions; low carries the 'a sharper kind exists' nudge.",
		Time:    time.Date(2026, 5, 20, 21, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	// A correct kind with mild desc drift sits below the contradiction
	// threshold — the validator must not block on metadata here.
	if hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected no high ref-metadata finding for topical desc drift with a correct kind, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Topical-drift case did not produce a high ref-metadata finding. Findings: %+v", result.Findings)
	}
}

// mentionsRefMetaPredicate is the predicate form of mentionsRefMeta for use
// with hasFindingAtSeverity.
func mentionsRefMetaPredicate(f llmops.Finding) bool {
	return mentionsRefMeta([]llmops.Finding{f})
}

// The scenarios below pin the principle-based ref-kind calibration. Correct or
// defensible choices must produce NO ref-meta finding (not a spurious medium —
// the noise d-prc-2is and this plan target); a genuine wrong kind must be high.
// Each maps to a row of the scenario→kind table or a known-debatable case.

// Generalized `addresses`: a plan that operationalizes an active directive.
// Under the principle vocabulary this is correct — addresses covers realizing a
// decision's commitment, not only responding to a signal. This is the exact
// shape that tripped a debatable medium on d-tac-6d4's own capture.
func TestPreflightEval_RefMeta_AddressesRealizesDecision_NoFinding(t *testing.T) {
	directive := &model.Entry{
		ID: "20260531-160017-d-cpt-voc", Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerConceptual,
		Content: "Redefine the ref-kind vocabulary by principle: rename, merge, and add kinds so each names why a pointer exists. Implementing the rename across CLI, skill, and rubric is the follow-up.",
		Time:    time.Date(2026, 5, 31, 16, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{directive})
	proposed := planWithACs("20260531-170000-d-tac-imp",
		"Implement the principle-based ref-kind vocabulary from d-cpt-voc across model, pre-flight, and skill surfaces — the work that decision commits to.",
		"The capturable set is the eight principle-based kinds",
		"Legacy values resolve at parse with no history rewrite",
	)
	proposed.Refs = []model.Ref{{ID: directive.ID, Kind: model.RefKindAddresses, Desc: "implements the vocabulary redefinition this decision commits to"}}

	result, raw := runEval(t, graph, proposed)
	if mentionsRefMeta(result.Findings) {
		t.Errorf("Expected NO ref-meta finding — addresses realizing a decision's commitment is correct under the generalized kind. Got: %+v\nRaw:\n%s", result.Findings, raw)
	}
}

// Augmenting pattern: a directive that `refines` an active plan, sharpening its
// approach in place. Correct (active target, in-place) — this is the shape that
// tripped a debatable medium on d-tac-kxt's capture.
func TestPreflightEval_RefMeta_RefinesActivePlan_NoFinding(t *testing.T) {
	plan := planWithACs("20260531-164326-d-tac-pln",
		"Implement the ref-kind vocabulary redefinition across model, pre-flight, and skill.",
		"The vocabulary is the eight principle-based kinds",
		"The skill and rubric define the vocabulary",
	)
	graph := model.NewGraph([]*model.Entry{plan})
	proposed := &model.Entry{
		Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
		Refs:    []model.Ref{{ID: plan.ID, Kind: model.RefKindRefines, Desc: "single-sources the vocabulary instead of restating it across surfaces"}},
		Content: "Single-source the ref-kind vocabulary rather than restating it in each surface, sharpening d-tac-pln's skill and rubric work. The eight kinds live in one canonical fragment that the skill install inlines and pre-flight injects, so the definitions exist once. The plan stays active; this directive closes alongside it.",
		Time:    time.Date(2026, 5, 31, 18, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if mentionsRefMeta(result.Findings) {
		t.Errorf("Expected NO ref-meta finding — refines on an active plan is the augmenting pattern. Got: %+v\nRaw:\n%s", result.Findings, raw)
	}
}

// Status-sensitive kind mismatch with quotable evidence: the target is ACTIVE
// and the body says outright that it "sharpens the plan's commitment in
// place" — the augmenting pattern, which is `refines`. builds-on on a live
// decision is *applicable* (the forward next-step reading exists, and the
// live graph carries accepted builds-on refs to active targets), so this is
// not a high — it is the textbook evidence-backed medium: the body supplies
// the quote that names the other admissible kind. Recalibrated from the
// earlier high expectation when applicability moved into the mechanical
// matrix (d-tac-tph AC 6).
func TestPreflightEval_RefMeta_BuildsOnActiveSharpened_EvidenceMedium(t *testing.T) {
	plan := planWithACs("20260531-164326-d-tac-pln",
		"Implement the ref-kind vocabulary redefinition across model, pre-flight, and skill.",
		"The vocabulary is the eight principle-based kinds",
	)
	graph := model.NewGraph([]*model.Entry{plan})
	proposed := &model.Entry{
		Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
		Refs:    []model.Ref{{ID: plan.ID, Kind: model.RefKindBuildsOn, Desc: "narrows the plan's skill AC to a single canonical source"}},
		Content: "Narrow d-tac-pln's skill acceptance criterion: the vocabulary must be single-sourced from one canonical fragment rather than restated per surface. This sharpens the plan's commitment in place — the plan stays active and this directive closes alongside it.",
		Time:    time.Date(2026, 5, 31, 18, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected no high (kind questions are never high — applicability is mechanical), got: %+v\nRaw:\n%s", result.Findings, raw)
	}
	if !hasFindingAtSeverity(result.Findings, llmops.SeverityMedium, mentionsRefMetaPredicate) &&
		!hasFindingAtSeverity(result.Findings, llmops.SeverityLow, mentionsRefMetaPredicate) {
		t.Errorf("Expected a medium (evidence-backed) or low ref-meta finding — the body quotes itself sharpening in place. Got: %+v\nRaw:\n%s", result.Findings, raw)
	}
}

// `builds-on` is correct when the target is CLOSED and the new entry is the next
// step after a finished line of work.
func TestPreflightEval_RefMeta_BuildsOnClosedTarget_NoFinding(t *testing.T) {
	oldPlan := planWithACs("20260501-100000-d-tac-old",
		"Original ref-metadata plan: add per-ref kinds to the graph.",
		"Refs carry a kind",
	)
	done := &model.Entry{
		ID: "20260510-100000-s-tac-don", Type: model.TypeSignal, Kind: model.KindDone,
		Closes: []string{oldPlan.ID}, Content: "Shipped per-ref kinds across the graph.",
		Time: time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{oldPlan, done})
	proposed := planWithACs("20260531-170000-d-tac-new",
		"Extend the now-shipped per-ref kind work (d-tac-old, closed) with label-aware heat weighting — the next step after that finished chain.",
		"Heat weighting multiplies each ref by a per-kind weight",
	)
	proposed.Refs = []model.Ref{{ID: oldPlan.ID, Kind: model.RefKindBuildsOn, Desc: "extends the shipped per-ref kind work"}}

	result, raw := runEval(t, graph, proposed)
	if mentionsRefMeta(result.Findings) {
		t.Errorf("Expected NO ref-meta finding — builds-on a closed target is correct. Got: %+v\nRaw:\n%s", result.Findings, raw)
	}
}

// `related` is correct as the floor: a genuine sibling the entry accounts for
// but does not act on, ground in, or depend on. Must not fire (the floor is not
// an error when no sharper kind fits).
func TestPreflightEval_RefMeta_RelatedFloor_NoFinding(t *testing.T) {
	sibling := &model.Entry{
		ID: "20260520-150000-d-tac-sib", Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical,
		Content: "Heat-weighting directive: weight refs by kind in sdd view ranking.",
		Time:    time.Date(2026, 5, 20, 15, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{sibling})
	proposed := &model.Entry{
		Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
		Refs:    []model.Ref{{ID: sibling.ID, Kind: model.RefKindRelated, Desc: "parallel ref-metadata sibling, runs independently"}},
		Content: "Add a refs-of() filter to sdd view for drill expansion. This runs in parallel with the heat-weighting work (d-tac-sib) — a sibling in the same ref-metadata cluster, but neither depends on nor realizes the other.",
		Time:    time.Date(2026, 5, 31, 18, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if mentionsRefMeta(result.Findings) {
		t.Errorf("Expected NO ref-meta finding — related is the correct floor for a genuine sibling. Got: %+v\nRaw:\n%s", result.Findings, raw)
	}
}

// `surfaced-by` is the ninth kind (d-tac-53l) — the backward inverse of
// `surfaces`. Pins the scenario the gap (s-cpt-seg) named: a surfaced entry
// captured AFTER its surfacer, where the surfacer is a terminal `done`. This is
// the case the lossy fallbacks fail — `addresses` is mechanically blocked on a
// terminal done, `grounded-in`/`builds-on` understate "raised by", and `related`
// is the floor. The validator must accept `surfaced-by` as the precise kind and
// not push any of those alternatives at a noise/blocking band. Advisory tier: a
// spurious sharper-kind nudge at low is tolerable; a medium/high recommending
// grounded-in/addresses/related is the boundary regression this guards.
func TestPreflightEval_RefMeta_SurfacedByAfterTerminalDone_NoMedium(t *testing.T) {
	done := &model.Entry{
		ID:      "20260610-090000-s-tac-idx",
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Content: "Shipped lazy search-index fill: sdd search now embeds missing entries on demand rather than requiring an upfront sdd index. Commit a1b2c3d. While wiring it, observed once in a parallel session that two participants embedding the same entry concurrently both rewrite the index manifest — not yet handled.",
		Time:    time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{done})

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Kind:       model.KindGap,
		Layer:      model.LayerTactical,
		Confidence: "medium",
		Refs: []model.Ref{
			// surfaced-by: the done's work raised this gap. addresses is blocked
			// (terminal done); grounded-in would understate that the work
			// *produced* this; this is the surfacer case the new kind fills.
			{ID: done.ID, Kind: model.RefKindSurfacedBy, Desc: "raised by the lazy-fill work, which exposed the concurrent manifest write"},
		},
		Content: "Concurrent search-index manifest writes can race: two participants embedding the same entry at once both rewrite the manifest, and the later write clobbers the earlier's entries. The lazy-fill implementation (s-tac-idx) raised this — its work surfaced the collision, and on-demand embedding makes the race likely rather than theoretical.",
		Time:    time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC),
	}

	runEvalPassRate(t, graph, proposed, advisoryTier, noMediumOrHighRefMeta)
}

// Terminal-`done` tie-break (d-prc-v0h / d-prc-uh3): the target is a terminal
// `done` whose body flagged a follow-up, and the source is the next step taking
// that follow-up up. `builds-on` (next step after a finished chain) is
// applicable, so the ceiling is `low` — the advisory must NOT block at `high`.
// This pins the oscillation case where identical input drew contradictory high
// verdicts that each recommended `addresses` — the one kind a terminal done
// (a completed fact, not a gap/commitment) forbids.
func TestPreflightEval_RefMeta_BuildsOnTerminalDoneFollowup_NotHigh(t *testing.T) {
	done := &model.Entry{
		ID:      "20260604-090000-s-ops-rec",
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Content: "Restored the cluster after the outage and brought all services back online. One follow-up remains: remote out-of-band management of the affected node is still broken and is tracked separately.",
		Time:    time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{done})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			// Terminal done target; the source is the next step after that
			// finished chain. builds-on is applicable, so this must not block
			// at high — and addresses (what the oscillating validator kept
			// recommending) is inapplicable to a completed fact.
			{ID: done.ID, Kind: model.RefKindBuildsOn, Desc: "next step after the recovery, which flagged remote management as still broken"},
		},
		Content: "Restore remote out-of-band management of the affected node — the follow-up the outage recovery (s-ops-rec) flagged as still broken. This is the next step after that finished recovery work.",
		Time:    time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC),
	}

	result, raw := runEval(t, graph, proposed)
	if hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsRefMetaPredicate) {
		t.Errorf("Expected no high ref-meta finding — builds-on on a terminal done (taking up a flagged follow-up) is applicable; the ceiling is low. Got: %+v\nRaw:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly did not block builds-on on a terminal done. Findings: %+v", result.Findings)
	}
}

// --- Pinned leak cases (plan d-tac-tph, AC 1) ---
//
// Each case below reconstructs a captured live leak from its attached verbatim
// transcript or findings list. They are the regression surface for the
// reliability rework: they must reproduce the reported failures before the fix
// (some intermittently — that is what the pass rate measures) and pass at
// their tier's rate after it.

// Pinned from s-prc-l2d (verbatim transcript attached there): an `addresses`
// ref to an OPEN gap drew a `[high]` ref-kind-inapplicable finding whose own
// prose concluded "this finding is withdrawn. No issue." yet still counted
// toward the blocking verdict (run 1), then flipped to a `[low]` recommending
// the opposite kind (`grounded-in`) on retry with the ref unchanged (run 2).
// `addresses` on an open gap is the textbook applicable case; the ceiling is
// low. Blocking tier: a spurious high here blocks capture.
func TestPreflightEval_RefMeta_AddressesOpenGap_NotHigh(t *testing.T) {
	gap := &model.Entry{
		ID:      "20260525-184931-s-cpt-prt",
		Type:    model.TypeSignal,
		Kind:    model.KindGap,
		Layer:   model.LayerConceptual,
		Content: "The graph access surface and skill content are bound to one agent runtime's bundle format and cannot run in other agent environments. Graph operations would need environment-specific adapters; skill content would need conversion to prompt templates. Instruction delivery to foreign runtimes is unproven — dynamic graph data injection remained hard-coded in the initial PoC.",
		Time:    time.Date(2026, 5, 25, 18, 49, 31, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{gap})

	proposed := &model.Entry{
		Type:       model.TypeDecision,
		Layer:      model.LayerConceptual,
		Kind:       model.KindDirective,
		Confidence: "low",
		Refs: []model.Ref{
			{ID: gap.ID, Kind: model.RefKindAddresses, Desc: "tests instruction delivery to foreign runtimes — the skill-content half of the gap"},
		},
		Content: "Run a workflow-agent experiment: an external generalist agent consults a graph-resident specialist that delivers per-mode instructions in-band through tool responses, instead of shipping skill bundles per runtime. Whether mode instructions can be delivered to a foreign runtime and followed reliably is exactly what the skill-content half of the portability gap (s-cpt-prt) predicts and this experiment directly tests. The experiment concludes when it returns a verdict on instruction compliance and coherence; that verdict decides the follow-up shape.",
		Time:    time.Date(2026, 6, 9, 23, 46, 56, 0, time.UTC),
	}

	runEvalPassRate(t, graph, proposed, blockingTier, noHighRefMeta)
}

// Pinned from s-prc-2lm (redacted verbatim transcript attached there): a
// `builds-on` ref to a terminal done whose relationship the body frames with
// provenance language ("the answer came in response to the created ticket").
// The single-run `[high]` performed the tie-break analysis correctly in its
// own prose — naming `grounded-in` and `surfaces` as applicable — while
// categorizing the defensible `builds-on` as inapplicable: internally coherent
// and confidently wrong, with no retraction to detect. On a terminal done only
// `addresses` is inapplicable; builds-on vs grounded-in is the documented
// defensible choice. Complements BuildsOnTerminalDoneFollowup_NotHigh above,
// whose clean next-step framing does not reproduce this leak — the
// provenance-leaning body is what tempted the escalation.
func TestPreflightEval_RefMeta_BuildsOnTerminalDoneProvenance_NotHigh(t *testing.T) {
	done := &model.Entry{
		ID:      "20260610-184443-s-ops-tkt",
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerOperational,
		Content: "Created vendor support ticket #4711 asking the open cross-organization identification questions. The question stays open until the vendor's answer arrives.",
		Time:    time.Date(2026, 6, 10, 18, 44, 43, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{done})

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Kind:       model.KindFact,
		Layer:      model.LayerConceptual,
		Confidence: "high",
		Refs: []model.Ref{
			{ID: done.ID, Kind: model.RefKindBuildsOn, Desc: "takes up the point the ticket step left open"},
		},
		Content: "The vendor answered the integration questions (ticket #4711). Key statements: matching identifiers are defined per organization and exist only in the central database; a second identifier is shared across all organizations, so cross-organization identification is possible by combining the two. The answer came in response to the created ticket (s-ops-tkt); open remains whether the mapping can be delivered over the interfaces.",
		Time:    time.Date(2026, 6, 10, 22, 0, 0, 0, time.UTC),
	}

	runEvalPassRate(t, graph, proposed, blockingTier, noHighRefMeta)
}

// --- Pinned advisory-precision cases (plan d-tac-tph, AC 1; from s-tac-4h7) ---
//
// Each reconstructs a verbatim `[medium]` ref-kind finding from s-tac-4h7's
// attachment where the chosen kind was defensible per the vocabulary and the
// suggested alternative was weaker or wrong. Under the evidence-gated
// calibration, a kind question with no body support for a different admissible
// kind stays at low — these must not fire medium or high.

// s-tac-4h7 example 1: `builds-on` to a CLOSED gap in the same lineage — the
// next observation after the prior gap's closure. The validator suggested
// grounded-in/related at medium; builds-on on a closed target is the
// vocabulary's own definition of the kind.
func TestPreflightEval_RefMeta_BuildsOnClosedSameLineage_NoMedium(t *testing.T) {
	priorGap := &model.Entry{
		ID:      "20260411-120000-s-prc-tha",
		Type:    model.TypeSignal,
		Kind:    model.KindGap,
		Layer:   model.LayerProcess,
		Content: "Worktree sessions lose their SDD config and search index because gitignored local state does not carry over into the new directory.",
		Time:    time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC),
	}
	closer := &model.Entry{
		ID:      "20260411-233406-s-tac-p5x",
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{priorGap.ID},
		Content: "Shipped the worktree include recipe: gitignored local state listed in .worktreeinclude carries into new worktrees. Commit 4f2c1aa.",
		Time:    time.Date(2026, 4, 11, 23, 34, 6, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{priorGap, closer})

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Kind:       model.KindGap,
		Layer:      model.LayerProcess,
		Confidence: "medium",
		Refs: []model.Ref{
			{ID: priorGap.ID, Kind: model.RefKindBuildsOn, Desc: "next observation in the same worktree-session lineage"},
		},
		Content: "A new friction point in the worktree session workflow, discovered while running the shipped include recipe: the background sync can rewrite history between conclude steps, so the merge-back races against the cooldown. This is the next observation in the lineage the local-state gap (s-prc-tha) opened — that gap was closed by the recipe work, and this discovery emerged from operating it.",
		Time:    time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC),
	}

	runEvalPassRate(t, graph, proposed, advisoryTier, noMediumOrHighRefMeta)
}

// s-tac-4h7 example 3: `related` where the validator suggested refines (the
// lifecycle-split augmenting pattern does not apply — the target is guidance
// this entry will trigger a future update to, not sharpen in place) and
// depends-on (backwards — the target is not a prerequisite). The floor is the
// honest choice.
func TestPreflightEval_RefMeta_RelatedFutureUpdate_NoMedium(t *testing.T) {
	guidance := &model.Entry{
		ID:      "20260420-100000-s-prc-jpx",
		Type:    model.TypeSignal,
		Kind:    model.KindInsight,
		Layer:   model.LayerProcess,
		Content: "Conflict-resolution guidance for concurrent graph work: rebase local commits onto the remote head before concluding, so entry files merge cleanly.",
		Time:    time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{guidance})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			{ID: guidance.ID, Kind: model.RefKindRelated, Desc: "sibling guidance that will need its own update once this lands"},
		},
		Content: "Switch graph sync to merge-based pulls — a rebase rewrites shared history and orphans concurrent branches, so the sync command pulls with merge only. Implementing this will make the rebase-shaped conflict guidance (s-prc-jpx) outdated; updating that guidance to a merge-shaped equivalent is its own follow-up, not part of this directive. The guidance is a sibling concern this directive accounts for but does not modify.",
		Time:    time.Date(2026, 6, 1, 21, 0, 0, 0, time.UTC),
	}

	runEvalPassRate(t, graph, proposed, advisoryTier, noMediumOrHighRefMeta)
}

// s-tac-4h7 example 4: `builds-on` to a closed directive for a deliberate
// PARTIAL reversal. The validator suggested "kind: supersedes" — which is not
// a ref kind at all (supersession is a status-effect field, never a ref), and
// a full supersede would overstate a partial reversal. The chosen kind is
// defensible: the conditions enabling the reversal were built on the prior
// decision's mechanics.
func TestPreflightEval_RefMeta_BuildsOnPartialReversal_NoMedium(t *testing.T) {
	prior := &model.Entry{
		ID:      "20260415-100000-d-tac-ar2",
		Type:    model.TypeDecision,
		Kind:    model.KindDirective,
		Layer:   model.LayerTactical,
		Content: "Keep worktree creation out of the CLI: sdd wip offers --branch only, and worktree setup stays a manual git operation. Rationale: the CLI should not own directory lifecycle.",
		Time:    time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
	}
	closer := &model.Entry{
		ID:      "20260420-110000-s-tac-arc",
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{prior.ID},
		Content: "Shipped sdd wip --branch; worktrees documented as manual git operations. Commit 9c01dd2.",
		Time:    time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{prior, closer})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			{ID: prior.ID, Kind: model.RefKindBuildsOn, Desc: "reverses one part of it; built on its branch mechanics"},
		},
		Content: "Reintroduce worktree support through the harness recipe (marker-on-base, include-file carry-over) while keeping plain --branch in the CLI unchanged. This deliberately reverses only the worktrees-stay-manual part of the prior stance (d-tac-ar2) — the branch mechanics that decision shipped are exactly what the recipe builds on, so the rest of it stands.",
		Time:    time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC),
	}

	runEvalPassRate(t, graph, proposed, advisoryTier, noMediumOrHighRefMeta)
}

// --- Pinned supersede-oscillation case (plan d-tac-tph, AC 8; from s-prc-vvd) ---

// Pinned from s-prc-vvd (per-run findings table attached there): a superseding
// plan whose substance was unchanged across five runs drew two different sets
// of `[high]` findings (runs 3 and 4), each a misread — a clause claimed
// missing that the AC text contains verbatim ("AND `refs` includes that head
// actor's entry ID"), and a contract-violation on framing identical to the
// superseded plan, which had passed at its own capture. Reconstructed analog:
// a superseding plan with acknowledged AC growth, the capture-time AND clause
// kept verbatim, and finder-layer framing inherited from its predecessor under
// an active decomposition contract. Expectation: no high finding of any
// category. The post-fix N-run verdict on this case decides s-prc-vvd's
// disposition (plan d-tac-tph AC 8).
func TestPreflightEval_Supersedes_PlanRestructure_NoHigh(t *testing.T) {
	contract := &model.Entry{
		ID:      "20260413-142536-d-cpt-cqr",
		Type:    model.TypeDecision,
		Kind:    model.KindContract,
		Layer:   model.LayerConceptual,
		Content: "Functionality decomposes across command, query, handler, finder, and model packages. Side effects live only in handlers; finders are pure reads; pure computation pushes down into the model layer. Plans that add functionality name where each piece lands in this decomposition.",
		Time:    time.Date(2026, 4, 13, 14, 25, 36, 0, time.UTC),
	}
	oldPlan := planWithACs("20260422-100000-d-cpt-thg",
		`Implement actor identity and role entries: kind: actor signals carry a canonical participant name, kind: role decisions bind to an actor chain. Role status derives from the actor chain rather than direct closure — derivation is a query concern and lands in a finder (pure read), conforming to the decomposition contract.

The legacy free-text participant convention is retired through the reclassification step in the final AC.`,
		"Actor capture validates the canonical against existing chains (write-once)",
		"Role capture validates the actor field against the current head canonical AND `refs` includes that head actor's entry ID",
		"Role status derives from the bound actor chain in a role-status finder",
		"Participant fields render grouped by canonical in status output",
	)
	graph := model.NewGraph([]*model.Entry{contract, oldPlan})

	proposed := planWithACs("20260423-195649-d-cpt-d34",
		`Restructure the actor/role plan with three material additions: a role-status cascade across full chain history, an orphan-role lint check, and a Participants status block. The AC set grows from four to six — the cascade and lint checks are new, the remaining ACs carry over with one refinement: capture-time validation still binds to the current head (AND-clause retained verbatim), while status derivation now walks the full chain history — capture-time and derivation-time are distinct requirements and both are kept.

Role-status derivation stays in a role-status finder (pure read), conforming to the decomposition contract — derivation is a query concern. The legacy free-text participant convention is retired through the reclassification step in the final AC.`,
		"Actor capture validates the canonical against existing chains (write-once)",
		"Role capture validates the actor field against the current head canonical AND `refs` includes that head actor's entry ID",
		"Role status derives from full chain history (cascade) in a role-status finder",
		"Orphan roles (no matching actor chain) surface in sdd lint",
		"Participant fields render grouped by canonical in status output",
		"Legacy free-text participants reclassify onto actor chains",
	)
	// Both plans are conceptual-layer (matching their d-cpt IDs) — the
	// baseline run proved the validator reads the ID's layer segment: a
	// tactical Layer field on a d-cpt ID drew a correct id-layer-mismatch
	// finding (at high, absent, and medium across three runs — the severity
	// oscillation s-prc-vvd describes, on a true positive).
	oldPlan.Layer = model.LayerConceptual
	proposed.Layer = model.LayerConceptual
	proposed.Supersedes = []string{oldPlan.ID}
	proposed.Time = time.Date(2026, 4, 23, 19, 56, 49, 0, time.UTC)

	runEvalPassRate(t, graph, proposed, blockingTier, noBlocking)
}

// mentionsSettled matches the settled-justification rubric's finding category.
func mentionsSettled(f llmops.Finding) bool {
	return strings.Contains(strings.ToLower(f.Category), "settled")
}

// TestPreflightEval_Settled_Unjustified_Medium: a settled directive that states
// a choice but gives no reason it needs no follow-up should draw a medium
// settled-unjustified finding.
func TestPreflightEval_Settled_Unjustified_Medium(t *testing.T) {
	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Intent:  model.IntentSettled,
		Content: "Use the existing JSON encoder for the export path.",
		Time:    time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph(nil)
	result, raw := runEval(t, graph, proposed)
	if !hasFindingAtSeverity(result.Findings, llmops.SeverityMedium, mentionsSettled) {
		t.Errorf("Expected a medium settled-unjustified finding, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly flagged unjustified settled directive. Findings: %+v", result.Findings)
	}
}

// TestPreflightEval_Settled_Justified_NoFinding: a settled directive whose body
// explains why no follow-up is needed should draw no settled finding.
func TestPreflightEval_Settled_Justified_NoFinding(t *testing.T) {
	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Intent:  model.IntentSettled,
		Content: "Keep the existing JSON encoder for the export path as-is. We weighed swapping to a streaming encoder and concluded the export corpora never approach the memory ceiling, so there is nothing to build — this records the deliberate no-change decision so it stops resurfacing in grooming.",
		Time:    time.Date(2026, 6, 20, 10, 5, 0, 0, time.UTC),
	}
	graph := model.NewGraph(nil)
	result, raw := runEval(t, graph, proposed)
	if hasFindingAtSeverity(result.Findings, llmops.SeverityMedium, mentionsSettled) ||
		hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, mentionsSettled) {
		t.Errorf("Expected no settled finding for a justified settled directive, got: %+v\nRaw output:\n%s", result.Findings, raw)
	} else {
		t.Logf("Correctly accepted justified settled directive. Findings: %+v", result.Findings)
	}
}

// --- Closing-signal rubric (d-tac-4yb): stated-why on non-question signal closures ---

// TestPreflightEval_ClosingSignal_NoStatedWhy_Flagged: a fact closes a fact
// while the narrative only states the new figure and never says why the old
// one is retired. Expected: a finding at medium or above (the closing-signal
// stated-why check, the unusual-close pattern flag, or both).
func TestPreflightEval_ClosingSignal_NoStatedWhy_Flagged(t *testing.T) {
	oldFact := &model.Entry{
		ID:         "20260410-120000-s-tac-old",
		Type:       model.TypeSignal,
		Kind:       model.KindFact,
		Layer:      model.LayerTactical,
		Confidence: "high",
		Content:    "The courier ships parcels for 4.50-5.80 EUR per parcel depending on volume tier, per the 2025 rate card.",
		Time:       time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{oldFact})

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Kind:       model.KindFact,
		Layer:      model.LayerTactical,
		Confidence: "high",
		Closes:     []string{oldFact.ID},
		Content:    "The courier ships parcels for a flat 4.80 EUR per parcel, per the March 2026 price sheet.",
	}

	anyMediumOrHigh := func(result *llmops.PreflightResult) error {
		if hasFindingAtSeverity(result.Findings, llmops.SeverityMedium, nil) ||
			hasFindingAtSeverity(result.Findings, llmops.SeverityHigh, nil) {
			return nil
		}
		return fmt.Errorf("no finding at medium or above for a closure with no stated why")
	}
	runEvalPassRate(t, graph, proposed, advisoryTier, anyMediumOrHigh)
}

// TestPreflightEval_ClosingSignal_StatedWhy_NoBlocking: a gap closes a gap,
// carrying why the earlier deviation no longer applies. This is the closure
// shape the close-validation rework legalized; a spurious high here blocks
// capture, so the case pins the blocking tier.
func TestPreflightEval_ClosingSignal_StatedWhy_NoBlocking(t *testing.T) {
	oldGap := &model.Entry{
		ID:         "20260410-120000-s-tac-dev",
		Type:       model.TypeSignal,
		Kind:       model.KindGap,
		Layer:      model.LayerTactical,
		Confidence: "high",
		Content:    "Deliveries from the timber supplier keep failing the grade check: the framing contract specifies C24, and the last three deliveries measured C16 on receipt.",
		Time:       time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	}
	graph := model.NewGraph([]*model.Entry{oldGap})

	proposed := &model.Entry{
		Type:       model.TypeSignal,
		Kind:       model.KindGap,
		Layer:      model.LayerTactical,
		Confidence: "high",
		Closes:     []string{oldGap.ID},
		Content:    "The timber supplier has ceased trading: the yard confirmed on 2026-04-09 that the business closed at the end of March. Expected per the framing contract: an active C24 supply. Actual: no supplier and no further deliveries. This closes the earlier deviation about failing grade checks, which no longer applies now that there are no deliveries to check; finding a replacement supplier is the deviation this entry carries forward.",
	}

	runEvalPassRate(t, graph, proposed, blockingTier, noBlocking)
}
