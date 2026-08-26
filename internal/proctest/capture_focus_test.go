// Capture behavior for the focus kind: a focus flows through the shared
// capture procedure on its involvement alone — no refs or topics the gate
// demands of ordinary kinds — its involvement targets must resolve, and
// playback renders the advances list. Ported from internal/engine.
package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/proctest"
)

func focusDraft() map[string]any {
	return map[string]any{
		"body":        "Advance the fixture directive to done this cycle — the current focus.",
		"entryKind":   "focus",
		"layer":       "tactical",
		"involvement": []any{map[string]any{"target": captureRefID}},
		"confidence":  "high",
		"widenReport": "checked the graph for an active focus over this target before drafting",
	}
}

func TestCapture_FocusGatePassesWithoutRefsOrTopics(t *testing.T) {
	world, session := newCaptureWorld(t, "focus-pass")
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance

	serve = session.Report(t, instance, focusDraft())
	proctest.RequireStep(t, serve, "playback")
	if !strings.Contains(serve.Instructions, "involvement:") || !strings.Contains(serve.Instructions, captureRefID) {
		t.Errorf("playback draft block should carry the involvement with its target, got %q", serve.Instructions)
	}

	serve = session.Answer(t, instance, "playback", "confirm", nil, "that's the focus")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("focus capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if len(entry.Involvement) != 1 || entry.Involvement[0].Target != captureRefID {
		t.Fatalf("persisted involvement = %+v", entry.Involvement)
	}
}

func TestCapture_FocusMissingInvolvementStalls(t *testing.T) {
	_, session := newCaptureWorld(t, "focus-missing")
	serve := session.Start(t, "capture", nil)
	draft := focusDraft()
	delete(draft, "involvement")
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, "does not satisfy its kind's structural rules") {
		t.Fatalf("diagnostics = %v, want the draftValidates message (involvement is the focus kind's structural rule)", serve.Diagnostics)
	}
}

func TestCapture_FocusUnresolvableTargetBlocks(t *testing.T) {
	_, session := newCaptureWorld(t, "focus-ghost")
	serve := session.Start(t, "capture", nil)
	draft := focusDraft()
	// A syntactically valid ID that resolves to no entry in the graph.
	draft["involvement"] = []any{map[string]any{"target": "20260101-090000-d-tac-ghost"}}
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "assemble")
	if !hasDiagnostic(serve, "does not satisfy its kind's structural rules") {
		t.Fatalf("diagnostics = %v, want the draftValidates message (involvement targets resolve inside the boundary's focus rules)", serve.Diagnostics)
	}
}

func TestCapture_FocusRendersActorsAndWhen(t *testing.T) {
	_, session := newCaptureWorld(t, "focus-render")
	serve := session.Start(t, "capture", nil)
	draft := focusDraft()
	draft["involvement"] = []any{map[string]any{
		"target": captureRefID,
		"actors": []any{"Christopher"},
		"when":   map[string]any{"from": "2026-01-01", "to": "2026-02-01"},
	}}
	draft["focusWhen"] = map[string]any{"from": "2026-01-01"}
	serve = session.Report(t, serve.Instance, draft)
	proctest.RequireStep(t, serve, "playback")
	for _, want := range []string{"Christopher", "2026-01-01", "2026-02-01", "focusWhen:"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Errorf("playback should render %q, got %q", want, serve.Instructions)
		}
	}
}

func TestCapture_FocusMalformedInvolvementRejected(t *testing.T) {
	cases := map[string]any{
		"bad target ID": []any{map[string]any{"target": "not-an-id"}},
		"bad when date": []any{map[string]any{"target": captureRefID, "when": map[string]any{"from": "January"}}},
		"empty actor":   []any{map[string]any{"target": captureRefID, "actors": []any{""}}},
	}
	for name, involvement := range cases {
		t.Run(name, func(t *testing.T) {
			_, session := newCaptureWorld(t, "focus-malformed-"+name)
			serve := session.Start(t, "capture", nil)
			draft := focusDraft()
			draft["involvement"] = involvement
			if _, err := session.ReportErr(t, serve.Instance, draft); err == nil {
				t.Fatalf("%s: malformed involvement should be rejected by the type validator, got no error", name)
			}
		})
	}
}

func TestReportSchema_AdvertisesInvolvementShape(t *testing.T) {
	_, session := newCaptureWorld(t, "focus-schema")
	serve := session.Start(t, "capture", nil)
	props, _ := serve.ReportSchema["properties"].(map[string]any)

	// involvement and focusWhen are optional state fields, so each renders as a
	// nullable anyOf wrapper — unwrap to the typed branch that carries the shape.
	inv := nullableBranch(t, props["involvement"].(map[string]any), "array")
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
	fw := nullableBranch(t, props["focusWhen"].(map[string]any), "object")
	if fw["minProperties"] != 1 {
		t.Errorf("focusWhen should advertise minProperties: 1, got %v", fw["minProperties"])
	}
}

// nullableBranch unwraps the anyOf wrapper an optional state field renders as,
// returning the branch of the wanted type.
func nullableBranch(t *testing.T, schema map[string]any, typ string) map[string]any {
	t.Helper()
	branches, _ := schema["anyOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("nullable schema = %#v", schema)
	}
	var typed map[string]any
	var hasNull bool
	for _, branch := range branches {
		candidate, _ := branch.(map[string]any)
		switch candidate["type"] {
		case typ:
			typed = candidate
		case "null":
			hasNull = true
		}
	}
	if typed == nil || !hasNull {
		t.Fatalf("nullable schema = %#v", schema)
	}
	return typed
}
