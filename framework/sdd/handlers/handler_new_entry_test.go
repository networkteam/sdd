package handlers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// emptyGraphLoader returns an empty graph — useful for tests that just want
// validation to run against known-empty state.
func emptyGraphLoader(_ string) (*model.Graph, error) {
	return model.NewGraph(nil), nil
}

// recordingCommitter captures commit calls so tests can assert whether a
// commit was attempted.
type recordingCommitter struct {
	calls int
}

func (r *recordingCommitter) Commit(message string, paths ...string) error {
	r.calls++
	return nil
}

// fakePreflight returns a fixed PreflightResult.
type fakePreflight struct {
	result *query.PreflightResult
	err    error
}

func (f *fakePreflight) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	return f.result, f.err
}

// TestNewEntry_DryRun_SavesStdinOnValidationFailure is the regression test for
// d-prc-31v / d-tac-q5p: dry-run + validation failure must still save stdin.
// The bug was that the reportSavedStdin call was placed after graph validation,
// so an invalid ref would return early without persisting stdin — contradicting
// the plan's "save on every dry-run (pass or fail)" spec.
func TestNewEntry_DryRun_SavesStdinOnValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	stderr := &bytes.Buffer{}
	committer := &recordingCommitter{}

	h := New(Options{
		GraphDir:  tmp,
		Committer: committer,
		LoadGraph: emptyGraphLoader,
		Stderr:    stderr,
	})

	stdinContent := []byte("plan content for iteration")
	onNewEntryCalled := false

	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "test signal",
		Refs:        []string{"nonexistent-dangling-ref"}, // triggers validation failure
		Attachments: []command.Attachment{
			{Source: "-", Target: "plan.md", Data: stdinContent},
		},
		SkipPreflight: true, // skip preflight so this test doesn't need a runner
		DryRun:        true,
		OnNewEntry:    func(id string) { onNewEntryCalled = true },
	}

	err := h.NewEntry(context.Background(), cmd)
	if err == nil {
		t.Fatal("NewEntry: expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("error = %v, want validation failure", err)
	}

	// The regression: on dry-run + validation failure, stdin MUST be saved.
	saveDir := filepath.Join(tmp, stdinSaveDir)
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf(".sdd-tmp/ should exist after dry-run + stdin: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf(".sdd-tmp/ should contain 1 file, got %d", len(entries))
	}

	saved := entries[0].Name()
	if !strings.HasSuffix(saved, "-plan.md") {
		t.Errorf("saved file %q should end with -plan.md", saved)
	}
	data, err := os.ReadFile(filepath.Join(saveDir, saved))
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(data) != string(stdinContent) {
		t.Errorf("saved content = %q, want %q", string(data), string(stdinContent))
	}

	// Rejection path must NOT fire the success callback and NOT commit.
	if onNewEntryCalled {
		t.Error("OnNewEntry should not be called on validation failure")
	}
	if committer.calls != 0 {
		t.Errorf("Committer.Commit called %d times; want 0 on failure", committer.calls)
	}

	// Stderr should mention the save for the user's benefit.
	if !strings.Contains(stderr.String(), "stdin attachment saved (dry-run)") {
		t.Errorf("stderr should announce the dry-run save, got:\n%s", stderr.String())
	}
}

// TestNewEntry_DryRun_Pass_SavesStdinOnce verifies that dry-run with a passing
// pre-flight still saves stdin, and that the save message appears only once
// (the defer + explicit post-pre-flight call are idempotent).
func TestNewEntry_DryRun_Pass_SavesStdinOnce(t *testing.T) {
	tmp := t.TempDir()
	stderr := &bytes.Buffer{}

	h := New(Options{
		GraphDir:  tmp,
		LoadGraph: emptyGraphLoader,
		Stderr:    stderr,
	})

	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "valid signal",
		Attachments: []command.Attachment{
			{Source: "-", Target: "plan.md", Data: []byte("hello")},
		},
		SkipPreflight: true,
		DryRun:        true,
	}

	if err := h.NewEntry(context.Background(), cmd); err != nil {
		t.Fatalf("NewEntry: %v", err)
	}

	saveDir := filepath.Join(tmp, stdinSaveDir)
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf(".sdd-tmp/ should exist: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf(".sdd-tmp/ should contain 1 file, got %d", len(entries))
	}

	// The save message should appear exactly once (idempotency).
	if got := strings.Count(stderr.String(), "stdin attachment saved"); got != 1 {
		t.Errorf("save message appears %d times, want 1; stderr:\n%s", got, stderr.String())
	}
}

// TestNewEntry_PreflightReject_SavesStdin covers the real-run rejection path:
// stdin gets saved for re-piping even when the user didn't ask for --dry-run.
func TestNewEntry_PreflightReject_SavesStdin(t *testing.T) {
	tmp := t.TempDir()
	stderr := &bytes.Buffer{}

	h := New(Options{
		GraphDir: tmp,
		Preflighter: &fakePreflight{
			result: &query.PreflightResult{Pass: false, Gaps: []string{"missing ref"}},
		},
		LoadGraph: emptyGraphLoader,
		Stderr:    stderr,
	})

	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "signal the validator will reject",
		Attachments: []command.Attachment{
			{Source: "-", Target: "plan.md", Data: []byte("rejection test content")},
		},
	}

	err := h.NewEntry(context.Background(), cmd)
	if err == nil {
		t.Fatal("NewEntry: expected pre-flight rejection error")
	}
	if !strings.Contains(err.Error(), "pre-flight rejected") {
		t.Errorf("error = %v, want pre-flight rejection", err)
	}

	saveDir := filepath.Join(tmp, stdinSaveDir)
	if _, err := os.Stat(saveDir); err != nil {
		t.Fatalf(".sdd-tmp/ should exist after rejection: %v", err)
	}
	entries, err := os.ReadDir(saveDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf(".sdd-tmp/ should contain 1 file; err=%v, entries=%v", err, entries)
	}
	if !strings.Contains(stderr.String(), "stdin attachment saved (pre-flight rejected)") {
		t.Errorf("stderr should announce the pre-flight save, got:\n%s", stderr.String())
	}
}
