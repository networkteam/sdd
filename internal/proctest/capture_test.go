// Core capture flow through the real application: assemble, writing guide,
// playback, pre-flight write gate, summary verification. Ported from
// internal/engine's capture behavior tests — fake counters became scripted-LLM
// call counts and real entries on disk.
package proctest_test

import (
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

const (
	captureRefID = "20260601-120000-d-tac-ref"
	// captureRef2ID resolves in the fixture graph but is never logged as read —
	// the refsInspected gate tests draft against it.
	captureRef2ID = "20260601-130000-s-tac-raw"
)

func captureFixtureEntries() []*model.Entry {
	return []*model.Entry{
		{
			ID: captureRefID, Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
			Summary: "A directive the fixture capture refs.", Content: "A directive the fixture capture refs.",
			Topics: proctest.MustTopics("cli/ux"),
		},
		{
			ID: captureRef2ID, Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical,
			Summary: "A signal no fixture session has read in full.", Content: "A signal no fixture session has read in full.",
		},
	}
}

// newCaptureWorld opens a session over the capture fixtures that has read the
// primary ref in full, so drafts against captureRefID pass refsInspected.
func newCaptureWorld(t *testing.T, connID string) (*proctest.World, *proctest.Session) {
	t.Helper()
	world := proctest.NewWorld(t, proctest.WithEntries(captureFixtureEntries()...))
	session := world.Open(t, connID)
	session.LogRead(t, "show", []string{captureRefID}, nil)
	return world, session
}

// captureDraft is the one-shot batched report covering every assemble field.
func captureDraft() map[string]any {
	return map[string]any{
		"body":        "A tactical gap: the fixture observes something.",
		"entryKind":   "gap",
		"layer":       "tactical",
		"refs":        []any{map[string]any{"id": captureRefID, "kind": "addresses", "desc": "realizes it"}},
		"topics":      []any{"cli/ux"},
		"confidence":  "medium",
		"widenReport": "searched from three angles, inspected d-tac-ref",
	}
}

func hasDiagnostic(serve *sdd.WorkflowServe, substr string) bool {
	for _, d := range serve.Diagnostics {
		if strings.Contains(d, substr) {
			return true
		}
	}
	return false
}

func TestCapture_OneShotHappyPath(t *testing.T) {
	world, session := newCaptureWorld(t, "core-happy")

	serve := session.Start(t, "capture", nil)
	proctest.RequireStep(t, serve, "assemble")
	if !strings.Contains(serve.Instructions, "cli/ux") {
		t.Errorf("assemble unit should carry the injected topics view, got %q", serve.Instructions)
	}
	if len(serve.Missing) == 0 {
		t.Error("fresh assemble should name missing required fields")
	}
	instance := serve.Instance

	// One-shot batched report cascades straight through assemble to the
	// playback chooser.
	serve = session.Report(t, instance, captureDraft())
	proctest.RequireStep(t, serve, "playback")
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("playback must serve a user chooser, got %+v", serve.PendingChooser)
	}
	if !strings.Contains(serve.Instructions, "A tactical gap") {
		t.Errorf("playback unit should render the body, got %q", serve.Instructions)
	}

	// User confirms → write gate → real newEntry (clean pre-flight) →
	// verifySummary agent chooser.
	serve = session.Answer(t, instance, "playback", "confirm", nil, "capture it")
	proctest.RequireStep(t, serve, "verifySummary")
	if got := world.LLM.Calls("preflight"); got != 1 {
		t.Fatalf("pre-flight ran %d times, want 1", got)
	}
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "agent" {
		t.Fatalf("verifySummary must serve an agent chooser, got %+v", serve.PendingChooser)
	}
	if !strings.Contains(serve.Instructions, "A generated summary.") {
		t.Errorf("verifySummary unit should render the stored summary, got %q", serve.Instructions)
	}

	serve = session.Answer(t, instance, "verifySummary", "faithful",
		map[string]any{"fidelityNote": "matches the body"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if !strings.Contains(entry.Content, "the fixture observes something") {
		t.Fatalf("persisted entry body = %q", entry.Content)
	}
}

func TestCapture_SelectedKindPointsAtItsAuthoringFact(t *testing.T) {
	_, session := newCaptureWorld(t, "core-kind")

	serve := session.Start(t, "capture", map[string]any{"kind": "directive"})
	if !strings.Contains(serve.Instructions, "Selected-kind authoring fact") ||
		!strings.Contains(serve.Instructions, basefacts.DirectiveFactID) {
		t.Fatalf("preselected kind guidance = %q", serve.Instructions)
	}
}

func TestCapture_WritingGuideFindingsServeReviewThenProceed(t *testing.T) {
	world, session := newCaptureWorld(t, "core-guide-review")
	world.LLM.GuideFindings = []proctest.GuideFinding{{
		Reasoning: "the body leans on 'the earlier approach' without naming or pointing at it, so a reader outside the dialogue cannot act",
		Axis:      "stranding", Quote: "the earlier approach", Repair: "write-in", Severity: "substantive",
	}}

	serve := session.Start(t, "capture", nil)
	serve = session.Report(t, serve.Instance, captureDraft())
	proctest.RequireStep(t, serve, "guideReview")
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "agent" {
		t.Fatalf("guideReview must serve an agent chooser, got %+v", serve.PendingChooser)
	}
	for _, want := range []string{"stranding", "the earlier approach", "write-in", "substantive", "drafting input"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Errorf("guideReview unit missing %q:\n%s", want, serve.Instructions)
		}
	}
	if got := world.LLM.Calls("writing-guide"); got != 1 {
		t.Fatalf("writing guide ran %d times, want 1", got)
	}

	serve = session.Answer(t, serve.Instance, "guideReview", "proceed", nil, "")
	proctest.RequireStep(t, serve, "playback")
}

func TestCapture_WritingGuideRunsOncePerCapture(t *testing.T) {
	world, session := newCaptureWorld(t, "core-guide-once")
	world.LLM.GuideFindings = []proctest.GuideFinding{{
		Reasoning: "removing the second sentence loses nothing", Axis: "dilution",
		Quote: "the fixture observes something", Repair: "cut", Severity: "minor",
	}}

	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	serve = session.Report(t, instance, captureDraft())
	proctest.RequireStep(t, serve, "guideReview")

	// Revise carries the changed body back through assemble, but the guide
	// judged this capture already — no second run, no second review; the
	// revised draft lands at playback.
	serve = session.Answer(t, instance, "guideReview", "revise",
		map[string]any{"body": "A tactical gap: the fixture observes one thing."}, "")
	proctest.RequireStep(t, serve, "playback")
	if got := world.LLM.Calls("writing-guide"); got != 1 {
		t.Fatalf("writing guide ran %d times, want 1 (once per capture)", got)
	}
	if !strings.Contains(serve.Instructions, "observes one thing") {
		t.Errorf("playback should render the revised body, got %q", serve.Instructions)
	}

	// A playback adjust passes through the guide step without another run.
	serve = session.Answer(t, instance, "playback", "adjust",
		map[string]any{"topics": []any{"cli/ux", "agent/ux"}}, "tweak topics")
	proctest.RequireStep(t, serve, "playback")
	if got := world.LLM.Calls("writing-guide"); got != 1 {
		t.Fatalf("writing guide ran %d times after adjust, want still 1", got)
	}
}

func TestCapture_WritingGuideRecheckRunsFreshOnReworkedDraft(t *testing.T) {
	world, session := newCaptureWorld(t, "core-guide-recheck")
	world.LLM.GuideFindings = []proctest.GuideFinding{{
		Reasoning: "two commitments fused", Axis: "conflation",
		Quote: "the fixture observes something", Repair: "split", Severity: "substantive",
	}}

	serve := session.Start(t, "capture", nil)
	serve = session.Report(t, serve.Instance, captureDraft())
	proctest.RequireStep(t, serve, "guideReview")

	// Recheck discards the recorded run: the reworked draft goes back through
	// assemble and the guide judges it fresh — clean this time.
	world.LLM.GuideFindings = nil
	serve = session.Answer(t, serve.Instance, "guideReview", "recheck",
		map[string]any{"body": "A tactical gap: a completely reworked observation."}, "")
	proctest.RequireStep(t, serve, "playback")
	if got := world.LLM.Calls("writing-guide"); got != 2 {
		t.Fatalf("writing guide ran %d times, want 2 (fresh run after recheck)", got)
	}
}

func TestCapture_PlaybackNamesTargetPrecedenceFromTypedAndServedState(t *testing.T) {
	_, session := newCaptureWorld(t, "core-target-unbound")
	serve := session.Start(t, "capture", nil)
	serve = session.Report(t, serve.Instance, captureDraft())
	proctest.RequireStep(t, serve, "playback")
	for _, want := range []string{
		"explicit typed `captureBranch` wins",
		"current session binding wins",
		"served session framing declares it",
		"configured default branch",
		"Do not invent a branch from cwd",
	} {
		if !strings.Contains(serve.Instructions, want) {
			t.Fatalf("unbound playback missing target-precedence wording %q:\n%s", want, serve.Instructions)
		}
	}

	// An explicit captureBranch routes reads and the eventual write to that
	// branch's graph, so the branch must exist and hold the fixture entries.
	branchDir := t.TempDir()
	for _, e := range captureFixtureEntries() {
		proctest.WriteEntry(t, branchDir, e)
	}
	world := proctest.NewWorld(t,
		proctest.WithEntries(captureFixtureEntries()...),
		proctest.WithBranchDir("feature/work", branchDir))
	explicit := world.Open(t, "core-target-explicit")
	explicit.LogRead(t, "show", []string{captureRefID}, nil)
	serve = explicit.Start(t, "capture", map[string]any{"captureBranch": "feature/work"})
	serve = explicit.Report(t, serve.Instance, captureDraft())
	if !strings.Contains(serve.Instructions, "- target branch: feature/work (explicit `captureBranch`)") {
		t.Fatalf("explicit playback did not render typed captureBranch:\n%s", serve.Instructions)
	}
}

// The non-replay half of the engine's fact-index test: set the index at
// assemble, playback renders it nested, a playback adjust clears it and the
// re-served playback drops it. (Replay durability stays in internal/engine.)
func TestCapture_FactIndexSetThenCleared(t *testing.T) {
	_, session := newCaptureWorld(t, "core-fact-index")

	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	draft := captureDraft()
	draft["entryKind"] = "fact"
	draft["body"] = "Graph views compose from layout expressions; this fact records how."
	draft["index"] = map[string]any{"title": "How to compose graph views", "topic": "cli/ux"}
	serve = session.Report(t, instance, draft)
	proctest.RequireStep(t, serve, "playback")
	const playback = "- index:\n    title: How to compose graph views\n    topic: cli/ux"
	if !strings.Contains(serve.Instructions, playback) {
		t.Fatalf("playback missing nested fact index:\n%s", serve.Instructions)
	}

	serve = session.Answer(t, instance, "playback", "adjust", map[string]any{"index": nil}, "remove the index")
	proctest.RequireStep(t, serve, "playback")
	if strings.Contains(serve.Instructions, "- index:") {
		t.Fatalf("playback retained cleared fact index:\n%s", serve.Instructions)
	}
}

func TestCapture_StallNamesExactlyWhatIsMissing(t *testing.T) {
	_, session := newCaptureWorld(t, "core-stall")
	serve := session.Start(t, "capture", nil)

	// Partial report: no widenReport, no confidence — both required collects,
	// so the step stays and names exactly what's missing.
	draft := captureDraft()
	delete(draft, "widenReport")
	delete(draft, "confidence")
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if got := strings.Join(serve.Missing, ","); got != "confidence,widenReport" {
		t.Fatalf("missing = %q, want confidence,widenReport", got)
	}
	if !strings.Contains(serve.Instructions, "missing: confidence, widenReport") {
		t.Errorf("stall instructions should name missing fields, got %q", serve.Instructions)
	}
}

// TestCapture_GateDelegatesKindRulesToBoundary confirms the assemble gate
// carries no per-kind required-field list of its own: a topic-less ordinary
// draft passes (topics are not a contract-backed requirement), while a done
// draft with no closes and no refs holds on the construction boundary's
// closes-or-refs rule for the done kind.
func TestCapture_GateDelegatesKindRulesToBoundary(t *testing.T) {
	_, session := newCaptureWorld(t, "core-gate-topicless")
	serve := session.Start(t, "capture", nil)
	draft := captureDraft()
	delete(draft, "topics")
	serve = session.Report(t, serve.Instance, draft)
	if serve.Step == "assemble" {
		t.Fatalf("a topic-less ordinary draft must pass assemble, diagnostics=%v", serve.Diagnostics)
	}

	_, session2 := newCaptureWorld(t, "core-gate-done")
	serve2 := session2.Start(t, "capture", nil)
	done := captureDraft()
	done["entryKind"] = "done"
	delete(done, "refs")
	serve2 = session2.Report(t, serve2.Instance, done)
	proctest.RequireStep(t, serve2, "assemble")
	if !hasDiagnostic(serve2, "does not satisfy its kind's structural rules") {
		t.Fatalf("diagnostics = %v, want the draftValidates message (done anchor rule)", serve2.Diagnostics)
	}
}

func TestCapture_HighFindingsRouteToOverride(t *testing.T) {
	world, session := newCaptureWorld(t, "core-override")
	world.LLM.PreflightFindings = []proctest.PreflightFinding{{
		Severity: "high", Category: "test-block", Observation: "blocked until override",
	}}

	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	session.Report(t, instance, captureDraft())
	serve = session.Answer(t, instance, "playback", "confirm", nil, "capture it")
	proctest.RequireStep(t, serve, "reviseOrOverride")
	if !strings.Contains(serve.Instructions, "Pre-flight findings") {
		t.Errorf("reviseOrOverride should render the findings unit, got %q", serve.Instructions)
	}
	if got := world.LLM.Calls("preflight"); got != 1 {
		t.Fatalf("pre-flight ran %d times before override, want 1", got)
	}

	// The override is a user-only chooser exit; it re-runs the write gate with
	// the recorded override, which skips pre-flight and writes the entry.
	serve = session.Answer(t, instance, "reviseOrOverride", "override", nil, "skip it, the finding is wrong")
	proctest.RequireStep(t, serve, "verifySummary")
	if got := world.LLM.Calls("preflight"); got != 1 {
		t.Fatalf("pre-flight ran %d times after override, want still 1 (override skips it)", got)
	}

	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("overridden capture produced no entryId: %+v", serve.Produced)
	}
	proctest.LoadEntry(t, world.GraphDir, entryID)
}

func TestCapture_EditAfterConfirmReopensPlayback(t *testing.T) {
	world, session := newCaptureWorld(t, "core-stale-confirm")
	world.LLM.PreflightFindings = []proctest.PreflightFinding{{
		Severity: "high", Category: "test-block", Observation: "blocked until override",
	}}

	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	session.Report(t, instance, captureDraft())
	serve = session.Answer(t, instance, "playback", "confirm", nil, "capture it")
	proctest.RequireStep(t, serve, "reviseOrOverride")

	// Blocked at reviseOrOverride. The agent edits the body — the confirmed
	// state is now stale.
	session.Report(t, instance, map[string]any{"body": "Edited after confirmation is a stale draft."})

	// Overriding would jump back to the write gate — but the confirmation no
	// longer covers the state, so playback reopens instead of writing.
	serve = session.Answer(t, instance, "reviseOrOverride", "override", nil, "just write it")
	proctest.RequireStep(t, serve, "playback")
	if got := world.LLM.Calls("preflight"); got != 1 {
		t.Fatalf("the write gate must not re-run on a stale confirmation, pre-flight ran %d times", got)
	}
	if id, ok := serve.Produced["entryId"]; ok {
		t.Fatalf("no entry may be written on a stale confirmation, produced %v", id)
	}

	// Re-confirming the edited state completes the write (the recorded
	// override still skips pre-flight).
	serve = session.Answer(t, instance, "playback", "confirm", nil, "yes, with the edit")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("re-confirmed capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if !strings.Contains(entry.Content, "Edited after confirmation") {
		t.Fatalf("persisted entry must carry the re-confirmed body, got %q", entry.Content)
	}
}

func TestCapture_AdjustLoopsBackAndRequiresReconfirm(t *testing.T) {
	_, session := newCaptureWorld(t, "core-adjust")
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	session.Report(t, instance, captureDraft())

	// Adjust with a revised body: back through assemble, fields still
	// complete, so the cascade returns to playback for a fresh confirm.
	serve = session.Answer(t, instance, "playback", "adjust",
		map[string]any{"body": "Sharper first sentence for the fixture observation."}, "tighten it")
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, "Sharper first sentence") {
		t.Errorf("playback should render the adjusted body, got %q", serve.Instructions)
	}
}

func TestCapture_AbortAndAbandon(t *testing.T) {
	_, session := newCaptureWorld(t, "core-abort")
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	session.Report(t, instance, captureDraft())
	serve = session.Answer(t, instance, "playback", "abort", nil, "not now")
	proctest.RequireStatus(t, serve, "abandoned")
	if _, err := session.ReportErr(t, instance, captureDraft()); err == nil {
		t.Error("reporting to an ended instance must fail")
	}

	// Explicit abandon of a second instance.
	serve2 := session.Start(t, "capture", nil)
	if err := abandonInstance(t, session, serve2.Instance, "session over"); err != nil {
		t.Fatal(err)
	}
	if err := abandonInstance(t, session, serve2.Instance, "twice"); err == nil {
		t.Error("double abandon must fail")
	}
}

// abandonInstance abandons one procedure instance through the application's
// workflow surface — the harness wraps Start/Report/Answer but not Abandon.
func abandonInstance(t *testing.T, s *proctest.Session, instance, reason string) error {
	t.Helper()
	_, err := s.WF.Abandon(t.Context(), s.World.Identity, instance, reason)
	return err
}

func TestCapture_ParamsValidatedAtStart(t *testing.T) {
	_, session := newCaptureWorld(t, "core-params")

	if _, err := session.StartErr(t, "capture", map[string]any{"anchor": "not-an-id"}); err == nil ||
		!strings.Contains(err.Error(), "anchor") {
		t.Errorf("malformed param must fail start, got %v", err)
	}
	if _, err := session.StartErr(t, "capture", map[string]any{"unknown": true}); err == nil ||
		!strings.Contains(err.Error(), "unknown start input") {
		t.Errorf("unknown start input must fail start, got %v", err)
	}
	if _, err := session.StartErr(t, "capture", map[string]any{"anchor": captureRefID}); err != nil {
		t.Errorf("valid param rejected: %v", err)
	}
}
