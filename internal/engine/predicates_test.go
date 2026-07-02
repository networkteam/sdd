package engine

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// predicateGraph builds a graph with one active actor (canonical
// Christopher) and one topic-carrying entry.
func predicateGraph(t *testing.T) *model.Graph {
	t.Helper()
	actor, err := model.ParseEntry("20260601-110000-s-prc-act.md", `---
type: signal
layer: prc
kind: actor
canonical: Christopher
aliases:
    - Chris
---

Christopher is a participant.
`)
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := model.ParseEntry("20260601-113000-d-tac-top.md", `---
type: decision
layer: tac
kind: directive
intent: pending
topics:
    - cli/ux
---

A directive carrying a topic.
`)
	if err != nil {
		t.Fatal(err)
	}
	return model.NewGraph([]*model.Entry{actor, tagged})
}

func predicateFixtureStore(t *testing.T) *Store {
	t.Helper()
	entry := specFixture(t, `state:
    topics: {type: list<label>, desc: labels}
    participants: {type: list<participant>, desc: canonicals}
steps:
    - id: only
      collect: ["topics?", "participants?"]
      transitions:
          - when: topicsKnown and participantsCanonical
            to: end(completed)
`, "")
	spec, err := ParseSpec(entry)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(spec)
}

func evalPredicate(t *testing.T, name string, ctx *Context) bool {
	t.Helper()
	p, ok := NewRegistry().Predicate(name)
	if !ok {
		t.Fatalf("predicate %s not registered", name)
	}
	v, err := p.Fn(ctx)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return v
}

func TestTopicsKnown(t *testing.T) {
	store := predicateFixtureStore(t)
	ctx := &Context{Store: store, Graph: predicateGraph(t)}

	if !evalPredicate(t, "topicsKnown", ctx) {
		t.Error("no topics in the store should pass")
	}
	if _, err := store.WriteState(map[string]any{"topics": []any{"cli/ux"}}); err != nil {
		t.Fatal(err)
	}
	if !evalPredicate(t, "topicsKnown", ctx) {
		t.Error("existing label (case-insensitive component match) should pass")
	}
	if _, err := store.WriteState(map[string]any{"topics": []any{"cli/ux", "brand/new"}}); err != nil {
		t.Fatal(err)
	}
	if evalPredicate(t, "topicsKnown", ctx) {
		t.Error("a label new to the graph should fail the guard")
	}
}

func TestParticipantsCanonical(t *testing.T) {
	store := predicateFixtureStore(t)
	ctx := &Context{Store: store, Graph: predicateGraph(t)}

	if _, err := store.WriteState(map[string]any{"participants": []any{"Christopher"}}); err != nil {
		t.Fatal(err)
	}
	if !evalPredicate(t, "participantsCanonical", ctx) {
		t.Error("active canonical should pass")
	}

	// An alias is a read-side convenience, never a valid participant value.
	if _, err := store.WriteState(map[string]any{"participants": []any{"Chris"}}); err != nil {
		t.Fatal(err)
	}
	if evalPredicate(t, "participantsCanonical", ctx) {
		t.Error("alias must fail the canonical check")
	}

	// Grace mode: a graph with no active actors skips the check.
	graceCtx := &Context{Store: store, Graph: fixtureGraph(t)}
	if !evalPredicate(t, "participantsCanonical", graceCtx) {
		t.Error("grace mode (no active actors) should pass")
	}
}

func TestRegistryListQuery(t *testing.T) {
	reg := NewRegistry()
	q, ok := reg.Query("registryList")
	if !ok {
		t.Fatal("registryList not registered")
	}
	result, err := q.Fn(&Context{}, map[string]any{"class": "predicate"})
	if err != nil {
		t.Fatal(err)
	}
	docs, ok := result.([]FuncDoc)
	if !ok || len(docs) == 0 {
		t.Fatalf("registryList = %v", result)
	}
	for _, d := range docs {
		if d.Class != ClassPredicate {
			t.Errorf("class filter leaked %s %s", d.Class, d.Name)
		}
	}
	if _, err := q.Fn(&Context{}, map[string]any{"class": "nope"}); err == nil {
		t.Error("unknown class must error")
	}

	// Registry names are unique across classes.
	if err := reg.RegisterQuery(Query{Doc: FuncDoc{Name: "hasBody"}}); err == nil {
		t.Error("cross-class name collision must be rejected")
	}
}
