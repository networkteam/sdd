package application_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
)

func TestSearchDiscoverySelectionAndCursor(t *testing.T) {
	runtime, graph, dir := preparedRuntime(t, "base")
	putSearchEntry(t, dir, "bbb", "Second")
	putSearchEntry(t, dir, "ccc", "Third")
	source, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Release(); err != nil {
			t.Error(err)
		}
	}()
	const a = "20260101-100000-s-tac-aaa"
	const b = "20260101-100000-s-tac-bbb"
	const c = "20260101-100000-s-tac-ccc"
	const absent = "20260101-100000-s-tac-zzz"
	collect := func(selection []string, cursor sdd.SearchDiscoveryCursor) ([]string, sdd.SearchDiscoveryCursor, error) {
		var ids []string
		for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, EntryIDs: selection, Cursor: cursor}) {
			if err != nil {
				return nil, cursor, err
			}
			ids = append(ids, item.Entry.Version.EntryID)
			cursor = item.Cursor
		}
		return ids, cursor, nil
	}
	ids, _, err := collect(nil, sdd.SearchDiscoveryCursor{})
	if err != nil || !reflect.DeepEqual(ids, []string{a, b, c}) {
		t.Fatalf("all=%v %v", ids, err)
	}
	ids, _, err = collect([]string{c, a, c, absent}, sdd.SearchDiscoveryCursor{})
	if err != nil || !reflect.DeepEqual(ids, []string{a, c}) {
		t.Fatalf("selection=%v %v", ids, err)
	}
	for _, selection := range [][]string{{}, {""}, {"s-tac-aaa"}, {"20260101-100000-x-tac-aaa"}, {"20260101-100000-s-tac-../aaa"}, {"20261301-100000-s-tac-aaa"}} {
		if _, _, err := collect(selection, sdd.SearchDiscoveryCursor{}); err == nil {
			t.Errorf("invalid selection accepted: %v", selection)
		}
	}
	ids, _, err = collect([]string{absent}, sdd.SearchDiscoveryCursor{})
	if err != nil || len(ids) != 0 {
		t.Fatalf("absent=%v %v", ids, err)
	}
	var cursor sdd.SearchDiscoveryCursor
	for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, EntryIDs: []string{c, a}}) {
		if err != nil {
			t.Fatal(err)
		}
		cursor = item.Cursor
		break
	}
	ids, _, err = collect([]string{a, c, a}, cursor)
	if err != nil || !reflect.DeepEqual(ids, []string{c}) {
		t.Fatalf("resume=%v %v", ids, err)
	}
	for _, selection := range [][]string{nil, {a, b}, {a, c, absent}} {
		if _, _, err := collect(selection, cursor); err == nil {
			t.Errorf("incompatible selection accepted: %v", selection)
		}
	}
	_, allCursor, err := collect(nil, sdd.SearchDiscoveryCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := collect([]string{a, b, c}, allCursor); err == nil {
		t.Fatal("whole-project cursor accepted for explicit selection")
	}
	wrong := cursor
	wrong.Revision = "changed"
	if _, _, err := collect([]string{a, c}, wrong); err == nil {
		t.Fatal("changed revision accepted")
	}
	wrong = cursor
	wrong.Namespace.Fingerprint = "changed"
	if _, _, err := collect([]string{a, c}, wrong); err == nil {
		t.Fatal("changed config accepted")
	}
}

func TestScopedDiscoveryPropagatesReadFailure(t *testing.T) {
	runtime, graph, dir := preparedRuntime(t, "base")
	addDiscoveryAttachment(t, dir)
	source, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Release(); err != nil {
			t.Error(err)
		}
	}()
	sentinel := errors.New("retained attachment unavailable")
	source.Attachments = &countingAttachmentPages{AttachmentPageReader: source.Attachments, reads: map[string]int{}, fail: sentinel}
	seen := false
	for _, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, EntryIDs: []string{"20260101-100000-s-tac-aaa"}}) {
		seen = true
		if !errors.Is(err, sentinel) {
			t.Fatalf("error=%v", err)
		}
	}
	if !seen {
		t.Fatal("source failure treated as absent")
	}
}

func addDiscoveryAttachment(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "2026/01/01-100000-s-tac-aaa/note.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("evidence"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestScopedDiscoveryUnreadableDocumentIsNotAbsent(t *testing.T) {
	runtime, graph, dir := preparedRuntime(t, "base")
	if err := os.WriteFile(filepath.Join(dir, "2026/01/01-100000-s-tac-bbb.md"), []byte("---\nbroken: [\n---\n"), 0644); err != nil {
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
	for _, selection := range [][]string{nil, {"20260101-100000-s-tac-bbb"}} {
		failed := false
		for _, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, EntryIDs: selection}) {
			if err != nil {
				failed = true
			}
		}
		if !failed {
			t.Fatalf("unreadable entry treated as absent: %v", selection)
		}
	}
	count := 0
	for _, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, EntryIDs: []string{"20260101-100000-s-tac-aaa"}}) {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("unrelated unreadable entry affected selection: count=%d", count)
	}
}

func TestScopedDiscoverySkipsUnselectedAndEarlierAttachmentReads(t *testing.T) {
	runtime, graph, dir := preparedRuntime(t, "base")
	addDiscoveryAttachment(t, dir)
	putSearchEntry(t, dir, "bbb", "Second")
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
	ids := []string{"20260101-100000-s-tac-aaa", "20260101-100000-s-tac-bbb"}
	var cursor sdd.SearchDiscoveryCursor
	for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source, EntryIDs: ids}) {
		if err != nil {
			t.Fatal(err)
		}
		cursor = item.Cursor
		break
	}
	if pages.reads[ids[0]] != 1 {
		t.Fatalf("reads=%v", pages.reads)
	}
	pages.fail = errors.New("earlier source must not be read")
	for _, q := range []sdd.DiscoverSearchEntriesQuery{{Source: source, EntryIDs: ids, Cursor: cursor}, {Source: source, EntryIDs: ids[1:]}} {
		count := 0
		for item, err := range runtime.DiscoverSearchEntries(t.Context(), q) {
			if err != nil {
				t.Fatal(err)
			}
			if item.Entry.Version.EntryID != ids[1] {
				t.Fatal(item)
			}
			count++
		}
		if count != 1 {
			t.Fatalf("count=%d", count)
		}
	}
	if pages.reads[ids[0]] != 1 {
		t.Fatalf("rehashed skipped entry: %v", pages.reads)
	}
}
