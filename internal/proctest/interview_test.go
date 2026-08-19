package proctest_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
)

const (
	interviewAnchorID = "20260601-150000-d-tac-sty"
	// interviewMissingID is well-formed but absent from the fixture graph.
	interviewMissingID = "20260601-140000-d-tac-gon"
)

func interviewAnchorEntry() *model.Entry {
	return &model.Entry{
		ID: interviewAnchorID, Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
		Summary: "A directive the interview story centers on.",
		Content: "A directive the interview story centers on.",
	}
}

func TestInterviewHappyPath(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "interview-happy")

	// Seeded framing (a dispatching move or start params) auto-advances past
	// frame to the ask loop.
	serve := session.Start(t, "interview", map[string]any{
		"goal":        "settle the staleness definition for WIP markers",
		"widenReport": "searched markers and lifecycle entries; assumption: staleness is age-based; tension: none found",
	})
	proctest.RequireStep(t, serve, "ask")
	instance := serve.Instance
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "agent" {
		t.Fatalf("seeded framing should reach the ask agent chooser, got %+v", serve.PendingChooser)
	}
	if !strings.Contains(serve.Instructions, "one question") {
		t.Errorf("ask unit should carry the one-question rule, got %q", serve.Instructions)
	}

	// Two cycles: each next answer updates the running transcript and loops.
	serve = session.Answer(t, instance, "ask", "next", map[string]any{
		"transcript": "Q1: what makes a marker stale to you? A: no activity on the entry since. Grounding: matches groom's constellation. Shift: age-based assumption corrected. Next: how to measure activity.",
	}, "cycle 1 done")
	proctest.RequireStep(t, serve, "ask")
	serve = session.Answer(t, instance, "ask", "next", map[string]any{
		"transcript": "Q1: ... Q2: measure by downstream entries? A: yes, downstream or commits. Grounding: consistent. Shift: settled. Next: nothing open.",
	}, "cycle 2 done")
	proctest.RequireStep(t, serve, "ask")

	serve = session.Answer(t, instance, "ask", "saturated", nil, "both triggers quiet")
	proctest.RequireStep(t, serve, "synthesize")

	serve = session.Report(t, instance, map[string]any{
		"synthesis": "Staleness = no downstream activity; crystallized: one directive candidate.",
	})
	proctest.RequireStep(t, serve, "junction")
	if serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("synthesis should reach the junction user chooser, got %+v", serve.PendingChooser)
	}

	serve = session.Answer(t, instance, "junction", "record",
		map[string]any{"selectedOutcomes": "the staleness directive"}, "record it")
	proctest.RequireStatus(t, serve, "completed")
}

func TestInterviewColdStartStallsAtFrame(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "interview-cold-start")

	serve, err := session.StartErr(t, "interview", nil)
	if err != nil {
		t.Fatalf("cold start should stall at frame, not error: %v", err)
	}
	proctest.RequireStep(t, serve, "frame")
	missing := strings.Join(serve.Missing, ",")
	if !strings.Contains(missing, "goal") || !strings.Contains(missing, "widenReport") {
		t.Errorf("frame should name goal and widenReport as missing (anchor optional), got %v", serve.Missing)
	}
}

func TestInterviewOptionalAnchorMustResolve(t *testing.T) {
	world := proctest.NewWorld(t)
	session := world.Open(t, "interview-anchor-unresolved")

	serve := session.Start(t, "interview", nil)
	serve = session.Report(t, serve.Instance, map[string]any{
		"goal":        "understand the new deployment vocabulary",
		"widenReport": "searched deployment terms; nothing captured yet",
		"anchor":      interviewMissingID,
	})
	proctest.RequireStep(t, serve, "frame")

	// Without an anchor the same framing advances — the anchor is optional,
	// resolution is only checked when one is given.
	session2 := world.Open(t, "interview-anchorless")
	serve2 := session2.Start(t, "interview", map[string]any{
		"goal":        "understand the new deployment vocabulary",
		"widenReport": "searched deployment terms; nothing captured yet",
	})
	proctest.RequireStep(t, serve2, "ask")
}

// The record option's declared capture handoff, exercised behaviorally: the
// answered dispatch seeds the interview's homework into a capture child, and
// the child is the real capture procedure writing the crystallized outcome to
// disk. Replaces the old spec-level Dispatch inspection. The dispatch also
// declares an anchor seed, but capture declares anchor as a param, not state,
// and the engine seeds only declared state — so the anchor handoff does not
// land and is not asserted here.
func TestInterviewRecordDispatchesRealCapture(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(interviewAnchorEntry()))
	session := world.Open(t, "interview-record-capture")

	homework := "searched markers and lifecycle entries; assumption: staleness is age-based; tension: none found"
	serve := session.Start(t, "interview", map[string]any{
		"goal":        "settle the staleness definition for WIP markers",
		"widenReport": homework,
		"anchor":      interviewAnchorID,
	})
	proctest.RequireStep(t, serve, "ask")
	instance := serve.Instance

	serve = session.Answer(t, instance, "ask", "saturated", nil, "aligned already")
	proctest.RequireStep(t, serve, "synthesize")
	serve = session.Report(t, instance, map[string]any{
		"synthesis": "Staleness means no downstream activity on the marked entry; crystallized: one insight.",
	})
	proctest.RequireStep(t, serve, "junction")
	serve = session.Answer(t, instance, "junction", "record",
		map[string]any{"selectedOutcomes": "the staleness insight"}, "record it")
	proctest.RequireStatus(t, serve, "completed")

	capture := startChild(t, session, "capture", instance, nil)
	proctest.RequireStep(t, capture, "assemble")
	if missingContains(capture, "widenReport") {
		t.Fatalf("widenReport should be seeded from the interview's homework, missing %v", capture.Missing)
	}
	if !strings.Contains(capture.Instructions, homework) {
		t.Errorf("assemble should render the inherited homework, got %q", capture.Instructions)
	}

	serve = session.Report(t, capture.Instance, map[string]any{
		"body":       "Staleness of a WIP marker means no downstream activity on the marked entry, not marker age.",
		"entryKind":  "insight",
		"layer":      "tactical",
		"topics":     []any{"wip/staleness"},
		"confidence": "medium",
	})
	proctest.RequireStep(t, serve, "playback")
	serve = session.Answer(t, capture.Instance, "playback", "confirm", nil, "capture it")
	proctest.RequireStep(t, serve, "verifySummary")
	serve = session.Answer(t, capture.Instance, "verifySummary", "faithful", map[string]any{"fidelityNote": "matches"}, "")
	proctest.RequireStatus(t, serve, "completed")

	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("outcome capture produced no entryId: %+v", serve.Produced)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if entry.Kind != "insight" {
		t.Errorf("persisted kind = %q, want insight", entry.Kind)
	}
}
