package application_test

import (
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/application"
)

// TestAdvanceRefusesTopLevelFieldBesideChooserAnswer pins the loud report
// envelope (20260811-233331-s-tac-bjn): once a report carries a chooser
// answer, only the envelope keys reach the engine — anything else used to be
// dropped without a word. It is now refused naming the field.
func TestAdvanceRefusesTopLevelFieldBesideChooserAnswer(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	application, _ := newConcludeApp(t, now)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, shell, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-envelope"})
	if err != nil {
		t.Fatal(err)
	}

	report := concludeReport()
	report["widenReport"] = "a correction that must not vanish"
	_, err = w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: shell.Instance, Report: report})
	if err == nil || !strings.Contains(err.Error(), `"widenReport"`) || !strings.Contains(err.Error(), "not applied") {
		t.Fatalf("top-level field beside a chooser answer must be refused by name, got %v", err)
	}

	// The refusal left the chooser unanswered: the same answer without the
	// stray field still lands.
	serve, err := w.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: shell.Instance, Report: concludeReport()})
	if err != nil {
		t.Fatal(err)
	}
	if serve.Status != "completed" {
		t.Fatalf("clean answer after refusal should complete the shell, got %s", serve.Status)
	}
}
