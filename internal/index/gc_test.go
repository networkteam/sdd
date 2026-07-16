package index

import (
	"slices"
	"sort"
	"testing"
	"time"
)

func TestCollectStaleVersions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	old := now.Add(-20 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)

	m := &Manifest{Version: 1, Entries: map[string]EntryState{
		// entryA: current version is OLD (must survive because it is current);
		// a recent non-current version survives on retention; a stale
		// non-current version is dropped.
		"entryA": {Versions: []EntryVersion{
			{Hash: "A-current", IndexedAt: old, ChunkIDs: []string{"A-current#1"}},
			{Hash: "A-recent", IndexedAt: recent, ChunkIDs: []string{"A-recent#1"}},
			{Hash: "A-stale", IndexedAt: old, ChunkIDs: []string{"A-stale#1", "A-stale#2"}},
		}},
		// entryB is absent from the writer's graph (removed): with every version
		// stale, it is collected entirely.
		"entryB": {Versions: []EntryVersion{
			{Hash: "B-old", IndexedAt: old, ChunkIDs: []string{"B-old#1"}},
		}},
	}}
	currentHashes := map[string]string{"entryA": "A-current"}

	dropped := m.CollectStaleVersions(currentHashes, now, VersionRetention)
	sort.Strings(dropped)
	want := []string{"A-stale#1", "A-stale#2", "B-old#1"}
	if !slices.Equal(dropped, want) {
		t.Errorf("dropped = %v, want %v", dropped, want)
	}

	a := m.Entries["entryA"]
	if len(a.Versions) != 2 || !hasHash(a, "A-current") || !hasHash(a, "A-recent") {
		t.Errorf("entryA versions = %+v, want A-current + A-recent kept", a.Versions)
	}
	if hasHash(a, "A-stale") {
		t.Error("stale non-current version survived collection")
	}
	if _, ok := m.Entries["entryB"]; ok {
		t.Error("entryB (no surviving version) was not removed from the manifest")
	}
}

func hasHash(s EntryState, hash string) bool {
	for _, v := range s.Versions {
		if v.Hash == hash {
			return true
		}
	}
	return false
}

func TestCollectStaleVersionsKeepsRecentNonCurrent(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	// A branch-divergence shape: one entry, two recent versions, neither is the
	// collecting writer's current version. Retention protects both.
	m := &Manifest{Version: 1, Entries: map[string]EntryState{
		"e": {Versions: []EntryVersion{
			{Hash: "h1", IndexedAt: now.Add(-2 * time.Hour), ChunkIDs: []string{"e#h1"}},
			{Hash: "h2", IndexedAt: now.Add(-1 * time.Hour), ChunkIDs: []string{"e#h2"}},
		}},
	}}
	dropped := m.CollectStaleVersions(map[string]string{"e": "h3"}, now, VersionRetention)
	if len(dropped) != 0 {
		t.Errorf("dropped %v; recent versions must survive even when not current", dropped)
	}
	if len(m.Entries["e"].Versions) != 2 {
		t.Errorf("recent versions were collected: %+v", m.Entries["e"].Versions)
	}
}
