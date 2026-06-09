package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Manifest is the sidecar tracking which entries are indexed, their
// content hash (for lazy-fill staleness detection), the fingerprint of
// the embedder used (for drift counting), and the chunk IDs each entry
// produced (so re-indexing can delete the old set before adding the new).
//
// Lives next to the chromem-go DB at .sdd/index/manifest.json.
type Manifest struct {
	// Version is bumped when the manifest schema changes. Currently 1.
	Version int                   `json:"version"`
	Entries map[string]EntryState `json:"entries"`
}

// EntryState tracks indexing status for a single entry.
type EntryState struct {
	// Hash is the content hash of the source material the index was
	// built from (entry body + each attachment). Changes invalidate the
	// entry's chunks and trigger re-indexing on lazy-fill.
	Hash string `json:"hash"`
	// Fingerprint is the embedder fingerprint that produced this entry's
	// chunks. Compared against the configured embedder at search-time
	// (lazy re-embed) and lint-time (drift count).
	Fingerprint string `json:"fingerprint"`
	// ChunkIDs are the IDs this entry produced last time it was indexed.
	// Used to delete stale rows before re-adding under new content/
	// fingerprint.
	ChunkIDs []string `json:"chunk_ids"`
	// IndexedAt is the time the entry was last indexed. Informational
	// only — not used for any lifecycle decision.
	IndexedAt time.Time `json:"indexed_at"`
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
// crash in the middle of indexing doesn't leave a partial JSON file.
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
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write manifest tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	return nil
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

// MismatchCount returns the number of entries whose recorded fingerprint
// differs from current. Empty fingerprint counts as a mismatch (the entry
// has never been embedded under the active embedder). Used by sdd lint.
func (m *Manifest) MismatchCount(current string) int {
	if current == "" {
		return 0
	}
	n := 0
	for _, e := range m.Entries {
		if e.Fingerprint != current {
			n++
		}
	}
	return n
}

// PendingCount returns how many of entryIDs are absent from the manifest or
// recorded under a different embedder fingerprint — the entries a build or
// lazy-fill would (re-)embed. Entry bodies are immutable, so a stored content
// hash never goes stale on its own; presence and fingerprint are the
// reconciliation axes that decide whether there is work worth showing a
// transient progress view for.
func (m *Manifest) PendingCount(entryIDs []string, fingerprint string) int {
	n := 0
	for _, id := range entryIDs {
		st, ok := m.Entries[id]
		if !ok || st.Fingerprint != fingerprint {
			n++
		}
	}
	return n
}

func manifestPath(indexDir string) string {
	return filepath.Join(indexDir, "manifest.json")
}
