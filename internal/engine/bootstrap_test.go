package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
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

// TestBootstrap_SecondClusterInSameRun walks two clusters within one run: the
// first founds the topic landscape, the second skips founding (already done)
// and the run still completes — multiple clusters, one lens.
func TestBootstrap_SecondClusterInSameRun(t *testing.T) {
	env := newBootstrapEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv = answerAgent(t, env, sv, "orient", "proceed", map[string]any{"readinessSynthesis": "greenfield"})

	// First cluster: capture it, then found the topic landscape.
	sv = answerAgent(t, env, sv, "converse", "cluster", map[string]any{"candidateCluster": "first cluster"})
	sv = answerUser(t, env, sv, "propose", "accept", nil, "capture it")
	sv = answerAgent(t, env, sv, "materialize", "captureEntry", map[string]any{"producedIds": "grounding"})
	sv = answerAgent(t, env, sv, "materialize", "clusterDone", map[string]any{"producedIds": "id1"})
	sv = answerAgent(t, env, sv, "foundTopics", "founded", map[string]any{"topicLandscape": "team/people"})
	sv = answerUser(t, env, sv, "refresh", "continue", map[string]any{"direction": "another corner"}, "keep going")
	if sv.Step != "converse" {
		t.Fatalf("continue should return to converse for a second cluster, got %q", sv.Step)
	}

	// Second cluster in the same run: landscape already founded, so skip.
	sv = answerAgent(t, env, sv, "converse", "cluster", map[string]any{"candidateCluster": "second cluster"})
	sv = answerUser(t, env, sv, "propose", "accept", nil, "yes")
	sv = answerAgent(t, env, sv, "materialize", "captureEntry", map[string]any{"producedIds": "id1"})
	sv = answerAgent(t, env, sv, "materialize", "clusterDone", map[string]any{"producedIds": "id1, id2"})
	if sv.Step != "foundTopics" {
		t.Fatalf("second clusterDone should reach foundTopics, got %q", sv.Step)
	}
	sv = answerAgent(t, env, sv, "foundTopics", "skip", nil)
	if sv.Step != "refresh" {
		t.Fatalf("skip should pass through to refresh, got %q", sv.Step)
	}
	sv = answerUser(t, env, sv, "refresh", "finish", nil, "done for now")
	sv = answerAgent(t, env, sv, "handoff", "brief", nil)
	if sv.Status != StatusCompleted {
		t.Fatalf("two-cluster run should complete, got %s at %q", sv.Status, sv.Step)
	}
}

// mutableGraphs is a Graphs whose backing graph the fake write gate grows, so a
// second capture can resolve a ref to an entry the first capture just produced.
type mutableGraphs struct{ g *model.Graph }

func (m *mutableGraphs) Current() (*model.Graph, error) { return m.g, nil }
func (m *mutableGraphs) Invalidate()                    {}

// bootstrapCaptureEnv loads bootstrap and capture against one registry over a
// mutable graph, with a fake newEntry that appends the drafted entry so
// dependency-ordered refs across dispatched captures resolve.
type bootstrapCaptureEnv struct {
	session   *Session
	bootstrap *Spec
	capture   *Spec
	graph     *mutableGraphs
	newCalls  int
}

func newBootstrapCaptureEnv(t *testing.T) *bootstrapCaptureEnv {
	t.Helper()
	env := &bootstrapCaptureEnv{graph: &mutableGraphs{g: model.NewGraph(nil)}}
	reg := NewRegistry()
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "viewLayout", Doc: "graph-reflecting fake view"},
		Fn: func(ctx *Context, _ map[string]any) (any, error) {
			var names []string
			for _, a := range ctx.Graph.ActiveActorHeads() {
				names = append(names, a.Canonical)
			}
			return "actors: " + strings.Join(names, ", "), nil
		},
	})
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "generatedSummary", Doc: "fake stored summary", Reads: []string{"entryId"}},
		Fn: func(ctx *Context, _ map[string]any) (any, error) {
			id, _ := ctx.Store.Get("entryId")
			s, _ := id.(string)
			if e, ok := ctx.Graph.ByID[s]; ok {
				return e.Summary, nil
			}
			return "", nil
		},
	})
	mustRegisterCommand(reg, Command{
		Doc:          FuncDoc{Name: "newEntry", Doc: "graph-appending fake write gate", Writes: []string{"entryId", "findings"}},
		MutatesGraph: true,
		Fn:           env.appendEntry,
	})
	mustRegisterCommand(reg, Command{
		Doc: FuncDoc{Name: "replaceSummary", Doc: "fake", Reads: []string{"entryId", "correctedSummary"}},
		Fn:  func(*Context) error { return nil },
	})

	var err error
	if env.bootstrap, err = LoadSpec(baseEntry(t, "bootstrap"), reg); err != nil {
		t.Fatalf("loading bootstrap: %v", err)
	}
	if env.capture, err = LoadSpec(baseEntry(t, "capture"), reg); err != nil {
		t.Fatalf("loading capture: %v", err)
	}
	eng := New(reg, env.graph)
	ts := time.Date(2026, 7, 19, 16, 0, 0, 0, time.UTC)
	env.session = eng.NewSession("s_bc", "christopher", &memorySink{}, WithClock(func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}))
	return env
}

// appendEntry is the fake newEntry: it materializes the drafted entry into the
// mutable graph with a real, resolvable ID, mirroring the trust machinery the
// real write gate writes (entryId + empty findings) and logging the read so a
// later capture's refsInspected gate sees it.
func (env *bootstrapCaptureEnv) appendEntry(ctx *Context) error {
	env.newCalls++
	kind, _ := ctx.Store.Get("entryKind")
	kindStr, _ := kind.(string)
	layer, _ := ctx.Store.Get("layer")
	layerStr, _ := layer.(string)
	body, _ := ctx.Store.Get("body")
	bodyStr, _ := body.(string)

	entryType, typeCode := model.TypeDecision, "d"
	if model.IsValidKindForType(model.TypeSignal, model.Kind(kindStr)) {
		entryType, typeCode = model.TypeSignal, "s"
	}
	layerCode := map[string]string{"process": "prc", "tactical": "tac", "strategic": "stg", "conceptual": "cpt", "operational": "ops"}[layerStr]
	id := fmt.Sprintf("20260719-1600%02d-%s-%s-x%02d", env.newCalls, typeCode, layerCode, env.newCalls)
	parts, err := model.ParseID(id)
	if err != nil {
		return err
	}
	e := &model.Entry{ID: id, Type: entryType, Kind: model.Kind(kindStr), Layer: model.Layer(layerStr), Content: bodyStr, Time: parts.Time, Summary: "summary of " + id}
	if c, ok := ctx.Store.Get("canonical"); ok {
		e.Canonical, _ = c.(string)
	}
	if a, ok := ctx.Store.Get("aliases"); ok {
		e.Aliases = asStrings(a)
	}
	if ra, ok := ctx.Store.Get("roleActor"); ok {
		e.Actor, _ = ra.(string)
	}
	if rv, ok := ctx.Store.Get("refs"); ok {
		for _, r := range asRefs(rv) {
			e.Refs = append(e.Refs, model.Ref{ID: r.ID, Kind: model.RefKind(r.Kind), Desc: r.Desc})
		}
	}
	env.graph.g = model.NewGraph(append(env.graph.g.Entries, e))
	ctx.Store.WriteEngine("findings", []query.Finding{})
	ctx.Store.WriteEngine("entryId", id)
	env.session.LogRead("newEntry", []string{id}, nil)
	return nil
}

// driveCapture starts a capture child under parent, drafts it, confirms, and
// verifies the summary — returning the produced entry ID. It fatals if the
// draft doesn't reach playback, so a blocked assemble gate is a test failure.
func (env *bootstrapCaptureEnv) driveCapture(t *testing.T, parent string, draft map[string]any) string {
	t.Helper()
	sv, err := env.session.Start(env.capture, nil, parent)
	if err != nil {
		t.Fatalf("start capture child: %v", err)
	}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatalf("capture draft: %v", err)
	}
	if sv.Step != "playback" {
		t.Fatalf("capture draft should reach playback, got %q failing=%+v missing=%v", sv.Step, sv.Failing, sv.Missing)
	}
	sv, err = env.session.Answer(sv.Instance, "playback", "confirm", nil, "yes")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if sv.Step != "verifySummary" {
		t.Fatalf("confirm should reach verifySummary, got %q", sv.Step)
	}
	inst, _ := env.session.Instance(sv.Instance)
	idVal, _ := inst.Store.Get("entryId")
	id, _ := idVal.(string)
	sv, err = env.session.Answer(sv.Instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "faithful"}, "")
	if err != nil {
		t.Fatalf("faithful: %v", err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("capture child should complete, got %s at %q", sv.Status, sv.Step)
	}
	return id
}

// TestBootstrap_DependencyOrderedRefsAcrossCaptures materializes an actor, then
// a signal that refs it by its produced ID: the second capture's resolve-or-
// block gate passes only because the first entry now exists in the graph.
func TestBootstrap_DependencyOrderedRefsAcrossCaptures(t *testing.T) {
	env := newBootstrapCaptureEnv(t)
	must := func(sv *Serve, err error) *Serve {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return sv
	}
	sv, err := env.session.Start(env.bootstrap, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	bs := sv.Instance
	must(env.session.Answer(bs, "orient", "proceed", map[string]any{"readinessSynthesis": "empty graph"}, ""))
	must(env.session.Answer(bs, "converse", "cluster", map[string]any{"candidateCluster": "an actor plus a gap about it"}, "cluster"))
	must(env.session.Answer(bs, "propose", "accept", nil, "capture both"))

	// First entry: the actor, captured before anything references it.
	must(env.session.Answer(bs, "materialize", "captureEntry", map[string]any{"producedIds": "grounding: searched the empty graph"}, ""))
	actorID := env.driveCapture(t, bs, map[string]any{
		"body": "Ada, a contributor from outside the project.", "entryKind": "actor", "layer": "process", "canonical": "Ada", "confidence": "high",
	})

	// Second entry: a gap that refs the actor by its produced ID. Its assemble
	// gate resolves that ref only because the actor was captured first.
	must(env.session.Answer(bs, "materialize", "captureEntry", map[string]any{"producedIds": "captured " + actorID}, ""))
	gapID := env.driveCapture(t, bs, map[string]any{
		"body": "A gap about Ada's onboarding, referencing " + actorID + ".", "entryKind": "gap", "layer": "tactical",
		"refs":       []any{map[string]any{"id": actorID, "kind": "related"}},
		"topics":     []any{"team/people"},
		"confidence": "medium",
	})

	if _, ok := env.graph.g.ByID[actorID]; !ok {
		t.Fatalf("actor %s not in the graph after capture", actorID)
	}
	if _, ok := env.graph.g.ByID[gapID]; !ok {
		t.Fatalf("gap %s not in the graph after capture", gapID)
	}
	gap := env.graph.g.ByID[gapID]
	if len(gap.Refs) != 1 || gap.Refs[0].ID != actorID {
		t.Fatalf("gap should ref the actor by its produced ID, got refs %+v", gap.Refs)
	}

	sv = must(env.session.Answer(bs, "materialize", "clusterDone", map[string]any{"producedIds": actorID + ", " + gapID}, ""))
	if sv.Step != "foundTopics" {
		t.Fatalf("clusterDone should reach foundTopics, got %q", sv.Step)
	}
}

// TestBootstrap_RepeatedFreshRunReflectsPopulatedGraph completes a run that
// captures an actor, then starts a fresh run on the now-populated graph: the
// second run's orient readiness view reflects the captured actor and the run
// proceeds normally.
func TestBootstrap_RepeatedFreshRunReflectsPopulatedGraph(t *testing.T) {
	env := newBootstrapCaptureEnv(t)
	must := func(sv *Serve, err error) *Serve {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return sv
	}

	// Run 1: orient sees an empty graph, then a cluster captures actor Ada.
	sv, err := env.session.Start(env.bootstrap, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	bs1 := sv.Instance
	if !strings.Contains(sv.Instructions, "actors: ") || strings.Contains(sv.Instructions, "Ada") {
		t.Fatalf("run 1 orient should read an empty graph, got %q", sv.Instructions)
	}
	must(env.session.Answer(bs1, "orient", "proceed", map[string]any{"readinessSynthesis": "empty"}, ""))
	must(env.session.Answer(bs1, "converse", "cluster", map[string]any{"candidateCluster": "Ada the contributor"}, "cluster"))
	must(env.session.Answer(bs1, "propose", "accept", nil, "yes"))
	must(env.session.Answer(bs1, "materialize", "captureEntry", map[string]any{"producedIds": "grounding"}, ""))
	env.driveCapture(t, bs1, map[string]any{"body": "Ada, a contributor.", "entryKind": "actor", "layer": "process", "canonical": "Ada", "confidence": "high"})
	must(env.session.Answer(bs1, "materialize", "clusterDone", map[string]any{"producedIds": "Ada"}, ""))
	must(env.session.Answer(bs1, "foundTopics", "founded", map[string]any{"topicLandscape": "team/people"}, ""))
	must(env.session.Answer(bs1, "refresh", "finish", nil, "done for now"))
	sv = must(env.session.Answer(bs1, "handoff", "brief", nil, ""))
	if sv.Status != StatusCompleted {
		t.Fatalf("run 1 should complete, got %s at %q", sv.Status, sv.Step)
	}

	// Run 2 on the populated graph: orient's readiness view now reflects Ada.
	sv, err = env.session.Start(env.bootstrap, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	bs2 := sv.Instance
	if !strings.Contains(sv.Instructions, "Ada") {
		t.Fatalf("run 2 orient should reflect the captured actor, got %q", sv.Instructions)
	}
	// And it proceeds normally to a clean finish.
	must(env.session.Answer(bs2, "orient", "proceed", map[string]any{"readinessSynthesis": "Ada is known now"}, ""))
	must(env.session.Answer(bs2, "converse", "finish", nil, ""))
	sv = must(env.session.Answer(bs2, "handoff", "brief", nil, ""))
	if sv.Status != StatusCompleted {
		t.Fatalf("run 2 should complete normally, got %s at %q", sv.Status, sv.Step)
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
