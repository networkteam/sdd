package engine_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

// laneEntry composes a synthetic procedure entry from a frontmatter fragment
// (params/state/steps YAML) and a body.
func laneEntry(t *testing.T, machine, body string) *model.Entry {
	t.Helper()
	content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: lanesproc\n" +
		machine + "\n---\n\n" + body
	entry, err := model.ParseEntry("20260826-120000-d-prc-lan.md", content)
	if err != nil {
		t.Fatalf("fixture entry: %v", err)
	}
	return entry
}

// laneMachine is the standard one-step machine: a gate stalled on hasBody, so
// the opening serve renders the draft unit.
const laneMachine = `state:
    note: {type: text, desc: x}
steps:
    - id: draft
      collect: [note]
      transitions:
          - when: hasBody
            to: end(completed)`

// injectMachine is laneMachine with the given inject lines (pre-indented
// `          - {…}` flow maps) on the draft step.
func injectMachine(injects string) string {
	return `state:
    note: {type: text, desc: x}
steps:
    - id: draft
      collect: [note]
      inject:
` + injects + `
      transitions:
          - when: hasBody
            to: end(completed)`
}

func mustParseLanes(t *testing.T, machine, body string) *engine.Spec {
	t.Helper()
	spec, err := engine.ParseSpec(laneEntry(t, machine, body))
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	return spec
}

func parseFails(t *testing.T, machine, body string) error {
	t.Helper()
	_, err := engine.ParseSpec(laneEntry(t, machine, body))
	if err == nil {
		t.Fatal("ParseSpec succeeded, want a load problem")
	}
	return err
}

func requireProblem(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("err = %v, want a load problem containing %q", err, substr)
	}
}

// requireUnitLanes asserts the unit's lanes carry exactly these names, in order.
func requireUnitLanes(t *testing.T, spec *engine.Spec, unit string, names ...string) {
	t.Helper()
	got := make([]string, 0, len(spec.Units[unit].Lanes))
	for _, l := range spec.Units[unit].Lanes {
		got = append(got, l.Name)
	}
	if !slices.Equal(got, names) {
		t.Fatalf("unit %s lanes = %v, want %v", unit, got, names)
	}
}

// requireServeLanes asserts the serve's rendered lanes carry exactly these
// names, in order.
func requireServeLanes(t *testing.T, sv *engine.Serve, names ...string) {
	t.Helper()
	got := make([]string, 0, len(sv.Lanes))
	for _, l := range sv.Lanes {
		got = append(got, l.Name)
	}
	if !slices.Equal(got, names) {
		t.Fatalf("serve lanes = %v, want %v", got, names)
	}
}

func serveLaneText(t *testing.T, sv *engine.Serve, name string) string {
	t.Helper()
	for _, l := range sv.Lanes {
		if l.Name == name {
			return l.Text
		}
	}
	t.Fatalf("serve has no lane %q, lanes: %+v", name, sv.Lanes)
	return ""
}

type nopSink struct{}

func (nopSink) Append(engine.Event) error { return nil }

// startServe loads machine+body against reg and returns a fresh session's
// opening serve — the draft gate stalls on the missing note, so it serves the
// draft unit's rendered lanes.
func startServe(t *testing.T, reg *engine.Registry, machine, body string) *engine.Serve {
	t.Helper()
	spec, err := engine.LoadSpec(laneEntry(t, machine, body), reg)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.New(reg, engine.StaticGraphs{Graph: model.NewGraph(nil)})
	sv, err := eng.NewSession("s_lanes", "tester", nopSink{}).Start(spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return sv
}

// --- parseUnits through ParseSpec ----------------------------------------

func TestUnitWithoutMarkersIsOneImplicitLane(t *testing.T) {
	spec := mustParseLanes(t, laneMachine, "## unit: draft\n\nDo the draft.\n")
	requireUnitLanes(t, spec, "draft", "draft")
	if got := spec.Units["draft"].Lanes[0].Text; got != "Do the draft." {
		t.Errorf("implicit lane text = %q", got)
	}
}

func TestLaneMarkersSplitUnitIntoOrderedLanes(t *testing.T) {
	body := "## unit: draft\n\n### lane: intro\n\nIntro text.\n\n### lane: detail\n\nDetail text.\n"
	spec := mustParseLanes(t, laneMachine, body)
	requireUnitLanes(t, spec, "draft", "intro", "detail")
}

func TestTextBeforeFirstLaneMarkerIsAProblem(t *testing.T) {
	err := parseFails(t, laneMachine, "## unit: draft\n\nstray prose\n\n### lane: intro\n\nIntro.\n")
	requireProblem(t, err, "unit draft: text before the first `### lane:` marker")
}

func TestDuplicateLaneNameIsAProblem(t *testing.T) {
	err := parseFails(t, laneMachine, "## unit: draft\n\n### lane: intro\n\nOne.\n\n### lane: intro\n\nTwo.\n")
	requireProblem(t, err, `unit draft: duplicate lane "intro"`)
}

func TestLaneMarkerInsideFenceIsContent(t *testing.T) {
	body := "## unit: draft\n\nQuote:\n\n```\n### lane: ghost\n```\n\nAfter.\n"
	spec := mustParseLanes(t, laneMachine, body)
	requireUnitLanes(t, spec, "draft", "draft")
	if !strings.Contains(spec.Units["draft"].Lanes[0].Text, "### lane: ghost") {
		t.Errorf("fenced marker must stay unit content, got %q", spec.Units["draft"].Lanes[0].Text)
	}
}

// --- inject and framing ids ------------------------------------------------

func TestSameFnInjectsWithoutIdsAreAProblem(t *testing.T) {
	err := parseFails(t, injectMachine("          - {fn: fakeQ}\n          - {fn: fakeQ}"), "")
	requireProblem(t, err, `duplicate inject id "fakeQ"`)
}

func TestSameFnInjectsWithDistinctIdsParse(t *testing.T) {
	spec := mustParseLanes(t, injectMachine("          - {id: first, fn: fakeQ}\n          - {id: second, fn: fakeQ}"), "")
	inj := spec.StepByID["draft"].Inject
	if len(inj) != 2 || inj[0].EffectiveID() != "first" || inj[1].EffectiveID() != "second" {
		t.Errorf("inject calls = %+v, want effective ids first, second", inj)
	}
}

func TestInjectIdCollidingWithStateIsAProblem(t *testing.T) {
	err := parseFails(t, injectMachine("          - {id: note, fn: fakeQ}"), "")
	requireProblem(t, err, `inject id "note" collides with a declared state field`)
}

func TestInjectIdCollidingWithParamIsAProblem(t *testing.T) {
	machine := "params:\n    anchor: {type: text, desc: y}\n" + injectMachine("          - {id: anchor, fn: fakeQ}")
	err := parseFails(t, machine, "")
	requireProblem(t, err, `inject id "anchor" collides with a declared param`)
}

func TestInvalidInjectIdIsAProblem(t *testing.T) {
	err := parseFails(t, injectMachine("          - {id: foo bar, fn: fakeQ}"), "")
	requireProblem(t, err, `invalid id "foo bar"`)
}

func TestFramingSameFnWithoutIdsIsAProblem(t *testing.T) {
	machine := "class: shell\nframing:\n    - {fn: fakeQ}\n    - {fn: fakeQ}\n" + laneMachine
	err := parseFails(t, machine, "")
	requireProblem(t, err, `framing[1]: duplicate lane id "fakeQ"`)
}

// --- per-lane template validation -------------------------------------------

func TestLaneSplittingATemplatePairIsAProblem(t *testing.T) {
	body := "## unit: draft\n\n### lane: opener\n\n{{if .note}}conditional\n\n### lane: closer\n\n{{end}}\n"
	err := parseFails(t, laneMachine, body)
	requireProblem(t, err, "unit draft: lane opener:")
	requireProblem(t, err, "unit draft: lane closer:")
}

// --- render path through a real session --------------------------------------

func TestServeDropsEmptyRenderedLane(t *testing.T) {
	body := "## unit: draft\n\n### lane: always\n\nDraft the note.\n\n### lane: conditional\n\n{{if .missingFlag}}Only when flagged.{{end}}\n"
	sv := startServe(t, engine.NewRegistry(), laneMachine, body)
	requireServeLanes(t, sv, "always")
	if sv.UnitText != "Draft the note." {
		t.Errorf("UnitText = %q, want the sole non-empty lane", sv.UnitText)
	}
}

func TestServeUnitTextIsTheLaneJoin(t *testing.T) {
	body := "## unit: draft\n\n### lane: intro\n\nIntro.\n\n### lane: detail\n\nDetail.\n"
	sv := startServe(t, engine.NewRegistry(), laneMachine, body)
	requireServeLanes(t, sv, "intro", "detail")
	if want := serveLaneText(t, sv, "intro") + "\n\n" + serveLaneText(t, sv, "detail"); sv.UnitText != want {
		t.Errorf("UnitText = %q, want the lane join %q", sv.UnitText, want)
	}
}

func TestServeRendersInjectResultInItsLane(t *testing.T) {
	reg := engine.NewRegistry()
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "echoQuery", Doc: "test echo"},
		Fn:  func(_ *engine.Context, _ map[string]any) (any, error) { return "injected-result", nil },
	}); err != nil {
		t.Fatal(err)
	}
	body := "## unit: draft\n\n### lane: intro\n\nDraft the note.\n\n### lane: injected\n\nResult: {{.myId}}\n"
	sv := startServe(t, reg, injectMachine("          - {id: myId, fn: echoQuery}"), body)
	requireServeLanes(t, sv, "intro", "injected")
	if got := serveLaneText(t, sv, "injected"); got != "Result: injected-result" {
		t.Errorf("injected lane = %q, want the query result under the explicit id", got)
	}
	if got := serveLaneText(t, sv, "intro"); strings.Contains(got, "injected-result") {
		t.Errorf("intro lane leaked the inject result: %q", got)
	}
}
