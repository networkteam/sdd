package index_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/index"
)

func cacheManifest(n int) *index.Manifest {
	m := &index.Manifest{Version: 1, Entries: map[string]index.EntryState{}}
	for i := range n {
		m.AddVersion(fmt.Sprintf("entry-%d", i), index.EntryVersion{Hash: "hash", Fingerprint: "space"})
	}
	return m
}

func TestManifestCacheRefreshAndReadAmplification(t *testing.T) {
	dir := t.TempDir()
	m := cacheManifest(10000)
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	cache := &index.ManifestCache{}
	for range 10000 {
		loaded, err := cache.Read(dir)
		if err != nil || len(loaded.Entries) != 10000 {
			t.Fatalf("read=%v", err)
		}
	}
	if cache.Loads() != 1 {
		t.Fatalf("10000 presence reads decoded manifest %d times", cache.Loads())
	}
	m.AddVersion("new", index.EntryVersion{Hash: "new", Fingerprint: "space"})
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := cache.Read(dir)
	if err != nil || len(loaded.Entries) != 10001 || cache.Loads() != 2 {
		t.Fatalf("publication refresh=%v loads=%d", err, cache.Loads())
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Read(dir); err == nil {
		t.Fatal("storage corruption hidden by cache")
	}
}

func BenchmarkManifestRead(b *testing.B) {
	dir := b.TempDir()
	if err := cacheManifest(10000).Save(dir); err != nil {
		b.Fatal(err)
	}
	b.Run("decode-every-check", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := index.LoadManifest(dir); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("cached-presence-check", func(b *testing.B) {
		cache := &index.ManifestCache{}
		if _, err := cache.Read(dir); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := cache.Read(dir); err != nil {
				b.Fatal(err)
			}
		}
	})
}
