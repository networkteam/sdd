package application_test

import (
	"context"
	"fmt"
	"io/fs"
	"testing/fstest"

	sdd "github.com/networkteam/sdd/pkg/application"
)

type exampleTreeAttachments struct {
	tree     fs.FS
	graphDir string
}

func (r exampleTreeAttachments) ReadAttachmentPage(_ context.Context, entry, name string, offset int64, limit int) (sdd.AttachmentPage, error) {
	return sdd.PageAttachment(r.tree, r.graphDir, entry, name, offset, limit)
}

func ExampleAcquiredSnapshot() {
	// A revision-backed adapter obtains this immutable tree from its storage.
	tree := fstest.MapFS{
		".sdd/config.yaml":                                {Data: []byte("repo_id: example\nlanguage: en\n")},
		".sdd/graph/2026/01/01-100000-s-tac-aaa.md":       {Data: []byte("---\ntype: signal\nkind: fact\nlayer: tactical\nsummary: Source fixture.\n---\n\nSource fixture.")},
		".sdd/graph/2026/01/01-100000-s-tac-aaa/note.txt": {Data: []byte("Same revision.")},
	}
	config, err := sdd.ReadProjectConfigFS(tree)
	if err != nil {
		panic(err)
	}
	snapshot, err := sdd.LoadSnapshotFS(context.Background(), "example", "R1", tree, config.GraphDir)
	if err != nil {
		panic(err)
	}
	source := &sdd.AcquiredSnapshot{
		Snapshot: snapshot, Config: &config, Attachments: exampleTreeAttachments{tree: tree, graphDir: config.GraphDir},
		Release: func() error { return nil },
	}
	page, err := source.Attachments.ReadAttachmentPage(context.Background(), "20260101-100000-s-tac-aaa", "note.txt", 0, 100)
	if err != nil {
		panic(err)
	}
	if err := source.Release(); err != nil {
		panic(err)
	}
	fmt.Println(source.Snapshot.Revision(), source.Config.Language, string(page.Content))
	// Output: R1 en Same revision.
}
