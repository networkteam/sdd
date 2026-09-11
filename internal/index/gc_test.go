package index_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/index"
)

const (
	fullHash  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	otherHash = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

// mixedManifest holds one entry with a version from every rule plus a removed
// entry: the store two binary generations and a branch leave behind.
func mixedManifest(t0 time.Time) *index.Manifest {
	return &index.Manifest{Version: 1, Entries: map[string]index.EntryState{
		"e1": {Versions: []index.EntryVersion{
			{Hash: fullHash, Derivation: index.DerivationCurrent, ChunkIDs: []string{"e1#v-" + fullHash + "#summary"}, IndexedAt: t0.Add(3 * time.Hour)},
			{Hash: otherHash, ChunkIDs: []string{"e1#v-" + otherHash + "#summary", "e1#v-" + otherHash + "#body-0"}, IndexedAt: t0.Add(2 * time.Hour)},
			{Hash: "old1", ChunkIDs: []string{"e1#v-abcdef01#summary"}, IndexedAt: t0},
			{Hash: "legacy1", ChunkIDs: []string{"e1#summary", "e1#body-0"}, IndexedAt: t0.Add(-time.Hour)},
		}},
		"gone": {Versions: []index.EntryVersion{
			{Hash: "g", ChunkIDs: []string{"gone#v-" + fullHash + "#summary"}, IndexedAt: t0.Add(time.Hour)},
		}},
	}}
}

func TestVersionGroups(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	m := mixedManifest(t0)
	groups := m.VersionGroups(map[string]string{"e1": fullHash})

	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	want := []string{index.GroupCurrent, index.GroupBranch, index.GroupLegacy, index.GroupPreDerivation}
	if !slices.Equal(names, want) {
		t.Fatalf("group order = %v, want %v", names, want)
	}

	byName := map[string]index.VersionGroup{}
	for _, g := range groups {
		byName[g.Name] = g
	}
	current := byName[index.GroupCurrent]
	if current.Droppable || current.Versions != 1 || current.Entries != 1 {
		t.Errorf("current group = %+v, want 1 undroppable version", current)
	}
	branch := byName[index.GroupBranch]
	if !branch.Droppable || branch.Versions != 2 || branch.Entries != 2 || len(branch.ChunkIDs) != 3 {
		t.Errorf("branch group = %+v, want 2 versions over 2 entries with 3 chunks", branch)
	}
	if !branch.Oldest.Equal(t0.Add(time.Hour)) || !branch.Newest.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("branch age range = %v..%v", branch.Oldest, branch.Newest)
	}
	if v0 := byName[index.GroupPreDerivation]; v0.Versions != 1 || !slices.Equal(v0.ChunkIDs, []string{"e1#v-abcdef01#summary"}) {
		t.Errorf("v0 group = %+v", v0)
	}
	if legacy := byName[index.GroupLegacy]; legacy.Versions != 1 || len(legacy.ChunkIDs) != 2 {
		t.Errorf("legacy group = %+v", legacy)
	}
}

func TestVersionGroupsZeroChunkVersionIsCurrentRule(t *testing.T) {
	t.Parallel()
	m := &index.Manifest{Version: 1, Entries: map[string]index.EntryState{
		"empty": {Versions: []index.EntryVersion{{Hash: "h"}}},
	}}
	groups := m.VersionGroups(map[string]string{"empty": "h"})
	if len(groups) != 1 || groups[0].Name != index.GroupCurrent {
		t.Fatalf("groups = %+v, want one current group", groups)
	}
}

func TestSelectGroups(t *testing.T) {
	t.Parallel()
	m := mixedManifest(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	current := map[string]string{"e1": fullHash}

	stale, missing, err := m.SelectGroups([]string{index.GroupStale}, current)
	if err != nil || len(missing) != 0 {
		t.Fatal(err, missing)
	}
	if want := []string{index.GroupLegacy, index.GroupPreDerivation, index.GroupBranch}; !slices.Equal(stale, want) {
		t.Errorf("stale = %v, want %v", stale, want)
	}

	one, _, err := m.SelectGroups([]string{index.GroupPreDerivation, index.GroupPreDerivation}, current)
	if err != nil || !slices.Equal(one, []string{index.GroupPreDerivation}) {
		t.Errorf("named selection = %v, %v", one, err)
	}

	if _, _, err := m.SelectGroups([]string{index.GroupCurrent}, current); err == nil {
		t.Error("selecting the current group must fail")
	}
	selected, missing, err := m.SelectGroups([]string{"v7", index.GroupPreDerivation}, current)
	if err != nil || !slices.Equal(selected, []string{index.GroupPreDerivation}) || !slices.Equal(missing, []string{"v7"}) {
		t.Errorf("absent group: selected %v, missing %v, err %v; want the present one selected and v7 reported missing", selected, missing, err)
	}
}

func TestDropGroups(t *testing.T) {
	t.Parallel()
	m := mixedManifest(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	current := map[string]string{"e1": fullHash}

	chunkIDs, versions := m.DropGroups([]string{index.GroupPreDerivation, index.GroupBranch}, current)
	if versions != 3 {
		t.Errorf("versions dropped = %d, want 3", versions)
	}
	slices.Sort(chunkIDs)
	want := []string{"e1#v-" + otherHash + "#body-0", "e1#v-" + otherHash + "#summary", "e1#v-abcdef01#summary", "gone#v-" + fullHash + "#summary"}
	slices.Sort(want)
	if !slices.Equal(chunkIDs, want) {
		t.Errorf("chunk IDs = %v, want %v", chunkIDs, want)
	}
	if _, ok := m.Entries["gone"]; ok {
		t.Error("entry with no surviving version stayed in the manifest")
	}
	if left := m.Entries["e1"].Versions; len(left) != 2 || left[0].Hash != fullHash || left[1].Hash != "legacy1" {
		t.Errorf("e1 versions after drop = %+v, want current and legacy", left)
	}
}

func TestSetDerivationVersionKeepsOtherRules(t *testing.T) {
	t.Parallel()
	m := mixedManifest(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if got := m.Entries["e1"].ChunkIDsOf(index.DerivationCurrent); len(got) != 3 {
		t.Fatalf("current-rule chunk IDs = %v, want 3", got)
	}
	fresh := index.EntryVersion{Hash: "new", Derivation: index.DerivationCurrent, ChunkIDs: []string{"e1#v-new#summary"}}
	m.SetDerivationVersion("e1", fresh)
	var hashes []string
	for _, v := range m.Entries["e1"].Versions {
		hashes = append(hashes, v.Hash)
	}
	if want := []string{"old1", "legacy1", "new"}; !slices.Equal(hashes, want) {
		t.Errorf("versions after force = %v, want %v", hashes, want)
	}
}

func TestDocumentsSizeMatchesStoredRows(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	row := index.Row{EntryID: "e1", EntryHash: "h", ChunkID: "e1#v-h#summary", Text: "alpha", Body: "alpha", IsSummary: true, ModelFingerprint: "fp", Embedding: []float32{1, 0}}
	err := index.WriteStore(context.Background(), dir, func(idx *index.Index) error {
		return idx.UpsertEntry(context.Background(), "e1", nil, []index.Row{row})
	})
	if err != nil {
		t.Fatal(err)
	}
	if size := index.DocumentsSize(dir, []string{row.ChunkID}); size == 0 {
		t.Error("stored row sized at 0 bytes: document path layout drifted from chromem's")
	}
	if size := index.DocumentsSize(dir, []string{"e1#v-missing#summary"}); size != 0 {
		t.Errorf("missing row sized at %d bytes, want 0", size)
	}
	total, err := index.StoreSize(dir)
	if err != nil || total == 0 {
		t.Errorf("StoreSize = %d, %v", total, err)
	}
}
