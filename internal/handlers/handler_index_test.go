package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

// withoutEmbedded filters the embedded base-procedure IDs out of a callback
// ID list — the graph load merges them in, and they get indexed like any
// entry, but fixture assertions count only the on-disk project entries.
func withoutEmbedded(t *testing.T, ids []string) []string {
	t.Helper()
	// Exclude every embedded entry the loader merges — base procedures and
	// base facts alike — so counts track project entries as the embedded set
	// grows. Uses the same assembly production does (finders.BaseEntries).
	base, err := finders.BaseEntries()
	if err != nil {
		t.Fatal(err)
	}
	embedded := make(map[string]bool, len(base))
	for _, e := range base {
		embedded[e.ID] = true
	}
	var project []string
	for _, id := range ids {
		if !embedded[id] {
			project = append(project, id)
		}
	}
	return project
}

// fakeEmbedder produces deterministic 4-dim embeddings keyed off the
// SHA-256 of the input. Stable across runs; usable for similarity tests
// because identical inputs always produce identical vectors.
type fakeEmbedder struct {
	calls       int
	totalInputs int
	fingerprint string
}

func (f *fakeEmbedder) Embed(_ context.Context, req embed.Request) (embed.Result, error) {
	f.calls++
	f.totalInputs += len(req.Texts)
	out := make([][]float32, len(req.Texts))
	for i, t := range req.Texts {
		h := sha256.Sum256([]byte(t))
		v := make([]float32, 4)
		for j := 0; j < 4; j++ {
			b := binary.BigEndian.Uint32(h[j*4 : j*4+4])
			v[j] = float32(b) / float32(^uint32(0))
		}
		out[i] = v
	}
	return embed.Result{Vectors: out}, nil
}

// fakeBatchSize is sized to hold the whole shipped base-procedure/base-fact set
// plus these tests' small project fixtures in a single embed round-trip, so the
// batch-count assertions stay stable as the base set grows (e.g. adding the
// bootstrap procedure). Raise it if the embedded set ever outgrows this.
const fakeBatchSize = 128

func indexEmbedder(f *fakeEmbedder) IndexEmbedder {
	return IndexEmbedder{Embedder: f, BatchSize: fakeBatchSize}
}
func (f *fakeEmbedder) Fingerprint() string {
	if f.fingerprint == "" {
		return "fake/v1/4"
	}
	return f.fingerprint
}

// readFinderFor builds a Finder the IndexHandler can use as Reader. It
// only needs LoadGraph to work; preflight stays nil and would error on
// invocation, but indexing never calls it.
func readFinderFor(t *testing.T) *finders.Finder {
	t.Helper()
	return finders.New(finders.Options{
		PreflightRunner: noopRunner{},
		Config:          &model.PerRepoConfig{},
	})
}

type noopRunner struct{}

func (noopRunner) Run(context.Context, llm.Request) (llm.Result, error) {
	return llm.Result{}, fmt.Errorf("no llm runner configured")
}

func writeEntry(t *testing.T, graphDir, id, body, summary string) {
	t.Helper()
	yyyy := id[:4]
	mm := id[4:6]
	short := id[6:]
	dir := filepath.Join(graphDir, yyyy, mm)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, short+".md")
	content := "---\ntype: signal\nlayer: tactical\nkind: gap\n"
	content += "confidence: medium\nparticipants:\n  - Test\n"
	if summary != "" {
		content += "summary: |-\n  " + summary + "\n"
	}
	content += "---\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIndexHandler_Build(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "## Section A\nFirst entry body.", "Summary of A.")
	writeEntry(t, graphDir, "20260101-100001-s-tac-bbb", "## Section B\nSecond entry body.", "Summary of B.")

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: indexEmbedder(emb),
		Reader:   readFinderFor(t),
	})

	var indexed []string
	chunksByID := map[string]int{}
	cmd := &command.BuildIndexCmd{
		OnEntryIndexed: func(id string, n int) { indexed = append(indexed, id); chunksByID[id] = n },
		OnEntrySkipped: func(id string) { t.Errorf("unexpected skip for %s", id) },
	}
	if err := h.Build(context.Background(), cmd); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := withoutEmbedded(t, indexed); len(got) != 2 {
		t.Errorf("expected 2 project entries indexed, got %d (%v)", len(got), got)
	}
	wantCalls := (emb.totalInputs + fakeBatchSize - 1) / fakeBatchSize
	if emb.calls != wantCalls {
		t.Errorf("batched embed calls = %d, want %d for %d inputs", emb.calls, wantCalls, emb.totalInputs)
	}
	// Each project entry: 1 summary + 1 body = 2 chunks.
	for _, id := range withoutEmbedded(t, indexed) {
		if chunksByID[id] != 2 {
			t.Errorf("entry %s: expected 2 chunks, got %d", id, chunksByID[id])
		}
	}
	// Manifest should record both project entries.
	manifest, err := index.LoadManifest(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"20260101-100000-s-tac-aaa", "20260101-100001-s-tac-bbb"} {
		if _, ok := manifest.Entries[id]; !ok {
			t.Errorf("manifest is missing entry %s", id)
		}
	}
}

func TestIndexHandler_BuildFiresOnBatchStart(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "## Section A\nFirst entry body.", "Summary of A.")
	writeEntry(t, graphDir, "20260101-100001-s-tac-bbb", "## Section B\nSecond entry body.", "Summary of B.")

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: indexEmbedder(emb),
		Reader:   readFinderFor(t),
	})

	var batches [][]string
	var batchChunks []int
	plannedEntries := -1
	indexedChunks := 0
	indexedEntries := 0
	cmd := &command.BuildIndexCmd{
		OnPlanned: func(total int) { plannedEntries = total },
		OnBatchStart: func(ids []string, chunks int) {
			batches = append(batches, ids)
			batchChunks = append(batchChunks, chunks)
		},
		OnEntryIndexed: func(_ string, chunks int) { indexedChunks += chunks; indexedEntries++ },
	}
	if err := h.Build(context.Background(), cmd); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(batches) != emb.calls {
		t.Fatalf("batch callbacks = %d, embed calls = %d", len(batches), emb.calls)
	}
	var batchedIDs []string
	var announcedChunks int
	for i, ids := range batches {
		batchedIDs = append(batchedIDs, ids...)
		announcedChunks += batchChunks[i]
	}
	if got := withoutEmbedded(t, batchedIDs); len(got) != 2 {
		t.Errorf("batches carried %d project entry IDs, want 2 (%v)", len(got), batchedIDs)
	}
	if plannedEntries <= 0 || plannedEntries != indexedEntries {
		t.Errorf("planned %d entries, published %d", plannedEntries, indexedEntries)
	}
	if announcedChunks != indexedChunks {
		t.Errorf("announced %d chunks, published %d", announcedChunks, indexedChunks)
	}

}

func TestIndexHandler_BuildSkipsUnchanged(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "body", "summary")

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: indexEmbedder(emb),
		Reader:   readFinderFor(t),
	})

	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	firstCalls := emb.calls

	var skipped []string
	cmd := &command.BuildIndexCmd{
		OnEntrySkipped: func(id string) { skipped = append(skipped, id) },
		OnEntryIndexed: func(id string, n int) { t.Errorf("unexpected re-index of %s", id) },
	}
	if err := h.Build(context.Background(), cmd); err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if got := withoutEmbedded(t, skipped); len(got) != 1 {
		t.Errorf("expected 1 project-entry skip on second build, got %d (%v)", len(got), skipped)
	}
	if emb.calls != firstCalls {
		t.Errorf("second build should not call embedder when nothing changed (got %d additional calls)", emb.calls-firstCalls)
	}
}

func TestIndexHandler_BuildForceReindexes(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "body", "summary")

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: indexEmbedder(emb),
		Reader:   readFinderFor(t),
	})

	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	firstCalls := emb.calls

	if err := h.Build(context.Background(), &command.BuildIndexCmd{Force: true}); err != nil {
		t.Fatalf("forced Build: %v", err)
	}
	if emb.calls != firstCalls*2 {
		t.Errorf("Force=true should repeat every embed batch; got %d additional calls, want %d", emb.calls-firstCalls, firstCalls)
	}
}

func TestIndexHandler_LazyFillCoversMissingEntries(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "body A", "summary A")

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: indexEmbedder(emb),
		Reader:   readFinderFor(t),
	})

	// First build covers the existing entry.
	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	calls := emb.calls

	// Add a new entry. LazyFill should pick it up.
	writeEntry(t, graphDir, "20260101-100001-s-tac-bbb", "body B", "summary B")

	var indexed []string
	cmd := &command.LazyFillIndexCmd{
		OnEntryIndexed: func(id string, n int) { indexed = append(indexed, id) },
	}
	if err := h.LazyFill(context.Background(), cmd); err != nil {
		t.Fatalf("LazyFill: %v", err)
	}
	if len(indexed) != 1 || indexed[0] != "20260101-100001-s-tac-bbb" {
		t.Errorf("LazyFill should re-index only the new entry, got %v", indexed)
	}
	if emb.calls != calls+1 {
		t.Errorf("expected one new embed call from LazyFill, got %d additional", emb.calls-calls)
	}
}

func TestIndexHandler_BuildPicksUpEntryEdits(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	id := "20260101-100000-s-tac-aaa"
	writeEntry(t, graphDir, id, "old body", "old summary")

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: indexEmbedder(emb),
		Reader:   readFinderFor(t),
	})
	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	calls := emb.calls

	// Edit the entry's body. (In practice entries are immutable, but the
	// summary regen + manual fix paths can change file bytes; the
	// indexer must follow.)
	writeEntry(t, graphDir, id, "new body", "old summary")

	var indexed []string
	cmd := &command.BuildIndexCmd{
		OnEntryIndexed: func(eid string, n int) { indexed = append(indexed, eid) },
	}
	if err := h.Build(context.Background(), cmd); err != nil {
		t.Fatalf("Build after edit: %v", err)
	}
	if len(indexed) != 1 {
		t.Errorf("expected re-index of edited entry, got %v", indexed)
	}
	if emb.calls != calls+1 {
		t.Errorf("expected one new embed call after edit, got %d additional", emb.calls-calls)
	}
}

// A changed entry accumulates a version on the write path and nothing collects
// it: versions leave the store only through DropVersions (d-tac-c9c).
func TestIndexHandler_WritesNeverDeleteVersions(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()
	id := "20260101-100000-s-tac-aaa"
	writeEntry(t, graphDir, id, "body", "old summary")

	clock := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: indexEmbedder(emb),
		Reader:   readFinderFor(t),
		Now:      func() time.Time { return clock },
	})

	if err := h.LazyFill(context.Background(), &command.LazyFillIndexCmd{}); err != nil {
		t.Fatalf("first fill: %v", err)
	}
	writeEntry(t, graphDir, id, "body", "new summary")
	clock = clock.Add(90 * 24 * time.Hour)
	if err := h.LazyFill(context.Background(), &command.LazyFillIndexCmd{}); err != nil {
		t.Fatalf("second fill: %v", err)
	}

	manifest, err := index.LoadManifest(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	versions := manifest.Entries[id].Versions
	if len(versions) != 2 {
		t.Fatalf("entry has %d versions after an edit, want 2 (nothing collects)", len(versions))
	}
	for _, v := range versions {
		if v.Derivation != index.DerivationCurrent {
			t.Errorf("version %s recorded derivation %q, want %q", v.Hash, v.Derivation, index.DerivationCurrent)
		}
	}
}

// seedForeignVersion stores a row and manifest version the way a binary of a
// previous release would: an eight-character version segment and no derivation
// record.
func seedForeignVersion(t *testing.T, indexDir, entryID string) {
	t.Helper()
	chunkID := entryID + "#v-abcdef01#summary"
	err := index.WriteStore(context.Background(), indexDir, func(idx *index.Index) error {
		manifest, err := index.LoadManifest(indexDir)
		if err != nil {
			return err
		}
		row := index.Row{EntryID: entryID, EntryHash: "foreign", ChunkID: chunkID, Text: "old", Body: "old", IsSummary: true, ModelFingerprint: "fake/v1/4", Embedding: []float32{1, 0, 0, 0}}
		if err := idx.UpsertEntry(context.Background(), entryID, nil, []index.Row{row}); err != nil {
			return err
		}
		manifest.AddVersion(entryID, index.EntryVersion{Hash: "foreign", Fingerprint: "fake/v1/4", ChunkIDs: []string{chunkID}, IndexedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)})
		return manifest.Save(indexDir)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, indexDir string) int {
	t.Helper()
	var n int
	err := index.ReadStore(context.Background(), indexDir, func(idx *index.Index) error {
		n = idx.Count()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// A force rebuild replaces only this binary's own derivation rule; a version
// another release wrote into the shared store survives, rows included.
func TestIndexHandler_ForceKeepsOtherDerivations(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()
	id := "20260101-100000-s-tac-aaa"
	writeEntry(t, graphDir, id, "body", "summary")
	seedForeignVersion(t, indexDir, id)

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{GraphDir: graphDir, IndexDir: indexDir, Embedder: indexEmbedder(emb), Reader: readFinderFor(t)})
	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	rowsBefore := countRows(t, indexDir)
	if err := h.Build(context.Background(), &command.BuildIndexCmd{Force: true}); err != nil {
		t.Fatalf("force build: %v", err)
	}

	manifest, err := index.LoadManifest(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	versions := manifest.Entries[id].Versions
	if len(versions) != 2 {
		t.Fatalf("entry has %d versions after force, want the foreign one plus the rebuilt one", len(versions))
	}
	if versions[0].Hash != "foreign" || versions[1].Derivation != index.DerivationCurrent {
		t.Errorf("versions after force = %+v", versions)
	}
	if rows := countRows(t, indexDir); rows != rowsBefore {
		t.Errorf("row count after force = %d, want %d (force replaces, never shrinks another rule)", rows, rowsBefore)
	}
}

func TestIndexHandler_DropVersions(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()
	id := "20260101-100000-s-tac-aaa"
	writeEntry(t, graphDir, id, "body", "summary")
	seedForeignVersion(t, indexDir, id)

	emb := &fakeEmbedder{}
	h := NewIndexHandler(IndexHandlerOptions{GraphDir: graphDir, IndexDir: indexDir, Embedder: indexEmbedder(emb), Reader: readFinderFor(t)})
	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	rowsBefore := countRows(t, indexDir)

	err := h.DropVersions(context.Background(), &command.DropIndexVersionsCmd{Groups: []string{index.GroupCurrent}})
	if err == nil {
		t.Fatal("dropping the current group must fail")
	}
	if rows := countRows(t, indexDir); rows != rowsBefore {
		t.Fatalf("a refused drop deleted rows: %d, want %d", rows, rowsBefore)
	}

	var got command.DroppedIndexVersions
	err = h.DropVersions(context.Background(), &command.DropIndexVersionsCmd{
		Groups:    []string{index.GroupPreDerivation, "v7"},
		OnDropped: func(d command.DroppedIndexVersions) { got = d },
	})
	if err != nil {
		t.Fatalf("drop v0: %v", err)
	}
	if got.Versions != 1 || got.Chunks != 1 || got.Bytes == 0 {
		t.Errorf("OnDropped = %+v, want 1 version, 1 chunk, >0 bytes", got)
	}
	if len(got.Missing) != 1 || got.Missing[0] != "v7" {
		t.Errorf("missing groups = %v, want the absent v7 reported, not an error", got.Missing)
	}
	if rows := countRows(t, indexDir); rows != rowsBefore-1 {
		t.Errorf("row count after drop = %d, want %d", rows, rowsBefore-1)
	}
	manifest, err := index.LoadManifest(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	if versions := manifest.Entries[id].Versions; len(versions) != 1 || versions[0].Derivation != index.DerivationCurrent {
		t.Errorf("versions after drop = %+v, want only the current one", versions)
	}

	// Nothing stale remains: the umbrella selector is a no-op, not an error.
	err = h.DropVersions(context.Background(), &command.DropIndexVersionsCmd{
		Groups:    []string{index.GroupStale},
		OnDropped: func(d command.DroppedIndexVersions) { got = d },
	})
	if err != nil || got.Versions != 0 || got.Chunks != 0 {
		t.Errorf("stale drop on a clean store = (%+v, %v), want a no-op", got, err)
	}
}

// Ensure the placeholder _ avoids "imported and not used" if the index
// reader-only contract drifts in the future. Using query.PreflightQuery
// here to assert the package import in a test surface.
var _ = query.PreflightQuery{}
