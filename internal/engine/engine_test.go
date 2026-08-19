package engine

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// The fixture spec is a synthetic capture-shaped procedure owned by this test
// package: collect fields across the domain types, a predicate-gated step, a
// once-only op, user and agent choosers, a guarded write op, and an injected
// serve — every engine feature the tests below exercise, with no coupling to
// a shipped procedure or an application registry contract. Shipped-procedure
// behavior is tested in internal/proctest against the real application.
const fixtureSpecID = "20260702-120000-d-prc-fix"

const fixtureSpecFrontmatter = `params:
    anchor: {type: entry-id, optional: true, desc: entry this test capture is anchored on}
state:
    body: {type: text, desc: entry description}
    entryKind: {type: entry-kind, desc: signal or decision kind}
    layer: {type: layer, desc: the layer}
    refs: {type: list<ref>, desc: refs}
    closes: {type: list<entry-id>, optional: true, desc: entries this draft resolves}
    supersedes: {type: list<entry-id>, optional: true, desc: entries this draft replaces}
    topics: {type: list<label>, desc: topic labels}
    index: {type: fact-index, optional: true, desc: fact retrieval cue}
    confidence: {type: confidence, desc: honest confidence}
    intent: {type: intent, optional: true, desc: directive intent}
    widenReport: {type: text, desc: grounding evidence}
    fidelityNote: {type: text, optional: true, desc: fidelity note}
    correctedSummary: {type: text, optional: true, desc: corrected summary}
steps:
    - id: assemble
      collect: [body, entryKind, layer, "refs?", "closes?", "supersedes?", "topics?", "index?", confidence, "intent?", widenReport]
      transitions:
          - when: hasBody and hasWidenReport
                  and refsResolve and refKindsValid and refsInspected
                  and participantsCanonical
            to: guide
    - id: guide
      op: fakeGuide
      transitions:
          - when: noGuideFindings
            to: playback
          - when: guideReviewed
            to: playback
          - otherwise: guideReview
    - id: guideReview
      chooser: agent
      options:
          - {choice: revise, collect: ["body?", "refs?", "topics?"], call: recordGuideReview, to: assemble}
          - {choice: proceed, call: recordGuideReview, to: playback}
          - {choice: recheck, collect: ["body?"], call: requestGuideRecheck, to: assemble}
    - id: playback
      chooser: user
      options:
          - {choice: confirm, call: confirmPlayback, to: write}
          - {choice: adjust, collect: ["body?", "refs?", "topics?", "index?"], to: assemble}
          - {choice: abort, to: end(abandoned)}
    - id: write
      guard: playbackConfirmed
      op: fakeWrite
      transitions:
          - when: noHighFindings
            to: verifySummary
          - otherwise: reviseOrOverride
    - id: reviseOrOverride
      chooser: user
      render: findings
      options:
          - {choice: revise, collect: ["body?"], to: assemble}
          - {choice: override, call: recordOverride, to: write}
          - {choice: abort, to: end(abandoned)}
    - id: verifySummary
      chooser: agent
      inject:
          - {fn: fakeSummary}
      options:
          - {choice: faithful, collect: [fidelityNote], to: end(completed)}
          - {choice: drifted, collect: [correctedSummary], call: fakeReplaceSummary, to: end(completed)}
`

const fixtureSpecBody = `A synthetic capture-shaped spec for engine tests.

## unit: assemble

Draft the test entry.

## unit: playback

Play back: {{.body}}
{{if .index}}- index:
    title: {{.index.title}}
    topic: {{.index.topic}}
{{end}}
## unit: findings

Findings blocked the write.

## unit: verifySummary

Verify: {{.fakeSummary}}
`

const fixtureRefID = "20260601-120000-d-tac-ref"

// fixtureRef2ID resolves in the fixture graph but is never logged as read —
// the refsInspected gate tests draft against it.
const fixtureRef2ID = "20260601-130000-s-tac-raw"

// fixtureGraph returns a graph holding the entries the fixture refs resolve
// against. No actor signals — participantsCanonical runs in grace mode.
func fixtureGraph(t *testing.T) *model.Graph {
	t.Helper()
	target, err := model.ParseEntry(fixtureRefID+".md", `---
type: decision
layer: tac
kind: directive
intent: pending
---

A directive the fixture capture refs.
`)
	if err != nil {
		t.Fatal(err)
	}
	uninspected, err := model.ParseEntry(fixtureRef2ID+".md", `---
type: signal
layer: tac
kind: gap
---

A signal no fixture session has read in full.
`)
	if err != nil {
		t.Fatal(err)
	}
	return model.NewGraph([]*model.Entry{target, uninspected})
}

// fixtureEnv is the test harness around one engine + session: a registry
// with fake spec ops (fakeWrite, fakeGuide, fakeReplaceSummary, fakeSummary)
// plus call counters for side-effect assertions.
type fixtureEnv struct {
	engine   *Engine
	spec     *Spec
	session  *Session
	sink     *memorySink
	newCalls int
	// highFindingsUnlessOverride makes fakeWrite return a high finding until
	// the preflight override is recorded.
	highFindingsUnlessOverride bool
	replaceCalls               int
	guideCalls                 int
	// guideFindings is what the fakeGuide op returns; nil means a clean pass
	// (empty findings, straight to playback).
	guideFindings []query.GuideFinding
}

type memorySink struct {
	events   []Event
	failWith error
}

func (m *memorySink) Append(e Event) error {
	if m.failWith != nil {
		return m.failWith
	}
	m.events = append(m.events, e)
	return nil
}

func newFixtureEnv(t *testing.T) *fixtureEnv {
	t.Helper()
	env := &fixtureEnv{}

	entry := procedureEntry(t, fixtureSpecID, "fixturecap", "", fixtureSpecFrontmatter, fixtureSpecBody)

	reg := NewRegistry()
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "fakeSummary", Doc: "fake stored summary", Reads: []string{"entryId"}},
		Fn: func(ctx *Context, _ map[string]any) (any, error) {
			id, _ := ctx.Store.Get("entryId")
			return fmt.Sprintf("summary of %v", id), nil
		},
	})
	mustRegisterCommand(reg, Command{
		Doc: FuncDoc{Name: "fakeWrite", Doc: "fake write gate", Writes: []string{"entryId", "findings"}},
		Fn: func(ctx *Context) error {
			env.newCalls++
			findings := []query.Finding{}
			if env.highFindingsUnlessOverride {
				if _, overridden := ctx.Store.Get(fieldPreflightOverride); !overridden {
					findings = append(findings, query.Finding{
						Severity:    query.SeverityHigh,
						Category:    "test-block",
						Observation: "blocked until override",
					})
				}
			}
			if err := ctx.Store.WriteEngine("findings", findings); err != nil {
				return err
			}
			if len(findings) == 0 {
				return ctx.Store.WriteEngine("entryId", fmt.Sprintf("20260702-13000%d-s-tac-new", env.newCalls))
			}
			return nil
		},
	})
	mustRegisterCommand(reg, Command{
		Doc: FuncDoc{Name: "fakeReplaceSummary", Doc: "fake summary replacement", Reads: []string{"entryId", "correctedSummary"}},
		Fn: func(_ *Context) error {
			env.replaceCalls++
			return nil
		},
	})
	mustRegisterCommand(reg, Command{
		Doc: FuncDoc{Name: "fakeGuide", Doc: "fake writing guide (once-only, like the real op)", Reads: []string{"body", "entryKind", "layer", "refs", "intent"}, Writes: []string{"guideFindings"}},
		Fn: func(ctx *Context) error {
			if v, ok := ctx.Store.Get("guideFindings"); ok && v != nil {
				return nil
			}
			env.guideCalls++
			findings := env.guideFindings
			if findings == nil {
				findings = []query.GuideFinding{}
			}
			return ctx.Store.WriteEngine("guideFindings", findings)
		},
	})

	spec, err := LoadSpec(entry, reg)
	if err != nil {
		t.Fatal(err)
	}

	env.engine = New(reg, StaticGraphs{Graph: fixtureGraph(t)})
	env.spec = spec
	env.sink = &memorySink{}
	ts := time.Date(2026, 7, 2, 23, 0, 0, 0, time.UTC)
	env.session = env.engine.NewSession("s_test", "christopher", env.sink, WithClock(func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}))
	// The fixture session inspected its ref in full — matching fullDraft's
	// widenReport claim — so the refsInspected gate passes. Gate tests draft
	// against fixtureRef2ID, which is never logged.
	env.session.LogRead("show", []string{fixtureRefID}, nil)
	return env
}

// fullDraft is the one-shot batched report covering every assemble field.
func fullDraft() map[string]any {
	return map[string]any{
		"body":        "A tactical gap: the fixture observes something.",
		"entryKind":   "gap",
		"layer":       "tac",
		"refs":        []any{map[string]any{"id": fixtureRefID, "kind": "addresses", "desc": "realizes it"}},
		"topics":      []any{"cli/ux"},
		"confidence":  "medium",
		"widenReport": "searched from three angles, inspected d-tac-ref",
	}
}

func TestTemplateValueCollisionFailsRender(t *testing.T) {
	entry := procedureEntry(t, "20260819-100000-d-prc-col", "collide", "", seedChildFrontmatter, "collision fixture\n\n## unit: draft\n\nDraft.\n")
	reg := NewRegistry()
	spec, err := LoadSpec(entry, reg)
	if err != nil {
		t.Fatal(err)
	}
	eng := New(reg, StaticGraphs{Graph: model.NewGraph(nil)}, WithTemplateValues(map[string]any{
		"body": "shadowing a declared state field",
	}))
	session := eng.NewSession("s_col", "tester", &memorySink{})
	if _, err := session.Start(spec, nil, ""); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("a template value shadowing declared state must fail the render, got err=%v", err)
	}
}

func TestTemplateValuesReachUnitAndInjectArgs(t *testing.T) {
	const frontmatter = `state:
    body: {type: text, desc: the draft body}
steps:
    - id: draft
      collect: [body]
      inject:
          - {fn: echoArg, args: {value: "{{.fixtureValue}}"}}
      transitions:
          - when: hasBody
            to: end(completed)
`
	entry := procedureEntry(t, "20260819-110000-d-prc-tv", "tmplvalues", "", frontmatter, "fixture\n\n## unit: draft\n\nvalue={{.fixtureValue}} echoed={{.echoArg}}\n")
	reg := NewRegistry()
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "echoArg", Doc: "echoes its value arg"},
		Fn: func(_ *Context, args map[string]any) (any, error) {
			value, _ := args["value"].(string)
			return "arg:" + value, nil
		},
	})
	spec, err := LoadSpec(entry, reg)
	if err != nil {
		t.Fatal(err)
	}
	eng := New(reg, StaticGraphs{Graph: model.NewGraph(nil)}, WithTemplateValues(map[string]any{
		"fixtureValue": "from-engine",
	}))
	session := eng.NewSession("s_tv", "tester", &memorySink{})
	sv, err := session.Start(spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.Instructions, "value=from-engine") {
		t.Errorf("unit template should see the engine template value, got %q", sv.Instructions)
	}
	if !strings.Contains(sv.Instructions, "echoed=arg:from-engine") {
		t.Errorf("inject-arg templates should see the engine template value, got %q", sv.Instructions)
	}
}

func TestFactIndexSetThenClearSurvivesReplay(t *testing.T) {
	env := newFixtureEnv(t)
	var log strings.Builder
	env.session.sink = NewWriterSink(&log)

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := fullDraft()
	draft["entryKind"] = "fact"
	draft["index"] = map[string]any{"title": "How to compose graph views", "topic": "cli/ux"}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	const playback = "- index:\n    title: How to compose graph views\n    topic: cli/ux"
	if !strings.Contains(sv.Instructions, playback) {
		t.Fatalf("playback missing nested fact index:\n%s", sv.Instructions)
	}
	index, ok := env.session.instances[sv.Instance].Store.Get("index")
	if !ok || !reflect.DeepEqual(index, map[string]any{"title": "How to compose graph views", "topic": "cli/ux"}) {
		t.Fatalf("stored index = %#v", index)
	}
	sv, err = env.session.Answer(sv.Instance, "playback", "adjust", map[string]any{"index": nil}, "remove the index")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sv.Instructions, "- index:") {
		t.Fatalf("playback retained cleared fact index:\n%s", sv.Instructions)
	}

	events, err := ReadEvents(strings.NewReader(log.String()))
	if err != nil {
		t.Fatal(err)
	}
	replayedEnv := newFixtureEnv(t)
	replayed, err := replayedEnv.engine.ReplaySession("s_test", "christopher", events,
		func(string) (*Spec, error) { return replayedEnv.spec, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	instance, ok := replayed.Instance(sv.Instance)
	if !ok {
		t.Fatal("replayed capture instance missing")
	}
	if index, ok := instance.Store.Get("index"); ok {
		t.Fatalf("replayed index = %#v, want cleared", index)
	}
	replayedServe, err := replayed.Serve(sv.Instance)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(replayedServe.Instructions, "- index:") {
		t.Fatalf("replayed playback retained cleared fact index:\n%s", replayedServe.Instructions)
	}
}

func TestReportCannotWriteUndeclaredOrTrustFields(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	for field, value := range map[string]any{
		"entryId":                 "20260702-140000-s-tac-fake", // engine-written
		fieldPlaybackConfirmation: map[string]any{"snapshot": "forged"},
		"findings":                []any{},
		"nonsense":                "x",
	} {
		if _, err := env.session.Report(sv.Instance, map[string]any{field: value}); err == nil {
			t.Errorf("report writing %q must be rejected", field)
		} else if !strings.Contains(err.Error(), "not declared in state") {
			t.Errorf("report writing %q: unexpected error %v", field, err)
		}
	}

	// Params are start-time only.
	if _, err := env.session.Report(sv.Instance, map[string]any{"anchor": fixtureRefID}); err == nil ||
		!strings.Contains(err.Error(), "param") {
		t.Errorf("report writing a param must name the param rule, got %v", err)
	}
}

func TestChooserSequenceCannotBeGamed(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst := sv.Instance

	// Early: no chooser pending at the assemble gate.
	if _, err := env.session.Answer(inst, "playback", "confirm", nil, "yes"); err == nil ||
		!strings.Contains(err.Error(), "gate") {
		t.Errorf("early answer must be rejected, got %v", err)
	}

	if _, err := env.session.Report(inst, fullDraft()); err != nil {
		t.Fatal(err)
	}

	// Wrong chooser name.
	if _, err := env.session.Answer(inst, "verifySummary", "faithful", nil, ""); err == nil ||
		!strings.Contains(err.Error(), `pending chooser is "playback"`) {
		t.Errorf("wrong chooser must be rejected naming the pending one, got %v", err)
	}
	// Unknown choice.
	if _, err := env.session.Answer(inst, "playback", "maybe", nil, ""); err == nil ||
		!strings.Contains(err.Error(), "not an option") {
		t.Errorf("unknown choice must be rejected, got %v", err)
	}
	// Fields outside the option's collect list.
	if _, err := env.session.Answer(inst, "playback", "adjust",
		map[string]any{"widenReport": "smuggled"}, ""); err == nil ||
		!strings.Contains(err.Error(), "not collected by option") {
		t.Errorf("fields outside the option collect must be rejected, got %v", err)
	}

	// Confirm, then try to answer playback again — late/double.
	if _, err := env.session.Answer(inst, "playback", "confirm", nil, "yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Answer(inst, "playback", "confirm", nil, "yes again"); err == nil ||
		!strings.Contains(err.Error(), `pending chooser is "verifySummary"`) {
		t.Errorf("double answer must be rejected, got %v", err)
	}
}

func TestSession_ParentLinksAndInterleaving(t *testing.T) {
	env := newFixtureEnv(t)
	sv1, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv2, err := env.session.Start(env.spec, nil, sv1.Instance)
	if err != nil {
		t.Fatal(err)
	}
	inst2, _ := env.session.Instance(sv2.Instance)
	if inst2.Parent != sv1.Instance {
		t.Errorf("parent link = %q, want %q", inst2.Parent, sv1.Instance)
	}
	if _, err := env.session.Start(env.spec, nil, "i_99"); err == nil {
		t.Error("unknown parent must fail")
	}

	// Serial interleaving: report to either instance independently.
	if _, err := env.session.Report(sv1.Instance, fullDraft()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Report(sv2.Instance, map[string]any{"body": "child draft"}); err != nil {
		t.Fatal(err)
	}
	i1, _ := env.session.Instance(sv1.Instance)
	i2, _ := env.session.Instance(sv2.Instance)
	if i1.Step != "playback" || i2.Step != "assemble" {
		t.Errorf("instances advance independently: i1=%s i2=%s", i1.Step, i2.Step)
	}
}

func TestSession_SinkFailureBlocksAdvance(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	env.sink.failWith = fmt.Errorf("disk full")
	if _, err := env.session.Report(sv.Instance, fullDraft()); err != nil {
		t.Fatal(err) // the append failure is deferred, the report itself lands
	}
	if _, err := env.session.Report(sv.Instance, fullDraft()); err == nil ||
		!strings.Contains(err.Error(), "durability") {
		t.Errorf("advance after a failed append must refuse, got %v", err)
	}
}

func TestRunCommandDiscardsStoreWritesOnError(t *testing.T) {
	env := newFixtureEnv(t)
	mustRegisterCommand(env.engine.Registry, Command{
		Doc: FuncDoc{Name: "failAfterWrite", Doc: "test failure", Writes: []string{"marker", "leaked", "composite"}},
		Fn: func(ctx *Context) error {
			composite, _ := ctx.Store.Get("composite")
			composite.([]any)[0] = "changed"
			if err := ctx.Store.WriteEngine("marker", "changed"); err != nil {
				return err
			}
			if err := ctx.Store.WriteEngine("leaked", true); err != nil {
				return err
			}
			return fmt.Errorf("injected command failure")
		},
	})
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst, _ := env.session.Instance(sv.Instance)
	if err := inst.Store.WriteEngine("marker", "original"); err != nil {
		t.Fatal(err)
	}
	if err := inst.Store.WriteEngine("composite", []any{"original"}); err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(env.sink.events)
	if err := env.session.runCommand(inst, "failAfterWrite"); err == nil || !strings.Contains(err.Error(), "injected command failure") {
		t.Fatalf("runCommand error = %v", err)
	}
	if marker, _ := inst.Store.Get("marker"); marker != "original" {
		t.Fatalf("failed command changed existing value to %v", marker)
	}
	if _, exists := inst.Store.Get("leaked"); exists {
		t.Fatal("failed command left a new engine value")
	}
	if composite, _ := inst.Store.Get("composite"); !reflect.DeepEqual(composite, []any{"original"}) {
		t.Fatalf("failed command mutated composite value: %#v", composite)
	}
	if len(env.sink.events) != beforeEvents {
		t.Fatal("failed command appended an op result")
	}
}

func TestRunCommandCannotWriteReportState(t *testing.T) {
	env := newFixtureEnv(t)
	var writeErr error
	mustRegisterCommand(env.engine.Registry, Command{
		Doc: FuncDoc{Name: "attemptStateWrite", Doc: "test command boundary"},
		Fn: func(ctx *Context) error {
			_, writeErr = ctx.Store.WriteState(map[string]any{"body": "hidden state write"})
			return nil
		},
	})
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst, _ := env.session.Instance(sv.Instance)
	beforeEvents := len(env.sink.events)
	if err := env.session.runCommand(inst, "attemptStateWrite"); err != nil {
		t.Fatal(err)
	}
	if writeErr == nil || !strings.Contains(writeErr.Error(), "only through WriteEngine") {
		t.Fatalf("WriteState error = %v", writeErr)
	}
	if _, exists := inst.Store.Get("body"); exists {
		t.Fatal("command produced an unlogged state write")
	}
	if len(env.sink.events) != beforeEvents+1 {
		t.Fatalf("command events = %d, want one op result", len(env.sink.events)-beforeEvents)
	}
}
