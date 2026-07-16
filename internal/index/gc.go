package index

import "time"

// VersionRetention is how long a stored entry version that is no longer any
// current version survives before version GC may drop it. The window protects
// recent branch work: a version indexed within it is kept even when the
// collecting writer's graph no longer holds it, so switching back to that
// branch shortly after does not re-embed. 14 days balances bounded store growth
// against the cost of re-embedding a branch revisited after a break.
//
// It is a fixed constant, not configuration: no existing config surface fits a
// vector-store retention knob, and inventing one for a self-healing
// derived-data cleanup is not worth the maintenance surface. The re-embed after
// collection is bounded and self-healing (vectors are derived data), so the
// exact value is not load-bearing.
const VersionRetention = 14 * 24 * time.Hour

// CollectStaleVersions drops every stored version that is neither a current
// version (its entry's hash appears in currentHashes) nor indexed within the
// retention window, and returns the chunk IDs whose rows the caller must delete
// from the index. It mutates the manifest in place — an entry left with no
// surviving version is removed entirely — but performs no I/O: the delete and
// the manifest save are the write session's job (the sole sanctioned delete
// paths are this GC under the write lock and an explicit force rebuild).
//
// currentHashes maps entry ID to the collecting writer's current state hash. An
// entry absent from it (removed from the writer's graph, or one whose hash
// could not be computed) has no current version, so only the retention window
// protects its versions.
func (m *Manifest) CollectStaleVersions(currentHashes map[string]string, now time.Time, retention time.Duration) []string {
	var dropped []string
	for id, state := range m.Entries {
		current := currentHashes[id]
		kept := state.Versions[:0:0]
		for _, v := range state.Versions {
			isCurrent := current != "" && v.Hash == current
			isRecent := now.Sub(v.IndexedAt) < retention
			if isCurrent || isRecent {
				kept = append(kept, v)
				continue
			}
			dropped = append(dropped, v.ChunkIDs...)
		}
		if len(kept) == 0 {
			delete(m.Entries, id)
			continue
		}
		if len(kept) != len(state.Versions) {
			m.Entries[id] = EntryState{Versions: kept}
		}
	}
	return dropped
}
