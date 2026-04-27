package handlers_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/llm"
)

// messageRecordingCommitter captures commit messages so summarize tests can
// assert that the manual marker is present in the explicit-text path. The
// shared recordingCommitter only counts calls, which isn't enough here.
type messageRecordingCommitter struct {
	messages []string
}

func (m *messageRecordingCommitter) Commit(message string, paths ...string) error {
	m.messages = append(m.messages, message)
	return nil
}

// TestSummarize_ExplicitText_WritesSummaryAndHash verifies the --text path:
// supplied summary is written verbatim (after trim), hash is recomputed from
// the current prompt input, and the LLM runner is never invoked.
func TestSummarize_ExplicitText_WritesSummaryAndHash(t *testing.T) {
	graphDir := t.TempDir()

	id := "20260427-120000-s-prc-tx1"
	writeEntryFile(t, graphDir, id, `---
type: signal
layer: process
kind: gap
participants:
  - Christopher
---

Some body content describing the gap.
`)

	committer := &messageRecordingCommitter{}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: committer,
		// LLMRunner intentionally nil — explicit-text path must not call it.
	})

	var cbID, cbSummary string
	manualSummary := "  Hand-authored summary that should be written verbatim.  "
	scmd := &command.SummarizeCmd{
		EntryIDs:     []string{id},
		ExplicitText: &manualSummary,
		OnSummarized: func(id, summary string) {
			cbID = id
			cbSummary = summary
		},
	}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if cbID != id {
		t.Errorf("callback id = %q, want %q", cbID, id)
	}
	wantTrimmed := strings.TrimSpace(manualSummary)
	if cbSummary != wantTrimmed {
		t.Errorf("callback summary = %q, want %q", cbSummary, wantTrimmed)
	}

	// File must contain the trimmed summary and a non-empty summary_hash.
	rel := filepath.Join("2026", "04", "27-120000-s-prc-tx1.md")
	data, err := os.ReadFile(filepath.Join(graphDir, rel))
	if err != nil {
		t.Fatalf("reading entry: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "summary: '"+wantTrimmed+"'") &&
		!strings.Contains(text, "summary: "+wantTrimmed) {
		t.Errorf("file missing summary line:\n%s", text)
	}
	if !strings.Contains(text, "summary_hash:") {
		t.Errorf("file missing summary_hash:\n%s", text)
	}

	// Commit was made with a manual marker.
	if len(committer.messages) != 1 {
		t.Fatalf("commit count = %d, want 1", len(committer.messages))
	}
	if !strings.Contains(committer.messages[0], "manual") {
		t.Errorf("commit msg = %q, expected to mention 'manual'", committer.messages[0])
	}
}

// TestSummarize_ExplicitText_HashMatchesPrompt verifies the recomputed hash
// equals what RenderSummaryPrompt + ComputePromptHash produces, so subsequent
// automatic regenerations skip-by-hash.
func TestSummarize_ExplicitText_HashMatchesPrompt(t *testing.T) {
	graphDir := t.TempDir()

	id := "20260427-120100-s-prc-tx2"
	writeEntryFile(t, graphDir, id, `---
type: signal
layer: process
kind: gap
participants:
  - Christopher
---

Body for hash validation.
`)

	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: &recordingCommitter{},
	})

	manual := "Manual summary."
	if err := h.Summarize(context.Background(), &command.SummarizeCmd{
		EntryIDs:     []string{id},
		ExplicitText: &manual,
	}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	// Reload entry and verify hash matches prompt-derived hash.
	graph, err := (&graphReader{}).LoadGraph(graphDir)
	if err != nil {
		t.Fatalf("reload graph: %v", err)
	}
	entry := graph.ByID[id]
	if entry == nil {
		t.Fatal("entry not found after summarize")
	}
	req, err := llm.RenderSummaryPrompt(entry, graph)
	if err != nil {
		t.Fatalf("RenderSummaryPrompt: %v", err)
	}
	wantHash := llm.ComputePromptHash(req.Combined())
	if entry.SummaryHash != wantHash {
		t.Errorf("summary_hash = %q, want %q", entry.SummaryHash, wantHash)
	}
}

// TestSummarize_ExplicitText_RejectsMultiEntry covers the constraint that
// --text requires exactly one entry ID.
func TestSummarize_ExplicitText_RejectsMultiEntry(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, "20260427-120200-s-prc-aa1", "---\ntype: signal\nlayer: process\nkind: gap\n---\n\nA.\n")
	writeEntryFile(t, graphDir, "20260427-120201-s-prc-aa2", "---\ntype: signal\nlayer: process\nkind: gap\n---\n\nB.\n")

	h := handlers.New(handlers.Options{
		GraphDir: graphDir,
		Reader:   &graphReader{},
	})

	manual := "Summary."
	err := h.Summarize(context.Background(), &command.SummarizeCmd{
		EntryIDs:     []string{"20260427-120200-s-prc-aa1", "20260427-120201-s-prc-aa2"},
		ExplicitText: &manual,
	})
	if err == nil {
		t.Fatal("expected error for multi-entry --text, got nil")
	}
	if !strings.Contains(err.Error(), "--text") {
		t.Errorf("error message %q does not mention --text", err.Error())
	}
}

// TestSummarize_ExplicitText_RejectsEmptyEntryIDs covers --all incompatibility:
// passing zero entry IDs (which the CLI uses for --all) is rejected when --text
// is set, because batch manual summary makes no sense.
func TestSummarize_ExplicitText_RejectsEmptyEntryIDs(t *testing.T) {
	graphDir := t.TempDir()

	h := handlers.New(handlers.Options{
		GraphDir: graphDir,
		Reader:   &graphReader{},
	})

	manual := "Summary."
	err := h.Summarize(context.Background(), &command.SummarizeCmd{
		EntryIDs:     nil, // simulates --all
		ExplicitText: &manual,
	})
	if err == nil {
		t.Fatal("expected error for empty EntryIDs with --text, got nil")
	}
}
