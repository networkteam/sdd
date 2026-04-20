package handlers_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// graphReader is a Reader that loads the graph by reading the filesystem.
// Used by rewrite tests where we need the real graph parser (so frontmatter
// changes from one RewriteEntry call are observed by a later one).
type graphReader struct {
	loadErr error
}

func (r *graphReader) LoadGraph(dir string) (*model.Graph, error) {
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	var entries []*model.Entry
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		id, err := model.RelPathToID(rel)
		if err != nil {
			return nil // skip non-entry files
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entry, err := model.ParseEntry(id+".md", string(data))
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return model.NewGraph(entries), nil
}

func (r *graphReader) LoadWIPMarkers(string) ([]*model.WIPMarker, error) { return nil, nil }
func (r *graphReader) Preflight(context.Context, query.PreflightQuery) (*query.PreflightResult, error) {
	return nil, nil
}
func (r *graphReader) SkillStatus(context.Context, query.SkillStatusQuery) (*query.SkillStatusResult, error) {
	return &query.SkillStatusResult{}, nil
}

// recordingMover records move calls and performs the rename on disk.
type recordingMover struct {
	calls []moveCall
}

type moveCall struct {
	src, dst string
}

func (m *recordingMover) Move(src, dst string) error {
	m.calls = append(m.calls, moveCall{src, dst})
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// writeEntryFile writes a minimal entry markdown file at the computed path.
func writeEntryFile(t *testing.T, graphDir, id, content string) {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(graphDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestRewriteEntry_ActionToDoneSignal is the primary migration case: an
// action (a-*) becomes a done-kind signal (s-*). The file moves, frontmatter
// flips type and kind, and every inbound reference updates to the new ID.
func TestRewriteEntry_ActionToDoneSignal(t *testing.T) {
	graphDir := t.TempDir()

	// Target action.
	writeEntryFile(t, graphDir, "20260418-101837-a-tac-r8l", `---
type: action
layer: tactical
refs:
  - 20260418-100000-d-tac-pln
closes:
  - 20260418-100000-d-tac-pln
participants:
  - Christopher
---

Shipped the plan.
`)

	// Plan it closes (target of closes).
	writeEntryFile(t, graphDir, "20260418-100000-d-tac-pln", `---
type: decision
layer: tactical
kind: plan
---

The plan.
`)

	// Inbound decision that refs the action.
	writeEntryFile(t, graphDir, "20260419-090000-d-cpt-inb", `---
type: decision
layer: conceptual
kind: directive
refs:
  - 20260418-101837-a-tac-r8l
  - 20260418-100000-d-tac-pln
---

Builds on the action.
`)

	mover := &recordingMover{}
	committer := &recordingCommitter{}

	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: committer,
		Mover:     mover,
	})

	var cbOld, cbNew string
	var cbInbound []string
	rcmd := &command.RewriteEntryCmd{
		EntryID: "20260418-101837-a-tac-r8l",
		NewType: model.TypeSignal,
		NewKind: model.KindDone,
		OnRewritten: func(oldID, newID string, inbound []string) {
			cbOld, cbNew = oldID, newID
			cbInbound = inbound
		},
	}
	if err := h.RewriteEntry(context.Background(), rcmd); err != nil {
		t.Fatalf("RewriteEntry: %v", err)
	}

	wantNew := "20260418-101837-s-tac-r8l"
	if cbOld != "20260418-101837-a-tac-r8l" || cbNew != wantNew {
		t.Errorf("callback IDs = (%q → %q), want (old → %q)", cbOld, cbNew, wantNew)
	}

	// Old file must no longer exist at the action path.
	oldPath := filepath.Join(graphDir, "2026/04/18-101837-a-tac-r8l.md")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file still exists at %s", oldPath)
	}

	// New file exists at the signal path, with rewritten frontmatter.
	newPath := filepath.Join(graphDir, "2026/04/18-101837-s-tac-r8l.md")
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("reading new file: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "type: signal") {
		t.Errorf("new file missing 'type: signal':\n%s", text)
	}
	if !strings.Contains(text, "kind: done") {
		t.Errorf("new file missing 'kind: done':\n%s", text)
	}

	// Inbound decision's refs must have been rewritten.
	inbPath := filepath.Join(graphDir, "2026/04/19-090000-d-cpt-inb.md")
	inb, err := os.ReadFile(inbPath)
	if err != nil {
		t.Fatalf("reading inbound: %v", err)
	}
	if strings.Contains(string(inb), "20260418-101837-a-tac-r8l") {
		t.Errorf("inbound still references the old action ID:\n%s", string(inb))
	}
	if !strings.Contains(string(inb), wantNew) {
		t.Errorf("inbound missing the new signal ID:\n%s", string(inb))
	}

	// Mover was called exactly once (main file). No attachments in this fixture.
	if len(mover.calls) != 1 {
		t.Errorf("mover calls = %d, want 1 (main file)", len(mover.calls))
	}

	// Committer called once — atomic commit covers both target and inbound.
	if committer.calls != 1 {
		t.Errorf("committer calls = %d, want 1 atomic commit", committer.calls)
	}

	// Inbound callback lists the entry we rewired.
	if len(cbInbound) != 1 || cbInbound[0] != "20260419-090000-d-cpt-inb" {
		t.Errorf("inbound callback = %v, want [inb]", cbInbound)
	}
}

// TestRewriteEntry_KindOnlyPreservesPath covers the same-type case: changing
// only the kind should rewrite frontmatter in place without invoking the
// Mover and without changing the file path.
func TestRewriteEntry_KindOnlyPreservesPath(t *testing.T) {
	graphDir := t.TempDir()

	writeEntryFile(t, graphDir, "20260418-120000-d-tac-zzz", `---
type: decision
layer: tactical
kind: directive
---

A directive that should be an activity.
`)

	mover := &recordingMover{}
	committer := &recordingCommitter{}

	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: committer,
		Mover:     mover,
	})

	rcmd := &command.RewriteEntryCmd{
		EntryID: "20260418-120000-d-tac-zzz",
		NewType: model.TypeDecision,
		NewKind: model.KindActivity,
	}
	if err := h.RewriteEntry(context.Background(), rcmd); err != nil {
		t.Fatalf("RewriteEntry: %v", err)
	}

	path := filepath.Join(graphDir, "2026/04/18-120000-d-tac-zzz.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading rewritten: %v", err)
	}
	if !strings.Contains(string(data), "kind: activity") {
		t.Errorf("expected kind: activity in frontmatter, got:\n%s", string(data))
	}

	if len(mover.calls) != 0 {
		t.Errorf("mover should not be called for kind-only rewrite, got %d calls", len(mover.calls))
	}
}

// TestRewriteEntry_DryRun_NoSideEffects verifies --dry-run: the callback fires
// with the computed IDs, but no files move, no frontmatter is rewritten, and
// no commit is made.
func TestRewriteEntry_DryRun_NoSideEffects(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, "20260418-130000-a-ops-qqq", `---
type: action
layer: operational
---

An action.
`)

	mover := &recordingMover{}
	committer := &recordingCommitter{}

	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: committer,
		Mover:     mover,
	})

	var seenNew string
	rcmd := &command.RewriteEntryCmd{
		EntryID: "20260418-130000-a-ops-qqq",
		NewType: model.TypeSignal,
		NewKind: model.KindDone,
		DryRun:  true,
		OnRewritten: func(_, newID string, _ []string) {
			seenNew = newID
		},
	}
	if err := h.RewriteEntry(context.Background(), rcmd); err != nil {
		t.Fatalf("RewriteEntry: %v", err)
	}

	if seenNew != "20260418-130000-s-ops-qqq" {
		t.Errorf("dry-run callback newID = %q, want the computed signal id", seenNew)
	}
	if len(mover.calls) != 0 {
		t.Errorf("dry-run must not move files, got %d calls", len(mover.calls))
	}
	if committer.calls != 0 {
		t.Errorf("dry-run must not commit, got %d calls", committer.calls)
	}

	// Old path still holds the original file; new path must not exist.
	if _, err := os.Stat(filepath.Join(graphDir, "2026/04/18-130000-a-ops-qqq.md")); err != nil {
		t.Errorf("original file should still exist on dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(graphDir, "2026/04/18-130000-s-ops-qqq.md")); !os.IsNotExist(err) {
		t.Errorf("new-id file must not exist after dry-run")
	}
}

// TestRewriteEntry_NoCommit writes to disk but skips the git commit.
func TestRewriteEntry_NoCommit(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, "20260418-140000-a-prc-nnn", `---
type: action
layer: process
---

Action.
`)

	committer := &recordingCommitter{}
	mover := &recordingMover{}

	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: committer,
		Mover:     mover,
	})

	rcmd := &command.RewriteEntryCmd{
		EntryID:  "20260418-140000-a-prc-nnn",
		NewType:  model.TypeSignal,
		NewKind:  model.KindDone,
		NoCommit: true,
	}
	if err := h.RewriteEntry(context.Background(), rcmd); err != nil {
		t.Fatalf("RewriteEntry: %v", err)
	}
	if committer.calls != 0 {
		t.Errorf("--no-commit: committer should not be called, got %d", committer.calls)
	}
	if len(mover.calls) != 1 {
		t.Errorf("move should still happen with --no-commit; got %d", len(mover.calls))
	}
}

// TestRewriteEntry_AttachmentDirectory exercises the attachment-dir rename
// path: when the type flips and the entry has a co-located attachment dir,
// both the file and the dir move.
func TestRewriteEntry_AttachmentDirectory(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, "20260418-150000-a-cpt-att", `---
type: action
layer: conceptual
---

Has attachments.
`)

	// Create the attachment directory and a file inside it — mimics a real
	// entry that carried a plan attachment.
	attachDir := filepath.Join(graphDir, "2026/04/18-150000-a-cpt-att")
	if err := os.MkdirAll(attachDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachDir, "plan.md"), []byte("plan body"), 0644); err != nil {
		t.Fatal(err)
	}

	mover := &recordingMover{}
	committer := &recordingCommitter{}

	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: committer,
		Mover:     mover,
	})

	rcmd := &command.RewriteEntryCmd{
		EntryID: "20260418-150000-a-cpt-att",
		NewType: model.TypeSignal,
		NewKind: model.KindDone,
	}
	if err := h.RewriteEntry(context.Background(), rcmd); err != nil {
		t.Fatalf("RewriteEntry: %v", err)
	}

	// Mover should have been called twice: once for the .md and once for the
	// attachment directory.
	if len(mover.calls) != 2 {
		t.Errorf("mover calls = %d, want 2 (file + attach dir)", len(mover.calls))
	}

	newAttachDir := filepath.Join(graphDir, "2026/04/18-150000-s-cpt-att")
	if _, err := os.Stat(filepath.Join(newAttachDir, "plan.md")); err != nil {
		t.Errorf("attachment not moved to new location: %v", err)
	}
}

// TestRewriteEntry_NoOpRejected prevents a silent no-op rewrite — the caller
// gets a clear error instead of a committed no-change.
func TestRewriteEntry_NoOpRejected(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, "20260418-160000-d-tac-xxx", `---
type: decision
layer: tactical
kind: directive
---

Already a directive.
`)

	h := handlers.New(handlers.Options{
		GraphDir: graphDir,
		Reader:   &graphReader{},
	})

	err := h.RewriteEntry(context.Background(), &command.RewriteEntryCmd{
		EntryID: "20260418-160000-d-tac-xxx",
		NewType: model.TypeDecision,
		NewKind: model.KindDirective,
	})
	if err == nil {
		t.Fatal("expected no-op error, got nil")
	}
	if !strings.Contains(err.Error(), "no-op") {
		t.Errorf("error = %v, want no-op message", err)
	}
}

// TestRewriteEntry_CollisionRejected catches the case where the computed new
// ID would overlap an already-present entry.
func TestRewriteEntry_CollisionRejected(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, "20260418-170000-a-tac-col", `---
type: action
layer: tactical
---

Action.
`)
	writeEntryFile(t, graphDir, "20260418-170000-s-tac-col", `---
type: signal
layer: tactical
kind: gap
---

Same-suffix signal — triggers collision.
`)

	h := handlers.New(handlers.Options{
		GraphDir: graphDir,
		Reader:   &graphReader{},
	})

	err := h.RewriteEntry(context.Background(), &command.RewriteEntryCmd{
		EntryID: "20260418-170000-a-tac-col",
		NewType: model.TypeSignal,
		NewKind: model.KindDone,
	})
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "collide") {
		t.Errorf("error = %v, want collision message", err)
	}
}
