package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
	sdd "github.com/networkteam/sdd/pkg/application"
)

const evalAnchorID = "20260601-121500-d-tac-anc"

func evalAnchorEntry() *model.Entry {
	return &model.Entry{
		ID: evalAnchorID, Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
		Summary: "A directive the evaluation anchors on.",
		Content: "A directive the evaluation anchors on.",
	}
}

// startChild starts a procedure under a parent instance, exercising the real
// dispatch-seed handoff. Package-local helper (shared by the per-procedure
// suites): the harness Session.Start carries no parent parameter.
func startChild(t *testing.T, s *proctest.Session, canonical, parent string, params map[string]any) *sdd.WorkflowServe {
	t.Helper()
	serve, err := s.WF.Start(t.Context(), s.World.Identity, sdd.WorkflowStartRequest{Canonical: canonical, Params: params, Parent: parent})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

func missingContains(serve *sdd.WorkflowServe, field string) bool {
	for _, m := range serve.Missing {
		if m == field {
			return true
		}
	}
	return false
}

func TestEvaluateHappyPath(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(evalAnchorEntry()))
	session := world.Open(t, "evaluate-happy")

	serve := session.Start(t, "evaluate", map[string]any{"anchor": evalAnchorID})
	proctest.RequireStep(t, serve, "scope")
	instance := serve.Instance
	if !strings.Contains(serve.Instructions, evalAnchorID) || !strings.Contains(serve.Instructions, "A directive the evaluation anchors on") {
		t.Errorf("scope unit should serve the anchor's real chain, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "verification") || !strings.Contains(serve.Instructions, "validation") {
		t.Errorf("scope unit should name the lens postures, got %q", serve.Instructions)
	}

	serve = session.Report(t, instance, map[string]any{
		"plan":        "Inner only: check the done's claims against the ACs and the project's Go guidelines.",
		"widenReport": "searched for post-landing signals and prior evaluation dones; none recorded yet",
	})
	proctest.RequireStep(t, serve, "carryOut")

	serve = session.Report(t, instance, map[string]any{
		"innerEvidence":   "read the diff against the ACs; go test ./... green",
		"innerEvaluation": "sound — matches the ACs; one rough edge in teardown",
	})
	proctest.RequireStep(t, serve, "junction")
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("carried-out evaluation should reach the junction user chooser, got %+v", serve.PendingChooser)
	}

	serve = session.Answer(t, instance, "junction", "record",
		map[string]any{"selectedFindings": "the teardown rough edge"}, "record the teardown finding")
	proctest.RequireStatus(t, serve, "completed")
}

func TestEvaluateAnchorResolvedByResolver(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(evalAnchorEntry()))
	session := world.Open(t, "evaluate-resolver")

	// The uniform anchor contract: a cold start with no anchor does not fail —
	// it stalls at the resolver step, naming the anchor as what advances it.
	serve, err := session.StartErr(t, "evaluate", nil)
	if err != nil {
		t.Fatalf("start without anchor should stall at the resolver, not error: %v", err)
	}
	proctest.RequireStep(t, serve, "anchor")
	if !missingContains(serve, "anchor") {
		t.Errorf("resolver should name anchor as missing, got %v", serve.Missing)
	}

	// A resolved anchor advances to scope — the same place a seeded anchor
	// auto-advances to on entry.
	serve = session.Report(t, serve.Instance, map[string]any{"anchor": evalAnchorID})
	proctest.RequireStep(t, serve, "scope")
}

func TestEvaluateLensGate(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(evalAnchorEntry()))
	session := world.Open(t, "evaluate-lens-gate")

	serve := session.Start(t, "evaluate", map[string]any{"anchor": evalAnchorID})
	instance := serve.Instance
	serve = session.Report(t, instance, map[string]any{
		"plan":        "both lenses",
		"widenReport": "widened",
	})
	proctest.RequireStep(t, serve, "carryOut")

	// Evidence alone is not a judgment — the gate holds until at least one
	// lens evaluation lands (evidence is instructed, never gated).
	serve = session.Report(t, instance, map[string]any{
		"innerEvidence": "ran the suite",
		"outerEvidence": "smoke test output",
	})
	proctest.RequireStep(t, serve, "carryOut")

	// A single lens judgment satisfies the gate — outer alone here; coverage
	// completeness is a graph property, not a per-run gate.
	serve = session.Report(t, instance, map[string]any{
		"outerEvaluation": "works in use; the user attested the flow end to end",
	})
	proctest.RequireStep(t, serve, "junction")
}

func TestEvaluateCleanPassRecordsWithoutFindings(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(evalAnchorEntry()))
	session := world.Open(t, "evaluate-clean-pass")

	serve := session.Start(t, "evaluate", map[string]any{"anchor": evalAnchorID})
	instance := serve.Instance
	session.Report(t, instance, map[string]any{
		"plan":        "inner only",
		"widenReport": "nothing new since landing",
	})
	serve = session.Report(t, instance, map[string]any{
		"innerEvaluation": "clean — claims match the commitment",
	})
	proctest.RequireStep(t, serve, "junction")

	// A clean pass records the evaluation done alone: record with no
	// selectedFindings is a valid answer (the field is optional).
	serve = session.Answer(t, instance, "junction", "record", nil, "record the evaluation, no findings")
	proctest.RequireStatus(t, serve, "completed")
}

func TestEvaluateConclude(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(evalAnchorEntry()))
	session := world.Open(t, "evaluate-conclude")

	serve := session.Start(t, "evaluate", map[string]any{"anchor": evalAnchorID})
	instance := serve.Instance
	session.Report(t, instance, map[string]any{
		"plan":        "outer only",
		"widenReport": "widened",
	})
	serve = session.Report(t, instance, map[string]any{
		"outerEvaluation": "fine in use",
	})
	proctest.RequireStep(t, serve, "junction")

	serve = session.Answer(t, instance, "junction", "conclude", nil, "don't record this one")
	proctest.RequireStatus(t, serve, "completed")
}

// The record option's declared capture handoff, exercised behaviorally: the
// answered dispatch seeds the evaluation's widenReport into a capture child,
// and that child is the real capture procedure — the evaluation done passes
// the real construction boundary (done needs closes or refs) and lands on
// disk. Replaces the old spec-level Dispatch inspection.
func TestEvaluateRecordDispatchesRealCapture(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(evalAnchorEntry()))
	session := world.Open(t, "evaluate-record-capture")

	serve := session.Start(t, "evaluate", map[string]any{"anchor": evalAnchorID})
	instance := serve.Instance
	widenReport := "searched for post-landing signals and prior evaluation dones; none recorded yet"
	session.Report(t, instance, map[string]any{
		"plan":        "inner only: verify the directive's realization against the project's guidelines",
		"widenReport": widenReport,
	})
	serve = session.Report(t, instance, map[string]any{
		"innerEvidence":   "read the realization in full; suite green",
		"innerEvaluation": "sound under the project's guidelines",
	})
	proctest.RequireStep(t, serve, "junction")
	serve = session.Answer(t, instance, "junction", "record", nil, "record the evaluation")
	proctest.RequireStatus(t, serve, "completed")

	capture := startChild(t, session, "capture", instance, nil)
	proctest.RequireStep(t, capture, "assemble")
	if missingContains(capture, "widenReport") {
		t.Fatalf("widenReport should be seeded from the evaluation's dispatch, missing %v", capture.Missing)
	}
	if !strings.Contains(capture.Instructions, widenReport) {
		t.Errorf("assemble should render the inherited grounding, got %q", capture.Instructions)
	}

	// The anchor was served in full by the evaluation's scope inject, so the
	// ref passes the session-level inspection gate without a fresh read.
	serve = session.Report(t, capture.Instance, map[string]any{
		"body":       "The anchored directive was verified against the project's guidelines and found sound; this records the evaluation as coverage.",
		"entryKind":  "done",
		"layer":      "tactical",
		"refs":       []any{map[string]any{"id": evalAnchorID, "kind": "builds-on"}},
		"topics":     []any{"evaluation/inner"},
		"confidence": "medium",
	})
	proctest.RequireStep(t, serve, "playback")
	if world.LLM.Calls("writing-guide") != 1 {
		t.Fatalf("writing guide ran %d times, want 1", world.LLM.Calls("writing-guide"))
	}

	serve = session.Answer(t, capture.Instance, "playback", "confirm", nil, "record it")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, capture.Instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")

	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if entry.Kind != "done" {
		t.Errorf("persisted kind = %q, want done", entry.Kind)
	}
	if len(entry.Refs) != 1 || entry.Refs[0].ID != evalAnchorID {
		t.Errorf("persisted refs = %+v, want the evaluated anchor", entry.Refs)
	}
}
