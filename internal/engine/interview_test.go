package engine

import (
	"strings"
	"testing"
)

// Per-procedure table tests for the embedded interview entry, driving the
// shipped base entry through the production loader (see engage_explore_test
// for the shared harness).

func TestInterview_HappyPath(t *testing.T) {
	env := newProcEnv(t, "interview")

	// Seeded framing (a dispatching move or start params) auto-advances past
	// frame to the ask loop.
	sv, err := env.session.Start(env.spec, map[string]any{
		"goal":        "settle the staleness definition for WIP markers",
		"widenReport": "searched markers and lifecycle entries; assumption: staleness is age-based; tension: none found",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "ask" || sv.Chooser == nil || sv.Chooser.Kind != ChooserAgent {
		t.Fatalf("seeded framing should reach the ask agent chooser, got %q", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "one question") {
		t.Errorf("ask unit should carry the one-question rule, got %q", sv.Instructions)
	}

	// Two cycles: each next answer updates the running transcript and loops.
	sv, err = env.session.Answer(sv.Instance, "ask", "next", map[string]any{
		"transcript": "Q1: what makes a marker stale to you? A: no activity on the entry since. Grounding: matches groom's constellation. Shift: age-based assumption corrected. Next: how to measure activity.",
	}, "cycle 1 done")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "ask" {
		t.Fatalf("next should loop back to ask, got %q", sv.Step)
	}
	sv, err = env.session.Answer(sv.Instance, "ask", "next", map[string]any{
		"transcript": "Q1: ... Q2: measure by downstream entries? A: yes, downstream or commits. Grounding: consistent. Shift: settled. Next: nothing open.",
	}, "cycle 2 done")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Answer(sv.Instance, "ask", "saturated", nil, "both triggers quiet")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "synthesize" {
		t.Fatalf("saturated should reach synthesize, got %q", sv.Step)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"synthesis": "Staleness = no downstream activity; crystallized: one directive candidate.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "junction" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("synthesis should reach the junction user chooser, got %q", sv.Step)
	}

	sv, err = env.session.Answer(sv.Instance, "junction", "record",
		map[string]any{"selectedOutcomes": "the staleness directive"}, "record it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("record should complete the interview, got %s at %q", sv.Status, sv.Step)
	}
}

func TestInterview_ColdStartStallsAtFrame(t *testing.T) {
	env := newProcEnv(t, "interview")

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatalf("cold start should stall at frame, not error: %v", err)
	}
	if sv.Step != "frame" {
		t.Fatalf("cold start step = %s, want frame", sv.Step)
	}
	missing := strings.Join(sv.Missing, ",")
	if !strings.Contains(missing, "goal") || !strings.Contains(missing, "widenReport") {
		t.Errorf("frame should name goal and widenReport as missing (anchor optional), got %v", sv.Missing)
	}
}

func TestInterview_OptionalAnchorMustResolve(t *testing.T) {
	env := newProcEnv(t, "interview")

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"goal":        "understand the new deployment vocabulary",
		"widenReport": "searched deployment terms; nothing captured yet",
		"anchor":      procMissingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "frame" {
		t.Fatalf("an unresolved anchor must hold frame, got %q", sv.Step)
	}

	// Without an anchor the same framing advances — the anchor is optional,
	// resolution is only checked when one is given.
	env2 := newProcEnv(t, "interview")
	sv2, err := env2.session.Start(env2.spec, map[string]any{
		"goal":        "understand the new deployment vocabulary",
		"widenReport": "searched deployment terms; nothing captured yet",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv2.Step != "ask" {
		t.Fatalf("anchorless framing should advance to ask, got %q", sv2.Step)
	}
}

func TestInterview_RecordDeclaresCaptureHandoff(t *testing.T) {
	env := newProcEnv(t, "interview")

	// The record dispatch seeds the homework (and the anchor when the story
	// had one) into every crystallized-outcome capture.
	var record *Option
	for _, step := range env.spec.Steps {
		if step.ID != "junction" {
			continue
		}
		for i := range step.Options {
			if step.Options[i].Choice == "record" {
				record = &step.Options[i]
			}
		}
	}
	if record == nil {
		t.Fatal("junction has no record option")
	}
	if record.Dispatch == nil || record.Dispatch.Procedure != "capture" {
		t.Fatalf("record must dispatch capture, got %+v", record.Dispatch)
	}
	if got := record.Dispatch.Seed["widenReport"]; got != "widenReport" {
		t.Errorf("record seed widenReport = %q, want widenReport", got)
	}
	if got := record.Dispatch.Seed["anchor"]; got != "anchor" {
		t.Errorf("record seed anchor = %q, want anchor", got)
	}
}
