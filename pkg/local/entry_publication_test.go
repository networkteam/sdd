package local_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/index"
	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/local"
)

func publicationRows(version sdd.SearchEntryVersion) []sdd.IndexedChunk {
	return []sdd.IndexedChunk{{Chunk: sdd.CanonicalChunk{ID: version.EntryID + "-" + version.EntryHash, EntryID: version.EntryID, EntryHash: version.EntryHash, Text: "entry", Body: "entry"}, Vector: []float32{1, 1}}}
}
func TestEntryPublicationConformance(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		name := "memory"
		if persistent {
			name = "disk"
		}
		t.Run(name, func(t *testing.T) {
			var store interface {
				sdd.SearchIndexStore
				sdd.SearchIndexEntryStore
			} = local.NewMemorySearchIndexStore()
			if persistent {
				store = local.NewPersistentSearchIndexStore("project", t.TempDir(), "project")
			}
			version := sdd.SearchEntryVersion{Namespace: sdd.IndexNamespace{Project: "project", Fingerprint: "space", Metric: "cosine"}, EntryID: "entry", EntryHash: "hash"}
			invalid := publicationRows(version)
			invalid = append(invalid, sdd.IndexedChunk{Chunk: sdd.CanonicalChunk{ID: "second", EntryID: "entry", EntryHash: "hash", Text: "bad"}, Vector: []float32{float32(math.NaN()), 1}})
			if err := store.PublishEntry(t.Context(), version, invalid); err == nil {
				t.Fatal("invalid vectors accepted")
			}
			if published, err := store.EntryPublished(t.Context(), version); err != nil || published {
				t.Fatalf("false completion=%v %v", published, err)
			}
			hits, err := store.Nearest(t.Context(), []sdd.IndexNamespace{version.Namespace}, []float32{1, 1}, 10)
			if err != nil || len(hits) != 0 {
				t.Fatalf("partial visibility=%v %v", hits, err)
			}
			for range 2 {
				if err := store.PublishEntry(t.Context(), version, publicationRows(version)); err != nil {
					t.Fatal(err)
				}
			}
			changed := version
			changed.EntryHash = "hash2"
			if err := store.PublishEntry(t.Context(), changed, publicationRows(changed)); err != nil {
				t.Fatal(err)
			}
			other := version
			other.Namespace.Fingerprint = "other"
			if published, err := store.EntryPublished(t.Context(), other); err != nil || published {
				t.Fatalf("config collision=%v %v", published, err)
			}
			empty := version
			empty.EntryID = "empty"
			if err := store.PublishEntry(t.Context(), empty, nil); err != nil {
				t.Fatal(err)
			}
			if published, err := store.EntryPublished(t.Context(), empty); err != nil || !published {
				t.Fatalf("zero chunks=%v %v", published, err)
			}
			hits, err = store.Nearest(t.Context(), []sdd.IndexNamespace{version.Namespace}, []float32{1, 1}, 10)
			if err != nil || len(hits) != 2 {
				t.Fatalf("versions=%v %v", hits, err)
			}
		})
	}
}

func TestDiskPublicationFailureLeavesRowsInvisibleAndRetryable(t *testing.T) {
	root := t.TempDir()
	store := local.NewPersistentSearchIndexStore("project", root, "project")
	version := sdd.SearchEntryVersion{Namespace: sdd.IndexNamespace{Project: "project", Fingerprint: "space", Metric: "cosine"}, EntryID: "entry", EntryHash: "hash"}
	dir := index.StoreDir(root, "project", "space")
	obstruction := filepath.Join(dir, "manifest.json.tmp")
	if err := os.MkdirAll(obstruction, 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishEntry(t.Context(), version, publicationRows(version)); err == nil {
		t.Fatal("expected storage failure")
	}
	store = local.NewPersistentSearchIndexStore("project", root, "project")
	hits, err := store.Nearest(t.Context(), []sdd.IndexNamespace{version.Namespace}, []float32{1, 1}, 1)
	if err != nil || len(hits) != 0 {
		t.Fatalf("unpublished rows visible after restart: %v %v", hits, err)
	}
	if err := os.Remove(obstruction); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishEntry(t.Context(), version, publicationRows(version)); err != nil {
		t.Fatal(err)
	}
	published, err := store.EntryPublished(t.Context(), version)
	if err != nil || !published {
		t.Fatalf("retry=%v %v", published, err)
	}
	hits, err = store.Nearest(t.Context(), []sdd.IndexNamespace{version.Namespace}, []float32{1, 1}, 1)
	if err != nil || len(hits) != 1 {
		t.Fatalf("retry visibility=%v %v", hits, err)
	}
}

func TestFullVersionChunkIdentity(t *testing.T) {
	a, b := "deadbeef"+strings.Repeat("a", 56), "deadbeef"+strings.Repeat("b", 56)
	if index.BodyChunkIDVersioned("entry", a, 0) == index.BodyChunkIDVersioned("entry", b, 0) {
		t.Fatal("full versions collided")
	}
}

func BenchmarkEntryPublicationPresence(b *testing.B) {
	store := local.NewPersistentSearchIndexStore("project", b.TempDir(), "project")
	version := sdd.SearchEntryVersion{Namespace: sdd.IndexNamespace{Project: "project", Fingerprint: "space", Metric: "cosine"}, EntryID: "entry", EntryHash: "hash"}
	if err := store.PublishEntry(context.Background(), version, publicationRows(version)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := store.EntryPublished(context.Background(), version); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublishedRetrieval(b *testing.B) {
	root := b.TempDir()
	ns := sdd.IndexNamespace{Project: "project", Fingerprint: "space", Metric: "cosine"}
	store := local.NewPersistentSearchIndexStore("project", root, "project")
	key := sdd.SearchEntryVersion{Namespace: ns, EntryID: "published", EntryHash: "h"}
	rows := make([]sdd.IndexedChunk, 1000)
	for i := range rows {
		rows[i] = sdd.IndexedChunk{Chunk: sdd.CanonicalChunk{ID: fmt.Sprintf("p-%d", i), EntryID: key.EntryID, EntryHash: key.EntryHash, Text: "published document text", Body: "body"}, Vector: []float32{1, 1}}
	}
	if err := store.PublishEntry(context.Background(), key, rows); err != nil {
		b.Fatal(err)
	}
	dir := index.StoreDir(root, "project", "space")
	if err := index.WriteStore(context.Background(), dir, func(idx *index.Index) error {
		unpublished := make([]index.Row, 1000)
		for i := range unpublished {
			unpublished[i] = index.Row{EntryID: "unpublished", EntryHash: "other", ChunkID: fmt.Sprintf("u-%d", i), Text: "unpublished", Embedding: []float32{1, 1}}
		}
		return idx.UpsertEntry(context.Background(), "unpublished", nil, unpublished)
	}); err != nil {
		b.Fatal(err)
	}
	hits, err := store.Nearest(context.Background(), []sdd.IndexNamespace{ns}, []float32{1, 1}, 10)
	if err != nil || len(hits) != 10 {
		b.Fatalf("query: %v %v", hits, err)
	}
	for _, hit := range hits {
		if hit.EntryID != "published" {
			b.Fatal("unpublished hit")
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		hits, err := store.Nearest(context.Background(), []sdd.IndexNamespace{ns}, []float32{1, 1}, 10)
		if err != nil || len(hits) != 10 {
			b.Fatal(err)
		}
	}
}
