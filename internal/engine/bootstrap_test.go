package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// Per-procedure table tests for the embedded bootstrap entry, driving the
// shipped base entry through the production loader like the other procedure
// tests. The outer loop is exercised directly; capture sub-moves are covered
// in the capture tests, so materialize's dispatch is walked without starting a
// real child except in the seeding test below.

// bootstrapEnv loads the shipped bootstrap spec against a registry with the
// fake viewLayout injection the orient step needs.
type bootstrapEnv struct {
	session *Session
	spec    *Spec
	sink    *memorySink
}

func newBootstrapEnv(t *testing.T) *bootstrapEnv {
	t.Helper()
	reg := NewRegistry()
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "viewLayout", Doc: "fake view pipeline"},
		Fn: func(_ *Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			return "lanes for " + layout, nil
		},
	})
	spec, err := LoadSpec(baseEntry(t, "bootstrap"), reg)
	if err != nil {
		t.Fatalf("loading bootstrap spec: %v", err)
	}
	eng := New(reg, StaticGraphs{Graph: model.NewGraph(nil)})
	sink := &memorySink{}
	ts := time.Date(2026, 7, 19, 14, 0, 0, 0, time.UTC)
	sess := eng.NewSession("s_bootstrap", "christopher", sink, WithClock(func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}))
	return &bootstrapEnv{session: sess, spec: spec, sink: sink}
}

func TestBootstrap_SpecLoadsAndInjectsReadinessAtOrient(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "orient" {
		t.Fatalf("start step = %s, want orient", sv.Step)
	}
	if sv.Chooser == nil || sv.Chooser.Kind != ChooserAgent {
		t.Fatalf("orient must serve an agent chooser, got %+v", sv.Chooser)
	}
	if !strings.Contains(sv.Instructions, "lanes for readiness") {
		t.Errorf("orient should inject the readiness view, got %q", sv.Instructions)
	}
}

func TestBootstrap_RecognitionModeDefaultsTrue(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst, _ := env.session.Instance(sv.Instance)
	v, ok := inst.Store.Get("recognitionMode")
	if !ok || v != true {
		t.Fatalf("bootstrap state should carry a constant recognitionMode=true, got %v (present=%v)", v, ok)
	}
}

// TestBootstrap_HappyPath walks the full outer loop: orient inspect →
// brownfield → converse cluster → propose accept → materialize captureEntry ×1
// → clusterDone → foundTopics founded → refresh continue (loop) → converse
// finish → handoff brief → end(completed).
func TestBootstrap_HappyPath(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	sv = answerAgent(t, env, sv, "orient", "inspect", map[string]any{"readinessSynthesis": "fresh graph, every lane empty"})
	if sv.Step != "brownfield" {
		t.Fatalf("inspect should route to brownfield, got %q", sv.Step)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{"brownfieldSynthesis": "a Go CLI, Devbox toolchain, recent commits by Christopher"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "converse" {
		t.Fatalf("brownfield synthesis should advance to converse, got %q", sv.Step)
	}

	sv = answerAgent(t, env, sv, "converse", "cluster", map[string]any{"candidateCluster": "an aspiration and Christopher's actor+role"})
	if sv.Step != "propose" || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("cluster should route to the propose user junction, got %q", sv.Step)
	}

	sv = answerUser(t, env, sv, "propose", "accept", nil, "yes, capture it")
	if sv.Step != "materialize" {
		t.Fatalf("accept should route to materialize, got %q", sv.Step)
	}

	sv = answerAgent(t, env, sv, "materialize", "captureEntry", map[string]any{"producedIds": "grounding: searched the empty graph"})
	if sv.Step != "materialize" {
		t.Fatalf("captureEntry should loop back to materialize, got %q", sv.Step)
	}

	sv = answerAgent(t, env, sv, "materialize", "clusterDone", map[string]any{"producedIds": "20260719-150000-s-prc-abc, 20260719-150005-d-prc-def"})
	if sv.Step != "foundTopics" {
		t.Fatalf("clusterDone should route to foundTopics, got %q", sv.Step)
	}

	sv = answerAgent(t, env, sv, "foundTopics", "founded", map[string]any{"topicLandscape": "product/vision, team/people"})
	if sv.Step != "refresh" || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("founded should route to the refresh user junction, got %q", sv.Step)
	}

	// continue loops back to converse for another round.
	sv = answerUser(t, env, sv, "refresh", "continue", map[string]any{"direction": "keep going on the team"}, "let's keep going")
	if sv.Step != "converse" {
		t.Fatalf("refresh continue should loop back to converse, got %q", sv.Step)
	}

	// This time finish → handoff.
	sv = answerAgent(t, env, sv, "converse", "finish", nil)
	if sv.Step != "handoff" || sv.Chooser.Kind != ChooserAgent {
		t.Fatalf("finish should route to the handoff agent chooser, got %q", sv.Step)
	}

	sv = answerAgent(t, env, sv, "handoff", "brief", nil)
	if sv.Status != StatusCompleted {
		t.Fatalf("brief should complete the bootstrap run, got %s at %q", sv.Status, sv.Step)
	}
}

func TestBootstrap_OrientProceedSkipsBrownfield(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv = answerAgent(t, env, sv, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield, nothing in the repo to read"})
	if sv.Step != "converse" {
		t.Fatalf("proceed should go straight to converse, got %q", sv.Step)
	}
}

func TestBootstrap_ConverseAbortAbandons(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv = answerAgent(t, env, sv, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield"})
	sv = answerAgent(t, env, sv, "converse", "abort", nil)
	if sv.Status != StatusAbandoned {
		t.Fatalf("abort should abandon the run, got %s at %q", sv.Status, sv.Step)
	}
}

func TestBootstrap_ProposeReshapeAndDefer(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv = answerAgent(t, env, sv, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield"})
	sv = answerAgent(t, env, sv, "converse", "cluster", map[string]any{"candidateCluster": "first draft"})

	// reshape loops back to propose with a revised cluster.
	sv = answerUser(t, env, sv, "propose", "reshape", map[string]any{"candidateCluster": "revised draft"}, "reword the pull")
	if sv.Step != "propose" {
		t.Fatalf("reshape should loop back to propose, got %q", sv.Step)
	}

	// defer folds back into the conversation, nothing captured.
	sv = answerUser(t, env, sv, "propose", "defer", map[string]any{"phaseSynthesis": "not ready yet"}, "let's keep talking")
	if sv.Step != "converse" {
		t.Fatalf("defer should return to converse, got %q", sv.Step)
	}
}

// TestBootstrap_MaterializeSeedsGroundingAndRecognition asserts the shipped
// dispatch contract: captureEntry dispatches capture, seeding the grounding
// (widenReport <- producedIds) and the recognition flag (recognitionMode <-
// recognitionMode). Spec-level, so drift between the entry and the seeding
// machinery cannot pass unnoticed.
func TestBootstrap_MaterializeSeedsGroundingAndRecognition(t *testing.T) {
	env := newBootstrapEnv(t)
	var opt *Option
	for _, step := range env.spec.Steps {
		if step.ID != "materialize" {
			continue
		}
		for i := range step.Options {
			if step.Options[i].Choice == "captureEntry" {
				opt = &step.Options[i]
			}
		}
	}
	if opt == nil || opt.Dispatch == nil {
		t.Fatal("materialize captureEntry must declare a dispatch")
	}
	if opt.Dispatch.Procedure != "capture" {
		t.Errorf("dispatch procedure = %q, want capture", opt.Dispatch.Procedure)
	}
	if opt.Dispatch.Seed["widenReport"] != "producedIds" {
		t.Errorf("seed widenReport = %q, want producedIds", opt.Dispatch.Seed["widenReport"])
	}
	if opt.Dispatch.Seed["recognitionMode"] != "recognitionMode" {
		t.Errorf("seed recognitionMode = %q, want recognitionMode", opt.Dispatch.Seed["recognitionMode"])
	}
}

// TestBootstrap_ColdResumeRetainsState replays the event log from a partway run
// and confirms the resumed instance recovers its step and last-beat state —
// the honest resume promise (graph plus state as of the last report beat).
func TestBootstrap_ColdResumeRetainsState(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv = answerAgent(t, env, sv, "orient", "inspect", map[string]any{"readinessSynthesis": "empty graph"})
	sv, err = env.session.Report(sv.Instance, map[string]any{"brownfieldSynthesis": "a Go CLI"})
	if err != nil {
		t.Fatal(err)
	}
	// Park the running move — forensic, state kept — then replay cold.
	if err := env.session.Park(sv.Instance, "shelved mid-bootstrap"); err != nil {
		t.Fatal(err)
	}

	resolve := func(canonical string) (*Spec, error) { return env.spec, nil }
	replayed, err := env.session.engine.ReplaySession("s_bootstrap", "christopher", env.sink.events, resolve, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	inst, ok := replayed.Instance(sv.Instance)
	if !ok {
		t.Fatal("replayed session lost the bootstrap instance")
	}
	if inst.Step != "converse" {
		t.Fatalf("resumed step = %q, want converse (post-brownfield)", inst.Step)
	}
	if got, _ := inst.Store.Get("brownfieldSynthesis"); got != "a Go CLI" {
		t.Errorf("resumed brownfieldSynthesis = %v, want the reported value", got)
	}
	if got, _ := inst.Store.Get("recognitionMode"); got != true {
		t.Errorf("resumed recognitionMode = %v, want the re-applied default true", got)
	}
}

func answerAgent(t *testing.T, env *bootstrapEnv, sv *Serve, chooser, choice string, fields ...map[string]any) *Serve {
	t.Helper()
	var f map[string]any
	if len(fields) > 0 {
		f = fields[0]
	}
	out, err := env.session.Answer(sv.Instance, chooser, choice, f, "")
	if err != nil {
		t.Fatalf("answer %s/%s: %v", chooser, choice, err)
	}
	return out
}

func answerUser(t *testing.T, env *bootstrapEnv, sv *Serve, chooser, choice string, fields map[string]any, words string) *Serve {
	t.Helper()
	out, err := env.session.Answer(sv.Instance, chooser, choice, fields, words)
	if err != nil {
		t.Fatalf("answer %s/%s: %v", chooser, choice, err)
	}
	return out
}
