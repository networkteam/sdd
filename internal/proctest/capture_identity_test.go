// Capture behavior for the identity kinds: actor and role flow through the
// shared capture procedure without the refs+topics the gate demands of
// ordinary kinds, playback renders the identity fields, and a role's bound
// actor must resolve. Ported from internal/engine.
package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

const identityActorID = "20260601-100000-s-prc-act"

// newIdentityWorld is newCaptureWorld plus one actor (canonical
// "Christopher"), so role captures resolve and participantsCanonical leaves
// grace mode with the resolved participant still valid.
func newIdentityWorld(t *testing.T, connID string) (*proctest.World, *proctest.Session) {
	t.Helper()
	entries := append(captureFixtureEntries(), &model.Entry{
		ID: identityActorID, Type: model.TypeSignal, Kind: model.KindActor, Layer: model.LayerProcess, Canonical: "Christopher",
		Summary: "Christopher, the known actor role captures bind to.",
		Content: "Christopher, the known actor role captures bind to.",
	})
	world := proctest.NewWorld(t, proctest.WithEntries(entries...))
	session := world.Open(t, connID)
	session.LogRead(t, "show", []string{captureRefID}, nil)
	return world, session
}

// actorDraft drafts a NEW actor: the real write gate blocks reusing an
// existing chain's canonical (actor-canonical-reused), so unlike the engine
// fixture the drafted identity must be distinct from the fixture actor.
func actorDraft() map[string]any {
	return map[string]any{
		"body":        "Maria is CEO of networkteam with a full-stack background — the identity she brings from outside this project.",
		"entryKind":   "actor",
		"layer":       "process",
		"canonical":   "Maria",
		"aliases":     []any{"Mia", "MW"},
		"confidence":  "high",
		"widenReport": "checked the graph for an existing Maria actor before drafting",
	}
}

func roleDraft() map[string]any {
	return map[string]any{
		"body":        "Christopher is the principal developer here and holds the strategic and conceptual calls.",
		"entryKind":   "role",
		"layer":       "process",
		"roleActor":   "Christopher",
		"confidence":  "high",
		"widenReport": "confirmed the Christopher actor exists to bind this role",
	}
}

func TestCapture_ActorGatePassesWithoutRefsOrTopics(t *testing.T) {
	world, session := newIdentityWorld(t, "identity-actor")
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance

	serve = session.Report(t, instance, actorDraft())
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, "canonical: Maria") {
		t.Errorf("playback should render the canonical, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "Mia") || !strings.Contains(serve.Instructions, "MW") {
		t.Errorf("playback should render aliases, got %q", serve.Instructions)
	}

	serve = session.Answer(t, instance, "playback", "confirm", nil, "that's me")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("actor capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if entry.Canonical != "Maria" {
		t.Fatalf("persisted canonical = %q", entry.Canonical)
	}
}

func TestCapture_ActorMissingCanonicalStalls(t *testing.T) {
	_, session := newIdentityWorld(t, "identity-no-canonical")
	serve := session.Start(t, "capture", nil)
	draft := actorDraft()
	delete(draft, "canonical")
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, "does not satisfy its kind's structural rules") {
		t.Fatalf("diagnostics = %v, want the draftValidates message (canonical is the actor kind's structural rule)", serve.Diagnostics)
	}
}

func TestCapture_ActorMalformedAliasStalls(t *testing.T) {
	_, session := newIdentityWorld(t, "identity-bad-alias")
	serve := session.Start(t, "capture", nil)
	draft := actorDraft()
	// An alias repeating the canonical is malformed.
	draft["aliases"] = []any{"Maria"}
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, "does not satisfy its kind's structural rules") {
		t.Fatalf("diagnostics = %v, want the draftValidates message (malformed alias)", serve.Diagnostics)
	}
}

func TestCapture_RoleGateResolvesBoundActor(t *testing.T) {
	_, session := newIdentityWorld(t, "identity-role")
	serve := session.Start(t, "capture", nil)
	serve = session.Report(t, serve.Instance, roleDraft())
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, "role of: Christopher") {
		t.Errorf("playback should render the bound actor, got %q", serve.Instructions)
	}
}

func TestCapture_RoleUnresolvableActorBlocks(t *testing.T) {
	_, session := newIdentityWorld(t, "identity-ghost-actor")
	serve := session.Start(t, "capture", nil)
	draft := roleDraft()
	draft["roleActor"] = "Ghost"
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, "roleActor does not name an actor known to the graph") {
		t.Fatalf("diagnostics = %v, want the roleActorResolves message", serve.Diagnostics)
	}
}

func TestCapture_RecognitionModeSoftensPlayback(t *testing.T) {
	_, session := newIdentityWorld(t, "identity-recognition")
	serve := session.Start(t, "capture", nil)
	draft := actorDraft()
	draft["recognitionMode"] = true
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, "recognition") {
		t.Errorf("recognition-mode playback should soften to recognition framing, got %q", serve.Instructions)
	}
	// The invariant holds: the body is still shown verbatim.
	if !strings.Contains(serve.Instructions, "CEO of networkteam") {
		t.Errorf("recognition-mode playback must still render the body verbatim, got %q", serve.Instructions)
	}
}

func TestCapture_OrdinaryKindStrayCanonicalBlocks(t *testing.T) {
	_, session := newIdentityWorld(t, "identity-stray-canonical")
	serve := session.Start(t, "capture", nil)
	// A canonical on a gap is a stray per-kind field — the boundary's
	// projection reports it instead of a kind branch silently ignoring it.
	serve = session.Report(t, serve.Instance, map[string]any{
		"body":        "A tactical gap carrying a stray canonical.",
		"entryKind":   "gap",
		"layer":       "tactical",
		"canonical":   "Christopher",
		"confidence":  "medium",
		"widenReport": "widened",
	})
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, "does not satisfy its kind's structural rules") {
		t.Fatalf("diagnostics = %v, want the draftValidates message (stray canonical)", serve.Diagnostics)
	}
}

// TestCapture_RootEntryPassesWithoutRefsOrTopics pins the boundary-delegated
// gate: refs and topics are not per-kind requirements the type-system
// contract backs, so a genuinely root entry is not forced to invent a
// reference and a topic-less draft passes to the guide.
func TestCapture_RootEntryPassesWithoutRefsOrTopics(t *testing.T) {
	_, session := newIdentityWorld(t, "identity-root")
	serve := session.Start(t, "capture", nil)
	serve = session.Report(t, serve.Instance, map[string]any{
		"body":        "A genuinely root observation with nothing upstream to point at.",
		"entryKind":   "gap",
		"layer":       "tactical",
		"confidence":  "medium",
		"widenReport": "searched for prior art from several angles; nothing bears on this",
	})
	if serve.Step == "assemble" {
		t.Fatalf("a root entry must pass assemble without refs or topics, diagnostics=%v", serve.Diagnostics)
	}
}
