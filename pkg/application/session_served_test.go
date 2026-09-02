package application_test

import (
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// TestServedLedgerSurvivesReplayAndReorientClearsIt covers the served-once
// memory living in the session ledger (d-cpt-aen): what one consumer recorded
// as served is known to the next one that loads the handle, a reorientation
// forgets it, and the forgetting replays too.
func TestServedLedgerSurvivesReplayAndReorientClearsIt(t *testing.T) {
	application, _, _, _ := newStampWorkflowApp(t, "", "", time.Now)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	w, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{ClientName: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	hashes := []string{"h-orientation", "h-moves"}
	if err := w.RecordServed(t.Context(), identity, hashes); err != nil {
		t.Fatal(err)
	}
	assertServed := func(t *testing.T, session *sdd.WorkflowSession, want bool) {
		t.Helper()
		for _, hash := range hashes {
			if got := session.ServedBefore(hash); got != want {
				t.Fatalf("ServedBefore(%q) = %v, want %v", hash, got, want)
			}
		}
		if session.ServedBefore("h-never") {
			t.Fatal("a hash never recorded reads served")
		}
	}
	assertServed(t, w, true)

	replayed, err := application.LoadWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), ClientName: "mcp-b"})
	if err != nil {
		t.Fatal(err)
	}
	assertServed(t, replayed, true)

	if err := replayed.Reorient(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	assertServed(t, replayed, false)

	again, _, err := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{SessionID: w.ID(), ClientName: "mcp-c"})
	if err != nil {
		t.Fatal(err)
	}
	assertServed(t, again, false)
}
