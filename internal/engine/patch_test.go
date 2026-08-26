package engine_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

// patchEntry composes a synthetic procedure around a body + bodyPatch pair.
func patchEntry(t *testing.T, machine string) *model.Entry {
	t.Helper()
	content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: patchproc\n" +
		machine + "\n---\n\nsynthetic patch procedure\n\n## unit: draft\n\nDraft.\n"
	entry, err := model.ParseEntry("20260826-130000-d-prc-pat.md", content)
	if err != nil {
		t.Fatalf("fixture entry: %v", err)
	}
	return entry
}

// patchMachine holds at draft until goal arrives, leaving room for iterating
// body reports and patches.
const patchMachine = `state:
    body: {type: text, desc: the draft}
    bodyPatch: {type: list<search-replace>, optional: true, patchOf: body, desc: exact edits to body}
    goal: {type: text, optional: true, desc: the closer}
steps:
    - id: draft
      collect: [body, "bodyPatch?", "goal?"]
      transitions:
          - when: hasGoal
            to: end(completed)`

// recordingSink keeps events so tests can assert what the log carries.
type recordingSink struct{ events []engine.Event }

func (s *recordingSink) Append(ev engine.Event) error {
	s.events = append(s.events, ev)
	return nil
}

func startPatchSession(t *testing.T) (*engine.Session, *recordingSink, string) {
	t.Helper()
	spec, err := engine.ParseSpec(patchEntry(t, patchMachine))
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	sink := &recordingSink{}
	sess := engine.New(engine.NewRegistry(), engine.StaticGraphs{Graph: model.NewGraph(nil)}).NewSession("s_patch", "tester", sink)
	sv, err := sess.Start(spec, map[string]any{"body": "alpha beta gamma"}, "")
	if err != nil {
		t.Fatal(err)
	}
	return sess, sink, sv.Instance
}

func storeText(t *testing.T, sess *engine.Session, instance, field string) string {
	t.Helper()
	inst, ok := sess.Instance(instance)
	if !ok {
		t.Fatalf("instance %q not found", instance)
	}
	v, _ := inst.Store.Get(field)
	s, _ := v.(string)
	return s
}

// TestPatchAppliesToTarget: a reported patch mutates the target atomically,
// in order, and the report event logs the target's new value — never the
// pairs — so replay re-writes the result instead of re-applying.
func TestPatchAppliesToTarget(t *testing.T) {
	sess, sink, instance := startPatchSession(t)

	if _, err := sess.Report(instance, map[string]any{
		"bodyPatch": []any{
			map[string]any{"old": "beta", "new": "BETA"},
			map[string]any{"old": "alpha BETA", "new": "start"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if got := storeText(t, sess, instance, "body"); got != "start gamma" {
		t.Fatalf("body = %q", got)
	}

	var reported map[string]any
	for _, ev := range sink.events {
		if ev.Event == engine.EventReport {
			reported, _ = ev.Data["fields"].(map[string]any)
		}
	}
	if reported == nil {
		t.Fatal("no report event logged")
	}
	if _, hasPairs := reported["bodyPatch"]; hasPairs {
		t.Fatal("report event must log the target's value, not the pairs")
	}
	if v, _ := reported["body"].(string); v != "start gamma" {
		t.Fatalf("logged body = %q", v)
	}
}

// TestPatchFailureRefusesWholeReport: a failing pair names itself and nothing
// from the batch lands — not the earlier pairs, not sibling fields.
func TestPatchFailureRefusesWholeReport(t *testing.T) {
	sess, _, instance := startPatchSession(t)

	_, err := sess.Report(instance, map[string]any{
		"goal": "should not land",
		"bodyPatch": []any{
			map[string]any{"old": "alpha", "new": "A"},
			map[string]any{"old": "absent", "new": "x"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "pair 2") || !strings.Contains(err.Error(), "no pair applied") {
		t.Fatalf("err = %v, want a refusal naming pair 2", err)
	}
	if got := storeText(t, sess, instance, "body"); got != "alpha beta gamma" {
		t.Fatalf("body mutated by a refused report: %q", got)
	}
	if got := storeText(t, sess, instance, "goal"); got != "" {
		t.Fatalf("sibling field landed from a refused report: %q", got)
	}
}

func TestPatchAndTargetInOneReportRefused(t *testing.T) {
	sess, _, instance := startPatchSession(t)

	_, err := sess.Report(instance, map[string]any{
		"body":      "a rewrite",
		"bodyPatch": []any{map[string]any{"old": "alpha", "new": "A"}},
	})
	if err == nil || !strings.Contains(err.Error(), "one or the other") {
		t.Fatalf("err = %v, want the both-reported refusal", err)
	}
}

func TestPatchWithUnsetTargetRefused(t *testing.T) {
	spec, err := engine.ParseSpec(patchEntry(t, patchMachine))
	if err != nil {
		t.Fatal(err)
	}
	sess := engine.New(engine.NewRegistry(), engine.StaticGraphs{Graph: model.NewGraph(nil)}).NewSession("s_unset", "tester", &recordingSink{})
	sv, err := sess.Start(spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = sess.Report(sv.Instance, map[string]any{
		"bodyPatch": []any{map[string]any{"old": "x", "new": "y"}},
	})
	if err == nil || !strings.Contains(err.Error(), "nothing to patch") {
		t.Fatalf("err = %v, want the unset-target refusal", err)
	}
}

// TestPatchDeclarationValidatedAtLoad pins the load-time rules: the patch
// field's own shape, a declared text target, and no patch-of-patch chains.
func TestPatchDeclarationValidatedAtLoad(t *testing.T) {
	cases := []struct {
		name    string
		machine string
		want    string
	}{
		{"wrong type", `state:
    body: {type: text, desc: d}
    bodyPatch: {type: text, optional: true, patchOf: body, desc: d}
steps:
    - id: draft
      collect: [body]
      transitions:
          - when: hasBody
            to: end(completed)`, "requires an optional list<search-replace>"},
		{"missing target", `state:
    bodyPatch: {type: list<search-replace>, optional: true, patchOf: body, desc: d}
steps:
    - id: draft
      collect: ["bodyPatch?"]
      transitions:
          - when: hasBodyPatch
            to: end(completed)`, `target "body" is not declared`},
		{"non-text target", `state:
    body: {type: list<label>, desc: d}
    bodyPatch: {type: list<search-replace>, optional: true, patchOf: body, desc: d}
steps:
    - id: draft
      collect: [body]
      transitions:
          - when: hasBody
            to: end(completed)`, "must be a text state field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.ParseSpec(patchEntry(t, tc.machine))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
