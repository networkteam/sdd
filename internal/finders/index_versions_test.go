package finders_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func TestIndexVersionsReport(t *testing.T) {
	t.Parallel()
	graphDir := t.TempDir()
	indexDir := t.TempDir()
	const id = "20260101-100000-s-tac-aaa"
	entryDir := filepath.Join(graphDir, "2026", "01")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := "---\ntype: signal\nlayer: tactical\nkind: gap\nconfidence: medium\nparticipants:\n  - Test\nsummary: |-\n  A summary.\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(entryDir, "01-100000-s-tac-aaa.md"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}

	f := finders.New(finders.Options{Config: &model.PerRepoConfig{}})
	g, err := f.CurrentGraph(graphDir)
	if err != nil {
		t.Fatal(err)
	}
	var target *model.Entry
	for _, e := range g.Entries {
		if e.ID == id {
			target = e
		}
	}
	if target == nil {
		t.Fatalf("entry %s not loaded", id)
	}
	current, err := chunking.EntryStateHash(context.Background(), target, chunking.DiskAttachmentReader{GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}

	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows := []index.Row{
		{EntryID: id, EntryHash: current, ChunkID: id + "#v-" + current + "#summary", Text: "a", Body: "a", IsSummary: true, ModelFingerprint: "fp", Embedding: []float32{1, 0}},
		{EntryID: id, EntryHash: "old", ChunkID: id + "#v-abcdef01#summary", Text: "b", Body: "b", IsSummary: true, ModelFingerprint: "fp", Embedding: []float32{0, 1}},
	}
	err = index.WriteStore(context.Background(), indexDir, func(idx *index.Index) error {
		if err := idx.UpsertEntry(context.Background(), id, nil, rows); err != nil {
			return err
		}
		m, err := index.LoadManifest(indexDir)
		if err != nil {
			return err
		}
		m.AddVersion(id, index.EntryVersion{Hash: current, Fingerprint: "fp", Derivation: index.DerivationCurrent, ChunkIDs: []string{rows[0].ChunkID}, IndexedAt: t0.Add(time.Hour)})
		m.AddVersion(id, index.EntryVersion{Hash: "old", Fingerprint: "fp", ChunkIDs: []string{rows[1].ChunkID}, IndexedAt: t0})
		return m.Save(indexDir)
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := f.IndexVersions(context.Background(), query.IndexVersionsQuery{Label: "local", GraphDir: graphDir, IndexDir: indexDir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Label != "local" || result.Entries != 1 || result.Bytes == 0 {
		t.Errorf("result header = %+v", result)
	}
	if len(result.Groups) != 2 {
		t.Fatalf("groups = %+v, want current and v0", result.Groups)
	}
	if g := result.Groups[0]; g.Name != index.GroupCurrent || g.Droppable || g.Versions != 1 || g.Bytes == 0 || !g.Newest.Equal(t0.Add(time.Hour)) {
		t.Errorf("current group = %+v", g)
	}
	if g := result.Groups[1]; g.Name != index.GroupPreDerivation || !g.Droppable || g.Versions != 1 || g.Bytes == 0 || !g.Oldest.Equal(t0) {
		t.Errorf("v0 group = %+v", g)
	}
}
