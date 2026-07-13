package main

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd"
)

type recordingLocalGit struct {
	committed bool
	messages  []string
	paths     [][]string
}

func (g *recordingLocalGit) HasCommitMessage(context.Context, string) (bool, error) {
	return g.committed, nil
}

func (g *recordingLocalGit) Commit(message string, paths ...string) error {
	g.committed = true
	g.messages = append(g.messages, message)
	g.paths = append(g.paths, append([]string(nil), paths...))
	return nil
}

func TestLocalGitFinalizerCommitsBatchOnce(t *testing.T) {
	git := &recordingLocalGit{}
	graphDir := t.TempDir()
	finalizer := localGitFinalizer{graphDir: graphDir, git: git}
	mutation := sdd.AppliedMutation{
		BatchID: "mutation-1",
		Batch: sdd.MutationBatch{
			Message: "sdd: signal tactical captured",
			Changes: []sdd.DocumentChange{{LogicalPath: "2026/07/13-120000-s-tac-api.md"}},
			Attachments: []sdd.AttachmentMaterialization{
				{LogicalPath: "2026/07/13-120000-s-tac-api/evidence.md"},
				{LogicalPath: "2026/07/13-120000-s-tac-api/evidence.md"},
			},
		},
	}
	if err := finalizer.Finalize(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	if err := finalizer.Finalize(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	if len(git.messages) != 1 || !strings.Contains(git.messages[0], "SDD-Mutation: mutation-1") {
		t.Fatalf("commits = %q", git.messages)
	}
	want := []string{
		filepath.Join(graphDir, "2026/07/13-120000-s-tac-api.md"),
		filepath.Join(graphDir, "2026/07/13-120000-s-tac-api/evidence.md"),
	}
	if len(git.paths) != 1 || !slices.Equal(git.paths[0], want) {
		t.Fatalf("paths = %v, want %v", git.paths, want)
	}
}
