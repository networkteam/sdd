package proctest_test

// Ported from internal/engine/bootstrap_test.go: the same bootstrap behavior,
// driven through the real application — real registry, real ops, real embedded
// procedures, real graph. The engine suite's spec-structure seeding test is
// ported behaviorally: a capture child started under the bootstrap instance
// shows the seeded widenReport and recognitionMode.

import (
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

const bootstrapAspirationID = "20260601-090000-d-stg-asp"

func bootstrapAspirationFixture() *model.Entry {
	return &model.Entry{
		ID: bootstrapAspirationID, Type: model.TypeDecision, Kind: model.KindAspiration, Layer: model.LayerStrategic,
		Summary: "Make project reasoning a shared searchable record.",
		Content: "Make project reasoning a shared searchable record.",
	}
}

// startBootstrapChild starts a procedure under an explicit parent instance. The harness
// Start always parents on the session shell; dispatched captures hang off the
// bootstrap instance, so this goes through WF.Start directly. Candidate to
// graduate into the harness.
func startBootstrapChild(t *testing.T, s *proctest.Session, canonical, parent string) *sdd.WorkflowServe {
	t.Helper()
	serve, err := s.WF.Start(t.Context(), s.World.Identity, sdd.WorkflowStartRequest{Canonical: canonical, Parent: parent})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

// bootstrapCollected returns an open instance's gathered values via the resume
// projection — the harness has no store peek, so state assertions read the
// surface a re-attaching agent would.
func bootstrapCollected(t *testing.T, s *proctest.Session, instance string) map[string]any {
	t.Helper()
	all, err := s.WF.ServeAll(t.Context(), s.World.Identity)
	if err != nil {
		t.Fatal(err)
	}
	for _, serve := range all.Open {
		if serve.Instance == instance {
			return serve.Collected
		}
	}
	t.Fatalf("instance %s is not open", instance)
	return nil
}

func requireBootstrapChooser(t *testing.T, serve *sdd.WorkflowServe, kind string) {
	t.Helper()
	if serve.PendingChooser == nil || string(serve.PendingChooser.Kind) != kind {
		t.Fatalf("step %s should serve a %s chooser, got %+v", serve.Step, kind, serve.PendingChooser)
	}
}

// driveBootstrapCapture runs one dispatched capture child under parent through
// the full real path — assemble, guide, playback, write, summary — returning
// the produced entry ID. widenReport arrives seeded from the parent's
// producedIds, so the draft does not report it.
func driveBootstrapCapture(t *testing.T, s *proctest.Session, parent string, draft map[string]any) string {
	t.Helper()
	serve := startBootstrapChild(t, s, "capture", parent)
	proctest.RequireStep(t, serve, "assemble")
	instance := serve.Instance
	serve = s.Report(t, instance, draft)
	proctest.RequireStep(t, serve, "playback")
	serve = s.Answer(t, instance, "playback", "confirm", nil, "yes")
	if serve.Step != "verifySummary" {
		t.Fatalf("confirm should reach verifySummary, got %q: %s", serve.Step, serve.Instructions)
	}
	serve = s.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "faithful"}, "")
	proctest.RequireStatus(t, serve, "completed")
	id, _ := serve.Produced["entryId"].(string)
	if id == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}
	return id
}

func TestBootstrap_SpecLoadsAndInjectsReadinessAtOrient(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(bootstrapAspirationFixture()))
	session := world.Open(t, "boot-orient")

	serve := session.Start(t, "bootstrap", nil)
	proctest.RequireStep(t, serve, "orient")
	requireBootstrapChooser(t, serve, "agent")
	if !strings.Contains(serve.Instructions, "Aspirations") {
		t.Errorf("orient should inject the readiness view lanes, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "shared searchable record") {
		t.Errorf("orient readiness view should carry the fixture aspiration, got %q", serve.Instructions)
	}
}

func TestBootstrap_RecognitionModeDefaultsTrue(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-recognition")

	serve := session.Start(t, "bootstrap", nil)
	values := bootstrapCollected(t, session, serve.Instance)
	if v, ok := values["recognitionMode"]; !ok || v != true {
		t.Fatalf("bootstrap state should carry a constant recognitionMode=true, got %v (present=%v)", v, ok)
	}
}

// TestBootstrap_HappyPath walks the full outer loop: orient inspect →
// brownfield → converse cluster → propose accept → materialize captureEntry ×1
// → clusterDone → foundTopics founded → refresh continue (loop) → converse
// finish → handoff brief → end(completed).
func TestBootstrap_HappyPath(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-happy")

	serve := session.Start(t, "bootstrap", nil)
	proctest.RequireStep(t, serve, "orient")
	bs := serve.Instance

	serve = session.Answer(t, bs, "orient", "inspect", map[string]any{"readinessSynthesis": "fresh graph, every lane empty"}, "")
	proctest.RequireStep(t, serve, "brownfield")

	serve = session.Report(t, bs, map[string]any{"brownfieldSynthesis": "a Go CLI, Devbox toolchain, recent commits by Christopher"})
	proctest.RequireStep(t, serve, "converse")

	serve = session.Answer(t, bs, "converse", "cluster", map[string]any{"candidateCluster": "an aspiration and Christopher's actor+role"}, "")
	proctest.RequireStep(t, serve, "propose")
	requireBootstrapChooser(t, serve, "user")

	serve = session.Answer(t, bs, "propose", "accept", nil, "yes, capture it")
	proctest.RequireStep(t, serve, "materialize")

	serve = session.Answer(t, bs, "materialize", "captureEntry", map[string]any{"producedIds": "grounding: searched the empty graph"}, "")
	proctest.RequireStep(t, serve, "materialize")

	serve = session.Answer(t, bs, "materialize", "clusterDone", map[string]any{"producedIds": "20260719-150000-s-prc-abc, 20260719-150005-d-prc-def"}, "")
	proctest.RequireStep(t, serve, "foundTopics")

	serve = session.Answer(t, bs, "foundTopics", "founded", map[string]any{"topicLandscape": "product/vision, team/people"}, "")
	proctest.RequireStep(t, serve, "refresh")
	requireBootstrapChooser(t, serve, "user")

	serve = session.Answer(t, bs, "refresh", "continue", map[string]any{"direction": "keep going on the team"}, "let's keep going")
	proctest.RequireStep(t, serve, "converse")

	serve = session.Answer(t, bs, "converse", "finish", nil, "")
	proctest.RequireStep(t, serve, "handoff")
	requireBootstrapChooser(t, serve, "agent")

	serve = session.Answer(t, bs, "handoff", "brief", nil, "")
	proctest.RequireStatus(t, serve, "completed")
}

func TestBootstrap_OrientProceedSkipsBrownfield(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-proceed")

	serve := session.Start(t, "bootstrap", nil)
	serve = session.Answer(t, serve.Instance, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield, nothing in the repo to read"}, "")
	proctest.RequireStep(t, serve, "converse")
}

func TestBootstrap_ConverseAbortAbandons(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-abort")

	serve := session.Start(t, "bootstrap", nil)
	bs := serve.Instance
	session.Answer(t, bs, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield"}, "")
	serve = session.Answer(t, bs, "converse", "abort", nil, "")
	proctest.RequireStatus(t, serve, "abandoned")
}

func TestBootstrap_ProposeReshapeAndDefer(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-reshape")

	serve := session.Start(t, "bootstrap", nil)
	bs := serve.Instance
	session.Answer(t, bs, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield"}, "")
	session.Answer(t, bs, "converse", "cluster", map[string]any{"candidateCluster": "first draft"}, "")

	serve = session.Answer(t, bs, "propose", "reshape", map[string]any{"candidateCluster": "revised draft"}, "reword the pull")
	proctest.RequireStep(t, serve, "propose")

	serve = session.Answer(t, bs, "propose", "defer", map[string]any{"phaseSynthesis": "not ready yet"}, "let's keep talking")
	proctest.RequireStep(t, serve, "converse")
}

// TestBootstrap_MaterializeSeedsGroundingAndRecognition asserts the shipped
// dispatch contract through the real registry: answering captureEntry records
// the dispatch, and a capture child started under the bootstrap instance
// receives the grounding (widenReport <- producedIds) and the recognition flag
// (recognitionMode <- recognitionMode).
func TestBootstrap_MaterializeSeedsGroundingAndRecognition(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-seeding")

	serve := session.Start(t, "bootstrap", nil)
	bs := serve.Instance
	session.Answer(t, bs, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield"}, "")
	session.Answer(t, bs, "converse", "cluster", map[string]any{"candidateCluster": "one actor"}, "")
	session.Answer(t, bs, "propose", "accept", nil, "capture it")
	session.Answer(t, bs, "materialize", "captureEntry", map[string]any{"producedIds": "grounding: searched the empty graph"}, "")

	child := startBootstrapChild(t, session, "capture", bs)
	values := bootstrapCollected(t, session, child.Instance)
	if got := values["widenReport"]; got != "grounding: searched the empty graph" {
		t.Errorf("dispatched capture should be seeded widenReport <- producedIds, got %v", got)
	}
	if got := values["recognitionMode"]; got != true {
		t.Errorf("dispatched capture should be seeded recognitionMode <- recognitionMode, got %v", got)
	}
}

// TestBootstrap_ColdResumeRetainsState re-attaches from a second connection —
// the real replay of the durable event log — and confirms the bootstrap
// instance recovers its step and last-beat state.
func TestBootstrap_ColdResumeRetainsState(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-resume-a")

	serve := session.Start(t, "bootstrap", nil)
	bs := serve.Instance
	session.Answer(t, bs, "orient", "inspect", map[string]any{"readinessSynthesis": "empty graph"}, "")
	serve = session.Report(t, bs, map[string]any{"brownfieldSynthesis": "a Go CLI"})
	proctest.RequireStep(t, serve, "converse")

	_, result := world.Resume(t, session.ID, "boot-resume-b")
	var boot *sdd.WorkflowServe
	for i := range result.Open {
		if result.Open[i].Instance == bs {
			boot = &result.Open[i]
		}
	}
	if boot == nil {
		t.Fatal("resumed session lost the bootstrap instance")
	}
	if boot.Step != "converse" {
		t.Fatalf("resumed step = %q, want converse (post-brownfield)", boot.Step)
	}
	if got := boot.Collected["brownfieldSynthesis"]; got != "a Go CLI" {
		t.Errorf("resumed brownfieldSynthesis = %v, want the reported value", got)
	}
	if got := boot.Collected["recognitionMode"]; got != true {
		t.Errorf("resumed recognitionMode = %v, want the re-applied default true", got)
	}
}

// TestBootstrap_SecondClusterInSameRun walks two clusters within one run: the
// first founds the topic landscape, the second skips founding (already done)
// and the run still completes — multiple clusters, one lens.
func TestBootstrap_SecondClusterInSameRun(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-two-clusters")

	serve := session.Start(t, "bootstrap", nil)
	bs := serve.Instance
	session.Answer(t, bs, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield"}, "")

	session.Answer(t, bs, "converse", "cluster", map[string]any{"candidateCluster": "first cluster"}, "")
	session.Answer(t, bs, "propose", "accept", nil, "capture it")
	session.Answer(t, bs, "materialize", "captureEntry", map[string]any{"producedIds": "grounding"}, "")
	session.Answer(t, bs, "materialize", "clusterDone", map[string]any{"producedIds": "id1"}, "")
	session.Answer(t, bs, "foundTopics", "founded", map[string]any{"topicLandscape": "team/people"}, "")
	serve = session.Answer(t, bs, "refresh", "continue", map[string]any{"direction": "another corner"}, "keep going")
	proctest.RequireStep(t, serve, "converse")

	session.Answer(t, bs, "converse", "cluster", map[string]any{"candidateCluster": "second cluster"}, "")
	session.Answer(t, bs, "propose", "accept", nil, "yes")
	session.Answer(t, bs, "materialize", "captureEntry", map[string]any{"producedIds": "id1"}, "")
	serve = session.Answer(t, bs, "materialize", "clusterDone", map[string]any{"producedIds": "id1, id2"}, "")
	proctest.RequireStep(t, serve, "foundTopics")
	serve = session.Answer(t, bs, "foundTopics", "skip", nil, "")
	proctest.RequireStep(t, serve, "refresh")
	session.Answer(t, bs, "refresh", "finish", nil, "done for now")
	serve = session.Answer(t, bs, "handoff", "brief", nil, "")
	proctest.RequireStatus(t, serve, "completed")
}

// TestBootstrap_DependencyOrderedRefsAcrossCaptures materializes an actor, then
// a signal that refs it by its produced ID: the second capture's resolve-or-
// block gate passes only because the first entry now exists on disk — the real
// graph grows under the run.
func TestBootstrap_DependencyOrderedRefsAcrossCaptures(t *testing.T) {
	// The participant is Ada: the real write gate's participant-drift check
	// requires the session participant to be an active actor canonical once any
	// actor exists, so the first capture records the participant's own actor —
	// the shape a real bootstrap takes.
	world := proctest.NewWorld(t, proctest.WithParticipant("Ada"))
	session := world.Open(t, "boot-deps")

	serve := session.Start(t, "bootstrap", nil)
	bs := serve.Instance
	session.Answer(t, bs, "orient", "proceed", map[string]any{"readinessSynthesis": "empty graph"}, "")
	session.Answer(t, bs, "converse", "cluster", map[string]any{"candidateCluster": "an actor plus a gap about it"}, "")
	session.Answer(t, bs, "propose", "accept", nil, "capture both")

	session.Answer(t, bs, "materialize", "captureEntry", map[string]any{"producedIds": "grounding: searched the empty graph"}, "")
	actorID := driveBootstrapCapture(t, session, bs, map[string]any{
		"body": "Ada, a contributor from outside the project.", "entryKind": "actor", "layer": "process",
		"canonical": "Ada", "confidence": "high",
	})

	// The gap refs the actor by its produced ID. Its assemble gate resolves
	// that ref only because the actor's entry landed first; the write-gate read
	// log covers refsInspected.
	session.Answer(t, bs, "materialize", "captureEntry", map[string]any{"producedIds": "captured " + actorID}, "")
	gapID := driveBootstrapCapture(t, session, bs, map[string]any{
		"body": "A gap about Ada's onboarding, referencing " + actorID + ".", "entryKind": "gap", "layer": "tactical",
		"refs":       []any{map[string]any{"id": actorID, "kind": "related"}},
		"topics":     []any{"team/people"},
		"confidence": "medium",
	})

	actor := proctest.LoadEntry(t, world.GraphDir, actorID)
	if actor.Canonical != "Ada" {
		t.Fatalf("actor canonical = %q, want Ada", actor.Canonical)
	}
	gap := proctest.LoadEntry(t, world.GraphDir, gapID)
	if len(gap.Refs) != 1 || gap.Refs[0].ID != actorID {
		t.Fatalf("gap should ref the actor by its produced ID, got refs %+v", gap.Refs)
	}

	serve = session.Answer(t, bs, "materialize", "clusterDone", map[string]any{"producedIds": actorID + ", " + gapID}, "")
	proctest.RequireStep(t, serve, "foundTopics")
}

// TestBootstrap_RepeatedFreshRunReflectsPopulatedGraph completes a run that
// captures an actor, then starts a fresh run on the now-populated graph: the
// second run's orient readiness view reflects the captured actor and the run
// proceeds normally.
func TestBootstrap_RepeatedFreshRunReflectsPopulatedGraph(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "boot-rerun")

	serve := session.Start(t, "bootstrap", nil)
	bs1 := serve.Instance
	if strings.Contains(serve.Instructions, "Ada") {
		t.Fatalf("run 1 orient should read an empty graph, got %q", serve.Instructions)
	}
	session.Answer(t, bs1, "orient", "proceed", map[string]any{"readinessSynthesis": "empty"}, "")
	session.Answer(t, bs1, "converse", "cluster", map[string]any{"candidateCluster": "Ada the contributor"}, "")
	session.Answer(t, bs1, "propose", "accept", nil, "yes")
	session.Answer(t, bs1, "materialize", "captureEntry", map[string]any{"producedIds": "grounding"}, "")
	adaID := driveBootstrapCapture(t, session, bs1, map[string]any{
		"body": "Ada, a contributor.", "entryKind": "actor", "layer": "process", "canonical": "Ada", "confidence": "high",
	})
	session.Answer(t, bs1, "materialize", "clusterDone", map[string]any{"producedIds": adaID}, "")
	session.Answer(t, bs1, "foundTopics", "founded", map[string]any{"topicLandscape": "team/people"}, "")
	session.Answer(t, bs1, "refresh", "finish", nil, "done for now")
	serve = session.Answer(t, bs1, "handoff", "brief", nil, "")
	proctest.RequireStatus(t, serve, "completed")

	serve = session.Start(t, "bootstrap", nil)
	bs2 := serve.Instance
	if !strings.Contains(serve.Instructions, "Ada") {
		t.Fatalf("run 2 orient should reflect the captured actor, got %q", serve.Instructions)
	}
	session.Answer(t, bs2, "orient", "proceed", map[string]any{"readinessSynthesis": "Ada is known now"}, "")
	session.Answer(t, bs2, "converse", "finish", nil, "")
	serve = session.Answer(t, bs2, "handoff", "brief", nil, "")
	proctest.RequireStatus(t, serve, "completed")
}
