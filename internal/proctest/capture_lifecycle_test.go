// Capture behavior for the lifecycle fields `closes` and `supersedes`: they
// stay writable and withdrawable while the draft is assembled, their
// obligation on the body is served before drafting, their targets clear the
// inspection gate like refs, and a wrong edge is correctable after a write
// rejection. Ported from internal/engine.
package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/proctest"
)

// lifecycleDraft is a done signal closing the fixture directive.
func lifecycleDraft() map[string]any {
	draft := captureDraft()
	draft["entryKind"] = "done"
	draft["body"] = "The fix landed and the directive it fulfils is retired."
	return draft
}

func TestCapture_LifecycleFieldsSetAtAssemble(t *testing.T) {
	world, session := newCaptureWorld(t, "lifecycle-set")
	// The supersede target's inspection is gated like a ref — read it in full.
	session.LogRead(t, "show", []string{captureRef2ID}, nil)

	serve := session.Start(t, "capture", nil)
	instance := serve.Instance

	// The closure surfaces before the rest of the draft is settled: reported
	// alone it holds assemble, and the re-serve carries the retirement
	// instruction the author now has to satisfy.
	serve = session.Report(t, instance, map[string]any{
		"closes":     []any{captureRefID},
		"supersedes": []any{captureRef2ID},
	})
	proctest.RequireStep(t, serve, "assemble")
	if !strings.Contains(serve.Instructions, "This capture retires") {
		t.Errorf("assemble should serve the retirement-rationale instruction once closes is set, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "This capture replaces") {
		t.Errorf("assemble should serve the replacement instruction once supersedes is set, got %q", serve.Instructions)
	}

	serve = session.Report(t, instance, lifecycleDraft())
	proctest.RequireStep(t, serve, "playback")
	// Playback is the verification contract: an edge the user cannot see is an
	// edge they never confirmed.
	for _, id := range []string{captureRefID, captureRef2ID} {
		if !strings.Contains(serve.Instructions, id) {
			t.Errorf("playback should render lifecycle target %s, got %q", id, serve.Instructions)
		}
	}

	serve = session.Answer(t, instance, "playback", "confirm", nil, "yes, that retires it")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("lifecycle capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if len(entry.Closes) != 1 || entry.Closes[0] != captureRefID {
		t.Fatalf("persisted closes = %v", entry.Closes)
	}
	if len(entry.Supersedes) != 1 || entry.Supersedes[0] != captureRef2ID {
		t.Fatalf("persisted supersedes = %v", entry.Supersedes)
	}
}

// The obligation a closure puts on the body must be served before the body is
// written, in every route the fields can arrive by. A draft that batches the
// lifecycle field into one complete report cascades straight to playback and
// never sees an assemble re-serve, so a conditional instruction would reach
// that author never — not late.
func TestCapture_LifecycleObligationServedBeforeDrafting(t *testing.T) {
	_, session := newCaptureWorld(t, "lifecycle-obligation")

	serve := session.Start(t, "capture", nil)
	proctest.RequireStep(t, serve, "assemble")
	for _, want := range []string{"why the target is retired", "why each predecessor is replaced"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Errorf("the opening assemble serve should state %q before any field is reported, got %q", want, serve.Instructions)
		}
	}

	// The batched route: one complete report, no re-serve of assemble.
	draft := lifecycleDraft()
	draft["closes"] = []any{captureRefID}
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "playback")
}

func TestCapture_LifecycleFieldsSeededAtStart(t *testing.T) {
	_, session := newCaptureWorld(t, "lifecycle-seeded")
	session.LogRead(t, "show", []string{captureRef2ID}, nil)

	serve := session.Start(t, "capture", map[string]any{
		"closes":     []any{captureRefID},
		"supersedes": []any{captureRef2ID},
	})
	if !strings.Contains(serve.Instructions, "This capture retires") {
		t.Errorf("a seeded closure should be stated at the opening serve, got %q", serve.Instructions)
	}

	serve = session.Report(t, serve.Instance, lifecycleDraft())
	proctest.RequireStep(t, serve, "playback")
	for _, id := range []string{captureRefID, captureRef2ID} {
		if !strings.Contains(serve.Instructions, id) {
			t.Errorf("playback should render seeded lifecycle target %s, got %q", id, serve.Instructions)
		}
	}
}

// Withdrawing a claimed edge at assemble: an author who reconsiders must be
// able to take the edge back out, not just put it in.
func TestCapture_LifecycleFieldsUnsetAtAssemble(t *testing.T) {
	_, session := newCaptureWorld(t, "lifecycle-unset")
	session.LogRead(t, "show", []string{captureRef2ID}, nil)

	serve := session.Start(t, "capture", map[string]any{
		"closes":     []any{captureRefID},
		"supersedes": []any{captureRef2ID},
	})
	instance := serve.Instance
	serve = session.Report(t, instance, lifecycleDraft())
	proctest.RequireStep(t, serve, "playback")

	serve = session.Report(t, instance, map[string]any{"closes": nil, "supersedes": nil})
	proctest.RequireStep(t, serve, "playback")
	for _, id := range []string{captureRefID, captureRef2ID} {
		if strings.Contains(serve.Instructions, "closes: "+id) || strings.Contains(serve.Instructions, "supersedes: "+id) {
			t.Errorf("playback still renders withdrawn lifecycle target %s, got %q", id, serve.Instructions)
		}
	}
}

// The same withdrawal through the playback adjust chooser — the step built for
// catching faults in what was just played back.
func TestCapture_LifecycleFieldsUnsetViaPlaybackAdjust(t *testing.T) {
	world, session := newCaptureWorld(t, "lifecycle-adjust")

	serve := session.Start(t, "capture", map[string]any{"closes": []any{captureRefID}})
	instance := serve.Instance
	serve = session.Report(t, instance, lifecycleDraft())
	proctest.RequireStep(t, serve, "playback")

	serve = session.Answer(t, instance, "playback", "adjust", map[string]any{
		"closes": nil,
		"body":   "The fix landed; it responds to the directive without retiring it.",
	}, "no, it doesn't close that one")
	proctest.RequireStep(t, serve, "playback")
	if strings.Contains(serve.Instructions, "closes: "+captureRefID) {
		t.Errorf("playback still renders the withdrawn closure, got %q", serve.Instructions)
	}
	if got := world.LLM.Calls("preflight"); got != 0 {
		t.Fatalf("nothing should have been written yet, pre-flight ran %d times", got)
	}
}

// A lifecycle target reported at assemble must clear the inspection gate the
// same way a ref does — the field must not open a path that writes an edge to
// an entry nobody read.
func TestCapture_LifecycleTargetMustBeInspected(t *testing.T) {
	_, session := newCaptureWorld(t, "lifecycle-uninspected")

	serve := session.Start(t, "capture", nil)
	draft := lifecycleDraft()
	// captureRef2ID resolves in the fixture graph but this session never read it.
	draft["closes"] = []any{captureRef2ID}
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, "not inspected at full depth") || !hasDiagnostic(serve, captureRef2ID) {
		t.Fatalf("diagnostics = %v, want the refsInspected message naming %s", serve.Diagnostics, captureRef2ID)
	}
}

// The write gate is where a wrong lifecycle field is caught — a `closes` the
// validator wants as `supersedes` rejects there, and the revise path must be
// able to correct it instead of wedging with the field out of reach.
// Correcting it reopens playback rather than re-writing: the lifecycle fields
// join the snapshot the confirmation binds to, so the user re-confirms the
// edge they get.
func TestCapture_LifecycleFieldsCorrectableAfterWriteRejection(t *testing.T) {
	world, session := newCaptureWorld(t, "lifecycle-correct")
	world.LLM.PreflightFindings = []proctest.PreflightFinding{{
		Severity: "high", Category: "test-block", Observation: "blocked until override",
	}}
	session.LogRead(t, "show", []string{captureRef2ID}, nil)

	serve := session.Start(t, "capture", map[string]any{"closes": []any{captureRefID}})
	instance := serve.Instance
	session.Report(t, instance, lifecycleDraft())
	serve = session.Answer(t, instance, "playback", "confirm", nil, "yes")
	proctest.RequireStep(t, serve, "reviseOrOverride")

	// The correction the rejection calls for: the edge moves fields.
	serve = session.Answer(t, instance, "reviseOrOverride", "revise", map[string]any{
		"closes":     nil,
		"supersedes": []any{captureRef2ID},
	}, "make it a supersede instead")
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, captureRef2ID) {
		t.Errorf("playback should render the corrected supersede target, got %q", serve.Instructions)
	}
	if strings.Contains(serve.Instructions, "closes: "+captureRefID) {
		t.Errorf("playback still renders the withdrawn closure, got %q", serve.Instructions)
	}
	if got := world.LLM.Calls("preflight"); got != 1 {
		t.Fatalf("the correction must not re-run the write before re-confirmation, pre-flight ran %d times", got)
	}
}

func TestReportSchema_AdvertisesLifecycleFieldsAtAssemble(t *testing.T) {
	_, session := newCaptureWorld(t, "lifecycle-schema")
	serve := session.Start(t, "capture", nil)
	props, _ := serve.ReportSchema["properties"].(map[string]any)

	for _, name := range []string{"closes", "supersedes"} {
		prop, ok := props[name].(map[string]any)
		if !ok {
			t.Fatalf("assemble schema does not advertise %q — an author who discovers a closure mid-draft cannot report it", name)
		}
		// Optional state fields render as a nullable anyOf wrapper; the null
		// branch is what makes withdrawal expressible.
		array := nullableBranch(t, prop, "array")
		item, _ := array["items"].(map[string]any)
		if item == nil || item["pattern"] == nil {
			t.Errorf("%s items should advertise the entry-id pattern, got %v", name, array["items"])
		}
	}
}
