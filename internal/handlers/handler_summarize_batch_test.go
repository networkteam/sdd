package handlers_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

// stubRunner returns a fixed summary for every call and counts invocations. It
// stands in for the LLM so the batch summarize tests stay deterministic.
type stubRunner struct {
	mu    sync.Mutex
	text  string
	calls int
}

func (r *stubRunner) Run(_ context.Context, _ llm.Request) (llm.Result, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return llm.Result{Text: r.text, Identity: llm.Identity{Provider: "test", Model: "test-model"}}, nil
}

func (r *stubRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func readEntryFile(t *testing.T, graphDir, id string) string {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(graphDir, rel))
	if err != nil {
		t.Fatalf("reading %s: %v", id, err)
	}
	return string(data)
}

const (
	summarizedEntry = "20260101-120000-s-prc-aaa" // ships with a summary
	emptyEntry      = "20260101-120001-s-prc-bbb" // has none
)

func writeSummarizePair(t *testing.T, graphDir string) {
	t.Helper()
	writeEntryFile(t, graphDir, summarizedEntry, "---\ntype: signal\nlayer: process\nkind: gap\nsummary: Existing summary.\n---\n\nHas a summary already.\n")
	writeEntryFile(t, graphDir, emptyEntry, "---\ntype: signal\nlayer: process\nkind: gap\n---\n\nNeeds a summary.\n")
}

// TestSummarize_All_FillsOnlyEmpty covers the --all fill-empty semantics
// (d-cpt-4qi): entries that already have a summary are skipped, only empty ones
// are generated.
func TestSummarize_All_FillsOnlyEmpty(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)

	runner := &stubRunner{text: "GENERATED"}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: &recordingCommitter{},
		LLMRunner: runner,
	})

	var summarized, skipped []string
	scmd := &command.SummarizeCmd{
		OnSummarized: func(id, _ string) { summarized = append(summarized, id) },
		OnSkipped:    func(id string) { skipped = append(skipped, id) },
	}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if runner.callCount() != 1 {
		t.Errorf("runner calls = %d, want 1", runner.callCount())
	}
	if len(summarized) != 1 || summarized[0] != emptyEntry {
		t.Errorf("summarized = %v, want [%s]", summarized, emptyEntry)
	}
	if len(skipped) != 1 || skipped[0] != summarizedEntry {
		t.Errorf("skipped = %v, want [%s]", skipped, summarizedEntry)
	}

	if withText := readEntryFile(t, graphDir, summarizedEntry); !strings.Contains(withText, "Existing summary.") || strings.Contains(withText, "GENERATED") {
		t.Errorf("existing entry should be untouched:\n%s", withText)
	}
	if emptyText := readEntryFile(t, graphDir, emptyEntry); !strings.Contains(emptyText, "summary: GENERATED") {
		t.Errorf("empty entry should be summarized:\n%s", emptyText)
	}
}

// TestSummarize_AllForce_RegeneratesAll covers `--all --force`: every
// non-embedded entry is regenerated regardless of an existing summary.
func TestSummarize_AllForce_RegeneratesAll(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)

	runner := &stubRunner{text: "GENERATED"}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: &recordingCommitter{},
		LLMRunner: runner,
	})

	var skipped []string
	scmd := &command.SummarizeCmd{
		Force:     true,
		OnSkipped: func(id string) { skipped = append(skipped, id) },
	}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if runner.callCount() != 2 {
		t.Errorf("runner calls = %d, want 2", runner.callCount())
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none with --force", skipped)
	}
	for _, id := range []string{summarizedEntry, emptyEntry} {
		if text := readEntryFile(t, graphDir, id); !strings.Contains(text, "summary: GENERATED") {
			t.Errorf("%s should be regenerated:\n%s", id, text)
		}
	}
}

// TestSummarize_NamedEntry_RegeneratesUnconditionally covers `sdd summarize
// <id>`: a named entry regenerates even when it already has a summary and even
// without --force.
func TestSummarize_NamedEntry_RegeneratesUnconditionally(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)

	runner := &stubRunner{text: "GENERATED"}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: &recordingCommitter{},
		LLMRunner: runner,
	})

	var summarized, skipped []string
	scmd := &command.SummarizeCmd{
		EntryIDs:     []string{summarizedEntry},
		OnSummarized: func(id, _ string) { summarized = append(summarized, id) },
		OnSkipped:    func(id string) { skipped = append(skipped, id) },
	}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if runner.callCount() != 1 {
		t.Errorf("runner calls = %d, want 1", runner.callCount())
	}
	if len(summarized) != 1 || summarized[0] != summarizedEntry {
		t.Errorf("summarized = %v, want [%s]", summarized, summarizedEntry)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none for a named entry", skipped)
	}
	if text := readEntryFile(t, graphDir, summarizedEntry); !strings.Contains(text, "summary: GENERATED") {
		t.Errorf("named entry should be regenerated:\n%s", text)
	}
}
