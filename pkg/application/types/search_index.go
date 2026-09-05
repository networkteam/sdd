package types

// ProjectID identifies a project within a composition.
type ProjectID string

// IndexNamespace keys one reconciled vector index. The fingerprint pins the
// embedding model (and thus the dimensionality), so dimensions are not part
// of the identity — stores enforce vector-length consistency per namespace
// at reconcile and query time instead.
type IndexNamespace struct {
	Project     ProjectID
	Fingerprint string
	Metric      string
}

type CanonicalChunk struct {
	ID      string
	EntryID string
	Ordinal int
	// Revision is deprecated: graph revision is a mutation-concurrency token,
	// never a vector-freshness token (d-cpt-65i). Reconciliation and hit
	// validity ignore it. Retained only for source compatibility.
	Revision    string
	ContentHash string
	Text        string
	// The following persisted citation and identity fields carry everything a
	// store needs to render a citation and answer entry-presence queries
	// without re-deriving chunks. Both the CLI indexer and the application
	// vector search populate them through the shared chunk-derivation helper.
	Body                 string
	Breadcrumb           []string
	Depth                int
	IsSummary            bool
	IsAttachment         bool
	SourceAttachmentPath string
	// EntryHash is the entry-state hash (entry content + summary + attachment
	// bytes) — the same definition as the CLI manifest state hash.
	EntryHash string
}

type IndexedChunk struct {
	Chunk  CanonicalChunk
	Vector []float32
}

type StoredChunkRef struct {
	ID string
	// Revision is deprecated and ignored by reconciliation (see CanonicalChunk).
	Revision    string
	ContentHash string
}

// StoredEntryRef identifies one stored (entry, version) pair in a persistent
// index. Presence is keyed by the pair: a store returns one ref per stored
// version of an entry, so a changed entry (a new EntryHash) reads as absent
// and is embedded as an added version rather than overwriting the old one.
type StoredEntryRef struct {
	EntryID string
	// EntryHash is the entry-state hash of this stored version — the same
	// definition as CanonicalChunk.EntryHash and the CLI manifest hash. Empty
	// only for a store that cannot report per-version identity.
	EntryHash string
}

type ScoredChunkHit struct {
	Namespace IndexNamespace
	ChunkID   string
	EntryID   string
	// EntryHash is the version this hit belongs to, resolved by the store (row
	// metadata, or the manifest for a legacy row). Read-time filtering keeps
	// the hit only when it equals the current entry's state hash. Empty means
	// the store cannot report a version, so the hit is not version-filtered.
	EntryHash string
	// Revision is deprecated and ignored by hit validity (see CanonicalChunk).
	Revision    string
	ContentHash string
	Score       float64
	// Persisted citation fields, rendered directly into search citations so a
	// hit needs no re-derivation of its source chunk.
	Body                 string
	Breadcrumb           []string
	Depth                int
	IsSummary            bool
	IsAttachment         bool
	SourceAttachmentPath string
}

type ReconcileSearchIndexCmd struct {
	// Callbacks run synchronously after persistence succeeds.
	OnEntryIndexed func(entryID string, chunkCount int)
	OnComplete     func(revision string, entriesIndexed, chunksStored int)
}

type SearchSyncMode string

const (
	SearchSyncNone  SearchSyncMode = "none"
	SearchSyncLocal SearchSyncMode = "local"
	SearchSyncAll   SearchSyncMode = "all"
)

func (m SearchSyncMode) Valid() bool {
	return m == SearchSyncNone || m == SearchSyncLocal || m == SearchSyncAll
}
