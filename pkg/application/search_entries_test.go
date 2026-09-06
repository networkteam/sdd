package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
)

type countingAttachmentPages struct {
	sdd.AttachmentPageReader
	reads map[string]int
	fail  error
}

func (c *countingAttachmentPages) ReadAttachmentPage(ctx context.Context, id, name string, offset int64, limit int) (sdd.AttachmentPage, error) {
	c.reads[id]++
	if c.fail != nil {
		return sdd.AttachmentPage{}, c.fail
	}
	return c.AttachmentPageReader.ReadAttachmentPage(ctx, id, name, offset, limit)
}

func TestEntryDiscoveryIsLazyAndCursorSkipsEarlierHashes(t *testing.T) {
	runtime, graph, dir := preparedRuntime(t, "base")
	putSearchEntry(t, dir, "bbb", "Second")
	for _, id := range []string{"aaa", "bbb"} {
		att := filepath.Join(dir, "2026", "01", "01-100000-s-tac-"+id, "note.md")
		if err := os.MkdirAll(filepath.Dir(att), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(att, []byte("Attached content"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	source, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Release(); err != nil {
			t.Error(err)
		}
	}()
	pages := &countingAttachmentPages{AttachmentPageReader: source.Attachments, reads: map[string]int{}}
	source.Attachments = pages
	seq := runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source})
	if len(pages.reads) != 0 {
		t.Fatal("eager discovery")
	}
	var cursor sdd.SearchDiscoveryCursor
	for item, err := range seq {
		if err != nil {
			t.Fatal(err)
		}
		cursor = item.Cursor
		break
	}
	if len(pages.reads) != 1 || pages.reads["20260101-100000-s-tac-aaa"] != 1 {
		t.Fatalf("reads=%v", pages.reads)
	}
	count := 0
	for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, Cursor: cursor}) {
		if err != nil {
			t.Fatal(err)
		}
		count++
		if item.Entry.Version.EntryID != "20260101-100000-s-tac-bbb" {
			t.Fatal(item)
		}
	}
	if count != 1 || pages.reads["20260101-100000-s-tac-aaa"] != 1 {
		t.Fatalf("resume rehashed earlier entry: %v", pages.reads)
	}
	cursor.Revision = "other"
	for _, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, Cursor: cursor}) {
		if err == nil {
			t.Fatal("wrong cursor accepted")
		}
		break
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, err := range runtime.DiscoverSearchEntries(canceled, sdd.DiscoverSearchEntriesQuery{Source: source}) {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel=%v", err)
		}
		break
	}
	pages.fail = errors.New("attachment unavailable")
	for _, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source}) {
		if !errors.Is(err, pages.fail) {
			t.Fatalf("source error=%v", err)
		}
		break
	}
}

func TestEntryIndexingIdempotencyAndExactSource(t *testing.T) {
	runtime, graph, dir := preparedRuntime(t, "base")
	source, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var descriptor sdd.SearchEntryDescriptor
	for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source}) {
		if err != nil {
			t.Fatal(err)
		}
		descriptor = item.Entry
		break
	}
	putSearchEntry(t, dir, "aaa", "Changed body")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := runtime.IndexSearchEntry(t.Context(), sdd.IndexSearchEntryCmd{Entry: descriptor}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := source.Release(); err != nil {
		t.Fatal(err)
	}
	// Publication skips source acquisition even after that exact source expires.
	if err := runtime.IndexSearchEntry(t.Context(), sdd.IndexSearchEntryCmd{Entry: descriptor}); err != nil {
		t.Fatal(err)
	}
	descriptor.Version.EntryHash = "different"
	if err := runtime.IndexSearchEntry(t.Context(), sdd.IndexSearchEntryCmd{Entry: descriptor}); err == nil {
		t.Fatal("moving branch substituted for exact source")
	}
	descriptor.Version.Namespace.Fingerprint = "different-config"
	if err := runtime.IndexSearchEntry(t.Context(), sdd.IndexSearchEntryCmd{Entry: descriptor}); err == nil {
		t.Fatal("configuration mismatch accepted")
	}
}

func TestZeroChunkEntryPublishes(t *testing.T) {
	runtime, graph, dir := preparedRuntime(t, "base")
	path := filepath.Join(dir, "2026", "01", "01-100000-s-tac-aaa.md")
	if err := os.WriteFile(path, []byte("---\ntype: signal\nkind: gap\nlayer: tactical\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	source, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Release(); err != nil {
			t.Error(err)
		}
	}()
	for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source}) {
		if err != nil {
			t.Fatal(err)
		}
		called := false
		err = runtime.IndexSearchEntry(t.Context(), sdd.IndexSearchEntryCmd{Entry: item.Entry, OnPublished: func(_ string, n int) {
			called = true
			if n != 0 {
				t.Fatalf("chunks=%d", n)
			}
		}})
		if err != nil || !called {
			t.Fatalf("publication=%v called=%v", err, called)
		}
	}
	for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source}) {
		if err != nil || !item.Published {
			t.Fatalf("zero completion=%v %v", item, err)
		}
	}
}
