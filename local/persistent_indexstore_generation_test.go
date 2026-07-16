package local

import (
	"context"
	"testing"

	app "github.com/networkteam/sdd/application"
)

// TestPersistentStoreReusesSnapshotUntilWrite is the counter-verified proof of
// the generation-checked snapshot cache: repeated reads of an unchanged store
// reload the snapshot exactly once, and a write bumps the generation so the
// next read reloads exactly once more.
func TestPersistentStoreReusesSnapshotUntilWrite(t *testing.T) {
	cacheRoot := t.TempDir()
	const project = app.ProjectID("gen")
	ns := app.IndexNamespace{Project: project, Fingerprint: "fp", Metric: "cosine"}
	store := NewPersistentSearchIndexStore(project, cacheRoot, "gen/repo")
	ctx := context.Background()
	query := []float32{1, 0}

	chunkA := app.IndexedChunk{Chunk: app.CanonicalChunk{
		ID: "e1#v-a#summary", EntryID: "e1", EntryHash: "a", ContentHash: "c1",
		Text: "alpha", Body: "alpha", IsSummary: true,
	}, Vector: []float32{1, 0}}
	if err := store.Reconcile(ctx, ns, "r1", []app.IndexedChunk{chunkA}, nil); err != nil {
		t.Fatalf("Reconcile 1: %v", err)
	}

	// Two reads of the unchanged store: the first loads, the second reuses.
	for i := 0; i < 2; i++ {
		if _, err := store.Nearest(ctx, []app.IndexNamespace{ns}, query, 5); err != nil {
			t.Fatalf("Nearest %d: %v", i, err)
		}
	}
	if got := store.reloads.Load(); got != 1 {
		t.Fatalf("reloads after two reads of an unchanged store = %d, want 1 (steady-state reuse)", got)
	}

	// A write bumps the store generation; the next read reloads exactly once.
	chunkB := app.IndexedChunk{Chunk: app.CanonicalChunk{
		ID: "e2#v-b#summary", EntryID: "e2", EntryHash: "b", ContentHash: "c2",
		Text: "beta", Body: "beta", IsSummary: true,
	}, Vector: []float32{0, 1}}
	if err := store.Reconcile(ctx, ns, "r2", []app.IndexedChunk{chunkB}, nil); err != nil {
		t.Fatalf("Reconcile 2: %v", err)
	}
	if _, err := store.Nearest(ctx, []app.IndexNamespace{ns}, query, 5); err != nil {
		t.Fatalf("Nearest after write: %v", err)
	}
	if got := store.reloads.Load(); got != 2 {
		t.Fatalf("reloads after a write = %d, want exactly 2 (one reload for the write)", got)
	}

	// Re-reading the now-unchanged store reuses the reloaded snapshot.
	if _, err := store.Nearest(ctx, []app.IndexNamespace{ns}, query, 5); err != nil {
		t.Fatalf("Nearest after reload: %v", err)
	}
	if got := store.reloads.Load(); got != 2 {
		t.Fatalf("reloads after re-reading the unchanged store = %d, want 2", got)
	}
}
