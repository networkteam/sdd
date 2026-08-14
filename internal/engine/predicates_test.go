package engine

import (
	"strings"
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

func refsFixtureStore(t *testing.T) *Store {
	t.Helper()
	entry := specFixture(t, `state:
    refs: {type: list<ref>, desc: refs}
    closes: {type: list<entry-id>, desc: closes}
steps:
    - id: only
      collect: ["refs?", "closes?"]
      transitions:
          - when: refsResolve
            to: end(completed)
`, "")
	spec, err := ParseSpec(entry)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(spec)
}

func TestRefsResolveCrossRepo(t *testing.T) {
	graph := predicateGraph(t)
	store := refsFixtureStore(t)
	ctx := &Context{Store: store, Graph: graph}

	// A syntactically valid cross-repo ref passes the gate without local
	// resolution; a malformed one blocks.
	if _, err := store.WriteState(map[string]any{"refs": []any{
		map[string]any{"id": "github.com/networkteam/other:20260601-120000-s-tac-abc", "kind": "grounded-in"},
	}}); err != nil {
		t.Fatal(err)
	}
	if !evalPredicate(t, "refsResolve", ctx) {
		t.Error("syntactically valid cross-repo ref should pass the resolve gate")
	}

	// A malformed cross-repo ref is rejected by the typed store before any
	// gate runs.
	if _, err := store.WriteState(map[string]any{"refs": []any{
		map[string]any{"id": "nohost:20260601-120000-s-tac-abc", "kind": "grounded-in"},
	}}); err == nil {
		t.Error("malformed cross-repo ref must be rejected at store write")
	}

	// Lifecycle fields get no cross-repo carve-out: entry-id typed fields
	// reject the colon form outright.
	if _, err := store.WriteState(map[string]any{
		"closes": []any{"github.com/networkteam/other:20260601-120000-s-tac-abc"},
	}); err == nil {
		t.Error("cross-repo ID in closes must be rejected at store write")
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

func specLoadsFixtureStore(t *testing.T) *Store {
	t.Helper()
	entry := specFixture(t, `state:
    entryKind: {type: entry-kind, desc: kind}
    procedureSpec: {type: procedure-spec, desc: the workflow declaration}
    canonical: {type: text, desc: name}
    class: {type: text, desc: class}
    body: {type: text, desc: body}
steps:
    - id: only
      collect: [entryKind, "procedureSpec?", "canonical?", "class?", "body?"]
      transitions:
          - when: specLoads
            to: end(completed)
`, "")
	spec, err := ParseSpec(entry)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(spec)
}

func TestSpecLoads(t *testing.T) {
	store := specLoadsFixtureStore(t)
	ctx := &Context{Store: store}

	if !evalPredicate(t, "specLoads", ctx) {
		t.Error("a draft with no kind yet should pass vacuously")
	}
	if _, err := store.WriteState(map[string]any{"entryKind": "gap"}); err != nil {
		t.Fatal(err)
	}
	if !evalPredicate(t, "specLoads", ctx) {
		t.Error("a non-procedure draft should pass vacuously")
	}

	if _, err := store.WriteState(map[string]any{"entryKind": "procedure", "canonical": "test-move"}); err != nil {
		t.Fatal(err)
	}
	if evalPredicate(t, "specLoads", ctx) {
		t.Error("a procedure draft without a workflow should fail")
	}

	workflow := func(guard string) map[string]any {
		return map[string]any{
			"state": map[string]any{
				"synthesis": map[string]any{"type": "text", "desc": "outcome"},
			},
			"steps": []any{
				map[string]any{
					"id":      "examine",
					"collect": []any{"synthesis"},
					"transitions": []any{
						map[string]any{"when": guard, "to": "end(completed)"},
					},
				},
			},
		}
	}
	if _, err := store.WriteState(map[string]any{"procedureSpec": workflow("hasSynthesis"), "body": "Move.\n\n## unit: examine\n\nExamine."}); err != nil {
		t.Fatal(err)
	}
	if !evalPredicate(t, "specLoads", ctx) {
		t.Error("a valid workflow should pass")
	}

	if _, err := store.WriteState(map[string]any{"procedureSpec": workflow("hasTimeline")}); err != nil {
		t.Fatal(err)
	}
	if evalPredicate(t, "specLoads", ctx) {
		t.Error("a workflow naming an unknown predicate should fail")
	}
	p, _ := NewRegistry().Predicate("specLoads")
	if detail := p.FailDetail(ctx); !strings.Contains(detail, "hasTimeline") {
		t.Errorf("FailDetail = %q, want the unknown predicate named", detail)
	}
}
