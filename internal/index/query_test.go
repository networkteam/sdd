package index_test

import (
	"context"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/index"
)

func TestQuery_ConcurrentQueriesShareASavedIndex(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	err := index.WriteStore(ctx, dir, func(idx *index.Index) error {
		row := index.Row{EntryID: "entry-a", EntryHash: "h1", ChunkID: "entry-a#summary", Text: "a", Body: "a", Embedding: []float32{1, 0}}
		if err := idx.UpsertEntry(ctx, row.EntryID, nil, []index.Row{row}); err != nil {
			return err
		}
		manifest, err := index.LoadManifest(dir)
		if err != nil {
			return err
		}
		manifest.AddVersion(row.EntryID, index.EntryVersion{Hash: row.EntryHash, ChunkIDs: []string{row.ChunkID}})
		return manifest.Save(dir)
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := index.OpenBuilt(dir)
	if err != nil {
		t.Fatal(err)
	}

	const queries = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	hits := make([][]index.Hit, queries)
	errs := make([]error, queries)
	for n := range queries {
		wg.Go(func() {
			<-start
			hits[n], errs[n] = saved.Query(ctx, []float32{1, 0}, 1)
		})
	}
	close(start)
	wg.Wait()
	for n := range queries {
		if errs[n] != nil || len(hits[n]) != 1 || hits[n][0].EntryID != "entry-a" {
			t.Errorf("query %d should find the saved row, got %v, %v", n, hits[n], errs[n])
		}
	}
}
