package handlers_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/handlers"
	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// fakeReader bundles every read the handler needs into one stub. Tests
// configure preflightResult/preflightErr and graph as needed; LoadWIPMarkers
// returns nil since the new-entry handler doesn't touch it.
type fakeReader struct {
	preflightResult *query.PreflightResult
	preflightErr    error
	graph           *model.Graph
}

func (f *fakeReader) LoadGraph(_ string) (*model.Graph, error) {
	if f.graph != nil {
		return f.graph, nil
	}
	return model.NewGraph(nil), nil
}

func (f *fakeReader) LoadWIPMarkers(_ string) ([]*model.WIPMarker, error) {
	return nil, nil
}

func (f *fakeReader) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	return f.preflightResult, f.preflightErr
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

// TestNewEntry_DryRun_SavesStdinOnValidationFailure is the regression test for
// d-prc-31v / d-tac-q5p: dry-run + validation failure must still save stdin.
// The bug was that the reportSavedStdin call was placed after graph validation,
// so an invalid ref would return early without persisting stdin — contradicting
// the plan's "save on every dry-run (pass or fail)" spec.
func TestNewEntry_DryRun_SavesStdinOnValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	os.MkdirAll(sddDir, 0755)
	stderr := &bytes.Buffer{}
	committer := &recordingCommitter{}

	h := handlers.New(handlers.Options{
		GraphDir:  tmp,
		SDDDir:    sddDir,
		Reader:    &fakeReader{},
		Committer: committer,
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
	saveDir := filepath.Join(sddDir, "tmp")
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf(".sdd/tmp/ should exist after dry-run + stdin: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf(".sdd/tmp/ should contain 1 file, got %d", len(entries))
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
	sddDir := filepath.Join(tmp, ".sdd")
	os.MkdirAll(sddDir, 0755)
	stderr := &bytes.Buffer{}

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader:   &fakeReader{},
		Stderr:   stderr,
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

	saveDir := filepath.Join(sddDir, "tmp")
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf(".sdd/tmp/ should exist: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf(".sdd/tmp/ should contain 1 file, got %d", len(entries))
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
	sddDir := filepath.Join(tmp, ".sdd")
	os.MkdirAll(sddDir, 0755)
	stderr := &bytes.Buffer{}

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader: &fakeReader{
			preflightResult: &query.PreflightResult{Findings: []query.Finding{
				{Severity: query.SeverityHigh, Category: "missing-ref", Observation: "missing ref"},
			}},
		},
		Stderr: stderr,
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

	saveDir := filepath.Join(sddDir, "tmp")
	if _, err := os.Stat(saveDir); err != nil {
		t.Fatalf(".sdd/tmp/ should exist after rejection: %v", err)
	}
	entries, err := os.ReadDir(saveDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf(".sdd/tmp/ should contain 1 file; err=%v, entries=%v", err, entries)
	}
	if !strings.Contains(stderr.String(), "stdin attachment saved (pre-flight rejected)") {
		t.Errorf("stderr should announce the pre-flight save, got:\n%s", stderr.String())
	}
}

// testEntry is a minimal helper that builds an entry for seeding the test graph.
// Parses type/layer from the ID so suffix-search resolution works the same way
// it does against a real graph.
func testEntry(id string) *model.Entry {
	parts, err := model.ParseID(id)
	if err != nil {
		panic(err)
	}
	typ := model.TypeFromAbbrev[parts.TypeCode]
	layer := model.LayerFromAbbrev[parts.LayerCode]
	kind := model.Kind("")
	if typ == model.TypeDecision {
		kind = model.KindDirective
	}
	return &model.Entry{
		ID:      id,
		Type:    typ,
		Layer:   layer,
		Kind:    kind,
		Content: id,
		Time:    parts.Time,
	}
}

// TestNewEntry_ResolvesShortRefs verifies that short-form IDs in --refs get
// resolved to full IDs before validation. Without resolution the dangling-ref
// check would surface a validation error for the short form; with it, the
// entry passes validation against the existing full-ID entry.
func TestNewEntry_ResolvesShortRefs(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	os.MkdirAll(sddDir, 0755)
	stderr := &bytes.Buffer{}

	existing := testEntry("20260406-100000-s-stg-aaa")
	graph := model.NewGraph([]*model.Entry{existing})

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader:   &fakeReader{graph: graph},
		Stderr:   stderr,
	})

	cmd := &command.NewEntryCmd{
		Type:          model.TypeDecision,
		Layer:         model.LayerTactical,
		Kind:          model.KindDirective,
		Description:   "new decision referencing short-form ID",
		Refs:          []string{"s-stg-aaa"},
		SkipPreflight: true,
		DryRun:        true, // stop before writing; we only care about resolution + validation
	}

	if err := h.NewEntry(context.Background(), cmd); err != nil {
		t.Fatalf("NewEntry: %v (short-form ref should resolve to an existing entry)", err)
	}

	// Stderr must not contain a validation-failure log about the short ref —
	// the handler logs each Warning before returning the validation error.
	if strings.Contains(stderr.String(), "dangling ref") {
		t.Errorf("stderr reports a dangling ref; resolution did not happen:\n%s", stderr.String())
	}
}

// TestNewEntry_AmbiguousShortRefErrors verifies that an ambiguous short-form
// ref in --refs is surfaced as a hard error (no fallback to "first match").
func TestNewEntry_AmbiguousShortRefErrors(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	os.MkdirAll(sddDir, 0755)

	graph := model.NewGraph([]*model.Entry{
		testEntry("20260406-100000-s-stg-xyz"),
		testEntry("20260407-110000-s-stg-xyz"),
	})

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader:   &fakeReader{graph: graph},
	})

	cmd := &command.NewEntryCmd{
		Type:          model.TypeDecision,
		Layer:         model.LayerTactical,
		Kind:          model.KindDirective,
		Description:   "decision with ambiguous short ref",
		Refs:          []string{"s-stg-xyz"},
		SkipPreflight: true,
		DryRun:        true,
	}

	err := h.NewEntry(context.Background(), cmd)
	if err == nil {
		t.Fatal("NewEntry: want error for ambiguous short ID in refs")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguous message", err)
	}
}
