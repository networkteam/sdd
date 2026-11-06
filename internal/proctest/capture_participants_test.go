// Capture behavior for an entry's participants: the drafting agent is served
// the names to choose from, a reported list reaches the entry and stays
// correctable at playback, an alias of a recorded actor stalls naming the
// canonical, and a name no actor declares yet passes.
package proctest_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

func newParticipantsWorld(t *testing.T, connID string) (*proctest.World, *proctest.Session) {
	t.Helper()
	entries := append(captureFixtureEntries(), &model.Entry{
		ID: identityActorID, Type: model.TypeSignal, Kind: model.KindActor, Layer: model.LayerProcess,
		Canonical: "Christopher", Aliases: []string{"Christopher Hlubek"},
		Summary: "Christopher, the recorded actor.", Content: "Christopher, the recorded actor.",
	})
	world := proctest.NewWorld(t, proctest.WithEntries(entries...))
	session := world.Open(t, connID)
	session.LogRead(t, "show", []string{captureRefID}, nil)
	return world, session
}

func confirmCapture(t *testing.T, world *proctest.World, session *proctest.Session, instance string) *model.Entry {
	t.Helper()
	serve := session.Answer(t, instance, "playback", "confirm", nil, "capture it")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}
	return proctest.LoadEntry(t, world.GraphDir, entryID)
}

func TestCapture_AssembleServesParticipantNames(t *testing.T) {
	_, session := newParticipantsWorld(t, "participants-lane")
	serve := session.Start(t, "capture", nil)
	proctest.RequireStep(t, serve, "assemble")

	lane := laneText(t, serve, "participants")
	for _, want := range []string{"Local participant: Christopher", "- Christopher (aliases: Christopher Hlubek)"} {
		if !strings.Contains(lane, want) {
			t.Errorf("participants lane missing %q, got %q", want, lane)
		}
	}
}

func TestCapture_ReportedParticipantsReachTheEntry(t *testing.T) {
	world, session := newParticipantsWorld(t, "participants-written")
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance

	draft := captureDraft()
	// Benno has no actor yet: the actor may follow the first attribution.
	draft["participants"] = []any{"Christopher", "Benno"}
	serve = session.Report(t, instance, draft)
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, "Benno") {
		t.Errorf("playback should render the participants, got %q", serve.Instructions)
	}

	entry := confirmCapture(t, world, session, instance)
	if want := []string{"Christopher", "Benno"}; !slices.Equal(entry.Participants, want) {
		t.Fatalf("participants = %v, want %v", entry.Participants, want)
	}
}

func TestCapture_UnreportedParticipantsWriteTheLocalParticipant(t *testing.T) {
	world, session := newParticipantsWorld(t, "participants-default")
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	serve = session.Report(t, instance, captureDraft())
	proctest.RequireStep(t, serve, "playback")

	entry := confirmCapture(t, world, session, instance)
	if want := []string{"Christopher"}; !slices.Equal(entry.Participants, want) {
		t.Fatalf("participants = %v, want %v", entry.Participants, want)
	}
}

func TestCapture_ParticipantAliasStallsNamingTheCanonical(t *testing.T) {
	_, session := newParticipantsWorld(t, "participants-alias")
	serve := session.Start(t, "capture", nil)

	draft := captureDraft()
	draft["participants"] = []any{"Christopher Hlubek"}
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, `"Christopher Hlubek" is an alias of actor "Christopher"`) {
		t.Fatalf("diagnostics = %v, want the alias named with its canonical", serve.Diagnostics)
	}
}

func TestCapture_ParticipantsCorrectableAtPlayback(t *testing.T) {
	world, session := newParticipantsWorld(t, "participants-adjust")
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	session.Report(t, instance, captureDraft())

	serve = session.Answer(t, instance, "playback", "adjust",
		map[string]any{"participants": []any{"Benno"}}, "Benno decided this, not me")
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, "Benno") {
		t.Errorf("playback should render the adjusted participants, got %q", serve.Instructions)
	}

	entry := confirmCapture(t, world, session, instance)
	if want := []string{"Benno"}; !slices.Equal(entry.Participants, want) {
		t.Fatalf("participants = %v, want %v", entry.Participants, want)
	}
}
