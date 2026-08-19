// Capture-time kind knowledge delivery (d-tac-vzz): the draftingKnowledge
// inject serves the type-system overview in capture's initial serve at most
// once per durable session, names a preselected kind's authoring fact, and
// fails loud when the overview cannot resolve. Ported from
// application/workflow_drafting_knowledge_test.go.
package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/proctest"
)

// Markers distinguishing a full overview serve from the suppressed pointer.
const (
	overviewBodyMarker      = "a signal records something noticed:"
	overviewServedMarker    = "served here in full"
	overviewSuppressedNote  = "was already served to this session in full"
	kindSequenceInstruction = "Work in this order: read the type-system overview, choose the kind, pull that kind's authoring fact in full, then draft."
)

func TestCaptureStartServesOverviewOncePerSession(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "dk-once")

	first := session.Start(t, "capture", nil)
	if !strings.Contains(first.Instructions, overviewServedMarker) || !strings.Contains(first.Instructions, overviewBodyMarker) {
		t.Fatalf("a fresh capture start must serve the overview body in full, got: %.300q", first.Instructions)
	}
	if !strings.Contains(first.Instructions, basefacts.OverviewFactID) {
		t.Errorf("the overview serve must name the overview fact ID")
	}
	if !strings.Contains(first.Instructions, kindSequenceInstruction) {
		t.Errorf("an unselected capture must carry the overview → kind → fact → draft sequence")
	}
	if strings.Contains(first.Instructions, overviewSuppressedNote) {
		t.Errorf("a fresh start must not carry the already-served note")
	}

	second := session.Start(t, "capture", nil)
	if strings.Contains(second.Instructions, overviewBodyMarker) {
		t.Fatalf("a second capture in the same session must not repeat the overview body")
	}
	if !strings.Contains(second.Instructions, overviewSuppressedNote) || !strings.Contains(second.Instructions, basefacts.OverviewFactID) {
		t.Errorf("the suppressed serve must still point at the overview fact, got: %.300q", second.Instructions)
	}
	// Capture adds no discriminator cue of its own: with the overview body
	// suppressed, the discrimination fact appears nowhere in the serve.
	if strings.Contains(second.Instructions, basefacts.DiscriminationFactID) {
		t.Errorf("capture must not carry a parallel discrimination-fact cue outside the overview body")
	}
	if !strings.Contains(first.Instructions, basefacts.DiscriminationFactID) {
		t.Errorf("the served overview body should remain the home of the discrimination pointer")
	}
}

func TestCaptureStartPriorShowReadSuppressesOverview(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "dk-show")
	session.LogRead(t, "show", []string{basefacts.OverviewFactID}, nil)

	serve := session.Start(t, "capture", nil)
	if strings.Contains(serve.Instructions, overviewBodyMarker) {
		t.Fatalf("a prior full show of the overview must suppress the automatic serve")
	}
	if !strings.Contains(serve.Instructions, overviewSuppressedNote) {
		t.Errorf("the suppressed serve must say the overview was already served")
	}
}

func TestCaptureStartSummaryReadDoesNotSuppressOverview(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "dk-summary")
	session.LogRead(t, "search", nil, []string{basefacts.OverviewFactID})

	serve := session.Start(t, "capture", nil)
	if !strings.Contains(serve.Instructions, overviewBodyMarker) {
		t.Fatalf("a summary-only read must not suppress the full overview serve")
	}
}

func TestCaptureStartPreselectedKindNamesAuthoringFact(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "dk-kind")

	serve := session.Start(t, "capture", map[string]any{"kind": "directive"})
	if !strings.Contains(serve.Instructions, basefacts.DirectiveFactID) {
		t.Fatalf("a directive-preselected capture must name the directive authoring fact, got: %.300q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "directive kind's authoring fact") {
		t.Errorf("the authoring-fact pointer should say which kind it belongs to")
	}

	// The pointer is instructional, never a gate: a valid draft reported
	// without any fact read advances past assemble.
	advanced := session.Report(t, serve.Instance, map[string]any{
		"body":        "New graph writes go through the construction boundary so validation stays single-path.",
		"entryKind":   "directive",
		"layer":       "tactical",
		"confidence":  "high",
		"intent":      "guiding",
		"widenReport": "searched the graph for prior write-path decisions before drafting",
	})
	if advanced.Step == "assemble" {
		t.Fatalf("an unread authoring fact must not gate assemble; still held with missing=%v diagnostics=%v", advanced.Missing, advanced.Diagnostics)
	}
}

func TestCaptureKindSelectedMidAssembleGetsAuthoringFactPointer(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "dk-midkind")

	serve := session.Start(t, "capture", nil)
	// The full overview body indexes every authoring fact, so absence is
	// asserted on the pointer block, not the ID.
	if strings.Contains(serve.Instructions, "Selected-kind authoring fact") {
		t.Fatalf("an unselected start must not carry the selected-kind pointer yet")
	}

	held := session.Report(t, serve.Instance, map[string]any{"entryKind": "directive"})
	proctest.RequireStep(t, held, "assemble")
	if !strings.Contains(held.Instructions, "Selected-kind authoring fact") || !strings.Contains(held.Instructions, basefacts.DirectiveFactID) {
		t.Fatalf("reporting the kind mid-assemble must surface its authoring-fact pointer on the re-serve")
	}
}

func TestCaptureStartKindWithoutAuthoringFactGetsNoPointer(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "dk-contract")

	serve := session.Start(t, "capture", map[string]any{"kind": "contract"})
	if strings.Contains(serve.Instructions, "Before writing the first draft, pull the") {
		t.Fatalf("a kind shipping no authoring fact must get no pointer, got: %.300q", serve.Instructions)
	}
}

func TestCaptureStartReattachedSessionKeepsOverviewGrounding(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "dk-conn1")
	first := session.Start(t, "capture", nil)
	if !strings.Contains(first.Instructions, overviewBodyMarker) {
		t.Fatalf("the first capture must serve the overview in full")
	}

	resumed, result := world.Resume(t, session.ID, "dk-conn2")
	for _, open := range result.Open {
		if open.Procedure == "capture" && strings.Contains(open.Instructions, overviewBodyMarker) {
			t.Errorf("re-attachment must not replay the overview body in the re-served capture step")
		}
	}

	serve := resumed.Start(t, "capture", nil)
	if strings.Contains(serve.Instructions, overviewBodyMarker) {
		t.Fatalf("a capture started after re-attachment must not repeat the overview body")
	}
	if !strings.Contains(serve.Instructions, overviewSuppressedNote) {
		t.Errorf("the re-attached serve must still point at the overview fact")
	}
}

func TestCaptureStartFailsLoudOnEmptyOverviewFact(t *testing.T) {
	dir := t.TempDir()
	// A project fact superseding the base type-system overview with an empty
	// body — the live-head resolution then lands on a fact with nothing to
	// serve.
	proctest.WriteRawEntry(t, dir, "20260819-100000-s-prc-ovr",
		"---\ntype: signal\nkind: fact\nlayer: process\nsummary: An empty project override of the type-system overview.\nsupersedes:\n    - "+basefacts.OverviewFactID+"\n---\n")

	world := proctest.NewWorld(t, proctest.WithGraphDir(dir))
	session := world.Open(t, "dk-fail")

	_, err := session.StartErr(t, "capture", nil)
	if err == nil {
		t.Fatal("a capture over an empty overview override must fail loudly, not drop the guidance")
	}
	if !strings.Contains(err.Error(), "entryChains") || !strings.Contains(err.Error(), "empty body") {
		t.Fatalf("the failure must name the inject and the empty fact, got: %v", err)
	}
}
