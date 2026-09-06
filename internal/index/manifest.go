package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Manifest is the sidecar tracking which entries are indexed and, per entry,
// the set of stored versions (each with its content hash, embedder
// fingerprint, chunk IDs, and index time). It lives next to the chromem-go DB
// at .sdd/index/manifest.json.
//
// Multi-version is the point: the shared machine-global store is written from
// several checkouts/branches (d-cpt-6cq), and a changed entry (summary
// regeneration, a mechanical fix) adds a version instead of overwriting the
// old one, so two branches never flip-flop each other's rows. A stale version
// is dropped only by write-session GC or an explicit `--force` rebuild — never
// by a read.
//
// Schema evolution: a v1 manifest recorded exactly one state per entry as a
// flat {hash, fingerprint, chunk_ids, indexed_at} object. That shape still
// loads — as an entry's sole version — with NO migration or re-embedding, and
// an entry still holding a single version serializes back in that same flat
// shape, so an untouched v1 manifest round-trips byte-for-byte. Only an entry
// that has accumulated more than one version serializes as {"versions": [...]}.
// The manifest is a sidecar file, not public API, so this per-entry
// self-describing format is free to evolve.
type Manifest struct {
	// Version is the manifest schema version. Kept at 1: the per-entry format
	// is self-describing (see EntryState), so no file-level bump is needed to
	// distinguish legacy single-version entries from multi-version ones.
	Version int                   `json:"version"`
	Entries map[string]EntryState `json:"entries"`
}

// EntryState is one entry's set of stored versions. See Manifest for the
// on-disk shape (legacy flat object for a single version, {"versions": [...]}
// for many).
type EntryState struct {
	Versions []EntryVersion
}

// EntryVersion is one stored version of an entry: the material it was built
// from (Hash), the embedder that produced it (Fingerprint), the chunk IDs it
// contributed (ChunkIDs), and when it was indexed (IndexedAt, which the
// retention side of GC reads).
type EntryVersion struct {
	// Hash is the entry-state hash (entry content + summary + attachment
	// bytes) this version was built from. Read-time freshness keeps a hit only
	// when its version's Hash equals the current entry's state hash.
	Hash string `json:"hash"`
	// Fingerprint is the embedder fingerprint that produced this version's
	// chunks. Within one store (keyed per fingerprint) this is constant, but
	// it is retained per version for lint drift reporting.
	Fingerprint string `json:"fingerprint"`
	// ChunkIDs are the IDs this version contributed. Used to resolve a hit's
	// version (legacy rows carry no entry_hash metadata) and to delete a
	// version's rows during GC or a force rebuild.
	ChunkIDs []string `json:"chunk_ids"`
	// IndexedAt is when this version was last written. The retention window in
	// version GC reads it; nothing else depends on it.
	IndexedAt time.Time `json:"indexed_at"`
}

// entryStateWire is the multi-version on-disk shape. A single-version entry
// serializes as a bare EntryVersion instead (see MarshalJSON) so a v1 manifest
// round-trips unchanged.
type entryStateWire struct {
	Versions []EntryVersion `json:"versions"`
}

// MarshalJSON emits the legacy flat shape for a single version and the
// versions-list shape for many. An entry with no versions should never be
// persisted (GC drops the map key instead), but it serializes harmlessly as an
// empty versions list.
func (s EntryState) MarshalJSON() ([]byte, error) {
	if len(s.Versions) == 1 {
		return json.Marshal(s.Versions[0])
	}
	return json.Marshal(entryStateWire(s))
}

// UnmarshalJSON loads either shape: a {"versions": [...]} object as-is, or a
// legacy flat {hash, fingerprint, chunk_ids, indexed_at} object as the entry's
// sole version.
func (s *EntryState) UnmarshalJSON(data []byte) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if _, ok := probe["versions"]; ok {
		var wire entryStateWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		s.Versions = wire.Versions
		return nil
	}
	var v EntryVersion
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	s.Versions = []EntryVersion{v}
	return nil
}

// HasVersion reports whether the entry has a stored version matching both the
// given state hash and embedder fingerprint — the presence test both the CLI
// indexer and the application reconcile use to decide whether to (re-)embed.
func (s EntryState) HasVersion(hash, fingerprint string) bool {
	for _, v := range s.Versions {
		if v.Hash == hash && v.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

// AllChunkIDs returns every chunk ID across all of the entry's versions —
// what a force rebuild deletes before writing the single current version.
func (s EntryState) AllChunkIDs() []string {
	var out []string
	for _, v := range s.Versions {
		out = append(out, v.ChunkIDs...)
	}
	return out
}

// LoadManifest reads .sdd/index/manifest.json or returns an empty manifest
// when the file does not exist.
func LoadManifest(indexDir string) (*Manifest, error) {
	path := manifestPath(indexDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Version: 1, Entries: map[string]EntryState{}}, nil
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Entries == nil {
		m.Entries = map[string]EntryState{}
	}
	if m.Version == 0 {
		m.Version = 1
	}
	return &m, nil
}

// Save writes the manifest atomically (write to temp then rename) so a
// crash in the middle of indexing doesn't leave a partial JSON file. The
// atomic rename also gives an unlocked presence read a torn-read-safe file and
// changes the manifest's on-disk identity on every write (the generation
// fallback for legacy stores relies on this).
func (m *Manifest) Save(indexDir string) error {
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return fmt.Errorf("creating index dir: %w", err)
	}
	final := manifestPath(indexDir)
	tmp := final + ".tmp"

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("write manifest tmp: %w", err)
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	return syncPublicationDirectory(indexDir)
}

// AddVersion records a version for an entry (monotonic accumulation). A version
// with the same Hash is replaced in place (idempotent re-embed of the same
// state); otherwise the version is appended, leaving prior versions intact —
// this is the no-delete lazy write path. The force/rebuild path uses
// SetSingleVersion instead.
func (m *Manifest) AddVersion(entryID string, v EntryVersion) {
	if m.Entries == nil {
		m.Entries = map[string]EntryState{}
	}
	state := m.Entries[entryID]
	for i := range state.Versions {
		if state.Versions[i].Hash == v.Hash {
			state.Versions[i] = v
			m.Entries[entryID] = state
			return
		}
	}
	state.Versions = append(state.Versions, v)
	m.Entries[entryID] = state
}

// SetSingleVersion collapses an entry to exactly the given version, discarding
// any others. Used by the force rebuild path, whose caller has already deleted
// the entry's old chunk rows from the index.
func (m *Manifest) SetSingleVersion(entryID string, v EntryVersion) {
	if m.Entries == nil {
		m.Entries = map[string]EntryState{}
	}
	m.Entries[entryID] = EntryState{Versions: []EntryVersion{v}}
}

// VersionHashForChunk returns the state hash of the version that owns chunkID,
// or "" when no version does. It resolves a hit whose row carries no entry_hash
// metadata (a legacy v1 row): the legacy version's ChunkIDs hold the
// unversioned chunk ID, so this recovers that version's recorded hash.
func (m *Manifest) VersionHashForChunk(entryID, chunkID string) string {
	for _, v := range m.Entries[entryID].Versions {
		for _, id := range v.ChunkIDs {
			if id == chunkID {
				return v.Hash
			}
		}
	}
	return ""
}

// EntryIDsSorted returns the manifest's entry IDs in lexicographic order.
// Useful for deterministic iteration in tests and lint output.
func (m *Manifest) EntryIDsSorted() []string {
	ids := make([]string, 0, len(m.Entries))
	for id := range m.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// MismatchCount returns the number of entries that have NO version recorded
// under the current embedder fingerprint — the entries a build/lazy-fill would
// re-embed on an embedder change. Empty current returns 0. Used by sdd lint.
func (m *Manifest) MismatchCount(current string) int {
	if current == "" {
		return 0
	}
	n := 0
	for _, e := range m.Entries {
		matched := false
		for _, v := range e.Versions {
			if v.Fingerprint == current {
				matched = true
				break
			}
		}
		if !matched {
			n++
		}
	}
	return n
}

// PendingCount returns how many of entryIDs are absent from the manifest or
// have no version under the current fingerprint — the entries a build or
// lazy-fill would embed at least once. It is presence- and fingerprint-based
// (not hash-based): it drives whether to show a transient progress view, and
// the current hash of every on-disk entry is not cheaply available here.
func (m *Manifest) PendingCount(entryIDs []string, fingerprint string) int {
	n := 0
	for _, id := range entryIDs {
		st, ok := m.Entries[id]
		if !ok {
			n++
			continue
		}
		matched := false
		for _, v := range st.Versions {
			if v.Fingerprint == fingerprint {
				matched = true
				break
			}
		}
		if !matched {
			n++
		}
	}
	return n
}

func manifestPath(indexDir string) string {
	return filepath.Join(indexDir, "manifest.json")
}
