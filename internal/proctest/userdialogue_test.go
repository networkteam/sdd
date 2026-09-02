package proctest_test

// Ported from internal/engine/userdialogue_test.go: the shipped user-dialogue
// shell driven through the real application door. OpenWorkflow itself enforces
// the shell class, the real registry serves sessionInfo/procedureList/factIndex,
// and the framing lanes render through the real serve-safe viewLayout queries.

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/baseprocedures"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
	sdd "github.com/networkteam/sdd/pkg/application"
)

// openShell opens the session through the real door and keeps the opening
// serve, which the harness's Open discards. Candidate to graduate into the
// harness if more suites need the opening serve.
func openShell(t *testing.T, world *proctest.World, connID string) (*proctest.Session, *sdd.WorkflowServe) {
	t.Helper()
	workflow, serve, err := world.App.OpenWorkflow(t.Context(), world.Identity, "proctest", sdd.WorkflowOpenRequest{ClientName: connID})
	if err != nil {
		t.Fatal(err)
	}
	return &proctest.Session{World: world, WF: workflow, ID: serve.Session}, serve
}

// writeIndexedFact writes an index-enrolled fact, a shape *model.Entry does
// not model (index frontmatter). Candidate to graduate into the harness.
func writeIndexedFact(t *testing.T, graphDir, id, title, topic string) {
	t.Helper()
	proctest.WriteRawEntry(t, graphDir, id, "---\n"+
		"type: signal\nkind: fact\nlayer: process\n"+
		"summary: "+title+".\n"+
		"topics:\n    - "+topic+"\n"+
		"index:\n    title: "+title+"\n    topic: "+topic+"\n"+
		"---\n\n"+title+", in full.\n")
}

func TestUserDialogueOpeningRendersFactPointersFromData(t *testing.T) {
	const id = "20991231-235959-s-prc-xyz"
	world := proctest.NewWorld(t)
	writeIndexedFact(t, world.GraphDir, id, "Standalone retrieval cue", "guides/views")
	_, serve := openShell(t, world, "dlg-facts")

	want := "- `" + id + "` — Standalone retrieval cue"
	if !strings.Contains(serve.Instructions, want) || !strings.Contains(serve.Instructions, "Pull the relevant fact in full first") {
		t.Fatalf("opening fact index missing %q:\n%s", want, serve.Instructions)
	}

	entries, err := baseprocedures.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Canonical == "user-dialogue" && strings.Contains(entry.Content, id) {
			t.Fatal("the shipped shell entry hard-codes a fact ID")
		}
	}
}

func TestUserDialogueOpeningOmitsEmptyFactIndex(t *testing.T) {
	t.Skip("not expressible over the real application: the shipped base facts " +
		"(internal/basefacts) merge into every graph and enroll in the fact index, " +
		"so the index is never empty without faking the embedded set; the " +
		"omit-when-empty template branch stays covered by the engine-level test")
}

func TestUserDialogue_OpeningServeAndConclude(t *testing.T) {
	world := proctest.NewWorld(t)
	session, serve := openShell(t, world, "dlg-serve")

	// OpenWorkflow rejects any non-shell procedure, so reaching a serve at all
	// pins the shell class the old test read off the spec.
	if serve.Procedure != "user-dialogue" {
		t.Fatalf("the session should open on the user-dialogue shell, got %q", serve.Procedure)
	}
	if serve.Step != "junction" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("the shell should rest on its user junction, got step %q chooser %+v", serve.Step, serve.PendingChooser)
	}
	if serve.Goal != "dialogue freely; start a move when something crystallizes" {
		t.Fatalf("the junction should carry the standing goal, got %q", serve.Goal)
	}
	// The unit keeps the real move enumeration and the standing goal. The
	// recovery line the old test faked is not asserted here: a real recovery
	// notice needs an interrupted write in the session store, which cannot be
	// staged without faking — a fresh world serves no recovery block.
	for _, want := range []string{"- capture", "- engage", "Standing goal"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Errorf("opening serve should carry %q, got %q", want, serve.Instructions)
		}
	}

	// Participant/language/search live in the application framing's info
	// block, not the unit.
	blocks, err := session.WF.Framing(t.Context(), world.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 || !strings.Contains(blocks[0], "Local participant: Christopher") || !strings.Contains(blocks[0], "Search:") {
		t.Fatalf("framing should lead with the session info block, got %q", blocks)
	}

	// Conclude cascades through wrap to completion in the user's one answer.
	done := session.Answer(t, serve.Instance, "junction", "conclude", nil, "we're done")
	proctest.RequireStatus(t, done, "completed")
}

// TestUserDialogue_ConcludeCarriesNoDischargeGate pins the un-gated conclude:
// the junction's one answer lands directly on completion, so a session is
// closable without first discharging what it raised — which loose ends deserve
// carrying is the user's judgment, not the engine's (d-tac-k4q). The old
// spec-structure checks (no threads step, wrap's single unconditional end)
// port as this behavior: one answer, nothing missing, no further chooser.
func TestUserDialogue_ConcludeCarriesNoDischargeGate(t *testing.T) {
	world := proctest.NewWorld(t)
	session, serve := openShell(t, world, "dlg-gate")

	// The teaching half of leaving threads behind: the agent names what is
	// open before the offer, since the answer is final.
	for _, want := range []string{"name each of those threads", "left behind", "stays resumable by its handle"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Errorf("the opening serve should teach ending the session; missing %q", want)
		}
	}

	done := session.Answer(t, serve.Instance, "junction", "conclude", nil, "wrap it up")
	proctest.RequireStatus(t, done, "completed")
	if len(done.Missing) > 0 || done.PendingChooser != nil {
		t.Fatalf("conclude must end without a gate, got missing %v chooser %+v", done.Missing, done.PendingChooser)
	}
}

// The framing lanes the old test stubbed as "view: <layout>" render here over
// the real graph: seeded content surfaces under the shell's declared lane
// names, one dedupable block per lane.
func TestUserDialogue_FramingLanesRenderRealGraphContent(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(
		&model.Entry{
			ID: "20260601-090000-d-tac-gdr", Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical,
			Intent:  model.IntentGuiding,
			Summary: "Always test through the exported surface.",
			Content: "Always test through the exported surface.",
		},
		&model.Entry{
			ID: "20260601-091000-s-prc-wpr", Type: model.TypeSignal, Kind: model.KindFact, Layer: model.LayerProcess,
			Summary: "Keep dialogue turns short.",
			Content: "Keep dialogue turns short.",
			Topics:  proctest.MustTopics("principles/interactive"),
		},
	))
	session, _ := openShell(t, world, "dlg-framing")

	blocks, err := session.WF.Framing(t.Context(), world.Identity)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(blocks, "\n===\n")
	for _, want := range []string{
		"Local participant: Christopher",
		"Working principles", "Keep dialogue turns short",
		"Guiding directives", "Always test through the exported surface",
		"Recent graph movement",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("framing should carry %q, got:\n%s", want, joined)
		}
	}
	if !strings.Contains(blocks[0], "Local participant: Christopher") {
		t.Errorf("the info block should lead the framing, got %q", blocks[0])
	}
}
