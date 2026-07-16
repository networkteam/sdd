package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
)

// These tests pin slice 2's version-aware freshness through the real public
// adapter: a legacy v1 store answers immediately, a changed entry adds a
// version (never overwriting), old-version hits are filtered at read time, and
// two branches sharing one store never flip-flop.

const counterFingerprint = "counter/v1"

func counterStoreDir(cacheRoot string) string {
	return index.StoreDir(cacheRoot, "counter/repo", counterFingerprint)
}

// entryStateHashOf computes an entry's state hash exactly as the application
// does — parse the file with model.ParseEntry (the snapshot loader's path),
// then chunking.EntryStateHash — so a seeded legacy manifest can record the
// hash that the running app will independently compute for the same entry.
func entryStateHashOf(t *testing.T, graphDir, id string) string {
	t.Helper()
	path := filepath.Join(graphDir, id[:4], id[4:6], id[6:]+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := model.ParseEntry(id+".md", string(data))
	if err != nil {
		t.Fatalf("ParseEntry(%s): %v", id, err)
	}
	hash, err := chunking.EntryStateHash(t.Context(), entry, chunking.DiskAttachmentReader{GraphDir: graphDir})
	if err != nil {
		t.Fatalf("EntryStateHash(%s): %v", id, err)
	}
	return hash
}

// entryVersionCount reports how many stored versions the manifest records for
// entryID (the shape GC and read-time filtering reason over).
func entryVersionCount(t *testing.T, cacheRoot, entryID string) int {
	t.Helper()
	manifest, err := index.LoadManifest(counterStoreDir(cacheRoot))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	return len(manifest.Entries[entryID].Versions)
}

// seedLegacyStore writes a v1-shaped store by hand: LEGACY (unversioned) chunk
// IDs, rows carrying NO entry_hash metadata, and a single-version manifest
// recording the given hash. This is exactly the on-disk shape a store built by
// a pre-slice-2 binary has, so the new code must answer it with no migration.
func seedLegacyStore(t *testing.T, cacheRoot, entryID, hash, summaryText, bodyText string) {
	t.Helper()
	dir := counterStoreDir(cacheRoot)
	summaryID := index.SummaryChunkID(entryID)
	bodyID := index.BodyChunkID(entryID, 0)
	rows := []index.Row{
		{EntryID: entryID, ChunkID: summaryID, Text: summaryText, Body: summaryText, IsSummary: true,
			ContentHash: index.HashContent(summaryText), ModelFingerprint: counterFingerprint, Embedding: keywordVec(summaryText)},
		{EntryID: entryID, ChunkID: bodyID, Text: bodyText, Body: bodyText,
			ContentHash: index.HashContent(bodyText), ModelFingerprint: counterFingerprint, Embedding: keywordVec(bodyText)},
	}
	err := index.WriteStore(t.Context(), dir, func(idx *index.Index) error {
		if err := idx.UpsertEntry(t.Context(), entryID, nil, rows); err != nil {
			return err
		}
		manifest, err := index.LoadManifest(dir)
		if err != nil {
			return err
		}
		// A single version serializes back in the legacy flat shape.
		manifest.AddVersion(entryID, index.EntryVersion{
			Hash: hash, Fingerprint: counterFingerprint, ChunkIDs: []string{summaryID, bodyID},
		})
		return manifest.Save(dir)
	})
	if err != nil {
		t.Fatalf("seedLegacyStore: %v", err)
	}
}

// TestVectorSearchLegacyStoreAnswersAndAccumulates is the hard compatibility
// guard: a store built with legacy chunk IDs and a legacy manifest answers
// immediately (no re-embed), and a changed entry then adds a new version.
func TestVectorSearchLegacyStoreAnswersAndAccumulates(t *testing.T) {
	graphDir := t.TempDir()
	cacheRoot := t.TempDir()
	const id = "20260101-100000-s-tac-aaa"
	writeCounterEntry(t, graphDir, id, "Alpha legacy summary", "## Section\nThe alpha entry legacy body.")

	// Seed a legacy store whose recorded hash matches the entry's current state.
	hash := entryStateHashOf(t, graphDir, id)
	seedLegacyStore(t, cacheRoot, id, hash, "Alpha legacy summary text", "The alpha legacy body text.")

	emb := &countingEmbeddings{}
	app := newCounterApp(t, graphDir, cacheRoot, emb)

	res := counterSearch(t, app, "alpha")
	if !strings.Contains(res.Results, "s-tac-aaa") {
		t.Fatalf("legacy store did not answer the search: %q", res.Results)
	}
	// The legacy-seeded entry must be recognized as present and NOT re-embedded
	// (the embedded base procedures the graph also carries are absent from this
	// hand-seeded store, so they embed on first touch — that is orthogonal to
	// the legacy-compat question, which is about the seeded entry's version).
	for _, docID := range emb.docInputIDs {
		if strings.HasPrefix(docID, id) {
			t.Errorf("legacy-seeded entry was re-embedded (chunk %q); presence check must see its legacy version", docID)
		}
	}
	// The answer must come from the seeded legacy rows.
	if !strings.Contains(res.Results, "legacy") {
		t.Errorf("legacy citation not surfaced: %q", res.Results)
	}

	// Change the entry (regenerate its summary): its current hash no longer
	// matches the legacy version, so the search adds a new version — only this
	// entry, and never deleting the legacy one.
	writeCounterEntry(t, graphDir, id, "Alpha refreshed summary", "## Section\nThe alpha entry legacy body.")
	emb.reset()
	res = counterSearch(t, app, "alpha")
	if emb.docEmbeds == 0 {
		t.Fatal("changed entry embedded nothing — a new version was not added")
	}
	for _, docID := range emb.docInputIDs {
		if !strings.HasPrefix(docID, id) {
			t.Errorf("embedded a chunk %q not belonging to the changed entry %q", docID, id)
		}
	}
	if got := entryVersionCount(t, cacheRoot, id); got != 2 {
		t.Errorf("entry has %d versions after a change, want 2 (legacy kept + new added)", got)
	}
	if !strings.Contains(res.Results, "s-tac-aaa") {
		t.Errorf("search lost the entry after the change: %q", res.Results)
	}
}

// TestVectorSearchSummaryRegenerationEmbedsOnceAndFiltersOldVersion covers the
// summary-regeneration scenario: the next search embeds exactly that entry once
// under a new version, and the stale version's hits are dropped at read time.
func TestVectorSearchSummaryRegenerationEmbedsOnceAndFiltersOldVersion(t *testing.T) {
	graphDir := t.TempDir()
	cacheRoot := t.TempDir()
	const id = "20260101-100000-s-tac-aaa"
	writeCounterEntry(t, graphDir, id, "Alpha OLDMARKER summary", "## Section\nThe alpha entry body text.")

	emb := &countingEmbeddings{}
	app := newCounterApp(t, graphDir, cacheRoot, emb)
	counterSearch(t, app, "alpha") // warm: embed the original version

	// Regenerate the summary only. The body is unchanged, but the entry-state
	// hash changes, so the whole entry re-embeds under a new version.
	writeCounterEntry(t, graphDir, id, "Alpha NEWMARKER summary", "## Section\nThe alpha entry body text.")
	emb.reset()
	res := counterSearch(t, app, "alpha")

	if emb.docEmbeds == 0 {
		t.Fatal("summary regeneration embedded nothing")
	}
	for _, docID := range emb.docInputIDs {
		if !strings.HasPrefix(docID, id) {
			t.Errorf("embedded a chunk %q not belonging to %q", docID, id)
		}
	}
	if got := entryVersionCount(t, cacheRoot, id); got != 2 {
		t.Errorf("entry has %d versions after regeneration, want 2", got)
	}
	// Read-time filtering: the current version's summary surfaces, the stale
	// one does not — even though its rows are still in the store.
	if !strings.Contains(res.Results, "NEWMARKER") {
		t.Errorf("current version summary not surfaced: %q", res.Results)
	}
	if strings.Contains(res.Results, "OLDMARKER") {
		t.Errorf("stale version hit leaked past the read-time filter: %q", res.Results)
	}
}

// TestVectorSearchBranchDivergenceNoFlipFlop covers two graph states holding
// different versions of one entry against one shared store: each search answers
// with its own version, and once both versions are stored, alternating searches
// re-embed nothing.
func TestVectorSearchBranchDivergenceNoFlipFlop(t *testing.T) {
	cacheRoot := t.TempDir()
	const id = "20260101-100000-s-tac-aaa"

	branchOne := t.TempDir()
	writeCounterEntry(t, branchOne, id, "Alpha BRANCHONE summary", "## Section\nThe alpha shared body.")
	branchTwo := t.TempDir()
	writeCounterEntry(t, branchTwo, id, "Alpha BRANCHTWO summary", "## Section\nThe alpha shared body.")

	emb1 := &countingEmbeddings{}
	app1 := newCounterApp(t, branchOne, cacheRoot, emb1)
	emb2 := &countingEmbeddings{}
	app2 := newCounterApp(t, branchTwo, cacheRoot, emb2)

	// First search on each branch embeds that branch's version.
	res1 := counterSearch(t, app1, "alpha")
	if emb1.docEmbeds == 0 {
		t.Fatal("branch one embedded nothing on first search")
	}
	res2 := counterSearch(t, app2, "alpha")
	if emb2.docEmbeds == 0 {
		t.Fatal("branch two embedded nothing on first search")
	}
	if got := entryVersionCount(t, cacheRoot, id); got != 2 {
		t.Fatalf("shared store holds %d versions of the entry, want 2 (one per branch)", got)
	}

	// Each branch sees its own version at read time.
	if !strings.Contains(res1.Results, "BRANCHONE") || strings.Contains(res1.Results, "BRANCHTWO") {
		t.Errorf("branch one saw the wrong version: %q", res1.Results)
	}
	if !strings.Contains(res2.Results, "BRANCHTWO") || strings.Contains(res2.Results, "BRANCHONE") {
		t.Errorf("branch two saw the wrong version: %q", res2.Results)
	}

	// Alternating searches now re-embed nothing — each branch's version is
	// already present, so there is no flip-flop delete-and-re-embed.
	emb1.reset()
	counterSearch(t, app1, "alpha")
	if emb1.docEmbeds != 0 {
		t.Errorf("branch one re-embedded %d documents on a repeat search; want 0 (flip-flop)", emb1.docEmbeds)
	}
	emb2.reset()
	counterSearch(t, app2, "alpha")
	if emb2.docEmbeds != 0 {
		t.Errorf("branch two re-embedded %d documents on a repeat search; want 0 (flip-flop)", emb2.docEmbeds)
	}
}
