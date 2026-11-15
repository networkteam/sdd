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
)

// changeReader serves a fixed set of modified paths and their HEAD content.
type changeReader struct {
	modified []string
	head     map[string]string
}

func (c changeReader) ModifiedFiles(context.Context, string) ([]string, error) {
	return c.modified, nil
}

func (c changeReader) HeadContent(_ context.Context, path string) ([]byte, error) {
	return []byte(c.head[path]), nil
}

type pathRecordingCommitter struct{ paths []string }

func (c *pathRecordingCommitter) Commit(_ string, paths ...string) error {
	c.paths = append(c.paths, paths...)
	return nil
}

func TestCommitStrandedTitles(t *testing.T) {
	graphDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const untitled = "---\ntype: signal\nlayer: process\nkind: gap\nsummary: S.\n---\n\nBody.\n"
	const titled = "---\ntype: signal\nlayer: process\nkind: gap\ntitle: A title\nsummary: S.\n---\n\nBody.\n"
	files := map[string]struct{ head, working string }{
		"20260101-120000-s-prc-aaa": {head: untitled, working: titled},                                         // stranded
		"20260101-120001-s-prc-bbb": {head: titled, working: strings.Replace(titled, "Body", "Edited", 1)},     // titled before
		"20260101-120002-s-prc-ccc": {head: untitled, working: strings.Replace(untitled, "Body", "Edited", 1)}, // no title gained
	}
	reader := changeReader{head: map[string]string{}}
	for id, f := range files {
		writeEntryFile(t, graphDir, id, f.working)
		rel, err := model.IDToRelPath(id)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(graphDir, rel)
		reader.modified = append(reader.modified, path)
		reader.head[path] = f.head
	}
	stray := filepath.Join(graphDir, "2026", "01", "notes.md")
	if err := os.WriteFile(stray, []byte("not an entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader.modified = append(reader.modified, stray)

	committer := &pathRecordingCommitter{}
	h := handlers.New(handlers.Options{GraphDir: graphDir, Reader: &graphReader{}, Committer: committer, Changes: reader})
	var committed int
	if err := h.CommitStrandedTitles(context.Background(), &command.CommitStrandedTitlesCmd{OnCommitted: func(n int) { committed = n }}); err != nil {
		t.Fatalf("CommitStrandedTitles: %v", err)
	}
	strandedRel, _ := model.IDToRelPath("20260101-120000-s-prc-aaa")
	if committed != 1 || len(committer.paths) != 1 || committer.paths[0] != filepath.Join(graphDir, strandedRel) {
		t.Errorf("only the entry that gained a title on disk should be committed, got %d reported, paths %v", committed, committer.paths)
	}
}

func TestCommitStrandedTitles_NothingStrandedCommitsNothing(t *testing.T) {
	committer := &pathRecordingCommitter{}
	h := handlers.New(handlers.Options{GraphDir: t.TempDir(), Reader: &graphReader{}, Committer: committer, Changes: changeReader{}})
	called := false
	if err := h.CommitStrandedTitles(context.Background(), &command.CommitStrandedTitlesCmd{OnCommitted: func(int) { called = true }}); err != nil {
		t.Fatalf("CommitStrandedTitles: %v", err)
	}
	if called || len(committer.paths) != 0 {
		t.Errorf("a clean graph should commit nothing, got %v", committer.paths)
	}
}
