package engine

import (
	"strings"
	"testing"
)

// Capture-through-the-engine tests for the slice-2 focus kind: a focus flows
// through the shared capture spec on its involvement alone — no refs or topics
// the gate demands of ordinary kinds — its involvement targets must resolve,
// and playback renders the advances list. The default fixture graph already
// holds fixtureRefID, so focus involvement resolves against it.

func focusDraft() map[string]any {
	return map[string]any{
		"body":        "Advance the fixture directive to done this cycle — the current focus.",
		"entryKind":   "focus",
		"layer":       "tactical",
		"involvement": []any{map[string]any{"target": fixtureRefID}},
		"confidence":  "high",
		"widenReport": "checked the graph for an active focus over this target before drafting",
	}
}

func TestCapture_FocusGatePassesWithoutRefsOrTopics(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Report(sv.Instance, focusDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("focus with involvement (no refs/topics) should reach playback, got %q with failures %+v", sv.Step, sv.Failing)
	}
	if !strings.Contains(sv.Instructions, "advances:") || !strings.Contains(sv.Instructions, fixtureRefID) {
		t.Errorf("playback should render the advances list with the target, got %q", sv.Instructions)
	}

	sv, err = env.session.Answer(sv.Instance, "playback", "confirm", nil, "that's the focus")
	if err != nil {
		t.Fatal(err)
	}
	if env.newCalls != 1 || sv.Step != "verifySummary" {
		t.Fatalf("focus confirm should write and reach verifySummary, got step=%q newCalls=%d", sv.Step, env.newCalls)
	}
}

func TestCapture_FocusMissingInvolvementStalls(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := focusDraft()
	delete(draft, "involvement")
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("focus without involvement must hold assemble, got %q", sv.Step)
	}
	if !hasFailing(sv.Failing, "draftValidates") {
		t.Fatalf("failing = %+v, want draftValidates (involvement is the focus kind's structural rule)", sv.Failing)
	}
}

func TestCapture_FocusUnresolvableTargetBlocks(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := focusDraft()
	// A syntactically valid ID that resolves to no entry in the graph.
	draft["involvement"] = []any{map[string]any{"target": "20260101-090000-d-tac-ghost"}}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("focus with an unresolvable target must hold assemble, got %q", sv.Step)
	}
	if !hasFailing(sv.Failing, "draftValidates") {
		t.Fatalf("failing = %+v, want draftValidates (involvement targets resolve inside the boundary's focus rules)", sv.Failing)
	}
}

func TestCapture_FocusRendersActorsAndWhen(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := focusDraft()
	draft["involvement"] = []any{map[string]any{
		"target": fixtureRefID,
		"actors": []any{"Christopher"},
		"when":   map[string]any{"from": "2026-01-01", "to": "2026-02-01"},
	}}
	draft["focusWhen"] = map[string]any{"from": "2026-01-01"}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("focus with per-target actors/when should reach playback, got %q failing=%+v", sv.Step, sv.Failing)
	}
	for _, want := range []string{"Christopher", "2026-01-01", "2026-02-01", "when: 2026-01-01"} {
		if !strings.Contains(sv.Instructions, want) {
			t.Errorf("playback should render %q, got %q", want, sv.Instructions)
		}
	}
}

func TestCapture_FocusMalformedInvolvementRejected(t *testing.T) {
	cases := map[string]any{
		"bad target ID": []any{map[string]any{"target": "not-an-id"}},
		"bad when date": []any{map[string]any{"target": fixtureRefID, "when": map[string]any{"from": "January"}}},
		"empty actor":   []any{map[string]any{"target": fixtureRefID, "actors": []any{""}}},
	}
	for name, involvement := range cases {
		t.Run(name, func(t *testing.T) {
			env := newFixtureEnv(t)
			sv, err := env.session.Start(env.spec, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			draft := focusDraft()
			draft["involvement"] = involvement
			if _, err := env.session.Report(sv.Instance, draft); err == nil {
				t.Fatalf("%s: malformed involvement should be rejected by the type validator, got no error", name)
			}
		})
	}
}

func TestReportSchema_AdvertisesInvolvementShape(t *testing.T) {
	env := newFixtureEnv(t)
	step := env.spec.StepByID["assemble"]
	schema := env.spec.ReportSchemaForStep(step)
	props, _ := schema["properties"].(map[string]any)

	// involvement and focusWhen are optional state fields, so each renders as a
	// nullable anyOf wrapper — unwrap to the typed branch that carries the shape.
	inv := nullableArrayBranch(t, props["involvement"].(map[string]any))
	item, _ := inv["items"].(map[string]any)
	if item == nil || item["type"] != "object" {
		t.Fatalf("involvement items = %v, want an object", inv["items"])
	}
	itemProps, _ := item["properties"].(map[string]any)
	target, _ := itemProps["target"].(map[string]any)
	if target == nil || target["pattern"] == nil {
		t.Errorf("involvement.target should advertise the entry-id pattern, got %v", itemProps["target"])
	}
	when, _ := itemProps["when"].(map[string]any)
	whenProps, _ := when["properties"].(map[string]any)
	if _, ok := whenProps["from"]; !ok {
		t.Errorf("involvement.when should advertise from/to, got %v", when)
	}
	// minProperties matches FocusWhen.Validate: an empty when {} is rejected,
	// so the advertised contract must forbid it too.
	if when["minProperties"] != 1 {
		t.Errorf("involvement.when should advertise minProperties: 1, got %v", when["minProperties"])
	}
	required, _ := item["required"].([]string)
	if len(required) != 1 || required[0] != "target" {
		t.Errorf("involvement required = %v, want [target]", required)
	}

	// The standalone focusWhen type advertises the same {from, to} object,
	// including the minProperties floor.
	fw := nullableObjectBranch(t, props["focusWhen"].(map[string]any))
	if fw["minProperties"] != 1 {
		t.Errorf("focusWhen should advertise minProperties: 1, got %v", fw["minProperties"])
	}
}

// nullableArrayBranch unwraps the anyOf wrapper an optional array-typed state
// field renders as, returning the array branch (mirrors nullableObjectBranch).
func nullableArrayBranch(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	branches, _ := schema["anyOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("nullable schema = %#v", schema)
	}
	var array map[string]any
	var hasNull bool
	for _, branch := range branches {
		candidate, _ := branch.(map[string]any)
		switch candidate["type"] {
		case "array":
			array = candidate
		case "null":
			hasNull = true
		}
	}
	if array == nil || !hasNull {
		t.Fatalf("nullable schema = %#v", schema)
	}
	return array
}
