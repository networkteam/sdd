package index

import "testing"

func TestManifest_PendingCount(t *testing.T) {
	m := &Manifest{Version: 1, Entries: map[string]EntryState{
		"a": {Fingerprint: "fp1"},
		"b": {Fingerprint: "fp1"},
		"c": {Fingerprint: "fp-old"}, // recorded under a stale embedder
	}}

	ids := []string{"a", "b", "c", "d"} // d is on disk but never indexed

	// Under the current fingerprint: c is stale, d is missing → 2 pending.
	if got := m.PendingCount(ids, "fp1"); got != 2 {
		t.Errorf("PendingCount = %d, want 2 (stale c + missing d)", got)
	}

	// A fresh fingerprint makes every recorded entry stale; d still missing.
	if got := m.PendingCount(ids, "fp2"); got != 4 {
		t.Errorf("PendingCount under new fingerprint = %d, want 4 (all)", got)
	}

	// Nothing requested → nothing pending.
	if got := m.PendingCount(nil, "fp1"); got != 0 {
		t.Errorf("PendingCount(nil) = %d, want 0", got)
	}
}
