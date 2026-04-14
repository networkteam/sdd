package handlers_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/finders"
	"github.com/networkteam/resonance/framework/sdd/handlers"
	"github.com/networkteam/resonance/framework/sdd/model"
)

func TestLintFix_AddsKindToDecisions(t *testing.T) {
	tmp := t.TempDir()
	stderr := &bytes.Buffer{}
	committer := &recordingCommitter{}

	// Write a decision entry without kind field.
	entryID := "20260414-120000-d-tac-abc"
	entryContent := "---\ntype: decision\nlayer: tactical\nconfidence: high\n---\n\nSome decision without kind.\n"
	relPath, err := model.IDToRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmp, relPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(entryContent), 0644); err != nil {
		t.Fatal(err)
	}

	h := handlers.New(handlers.Options{
		GraphDir:  tmp,
		Reader:    finders.New(nil),
		Committer: committer,
		Stderr:    stderr,
	})

	var fixedEntries []string
	fixCmd := &command.LintFixCmd{
		OnFixed: func(id string, fixes []string) {
			fixedEntries = append(fixedEntries, id)
		},
	}

	if err := h.LintFix(context.Background(), fixCmd); err != nil {
		t.Fatalf("LintFix: %v", err)
	}

	// Verify the entry was fixed.
	if len(fixedEntries) != 1 || fixedEntries[0] != entryID {
		t.Errorf("fixedEntries = %v, want [%s]", fixedEntries, entryID)
	}

	// Verify the file now contains kind: directive.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "kind: directive") {
		t.Errorf("patched file should contain 'kind: directive', got:\n%s", string(data))
	}

	// Verify commit was made.
	if committer.calls != 1 {
		t.Errorf("Committer.Commit called %d times, want 1", committer.calls)
	}
}

func TestLintFix_SkipsEntriesWithKind(t *testing.T) {
	tmp := t.TempDir()
	committer := &recordingCommitter{}

	// Write a decision entry WITH kind field.
	entryID := "20260414-120000-d-tac-abc"
	entryContent := "---\ntype: decision\nlayer: tactical\nkind: contract\nconfidence: high\n---\n\nSome contract.\n"
	relPath, err := model.IDToRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmp, relPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(entryContent), 0644); err != nil {
		t.Fatal(err)
	}

	h := handlers.New(handlers.Options{
		GraphDir:  tmp,
		Reader:    finders.New(nil),
		Committer: committer,
		Stderr:    &bytes.Buffer{},
	})

	fixCmd := &command.LintFixCmd{}
	if err := h.LintFix(context.Background(), fixCmd); err != nil {
		t.Fatalf("LintFix: %v", err)
	}

	// No commit — nothing to fix.
	if committer.calls != 0 {
		t.Errorf("Committer.Commit called %d times, want 0", committer.calls)
	}
}
