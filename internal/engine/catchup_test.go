package engine

import (
	"strings"
	"testing"
)

// Per-procedure table tests for the embedded catch-up entry, driving the
// shipped base entry through the production loader (see engage_explore_test
// for the shared harness).

func TestCatchup_HappyPath(t *testing.T) {
	env := newProcEnv(t, "catch-up")

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "compose" {
		t.Fatalf("start step = %s, want compose", sv.Step)
	}
	// The injected multi-section layout carries every lane, focus included.
	for _, lane := range []string{"focus,", "Recent done", "Active and hot", "Open loops", "Open and warm", ",wip"} {
		if !strings.Contains(sv.Instructions, lane) {
			t.Errorf("compose unit should inject the %q lane, got %q", lane, sv.Instructions)
		}
	}
	if !strings.Contains(sv.Instructions, "explicit recovery choice is available") {
		t.Fatalf("catch-up instructions do not project actionable recovery notices: %q", sv.Instructions)
	}
	if got := strings.Join(sv.Missing, ","); got != "briefing" {
		t.Fatalf("missing = %q, want briefing", got)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"briefing": "*Current focus: ship the engine.*\n\n**Slice work**\n\n1. Continue (`d-tac-ry0`).\n\n**What do you want to move forward?**",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "junction" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("briefing should reach the junction user chooser, got step %q chooser %+v", sv.Step, sv.Chooser)
	}

	sv, err = env.session.Answer(sv.Instance, "junction", "pursue",
		map[string]any{"selectedThread": "continue the engine plan"}, "let's continue the engine work")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("pursue should complete the check-in, got %s at %q", sv.Status, sv.Step)
	}
}

func TestCatchup_ConcludeEndsQuiet(t *testing.T) {
	env := newProcEnv(t, "catch-up")

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{"briefing": "**Quiet week.**\n\n**What do you want to move forward?**"})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "nothing right now")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("conclude should complete, got %s at %q", sv.Status, sv.Step)
	}
}
