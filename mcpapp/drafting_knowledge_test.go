package mcpapp_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	mcpserver "github.com/networkteam/sdd/mcpapp"
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
