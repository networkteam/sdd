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

// A parent that holds grounding behind a gate, then rests on a dispatch
// junction. Seeded widenReport + anchor satisfy the gate on entry and
// auto-advance to the junction; answering `dispatch` records the handoff a
// child inherits, while `conclude` records none.
const seedParentFrontmatter = `state:
    widenReport: {type: text, desc: grounding evidence}
    anchor: {type: entry-id, desc: the anchor}
steps:
    - id: hold
      transitions:
          - when: hasWidenReport and hasAnchor
            to: junction
    - id: junction
      chooser: user
      options:
          - choice: dispatch
            dispatch:
                seed:
                    widenReport: widenReport
                    anchor: anchor
            to: end(completed)
          - {choice: conclude, to: end(completed)}
`

// A child whose draft gate needs a body plus grounding — the grounding is
// what the parent hands down. It also declares anchor as state, so the default
// handoff carries the parent's anchor alongside its widenReport.
const seedChildFrontmatter = `state:
    widenReport: {type: text, desc: grounding evidence}
    anchor: {type: entry-id, desc: the anchor}
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

// startHeldParent starts the parent with its grounding seeded (auto-advancing
// past the gate to the dispatch junction) and answers `dispatch`, so its
// declared handoff is recorded for the child to inherit and the parent
// completes.
func (e *seedEnv) startHeldParent(t *testing.T, widen string) *Serve {
	t.Helper()
	sv, err := e.session.Start(e.parent, map[string]any{"widenReport": widen, "anchor": procAnchorID}, "")
	if err != nil {
		t.Fatalf("starting parent: %v", err)
	}
	if sv.Status != StatusRunning || sv.Step != "junction" {
		t.Fatalf("seeded parent should advance to its dispatch junction, got %s at %q", sv.Status, sv.Step)
	}
	sv, err = e.session.Answer(sv.Instance, "junction", "dispatch", nil, "dispatch the child with the grounding")
	if err != nil {
		t.Fatalf("answering the dispatch junction: %v", err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("answering dispatch should complete the parent, got %s at %q", sv.Status, sv.Step)
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

	// Seeding the fields the gate reads satisfies it on entry, so the parent
	// auto-advances past `hold` to its dispatch junction with no report.
	env := newSeedEnv(t)
	seeded, err := env.session.Start(env.parent, map[string]any{"widenReport": "searched three angles pre-dispatch", "anchor": procAnchorID}, "")
	if err != nil {
		t.Fatalf("seeded start: %v", err)
	}
	if seeded.Status != StatusRunning || seeded.Step != "junction" {
		t.Fatalf("a seeded gate should auto-advance to the junction, got %s at %q", seeded.Status, seeded.Step)
	}
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

// TestHandoff_AnchorInherited covers the anchor half of the default handoff:
// the child declares anchor as state, so the parent's anchor rides down
// alongside its widenReport (the review's zero-coverage gap, inherit case).
func TestHandoff_AnchorInherited(t *testing.T) {
	env := newSeedEnv(t)
	parent := env.startHeldParent(t, "parent widened")

	sv, err := env.session.Start(env.child, nil, parent.Instance)
	if err != nil {
		t.Fatalf("starting child under parent: %v", err)
	}
	if got := fieldValue(t, env.session, sv.Instance, "anchor"); got != procAnchorID {
		t.Errorf("anchor should be inherited from the parent, got %v", got)
	}
}

// A parent that carries its grounding under differently-named fields and
// declares a per-junction seed mapping onto the child's field names — the
// exact case the fixed global set lost silently (s-tac-lbc).
const seedRemapParentFrontmatter = `state:
    scanReport: {type: text, desc: grounding under a different name}
    candidateId: {type: entry-id, desc: the anchor under a different name}
steps:
    - id: junction
      chooser: user
      options:
          - choice: record
            dispatch:
                procedure: seedchild
                seed:
                    widenReport: scanReport
                    anchor: candidateId
            to: end(completed)
`

// TestHandoff_DeclaredSeedRemapsFields covers the per-dispatch override: a
// parent whose evidence lives under scanReport/candidateId hands it to the
// child's widenReport/anchor via the junction's declared seed mapping — the
// handoff the default set would have missed.
func TestHandoff_DeclaredSeedRemapsFields(t *testing.T) {
	env := newSeedEnv(t)
	reg := NewRegistry()
	remap, err := LoadSpec(procedureEntry(t, "20260705-010003-d-prc-rmp", "seedremap", "", seedRemapParentFrontmatter, "remap parent"), reg)
	if err != nil {
		t.Fatalf("loading remap parent: %v", err)
	}

	parent, err := env.session.Start(remap, map[string]any{"scanReport": "scanned three angles", "candidateId": procAnchorID}, "")
	if err != nil {
		t.Fatalf("starting remap parent: %v", err)
	}
	if parent.Status != StatusRunning || parent.Step != "junction" {
		t.Fatalf("remap parent should rest on its junction, got %s at %q", parent.Status, parent.Step)
	}
	parent, err = env.session.Answer(parent.Instance, "junction", "record", nil, "record the finding")
	if err != nil {
		t.Fatalf("answering the remap junction: %v", err)
	}

	sv, err := env.session.Start(env.child, nil, parent.Instance)
	if err != nil {
		t.Fatalf("starting child under remap parent: %v", err)
	}
	if got := fieldValue(t, env.session, sv.Instance, "widenReport"); got != "scanned three angles" {
		t.Errorf("widenReport should carry the parent's scanReport, got %v", got)
	}
	if got := fieldValue(t, env.session, sv.Instance, "anchor"); got != procAnchorID {
		t.Errorf("anchor should carry the parent's candidateId, got %v", got)
	}
}

// TestSeedMapping_MissingSourceFailsLoad covers the load-time guard: a seed
// whose source names no declared field of the parent fails spec load, so a
// mistyped source can never silently carry nothing.
func TestSeedMapping_MissingSourceFailsLoad(t *testing.T) {
	const frontmatter = `state:
    scanReport: {type: text, desc: grounding}
steps:
    - id: junction
      chooser: user
      options:
          - choice: record
            dispatch:
                procedure: seedchild
                seed:
                    widenReport: nope
            to: end(completed)
`
	_, err := LoadSpec(procedureEntry(t, "20260705-010004-d-prc-bad", "seedbad", "", frontmatter, "bad seed"), NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("a seed sourcing an undeclared parent field must fail load naming it, got %v", err)
	}
}

// TestHandoff_UnseededOptionInheritsNothing covers the explicit contract: a
// child dispatched after an answered option that declares no seed (the parent's
// `conclude`) inherits nothing, even though the parent holds grounding. The
// handoff is tied to the answered junction's declaration, not to the parent
// merely holding a field a child happens to name.
func TestHandoff_UnseededOptionInheritsNothing(t *testing.T) {
	env := newSeedEnv(t)
	sv, err := env.session.Start(env.parent, map[string]any{"widenReport": "held but not handed off", "anchor": procAnchorID}, "")
	if err != nil {
		t.Fatalf("starting parent: %v", err)
	}
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "nothing to hand off")
	if err != nil {
		t.Fatalf("answering conclude: %v", err)
	}

	child, err := env.session.Start(env.child, nil, sv.Instance)
	if err != nil {
		t.Fatalf("starting child under a seedless dispatch: %v", err)
	}
	if !missing(child, "widenReport") {
		t.Errorf("a child dispatched after a seedless option must inherit nothing, got widenReport %v", fieldValue(t, env.session, child.Instance, "widenReport"))
	}
}

// TestHandoff_ProcedureGuardSkipsMismatch covers the procedure guard: a seed
// declared for one child procedure is not applied to a child of a different
// procedure dispatched under the same parent.
func TestHandoff_ProcedureGuardSkipsMismatch(t *testing.T) {
	env := newSeedEnv(t)
	remap, err := LoadSpec(procedureEntry(t, "20260705-010008-d-prc-grd", "seedguard", "", seedRemapParentFrontmatter, "guard parent"), NewRegistry())
	if err != nil {
		t.Fatalf("loading guard parent: %v", err)
	}
	other, err := LoadSpec(procedureEntry(t, "20260705-010009-d-prc-oth", "seedother", "", seedGatedChooserChildFrontmatter, "other child"), NewRegistry())
	if err != nil {
		t.Fatalf("loading other child: %v", err)
	}

	parent, err := env.session.Start(remap, map[string]any{"scanReport": "scanned", "candidateId": procAnchorID}, "")
	if err != nil {
		t.Fatalf("starting guard parent: %v", err)
	}
	parent, err = env.session.Answer(parent.Instance, "junction", "record", nil, "record it")
	if err != nil {
		t.Fatalf("answering the guard junction: %v", err)
	}

	// The seed named procedure seedchild; seedother must not receive it.
	sv, err := env.session.Start(other, nil, parent.Instance)
	if err != nil {
		t.Fatalf("starting the mismatched child: %v", err)
	}
	if got := fieldValue(t, env.session, sv.Instance, "widenReport"); got != nil {
		t.Errorf("a seed declared for a different procedure must not apply, got widenReport %v", got)
	}
}

// A parent with two junctions: the first seeds a handoff, the second grants
// none — used to lock that answering a plain option clears a prior stash.
const seedMultiJunctionParentFrontmatter = `state:
    widenReport: {type: text, desc: grounding}
    anchor: {type: entry-id, desc: the anchor}
steps:
    - id: first
      chooser: user
      options:
          - choice: seed
            dispatch:
                seed:
                    widenReport: widenReport
                    anchor: anchor
            to: second
    - id: second
      chooser: user
      options:
          - {choice: plain, to: end(completed)}
`

// TestHandoff_NonDispatchOptionClearsSeed covers the stash lifecycle: after a
// seed-bearing option is answered, answering a later plain option on the same
// parent clears the mapping, so a child dispatched afterward inherits nothing
// its own junction never granted. The clear derives from the logged chooser
// answers, so a replayed session agrees with the live one.
func TestHandoff_NonDispatchOptionClearsSeed(t *testing.T) {
	env := newSeedEnv(t)
	multi, err := LoadSpec(procedureEntry(t, "20260706-010000-d-prc-mlt", "seedmulti", "", seedMultiJunctionParentFrontmatter, "multi-junction parent"), NewRegistry())
	if err != nil {
		t.Fatalf("loading multi-junction parent: %v", err)
	}

	parent, err := env.session.Start(multi, map[string]any{"widenReport": "held grounding", "anchor": procAnchorID}, "")
	if err != nil {
		t.Fatalf("starting multi-junction parent: %v", err)
	}
	// Answer the seed-bearing junction, then a plain one on the same parent.
	parent, err = env.session.Answer(parent.Instance, "first", "seed", nil, "hand off")
	if err != nil {
		t.Fatalf("answering the seed junction: %v", err)
	}
	if _, err := env.session.Answer(parent.Instance, "second", "plain", nil, "no handoff here"); err != nil {
		t.Fatalf("answering the plain junction: %v", err)
	}

	child, err := env.session.Start(env.child, nil, parent.Instance)
	if err != nil {
		t.Fatalf("starting child after the plain answer: %v", err)
	}
	if got := fieldValue(t, env.session, child.Instance, "widenReport"); got != nil {
		t.Errorf("a later plain answer must clear the stashed seed; child inherited %v", got)
	}

	// Replay agreement: fold the log and dispatch another child under the
	// replayed parent — the clear must survive replay, so it inherits nothing
	// too.
	resolve := func(canonical string) (*Spec, error) {
		switch canonical {
		case "seedmulti":
			return multi, nil
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
	child2, err := replayed.Start(env.child, nil, parent.Instance)
	if err != nil {
		t.Fatalf("dispatching under the replayed parent: %v", err)
	}
	if got := fieldValue(t, replayed, child2.Instance, "widenReport"); got != nil {
		t.Errorf("the cleared stash must survive replay; replayed child inherited %v", got)
	}
}

// A child whose entry is a grounding gate the seed satisfies, followed by a
// user chooser — used to lock that seeding advances gates but never skips a
// chooser.
const seedGatedChooserChildFrontmatter = `state:
    widenReport: {type: text, desc: grounding}
steps:
    - id: ground
      transitions:
          - when: hasWidenReport
            to: decide
    - id: decide
      chooser: user
      options:
          - {choice: go, to: end(completed)}
`

// TestHandoff_SeedNeverSkipsChooser locks the invariant against slice B
// refactors: a seed satisfies the entry gate and auto-advances the child, but
// the cascade stops at the following user chooser — it is never skipped.
func TestHandoff_SeedNeverSkipsChooser(t *testing.T) {
	env := newSeedEnv(t)
	child, err := LoadSpec(procedureEntry(t, "20260705-010006-d-prc-gch", "seedgated", "", seedGatedChooserChildFrontmatter, "gated chooser child"), NewRegistry())
	if err != nil {
		t.Fatalf("loading gated child: %v", err)
	}
	parent := env.startHeldParent(t, "grounding to inherit")

	sv, err := env.session.Start(child, nil, parent.Instance)
	if err != nil {
		t.Fatalf("starting gated child under parent: %v", err)
	}
	if sv.Status != StatusRunning {
		t.Fatalf("the seed satisfies the gate but must not end the child at a chooser, got %s", sv.Status)
	}
	if sv.Step != "decide" || sv.Chooser == nil {
		t.Fatalf("cascade should stop at the user chooser, got step %q chooser %+v", sv.Step, sv.Chooser)
	}
	if sv.Chooser.Chooser != "decide" {
		t.Errorf("the pending chooser should name its step id, got %q", sv.Chooser.Chooser)
	}
}

// TestAnswerSchema_NestsFieldsUnderFields locks the chooser-answer schema
// shape (s-tac-keb): the chooser is named by step id, collected fields nest
// under `fields`, and none appear at the top level.
func TestAnswerSchema_NestsFieldsUnderFields(t *testing.T) {
	const frontmatter = `state:
    note: {type: text, desc: a collected note}
steps:
    - id: pick
      chooser: agent
      options:
          - {choice: keep, collect: [note], to: end(completed)}
`
	spec, err := LoadSpec(procedureEntry(t, "20260705-010007-d-prc-pik", "seedpick", "", frontmatter, "pick"), NewRegistry())
	if err != nil {
		t.Fatalf("loading pick spec: %v", err)
	}
	schema := spec.AnswerSchemaForStep(spec.StepByID["pick"])
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("answer schema has no properties: %+v", schema)
	}
	if _, ok := props["note"]; ok {
		t.Errorf("collected field must not appear at the top level: %+v", props)
	}
	chooser, _ := props["chooser"].(map[string]any)
	if chooser == nil || chooser["const"] != "pick" {
		t.Errorf("chooser property should be a const of the step id, got %+v", props["chooser"])
	}
	fields, _ := props["fields"].(map[string]any)
	if fields == nil {
		t.Fatalf("collected fields should be nested under a fields object, got %+v", props)
	}
	fieldProps, _ := fields["properties"].(map[string]any)
	if _, ok := fieldProps["note"]; !ok {
		t.Errorf("the note field should live under fields.properties, got %+v", fields)
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
