package local

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	app "github.com/networkteam/sdd/pkg/application"
)

func TestFilesystemGraphStoreRollsBackMidBatchFailure(t *testing.T) {
	store, initialRevision, batch, originals := atomicGraphFixture(t)
	store.beforeApplyOperation = func(index int) error {
		if index == 1 {
			return errors.New("injected second operation failure")
		}
		return nil
	}

	result, err := store.Apply(t.Context(), initialRevision, batch, nil)
	if err == nil || result.State != app.MutationNotApplied {
		t.Fatalf("Apply = %+v, %v; want not_applied with injected error", result, err)
	}
	assertAtomicFiles(t, store.dir, originals)
	current, err := store.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision() != initialRevision {
		t.Fatalf("revision after rollback = %q, want %q", current.Revision(), initialRevision)
	}
	reconciled, err := store.Reconcile(t.Context(), batch.ID, batch.Digest)
	if err != nil || reconciled.State != app.MutationNotApplied || reconciled.Revision != initialRevision {
		t.Fatalf("Reconcile = %+v, %v", reconciled, err)
	}
}

func TestFilesystemGraphStoreRestartRecoversInterruptedRollback(t *testing.T) {
	store, initialRevision, batch, originals := atomicGraphFixture(t)
	store.beforeApplyOperation = func(index int) error {
		if index == 1 {
			return errors.New("injected process interruption")
		}
		return nil
	}
	store.beforeRollbackOperation = func(int) error {
		return errors.New("injected rollback interruption")
	}

	result, err := store.Apply(t.Context(), initialRevision, batch, nil)
	if err == nil || result.State != app.MutationUnknown {
		t.Fatalf("Apply = %+v, %v; want unknown interrupted transaction", result, err)
	}
	firstPath := filepath.Join(store.dir, filepath.FromSlash(batch.Changes[0].LogicalPath))
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) == string(originals[batch.Changes[0].LogicalPath]) {
		t.Fatal("fault did not leave the first operation applied for restart recovery")
	}

	restarted, err := NewFilesystemGraphStore(FilesystemGraphStoreOptions{Project: "atomic", GraphDir: store.dir})
	if err != nil {
		t.Fatal(err)
	}
	current, err := restarted.Current(t.Context())
	if err != nil {
		t.Fatalf("Current after restart recovery: %v", err)
	}
	assertAtomicFiles(t, restarted.dir, originals)
	if current.Revision() != initialRevision {
		t.Fatalf("revision after restart rollback = %q, want %q", current.Revision(), initialRevision)
	}
	reconciled, err := restarted.Reconcile(t.Context(), batch.ID, batch.Digest)
	if err != nil || reconciled.State != app.MutationNotApplied || reconciled.Revision != initialRevision {
		t.Fatalf("Reconcile after restart = %+v, %v", reconciled, err)
	}
	transactionDir := filepath.Join(restarted.dir, ".sdd-runtime", "transactions", batch.ID)
	if _, err := os.Stat(transactionDir); !os.IsNotExist(err) {
		t.Fatalf("transaction directory remains after recovery: %v", err)
	}
}

func TestFilesystemGraphStoreCurrentCannotObservePartialBatch(t *testing.T) {
	store, initialRevision, batch, originals := atomicGraphFixture(t)
	firstApplied := make(chan struct{})
	releaseFailure := make(chan struct{})
	store.beforeApplyOperation = func(index int) error {
		if index != 1 {
			return nil
		}
		close(firstApplied)
		<-releaseFailure
		return errors.New("injected second operation failure")
	}
	applyDone := make(chan app.ApplyResult, 1)
	go func() {
		result, _ := store.Apply(t.Context(), initialRevision, batch, nil)
		applyDone <- result
	}()
	<-firstApplied

	reader, err := NewFilesystemGraphStore(FilesystemGraphStoreOptions{Project: "atomic", GraphDir: store.dir})
	if err != nil {
		t.Fatal(err)
	}
	currentDone := make(chan *app.Snapshot, 1)
	go func() {
		snapshot, _ := reader.Current(t.Context())
		currentDone <- snapshot
	}()
	select {
	case <-currentDone:
		t.Fatal("Current returned while a partial batch held the graph lock")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseFailure)
	if result := <-applyDone; result.State != app.MutationNotApplied {
		t.Fatalf("Apply state = %s, want not_applied", result.State)
	}
	current := <-currentDone
	if current == nil || current.Revision() != initialRevision {
		t.Fatalf("Current after rollback = %#v, want revision %s", current, initialRevision)
	}
	assertAtomicFiles(t, store.dir, originals)
}

func atomicGraphFixture(t *testing.T) (*FilesystemGraphStore, string, app.MutationBatch, map[string][]byte) {
	t.Helper()
	dir := canonicalTempDir(t)
	originals := map[string][]byte{
		"2026/07/13-070000-s-tac-one.md": atomicEntry("First original summary.", "First original body."),
		"2026/07/13-070100-s-tac-two.md": atomicEntry("Second original summary.", "Second original body."),
	}
	for logicalPath, data := range originals {
		path := filepath.Join(dir, filepath.FromSlash(logicalPath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewFilesystemGraphStore(FilesystemGraphStoreOptions{Project: "atomic", GraphDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	batch := app.MutationBatch{ID: "atomic-mid-batch", Changes: []app.DocumentChange{
		{LogicalPath: "2026/07/13-070000-s-tac-one.md", CanonicalBytes: atomicEntry("First replacement summary.", "First replacement body.")},
		{LogicalPath: "2026/07/13-070100-s-tac-two.md", CanonicalBytes: atomicEntry("Second replacement summary.", "Second replacement body.")},
	}}
	batch.Digest, err = app.MutationBatchDigest(batch)
	if err != nil {
		t.Fatal(err)
	}
	return store, initial.Revision(), batch, originals
}

func atomicEntry(summary, body string) []byte {
	return []byte("---\ntype: signal\nkind: gap\nlayer: tactical\nconfidence: high\nsummary: " + summary + "\n---\n\n" + body + "\n")
}

func assertAtomicFiles(t *testing.T, root string, originals map[string][]byte) {
	t.Helper()
	for logicalPath, want := range originals {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(logicalPath)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s contains a partial batch result\ngot:  %q\nwant: %q", logicalPath, got, want)
		}
	}
}
