package engine_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

const draftServeMachine = `state:
    body: {type: prose, desc: the draft}
    topics: {type: list<label>, optional: true, desc: labels}
    layer: {type: layer, optional: true, desc: layer}
    goal: {type: text, optional: true, desc: closer}
steps:
    - id: draft
      collect: [body, "topics?", "layer?", "goal?"]
      serveDelta: [body, topics, layer]
      transitions:
          - when: hasGoal
            to: end(completed)`

func draftServeEntry(t *testing.T) *model.Entry {
	t.Helper()
	content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: draftproc\n" +
		draftServeMachine + "\n---\n\ndraft procedure\n\n## unit: draft\n\nDraft guidance.\n"
	entry, err := model.ParseEntry("20260826-140000-d-prc-drf.md", content)
	if err != nil {
		t.Fatalf("fixture entry: %v", err)
	}
	return entry
}

func longBody() string {
	return strings.Join([]string{
		"line one", "line two", "line three", "line four", "line five",
		"line six", "line seven", "line eight", "line nine", "line ten",
		"line eleven", "line twelve", "line thirteen", "line fourteen",
	}, "\n")
}

// TestDraftServe_Rounds drives the full arc: bounded first round, item-level
// list delta, scalar whole-when-changed, prose content diff, an explicit
// nothing-changed round, and the rehydrate serve whole.
func TestDraftServe_Rounds(t *testing.T) {
	spec, err := engine.ParseSpec(draftServeEntry(t))
	if err != nil {
		t.Fatal(err)
	}
	sess := engine.New(engine.NewRegistry(), engine.StaticGraphs{Graph: model.NewGraph(nil)}).NewSession("s_rounds", "tester", &recordingSink{})
	sv, err := sess.Start(spec, map[string]any{"body": longBody(), "topics": []any{"cli/ux"}, "layer": "tactical"}, "")
	if err != nil {
		t.Fatal(err)
	}
	instance := sv.Instance

	// Round one: whole block, prose bounded with the middle marked elided.
	if !strings.Contains(sv.UnitText, "lines elided") {
		t.Fatalf("first round should bound the prose body, got %q", sv.UnitText)
	}
	if !strings.Contains(sv.UnitText, "- layer: tactical") || !strings.Contains(sv.UnitText, `- topics: "cli/ux"`) {
		t.Fatalf("first round should serve small fields whole, got %q", sv.UnitText)
	}

	// A list edit serves item-level: one added, one removed.
	sv, err = sess.Report(instance, map[string]any{"topics": []any{"engine/serve"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.UnitText, "Draft delta since last served") ||
		!strings.Contains(sv.UnitText, `+ "engine/serve"`) || !strings.Contains(sv.UnitText, `- "cli/ux"`) {
		t.Fatalf("list delta should be item-level, got %q", sv.UnitText)
	}
	if strings.Contains(sv.UnitText, "line one") {
		t.Fatalf("unchanged prose must not re-serve, got %q", sv.UnitText)
	}
	if !strings.Contains(sv.UnitText, "unchanged: body, layer") {
		t.Fatalf("delta should name unchanged fields, got %q", sv.UnitText)
	}

	// A prose edit serves as a content diff, not the whole text.
	edited := strings.Replace(longBody(), "line seven", "line SEVEN edited", 1)
	sv, err = sess.Report(instance, map[string]any{"body": edited})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.UnitText, "-line seven") || !strings.Contains(sv.UnitText, "+line SEVEN edited") {
		t.Fatalf("prose delta should be a content diff, got %q", sv.UnitText)
	}
	if strings.Contains(sv.UnitText, "line twelve") {
		t.Fatalf("a content diff must not carry distant unchanged lines, got %q", sv.UnitText)
	}

	// A round in which nothing changed says so explicitly.
	sv, err = sess.Report(instance, map[string]any{"body": edited})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.UnitText, "Draft unchanged since last served") {
		t.Fatalf("a no-change round must say so, got %q", sv.UnitText)
	}

	// The rehydrate path serves the draft whole — prose included, unbounded.
	sv, err = sess.Serve(instance)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.UnitText, "Draft as it stands") || !strings.Contains(sv.UnitText, "line twelve") ||
		strings.Contains(sv.UnitText, "lines elided") {
		t.Fatalf("rehydrate must serve the draft whole, got %q", sv.UnitText)
	}

	// The full serve reset the base: an unchanged round after it still deltas.
	sv, err = sess.Report(instance, map[string]any{"layer": "process"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.UnitText, "- layer: process") || strings.Contains(sv.UnitText, "line one") {
		t.Fatalf("post-rehydrate delta should carry only the change, got %q", sv.UnitText)
	}
}
