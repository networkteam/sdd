package index

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func mkRow(entryID, chunkID, text string, embedding []float32, opts ...func(*Row)) Row {
	r := Row{
		EntryID:          entryID,
		ChunkID:          chunkID,
		Text:             text,
		Body:             text,
		ContentHash:      HashContent(text),
		ModelFingerprint: "test/model/4",
		Embedding:        embedding,
	}
	for _, o := range opts {
		o(&r)
	}
	return r
}

func TestIndex_UpsertAndQuery(t *testing.T) {
	t.Parallel()

	idx := OpenInMemory()
	ctx := context.Background()

	rows := []Row{
		mkRow("entry-A", "entry-A#summary", "summary A", []float32{1, 0, 0, 0}, func(r *Row) {
			r.IsSummary = true
		}),
		mkRow("entry-A", "entry-A#body-0", "body A", []float32{0.9, 0.1, 0, 0}, func(r *Row) {
			r.Breadcrumb = []string{"Section"}
			r.Depth = 2
		}),
	}
	if err := idx.UpsertEntry(ctx, "entry-A", nil, rows); err != nil {
		t.Fatalf("UpsertEntry: %v", err)
	}
	if got := idx.Count(); got != 2 {
		t.Errorf("Count: got %d, want 2", got)
	}

	hits, err := idx.Query(ctx, []float32{1, 0, 0, 0}, 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	// First hit must be the summary chunk (cosine identical to query).
	if hits[0].ChunkID != "entry-A#summary" {
		t.Errorf("top hit: got %q", hits[0].ChunkID)
	}
	if !hits[0].IsSummary {
		t.Errorf("top hit IsSummary should be true")
	}
	if hits[1].Depth != 2 {
		t.Errorf("body chunk depth: got %d, want 2", hits[1].Depth)
	}
	if !reflect.DeepEqual(hits[1].Breadcrumb, []string{"Section"}) {
		t.Errorf("body chunk breadcrumb: got %#v", hits[1].Breadcrumb)
	}
}

func TestIndex_UpsertReplacesOldChunks(t *testing.T) {
	t.Parallel()

	idx := OpenInMemory()
	ctx := context.Background()

	old := []Row{
		mkRow("entry-B", "entry-B#summary", "old summary", []float32{1, 0, 0, 0}, func(r *Row) {
			r.IsSummary = true
		}),
		mkRow("entry-B", "entry-B#body-0", "old body", []float32{0, 1, 0, 0}),
	}
	if err := idx.UpsertEntry(ctx, "entry-B", nil, old); err != nil {
		t.Fatal(err)
	}

	// Re-upsert with new content; pass the old chunk IDs so the index
	// drops them before adding the new set.
	oldIDs := []string{"entry-B#summary", "entry-B#body-0"}
	updated := []Row{
		mkRow("entry-B", "entry-B#summary", "new summary", []float32{1, 0, 0, 0}, func(r *Row) {
			r.IsSummary = true
		}),
	}
	if err := idx.UpsertEntry(ctx, "entry-B", oldIDs, updated); err != nil {
		t.Fatal(err)
	}

	if got := idx.Count(); got != 1 {
		t.Errorf("Count after re-upsert: got %d, want 1", got)
	}

	hits, err := idx.Query(ctx, []float32{1, 0, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Body != "new summary" {
		t.Errorf("expected single new-summary hit, got %#v", hits)
	}
}

func TestIndex_DeleteEntry(t *testing.T) {
	t.Parallel()

	idx := OpenInMemory()
	ctx := context.Background()

	rows := []Row{
		mkRow("entry-C", "entry-C#summary", "C summary", []float32{1, 0, 0, 0}, func(r *Row) {
			r.IsSummary = true
		}),
	}
	if err := idx.UpsertEntry(ctx, "entry-C", nil, rows); err != nil {
		t.Fatal(err)
	}
	if err := idx.DeleteEntry(ctx, []string{"entry-C#summary"}); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	if got := idx.Count(); got != 0 {
		t.Errorf("Count after delete: got %d, want 0", got)
	}
}

func TestIndex_RejectsRowsWithoutEmbedding(t *testing.T) {
	t.Parallel()
	idx := OpenInMemory()
	rows := []Row{{EntryID: "x", ChunkID: "x#summary"}}
	err := idx.UpsertEntry(context.Background(), "x", nil, rows)
	if err == nil {
		t.Error("expected error for row without embedding")
	}
}

func TestIndex_RejectsMismatchedEntryID(t *testing.T) {
	t.Parallel()
	idx := OpenInMemory()
	rows := []Row{{EntryID: "wrong", ChunkID: "x#summary", Embedding: []float32{1, 0}}}
	err := idx.UpsertEntry(context.Background(), "x", nil, rows)
	if err == nil {
		t.Error("expected error for mismatched entry id")
	}
}

func TestIndex_QueryEmptyCollection(t *testing.T) {
	t.Parallel()
	idx := OpenInMemory()
	hits, err := idx.Query(context.Background(), []float32{1, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if hits != nil {
		t.Errorf("expected nil hits on empty collection, got %#v", hits)
	}
}

func TestIndex_PersistRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx := context.Background()

	idx, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	rows := []Row{
		mkRow("entry-P", "entry-P#summary", "persisted summary", []float32{1, 0, 0, 0}, func(r *Row) {
			r.IsSummary = true
		}),
	}
	if err := idx.UpsertEntry(ctx, "entry-P", nil, rows); err != nil {
		t.Fatal(err)
	}
	// Verify chromem subdirectory exists at the expected place.
	if _, err := os.Stat(filepath.Join(dir, "chromem")); err != nil {
		t.Errorf("chromem dir not created: %v", err)
	}

	// Re-open and confirm the row survived.
	idx2, err := Open(dir)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	if got := idx2.Count(); got != 1 {
		t.Errorf("Count after reopen: got %d, want 1", got)
	}
	hits, err := idx2.Query(ctx, []float32{1, 0, 0, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Body != "persisted summary" {
		t.Errorf("expected persisted summary, got %#v", hits)
	}
}

func TestManifest_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 0 {
		t.Errorf("fresh manifest should be empty, got %#v", m.Entries)
	}

	m.Entries["entry-Q"] = EntryState{
		Hash:        "abc",
		Fingerprint: "test/m/4",
		ChunkIDs:    []string{"entry-Q#summary"},
	}
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}

	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Entries["entry-Q"].ChunkIDs, []string{"entry-Q#summary"}) {
		t.Errorf("round-trip mismatch: %#v", got.Entries["entry-Q"])
	}
}

func TestManifest_MismatchCount(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		Version: 1,
		Entries: map[string]EntryState{
			"a": {Fingerprint: "x"},
			"b": {Fingerprint: "x"},
			"c": {Fingerprint: "y"},
			"d": {Fingerprint: ""},
		},
	}
	if got := m.MismatchCount("x"); got != 2 {
		t.Errorf("got %d, want 2 (c and d differ from x)", got)
	}
	if got := m.MismatchCount(""); got != 0 {
		t.Errorf("empty current returns 0; got %d", got)
	}
}

func TestChunkIDHelpers(t *testing.T) {
	t.Parallel()
	if SummaryChunkID("e") != "e#summary" {
		t.Errorf("SummaryChunkID: got %q", SummaryChunkID("e"))
	}
	if BodyChunkID("e", 3) != "e#body-3" {
		t.Errorf("BodyChunkID: got %q", BodyChunkID("e", 3))
	}
	a1 := AttachmentChunkID("e", "design.md", 0)
	a2 := AttachmentChunkID("e", "other.md", 0)
	if a1 == a2 {
		t.Errorf("attachment chunk IDs should differ across attachment paths: %q == %q", a1, a2)
	}
	if got := HashContent("hello"); len(got) != 64 {
		t.Errorf("HashContent: expected 64 hex chars, got %d (%q)", len(got), got)
	}
}
