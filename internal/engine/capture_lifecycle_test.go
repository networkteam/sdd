package engine

import (
	"strings"
	"testing"
)

// Capture-through-the-engine tests for the lifecycle fields `closes` and
// `supersedes`. A closure is usually discovered mid-draft — it is the substance
// being recorded — so these must be reachable while assembling, not only at move
// start: an author who cannot write the edge at assemble writes the target into
// `refs` instead, where `addresses` is legitimate, and the entry then claims a
// closure its frontmatter never carries (20260731-083611-s-cpt-wag). Withdrawing
// a claimed edge must work at the same steps, or the correction is unreachable.

// lifecycleDraft is a done signal closing the fixture directive — the shape the
// external report arrived as. Lifecycle legality is the write gate's business;
// what the engine owns is whether the field can be written at all.
func lifecycleDraft() map[string]any {
	draft := fullDraft()
	draft["entryKind"] = "done"
	draft["body"] = "The fix landed and the directive it fulfils is retired."
	return draft
}

func TestCapture_LifecycleFieldsSetAtAssemble(t *testing.T) {
	env := newFixtureEnv(t)
	// The supersede target is a signal, and the inspection gate covers lifecycle
	// targets as well as refs — so it must be read in full first.
	env.session.LogRead("show", []string{fixtureRef2ID}, nil)

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// The closure surfaces before the rest of the draft is settled: reported
	// alone it holds assemble, and the re-serve carries the retirement
	// instruction the author now has to satisfy.
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"closes":     []any{fixtureRefID},
		"supersedes": []any{fixtureRef2ID},
	})
	if err != nil {
		t.Fatalf("lifecycle fields must be writable at assemble: %v", err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("an incomplete draft must hold assemble, got %q", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "This capture retires") {
		t.Errorf("assemble should serve the retirement-rationale instruction once closes is set, got %q", sv.Instructions)
	}
	if !strings.Contains(sv.Instructions, "This capture replaces") {
		t.Errorf("assemble should serve the replacement instruction once supersedes is set, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, lifecycleDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("completed draft should reach playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
	// Playback is the verification contract: an edge the user cannot see is an
	// edge they never confirmed.
	for _, id := range []string{fixtureRefID, fixtureRef2ID} {
		if !strings.Contains(sv.Instructions, id) {
			t.Errorf("playback should render lifecycle target %s, got %q", id, sv.Instructions)
		}
	}

	sv, err = env.session.Answer(sv.Instance, "playback", "confirm", nil, "yes, that retires it")
	if err != nil {
		t.Fatal(err)
	}
	if env.newCalls != 1 || sv.Step != "verifySummary" {
		t.Fatalf("confirm should write once and reach verifySummary, got step=%q newCalls=%d", sv.Step, env.newCalls)
	}
}

// The obligation a closure puts on the body must be served before the body is
// written, in every route the fields can arrive by. A draft that batches the
// lifecycle field into one complete report cascades straight to playback and
// never sees an assemble re-serve, so a conditional {{if .closes}} instruction
// would reach that author never — not late.
func TestCapture_LifecycleObligationServedBeforeDrafting(t *testing.T) {
	env := newFixtureEnv(t)

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("start step = %s, want assemble", sv.Step)
	}
	for _, want := range []string{"why the target is retired", "why each predecessor is replaced"} {
		if !strings.Contains(sv.Instructions, want) {
			t.Errorf("the opening assemble serve should state %q before any field is reported, got %q", want, sv.Instructions)
		}
	}

	// The batched route: one complete report, no re-serve of assemble.
	draft := lifecycleDraft()
	draft["closes"] = []any{fixtureRefID}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("a complete batched draft should reach playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
}

func TestCapture_LifecycleFieldsSeededAtStart(t *testing.T) {
	env := newFixtureEnv(t)
	env.session.LogRead("show", []string{fixtureRef2ID}, nil)

	sv, err := env.session.Start(env.spec, map[string]any{
		"closes":     []any{fixtureRefID},
		"supersedes": []any{fixtureRef2ID},
	}, "")
	if err != nil {
		t.Fatalf("start-time seeding must keep working: %v", err)
	}
	if !strings.Contains(sv.Instructions, "This capture retires") {
		t.Errorf("a seeded closure should be stated at the opening serve, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, lifecycleDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("seeded draft should reach playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
	for _, id := range []string{fixtureRefID, fixtureRef2ID} {
		if !strings.Contains(sv.Instructions, id) {
			t.Errorf("playback should render seeded lifecycle target %s, got %q", id, sv.Instructions)
		}
	}
}

// Withdrawing a claimed edge at assemble: an author who reconsiders must be able
// to take the edge back out, not just put it in.
func TestCapture_LifecycleFieldsUnsetAtAssemble(t *testing.T) {
	env := newFixtureEnv(t)
	env.session.LogRead("show", []string{fixtureRef2ID}, nil)

	sv, err := env.session.Start(env.spec, map[string]any{
		"closes":     []any{fixtureRefID},
		"supersedes": []any{fixtureRef2ID},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, lifecycleDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("seeded draft should reach playback, got %q", sv.Step)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{"closes": nil, "supersedes": nil})
	if err != nil {
		t.Fatalf("lifecycle fields must be clearable at assemble: %v", err)
	}
	if sv.Step != "playback" {
		t.Fatalf("clearing optional fields should re-reach playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
	for _, id := range []string{fixtureRefID, fixtureRef2ID} {
		if strings.Contains(sv.Instructions, "closes: "+id) || strings.Contains(sv.Instructions, "supersedes: "+id) {
			t.Errorf("playback still renders withdrawn lifecycle target %s, got %q", id, sv.Instructions)
		}
	}
}

// The same withdrawal through the playback adjust chooser — the step built for
// catching faults in what was just played back.
func TestCapture_LifecycleFieldsUnsetViaPlaybackAdjust(t *testing.T) {
	env := newFixtureEnv(t)

	sv, err := env.session.Start(env.spec, map[string]any{"closes": []any{fixtureRefID}}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, lifecycleDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("draft should reach playback, got %q", sv.Step)
	}

	sv, err = env.session.Answer(sv.Instance, "playback", "adjust", map[string]any{
		"closes": nil,
		"body":   "The fix landed; it responds to the directive without retiring it.",
	}, "no, it doesn't close that one")
	if err != nil {
		t.Fatalf("adjust must reach the lifecycle fields: %v", err)
	}
	if sv.Step != "playback" {
		t.Fatalf("adjust should re-assemble back to playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
	if strings.Contains(sv.Instructions, "closes: "+fixtureRefID) {
		t.Errorf("playback still renders the withdrawn closure, got %q", sv.Instructions)
	}
	if env.newCalls != 0 {
		t.Fatalf("nothing should have been written yet, newCalls=%d", env.newCalls)
	}
}

// A lifecycle target reported at assemble must clear the inspection gate the
// same way a ref does — moving the field into state must not open a path that
// writes an edge to an entry nobody read.
func TestCapture_LifecycleTargetMustBeInspected(t *testing.T) {
	env := newFixtureEnv(t)

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := lifecycleDraft()
	// fixtureRef2ID resolves in the fixture graph but this session never read it.
	draft["closes"] = []any{fixtureRef2ID}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("an uninspected closes target must hold assemble, got %q", sv.Step)
	}
	if !hasFailing(sv.Failing, "refsInspected") {
		t.Fatalf("failing = %+v, want refsInspected", sv.Failing)
	}
}

// The write gate is where a wrong lifecycle field is caught — a `closes` the
// validator wants as `supersedes` rejects there, and the revise path must be able
// to correct it instead of wedging with the field out of reach
// (20260707-170235-s-prc-0p1). Correcting it reopens playback rather than
// re-writing: under state provenance the lifecycle fields join the snapshot the
// confirmation binds to, so the user re-confirms the edge they get.
func TestCapture_LifecycleFieldsCorrectableAfterWriteRejection(t *testing.T) {
	env := newFixtureEnv(t)
	env.highFindingsUnlessOverride = true
	env.session.LogRead("show", []string{fixtureRef2ID}, nil)

	sv, err := env.session.Start(env.spec, map[string]any{"closes": []any{fixtureRefID}}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, lifecycleDraft())
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "playback", "confirm", nil, "yes")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "reviseOrOverride" {
		t.Fatalf("a blocked write should serve reviseOrOverride, got %q", sv.Step)
	}

	// The correction the rejection calls for: the edge moves fields.
	sv, err = env.session.Answer(sv.Instance, "reviseOrOverride", "revise", map[string]any{
		"closes":     nil,
		"supersedes": []any{fixtureRef2ID},
	}, "make it a supersede instead")
	if err != nil {
		t.Fatalf("revise must reach the lifecycle fields: %v", err)
	}
	if sv.Step != "playback" {
		t.Fatalf("a corrected lifecycle field must reopen playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
	if !strings.Contains(sv.Instructions, fixtureRef2ID) {
		t.Errorf("playback should render the corrected supersede target, got %q", sv.Instructions)
	}
	if strings.Contains(sv.Instructions, "closes: "+fixtureRefID) {
		t.Errorf("playback still renders the withdrawn closure, got %q", sv.Instructions)
	}
	if env.newCalls != 1 {
		t.Fatalf("the correction must not re-run the write before re-confirmation, newCalls=%d", env.newCalls)
	}
}

func TestReportSchema_AdvertisesLifecycleFieldsAtAssemble(t *testing.T) {
	env := newFixtureEnv(t)
	step := env.spec.StepByID["assemble"]
	schema := env.spec.ReportSchemaForStep(step)
	props, _ := schema["properties"].(map[string]any)

	for _, name := range []string{"closes", "supersedes"} {
		prop, ok := props[name].(map[string]any)
		if !ok {
			t.Fatalf("assemble schema does not advertise %q — an author who discovers a closure mid-draft cannot report it", name)
		}
		// Optional state fields render as a nullable anyOf wrapper; the null
		// branch is what makes withdrawal expressible.
		array := nullableArrayBranch(t, prop)
		item, _ := array["items"].(map[string]any)
		if item == nil || item["pattern"] == nil {
			t.Errorf("%s items should advertise the entry-id pattern, got %v", name, array["items"])
		}
	}
}
