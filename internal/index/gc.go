package index

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// DerivationCurrent names the entry-derivation rule this binary hashes and
// chunks under (the prefix of chunking.EntryStateHash). Every version written
// records it, so binaries with different rules sharing one store keep their
// versions apart instead of collecting each other's (d-tac-c9c). Bump it when a
// fixed derivation rule changes.
const DerivationCurrent = "v1"

// Version groups as `sdd index gc` reports and drops them. A version's group
// is its derivation rule, except that the current rule splits by whether the
// version is current for the collecting checkout.
const (
	// GroupCurrent holds the versions the checkout's search reads. Never
	// droppable.
	GroupCurrent = DerivationCurrent + "-current"
	// GroupBranch holds current-rule versions no entry of the checkout has: other
	// branches' states, edited entries, removed entries.
	GroupBranch = DerivationCurrent + "-branch"
	// GroupPreDerivation is the rule before derivation was recorded, recognised
	// by its eight-character version segment in chunk IDs.
	GroupPreDerivation = "v0"
	// GroupLegacy is the single-version store's unversioned chunk IDs.
	GroupLegacy = "legacy"
	// GroupStale selects every droppable group at once.
	GroupStale = "stale"
)

// derivation returns the rule a version was written under: the recorded field
// when present, else inferred from its chunk-ID shape. A zero-chunk version
// without a record is current-rule: only per-entry publication wrote those.
func (v EntryVersion) derivation() string {
	if v.Derivation != "" {
		return v.Derivation
	}
	if len(v.ChunkIDs) == 0 {
		return DerivationCurrent
	}
	_, rest, ok := strings.Cut(v.ChunkIDs[0], "#v-")
	if !ok {
		return GroupLegacy
	}
	if segment, _, _ := strings.Cut(rest, "#"); len(segment) == 8 {
		return GroupPreDerivation
	}
	return DerivationCurrent
}

// ChunkIDsOf returns the chunk IDs of the entry's versions under the given
// derivation rule — what a force rebuild replaces, leaving other rules' rows
// in place.
func (s EntryState) ChunkIDsOf(derivation string) []string {
	var out []string
	for _, v := range s.Versions {
		if v.derivation() == derivation {
			out = append(out, v.ChunkIDs...)
		}
	}
	return out
}

// SetDerivationVersion replaces every version of v's derivation rule with v,
// keeping versions written under other rules. The force rebuild path uses it
// after deleting the replaced versions' rows (see EntryState.ChunkIDsOf).
func (m *Manifest) SetDerivationVersion(entryID string, v EntryVersion) {
	if m.Entries == nil {
		m.Entries = map[string]EntryState{}
	}
	state := m.Entries[entryID]
	kept := state.Versions[:0:0]
	for _, existing := range state.Versions {
		if existing.derivation() != v.derivation() {
			kept = append(kept, existing)
		}
	}
	m.Entries[entryID] = EntryState{Versions: append(kept, v)}
}

// VersionGroup is one group of stored versions in the `sdd index gc` report.
type VersionGroup struct {
	Name     string
	Versions int
	Entries  int
	Oldest   time.Time
	Newest   time.Time
	ChunkIDs []string
	// Droppable is false only for GroupCurrent.
	Droppable bool
}

func (m *Manifest) groupOf(entryID string, v EntryVersion, currentHashes map[string]string) string {
	d := v.derivation()
	if d != DerivationCurrent {
		return d
	}
	if current := currentHashes[entryID]; current != "" && v.Hash == current {
		return GroupCurrent
	}
	return GroupBranch
}

// VersionGroups groups every stored version for the report. currentHashes maps
// entry ID to the checkout's current state hash; an entry absent from it has
// no current version. Groups come current first, then branch, then the other
// rules by name.
func (m *Manifest) VersionGroups(currentHashes map[string]string) []VersionGroup {
	byName := map[string]*VersionGroup{}
	entries := map[string]map[string]bool{}
	for id, state := range m.Entries {
		for _, v := range state.Versions {
			name := m.groupOf(id, v, currentHashes)
			g := byName[name]
			if g == nil {
				g = &VersionGroup{Name: name, Droppable: name != GroupCurrent}
				byName[name] = g
				entries[name] = map[string]bool{}
			}
			g.Versions++
			g.ChunkIDs = append(g.ChunkIDs, v.ChunkIDs...)
			entries[name][id] = true
			if g.Oldest.IsZero() || v.IndexedAt.Before(g.Oldest) {
				g.Oldest = v.IndexedAt
			}
			if v.IndexedAt.After(g.Newest) {
				g.Newest = v.IndexedAt
			}
		}
	}
	out := make([]VersionGroup, 0, len(byName))
	for name, g := range byName {
		g.Entries = len(entries[name])
		sort.Strings(g.ChunkIDs)
		out = append(out, *g)
	}
	rank := func(name string) int {
		switch name {
		case GroupCurrent:
			return 0
		case GroupBranch:
			return 1
		}
		return 2
	}
	sort.Slice(out, func(i, j int) bool {
		if rank(out[i].Name) != rank(out[j].Name) {
			return rank(out[i].Name) < rank(out[j].Name)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// SelectGroups resolves `--drop` names against the groups present: GroupStale
// expands to every droppable group, a present droppable group selects itself,
// and a name this store does not hold is returned in missing so the caller can
// say so — cleanup is best effort across stores, not all-or-nothing. Only
// GroupCurrent is an error: dropping what the checkout searches is never meant.
func (m *Manifest) SelectGroups(names []string, currentHashes map[string]string) (selected, missing []string, err error) {
	var available []string
	for _, g := range m.VersionGroups(currentHashes) {
		if g.Droppable {
			available = append(available, g.Name)
		}
	}
	for _, name := range names {
		switch {
		case name == GroupStale:
			selected = append(selected, available...)
		case name == GroupCurrent:
			return nil, nil, fmt.Errorf("group %s is what this checkout searches and cannot be dropped", name)
		case slices.Contains(available, name):
			selected = append(selected, name)
		default:
			missing = append(missing, name)
		}
	}
	slices.Sort(selected)
	slices.Sort(missing)
	return slices.Compact(selected), slices.Compact(missing), nil
}

// DropGroups removes every version in the selected groups and returns the
// chunk IDs whose rows the caller must delete, with the count of versions
// removed. It mutates the manifest in place — an entry left with no version is
// removed — and performs no I/O: the row delete and the manifest save are the
// write session's job.
func (m *Manifest) DropGroups(selected []string, currentHashes map[string]string) (chunkIDs []string, versions int) {
	for id, state := range m.Entries {
		kept := state.Versions[:0:0]
		for _, v := range state.Versions {
			if slices.Contains(selected, m.groupOf(id, v, currentHashes)) {
				chunkIDs = append(chunkIDs, v.ChunkIDs...)
				versions++
				continue
			}
			kept = append(kept, v)
		}
		switch {
		case len(kept) == 0:
			delete(m.Entries, id)
		case len(kept) != len(state.Versions):
			m.Entries[id] = EntryState{Versions: kept}
		}
	}
	return chunkIDs, versions
}
