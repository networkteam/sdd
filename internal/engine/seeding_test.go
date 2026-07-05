package engine

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// Tests for the slice-A seeding contract (d-tac-tlo): start inputs seed state
// fields, a dispatching parent hands its grounding down to a spawned child,
// and the seed survives replay. Plus the task procedure class.

const (
	seedParentID = "20260705-010000-d-prc-par"
	seedChildID  = "20260705-010001-d-prc-chd"
)

// A parent that simply holds grounding: seeded widenReport + anchor satisfy
// its gate on entry, so it completes immediately while its store retains the
// values a child inherits.
const seedParentFrontmatter = `state:
    widenReport: {type: text, desc: grounding evidence}
    anchor: {type: entry-id, desc: the anchor}
steps:
    - id: hold
      transitions:
          - when: hasWidenReport and hasAnchor
            to: end(completed)
`

// A child whose draft gate needs a body plus grounding — the grounding is
// what the parent hands down.
const seedChildFrontmatter = `state:
    widenReport: {type: text, desc: grounding evidence}
    body: {type: text, desc: the draft}
steps:
    - id: draft
      collect: [body, widenReport]
      transitions:
          - when: hasBody and hasWidenReport
            to: end(completed)
`

func procedureEntry(t *testing.T, id, canonical, class, frontmatter, body string) *model.Entry {
	t.Helper()
	head := "---\ntype: decision\nlayer: process\nkind: procedure\ncanonical: " + canonical + "\n"
	if class != "" {
		head += "class: " + class + "\n"
	}
	entry, err := model.ParseEntry(id+".md", head+frontmatter+"---\n\n"+body)
	if err != nil {
		t.Fatalf("parsing %s: %v", canonical, err)
	}
	return entry
}

// seedEnv is a session over a graph holding the anchor, with the parent and
// child specs loaded against the builtin registry.
type seedEnv struct {
	session *Session
	sink    *memorySink
	parent  *Spec
	child   *Spec
}

func newSeedEnv(t *testing.T) *seedEnv {
	t.Helper()
	anchor, err := model.ParseEntry(procAnchorID+".md", "---\ntype: decision\nlayer: tac\nkind: directive\nintent: pending\n---\n\nThe anchor.\n")
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	parent, err := LoadSpec(procedureEntry(t, seedParentID, "seedparent", "", seedParentFrontmatter, "parent"), reg)
	if err != nil {
		t.Fatalf("loading parent spec: %v", err)
	}
	child, err := LoadSpec(procedureEntry(t, seedChildID, "seedchild", "", seedChildFrontmatter, "child"), reg)
	if err != nil {
		t.Fatalf("loading child spec: %v", err)
	}
	eng := New(reg, model.NewGraph([]*model.Entry{anchor}))
	sink := &memorySink{}
	ts := time.Date(2026, 7, 5, 1, 0, 0, 0, time.UTC)
	sess := eng.NewSession("s_seed", "christopher", sink, WithClock(func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}))
	return &seedEnv{session: sess, sink: sink, parent: parent, child: child}
}

// startHeldParent starts the parent with its grounding seeded — the direct
// "start inputs seed state" path, which also auto-advances the parent to
// completion since its gate is satisfied on entry.
func (e *seedEnv) startHeldParent(t *testing.T, widen string) *Serve {
	t.Helper()
	sv, err := e.session.Start(e.parent, map[string]any{"widenReport": widen, "anchor": procAnchorID}, "")
	if err != nil {
		t.Fatalf("starting parent: %v", err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("parent should auto-complete once grounding is seeded, got %s at %q", sv.Status, sv.Step)
	}
	return sv
}

func missing(sv *Serve, name string) bool {
	return slices.Contains(sv.Missing, name)
}

func fieldValue(t *testing.T, sess *Session, instance, field string) any {
	t.Helper()
	inst, ok := sess.Instance(instance)
	if !ok {
		t.Fatalf("instance %q not found", instance)
	}
	v, _ := inst.Store.Get(field)
	return v
}

// TestSeedOnStart_StateInputAdvancesGate covers the "params seed state fields,
// gates evaluate on entry, satisfied steps auto-advance" primitive, as a
// contrast pair so the advance is attributable to the seed: without a seed
// the entry gate holds at the first step; seeding the state fields it reads
// satisfies it on arrival and the step auto-advances with no report.
func TestSeedOnStart_StateInputAdvancesGate(t *testing.T) {
	cold := newSeedEnv(t)
	sv, err := cold.session.Start(cold.parent, nil, "")
	if err != nil {
		t.Fatalf("cold start: %v", err)
	}
	if sv.Status != StatusRunning || sv.Step != "hold" {
		t.Fatalf("without a seed the entry gate should hold at the first step, got %s at %q", sv.Status, sv.Step)
	}

	env := newSeedEnv(t)
	env.startHeldParent(t, "searched three angles pre-dispatch")
}

// TestHandoff_ChildInheritsGrounding covers the declared handoff: a child
// dispatched with a parent inherits the parent's grounding into its own
// state, so its grounding gate no longer holds and only the fresh work
// (body) remains to report.
func TestHandoff_ChildInheritsGrounding(t *testing.T) {
	env := newSeedEnv(t)
	parent := env.startHeldParent(t, "parent widened from three angles")

	sv, err := env.session.Start(env.child, nil, parent.Instance)
	if err != nil {
		t.Fatalf("starting child under parent: %v", err)
	}
	if sv.Step != "draft" {
		t.Fatalf("child step = %s, want draft", sv.Step)
	}
	if missing(sv, "widenReport") {
		t.Errorf("widenReport should be inherited, not missing; missing = %v", sv.Missing)
	}
	if !missing(sv, "body") {
		t.Errorf("body is fresh work and should still be missing; missing = %v", sv.Missing)
	}
	if got := fieldValue(t, env.session, sv.Instance, "widenReport"); got != "parent widened from three angles" {
		t.Errorf("inherited widenReport = %v, want the parent's value", got)
	}
}

// TestHandoff_CallerOverridesSeed covers "overridable per dispatch": a value
// the caller supplies at start wins over the parent's — the handoff never
// overwrites an already-set field.
func TestHandoff_CallerOverridesSeed(t *testing.T) {
	env := newSeedEnv(t)
	parent := env.startHeldParent(t, "parent's widen")

	sv, err := env.session.Start(env.child, map[string]any{"widenReport": "the caller's own widen"}, parent.Instance)
	if err != nil {
		t.Fatalf("starting child: %v", err)
	}
	if got := fieldValue(t, env.session, sv.Instance, "widenReport"); got != "the caller's own widen" {
		t.Errorf("caller-supplied widenReport should win over the seed, got %v", got)
	}
}

// TestHandoff_ColdStartSeedsNothing covers the no-parent path: nothing is
// seeded, so the grounding gate is satisfied only by a fresh report.
func TestHandoff_ColdStartSeedsNothing(t *testing.T) {
	env := newSeedEnv(t)

	sv, err := env.session.Start(env.child, nil, "")
	if err != nil {
		t.Fatalf("cold-starting child: %v", err)
	}
	if !missing(sv, "widenReport") || !missing(sv, "body") {
		t.Errorf("a cold start should seed nothing; missing = %v", sv.Missing)
	}
}

// TestHandoff_ReplayPreservesSeed covers resume fidelity: the seed is logged
// on the started event, so a replayed child keeps the inherited grounding —
// a parked-and-resumed child does not lose the record its gate passed on.
func TestHandoff_ReplayPreservesSeed(t *testing.T) {
	env := newSeedEnv(t)
	parent := env.startHeldParent(t, "grounding to survive a restart")

	child, err := env.session.Start(env.child, nil, parent.Instance)
	if err != nil {
		t.Fatalf("starting child: %v", err)
	}

	resolve := func(canonical string) (*Spec, error) {
		switch canonical {
		case "seedparent":
			return env.parent, nil
		case "seedchild":
			return env.child, nil
		default:
			return nil, nil
		}
	}
	replayed, err := env.session.engine.ReplaySession("s_seed", "christopher", env.sink.events, resolve, nil)
	if err != nil {
		t.Fatalf("replaying session: %v", err)
	}
	if got := fieldValue(t, replayed, child.Instance, "widenReport"); got != "grounding to survive a restart" {
		t.Errorf("replayed child lost its inherited grounding, got %v", got)
	}
}

// TestTaskClass_ExploreIsTask covers the reclassification: the shipped explore
// procedure now loads as a task.
func TestTaskClass_ExploreIsTask(t *testing.T) {
	entry := baseEntry(t, "explore")
	if !entry.IsTaskProcedure() {
		t.Fatalf("explore entry class = %q, want task", entry.Class)
	}
	spec, err := ParseSpec(entry)
	if err != nil {
		t.Fatalf("parsing explore spec: %v", err)
	}
	if spec.Class != model.ProcedureClassTask {
		t.Errorf("explore spec class = %q, want task", spec.Class)
	}
}

// TestTaskClass_RejectsUserChooser covers the shape rule: a task runs with no
// user present, so a user chooser in a task spec is a load error.
func TestTaskClass_RejectsUserChooser(t *testing.T) {
	const frontmatter = `state:
    pick: {type: text, desc: a choice}
steps:
    - id: choose
      chooser: user
      options:
          - {choice: go, to: end(completed)}
`
	_, err := ParseSpec(procedureEntry(t, "20260705-010002-d-prc-tsk", "sometask", "task", frontmatter, "a task"))
	if err == nil || !strings.Contains(err.Error(), "user chooser") {
		t.Fatalf("a task with a user chooser must fail load naming it, got %v", err)
	}
}
