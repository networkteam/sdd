package mcpapp_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
)

// The initial MCP start response is the surface d-tac-vzz targets: a fresh
// capture serves the type-system overview in full before any draft report,
// and a later capture in the same session gets the pointer alone.
func TestStartProcedureCaptureServesTypeSystemOverview(t *testing.T) {
	// A bare graph dir, not the fixture graph — the fixture plants a simplified
	// capture override, and this test must drive the shipped base capture.
	env := newTestServer(t, nil, t.TempDir(), "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)

	var first mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &first)
	if !strings.Contains(first.Instructions, basefacts.OverviewFactID) || !strings.Contains(first.Instructions, "a signal records something noticed:") {
		t.Fatalf("the initial capture start response must serve the overview in full, got: %.300q", first.Instructions)
	}

	var second mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &second)
	if strings.Contains(second.Instructions, "a signal records something noticed:") {
		t.Fatalf("a second capture start in the same session must not repeat the overview body")
	}
	if !strings.Contains(second.Instructions, basefacts.OverviewFactID) {
		t.Errorf("the suppressed serve must still point at the overview fact")
	}
}

// The overview grounding is durable session state: a second server over the
// same stores — a real restart, not an in-process resume — must still know the
// overview was served.
func TestCaptureOverviewGroundingSurvivesServerRestart(t *testing.T) {
	env := newTestServer(t, nil, t.TempDir(), "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)

	var first mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &first)
	if !strings.Contains(first.Instructions, "a signal records something noticed:") {
		t.Fatalf("the first capture must serve the overview in full")
	}

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": door.Session}, &resumed)

	var reServed *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Procedure == "capture" {
			reServed = &resumed.Open[i]
		}
	}
	if reServed == nil {
		t.Fatalf("the resumed session must re-serve the open capture, got %+v", resumed.Open)
	}
	if strings.Contains(reServed.Instructions, "a signal records something noticed:") {
		t.Fatalf("the overview grounding must survive a server restart — the re-serve repeated the body")
	}
	if !strings.Contains(reServed.Instructions, basefacts.OverviewFactID) {
		t.Errorf("the re-served capture must still point at the overview fact")
	}
}
