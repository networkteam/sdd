package engine

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// Capture-through-the-engine tests for the slice-1 identity kinds: actor and
// role flow through the shared capture spec without the refs+topics the gate
// demands of ordinary kinds, playback renders the new fields, and the role's
// bound actor must resolve. They drive the shipped capture entry through the
// production loader, like the other per-procedure tests.

const identityActorID = "20260601-100000-s-prc-act"

// identityGraph holds one actor (canonical "Christopher") so role captures have
// something to resolve against, plus the ordinary fixture ref target.
func identityGraph(t *testing.T) *model.Graph {
	t.Helper()
	actor, err := model.ParseEntry(identityActorID+".md", `---
type: signal
layer: process
kind: actor
canonical: Christopher
---

Christopher, the known actor role captures bind to.
`)
	if err != nil {
		t.Fatal(err)
	}
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
	return model.NewGraph([]*model.Entry{actor, target})
}

// newIdentityEnv is newFixtureEnv over identityGraph — same fake write gate and
// injections, a graph that already knows an actor.
func newIdentityEnv(t *testing.T) *fixtureEnv {
	env := newFixtureEnv(t)
	env.engine.Graphs = StaticGraphs{Graph: identityGraph(t)}
	return env
}

func actorDraft() map[string]any {
	return map[string]any{
		"body":        "Christopher is CEO of networkteam with a full-stack background — the identity he brings from outside this project.",
		"entryKind":   "actor",
		"layer":       "process",
		"canonical":   "Christopher",
		"aliases":     []any{"Chris", "CH"},
		"confidence":  "high",
		"widenReport": "checked the graph for an existing Christopher actor before drafting",
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
	env := newIdentityEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Report(sv.Instance, actorDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("actor with canonical (no refs/topics) should reach playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
	if !strings.Contains(sv.Instructions, "canonical: Christopher") {
		t.Errorf("playback should render the canonical, got %q", sv.Instructions)
	}
	if !strings.Contains(sv.Instructions, "Chris") || !strings.Contains(sv.Instructions, "CH") {
		t.Errorf("playback should render aliases, got %q", sv.Instructions)
	}

	sv, err = env.session.Answer(sv.Instance, "playback", "confirm", nil, "that's me")
	if err != nil {
		t.Fatal(err)
	}
	if env.newCalls != 1 || sv.Step != "verifySummary" {
		t.Fatalf("actor confirm should write and reach verifySummary, got step=%q newCalls=%d", sv.Step, env.newCalls)
	}
}

func TestCapture_ActorMissingCanonicalStalls(t *testing.T) {
	env := newIdentityEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := actorDraft()
	delete(draft, "canonical")
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("actor without canonical must hold assemble, got %q", sv.Step)
	}
	if !hasFailing(sv.Failing, "hasCanonical") {
		t.Fatalf("failing = %+v, want hasCanonical", sv.Failing)
	}
}

func TestCapture_ActorMalformedAliasStalls(t *testing.T) {
	env := newIdentityEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := actorDraft()
	// An alias repeating the canonical is malformed.
	draft["aliases"] = []any{"Christopher"}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" || !hasFailing(sv.Failing, "aliasesWellFormed") {
		t.Fatalf("malformed alias must hold assemble on aliasesWellFormed, got step=%q failing=%+v", sv.Step, sv.Failing)
	}
}

func TestCapture_RoleGateResolvesBoundActor(t *testing.T) {
	env := newIdentityEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, roleDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("role bound to a known actor should reach playback, got %q failing=%+v", sv.Step, sv.Failing)
	}
	if !strings.Contains(sv.Instructions, "role of: Christopher") {
		t.Errorf("playback should render the bound actor, got %q", sv.Instructions)
	}
}

func TestCapture_RoleUnresolvableActorBlocks(t *testing.T) {
	env := newIdentityEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := roleDraft()
	draft["roleActor"] = "Ghost"
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("role bound to an unknown actor must hold assemble, got %q", sv.Step)
	}
	if !hasFailing(sv.Failing, "roleActorResolves") {
		t.Fatalf("failing = %+v, want roleActorResolves", sv.Failing)
	}
}

func TestCapture_RecognitionModeSoftensPlayback(t *testing.T) {
	env := newIdentityEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := actorDraft()
	draft["recognitionMode"] = true
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("recognition-mode actor should reach playback, got %q", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "recognition") {
		t.Errorf("recognition-mode playback should soften to recognition framing, got %q", sv.Instructions)
	}
	// The invariant holds: the body is still shown verbatim.
	if !strings.Contains(sv.Instructions, "CEO of networkteam") {
		t.Errorf("recognition-mode playback must still render the body verbatim, got %q", sv.Instructions)
	}
}

func TestCapture_OrdinaryKindStillDemandsRefsAndTopics(t *testing.T) {
	env := newIdentityEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// A gap with a canonical but no refs/topics must not slip through the
	// actor branch — the "other" branch guards on not-actor-and-not-role.
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"body":        "A tactical gap with no refs or topics.",
		"entryKind":   "gap",
		"layer":       "tac",
		"canonical":   "Christopher",
		"confidence":  "medium",
		"widenReport": "widened",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("a gap without refs/topics must hold assemble even with a canonical set, got %q", sv.Step)
	}
	if !hasFailing(sv.Failing, "hasRefs") && !hasFailing(sv.Failing, "hasTopics") {
		t.Fatalf("ordinary-kind gate must still demand refs/topics, failing=%+v", sv.Failing)
	}
}

func hasFailing(failing []FailedPredicate, name string) bool {
	for _, f := range failing {
		if f.Name == name {
			return true
		}
	}
	return false
}
