package local_test

import (
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/local"
)

func TestFilesystemSnapshotBranchScope(t *testing.T) {
	graph, err := local.NewFilesystemGraphStore(local.FilesystemGraphStoreOptions{Project: "test", GraphDir: t.TempDir(), Branch: "work"})
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"", "work"} {
		source, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{Branch: branch})
		if err != nil {
			t.Fatal(err)
		}
		if err := source.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{Branch: "main"}); err == nil {
		t.Fatal("wrong branch accepted")
	}
	unscoped, err := local.NewFilesystemGraphStore(local.FilesystemGraphStoreOptions{Project: "test", GraphDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unscoped.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{Branch: "work"}); err == nil {
		t.Fatal("unscoped store accepted branch authority")
	}
}
