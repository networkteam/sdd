package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
)

func TestAppliedMutationAffectedEntryIDs(t *testing.T) {
	const first = "20260101-100000-s-tac-aaa"
	const second = "20260101-100000-s-tac-bbb"
	const path = "2026/01/01-100000-s-tac-aaa.md"
	tests := []struct {
		name  string
		batch sdd.MutationBatch
		want  []string
		fails bool
	}{
		{name: "structured and raw duplicates", batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: path, Document: &sdd.EntryDocument{LogicalPath: path}}, {LogicalPath: path}}}, want: []string{first}},
		{name: "deletion without document", batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: path, Delete: true}}}, want: []string{first}},
		{name: "attachment only", batch: sdd.MutationBatch{Attachments: []sdd.AttachmentMaterialization{{LogicalPath: "2026/01/01-100000-s-tac-aaa/evidence.md"}}}, want: []string{first}},
		{name: "attachment deletion and other owner", batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: "2026/01/01-100000-s-tac-bbb/note.txt", Delete: true}, {LogicalPath: path}}}, want: []string{first, second}},
		{name: "no entries", batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: "wip/marker.md"}}}},
		{name: "empty"},
		{name: "invalid graph path", batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: "2026/01/broken.md"}}}, fails: true},
		{name: "invalid attachment owner", batch: sdd.MutationBatch{Attachments: []sdd.AttachmentMaterialization{{LogicalPath: "2026/01/broken/note.md"}}}, fails: true},
		{name: "path traversal", batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: "../outside.md"}}}, fails: true},
		{name: "inconsistent structured path", batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: path, Document: &sdd.EntryDocument{LogicalPath: "other.md"}}}}, fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := (sdd.AppliedMutation{Project: "base", Revision: "workspace-only", Batch: tt.batch}).AffectedEntryIDs()
			if (err != nil) != tt.fails {
				t.Fatalf("error=%v", err)
			}
			if !tt.fails && !reflect.DeepEqual(ids, tt.want) {
				t.Fatalf("ids=%v want=%v", ids, tt.want)
			}
		})
	}
}

func TestDiscoveryFinalizerRequiresDurableSource(t *testing.T) {
	sentinel := errors.New("source not yet durable")
	called := false
	finalizer := indexingDiscoveryFinalizer{
		retainFinalizedSource: func(context.Context, sdd.AppliedMutation) (string, error) { called = true; return "", sentinel },
		enqueueDiscovery: func(context.Context, sdd.ProjectID, string, []string) error {
			t.Fatal("queued without durable source")
			return nil
		},
	}
	if err := finalizer.Finalize(t.Context(), sdd.AppliedMutation{}); err != nil || called {
		t.Fatalf("empty mutation finalized source: called=%v error=%v", called, err)
	}
	mutation := sdd.AppliedMutation{Revision: "workspace", Batch: sdd.MutationBatch{Changes: []sdd.DocumentChange{{LogicalPath: "2026/01/01-100000-s-tac-aaa.md"}}}}
	if err := finalizer.Finalize(t.Context(), mutation); !errors.Is(err, sentinel) {
		t.Fatalf("error=%v", err)
	}
}
