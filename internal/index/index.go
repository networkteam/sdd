// Package index wraps chromem-go's persistent vector store with the SDD
// chunk and citation model. It is the side-effecting backing store for
// the search index — no domain reasoning happens here. The IndexHandler
// (internal/handlers) orchestrates: load entry → split → embed → upsert
// via this package.
//
// Storage layout (under .sdd/index/):
//
//	chromem/        # chromem-go gob persistence
//	manifest.json   # entry tracking sidecar (see manifest.go)
package index

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

// CollectionName is the chromem-go collection used for SDD chunks.
const CollectionName = "sdd-graph"

// MetadataKey constants name the per-row chromem metadata fields. Kept as
// constants so the indexer, finder, and tests share one source of truth.
const (
	MetaEntryID              = "entry_id"
	MetaEntryHash            = "entry_hash"
	MetaChunkPath            = "chunk_path"
	MetaDepth                = "depth"
	MetaContentHash          = "content_hash"
	MetaModelFingerprint     = "model_fingerprint"
	MetaIsSummary            = "is_summary"
	MetaIsAttachment         = "is_attachment"
	MetaSourceAttachmentPath = "source_attachment_path"
)

// Row is one chunk-as-it-lives-in-the-index. The Index ingests Rows the
// IndexHandler has already populated from the splitter + embedder.
// Embedding must be non-empty; the index does not call out to embedders.
type Row struct {
	EntryID string
	// EntryHash is the entry-state hash of the version this row belongs to.
	// Persisted so a read can decide the hit is fresh (its version equals the
	// current entry state) without re-deriving the chunk. Empty only for
	// legacy v1 rows, whose version is recovered from the manifest.
	EntryHash            string
	ChunkID              string
	Text                 string // embedded text (with Entry/Breadcrumb preamble)
	Body                 string // citation snippet source (without preamble)
	Breadcrumb           []string
	Depth                int
	IsSummary            bool
	IsAttachment         bool
	SourceAttachmentPath string
	ContentHash          string
	ModelFingerprint     string
	Embedding            []float32
}

// Hit is one query result with the metadata needed to render a citation
// and to decide whether to re-embed (for fingerprint drift).
type Hit struct {
	EntryID string
	// EntryHash is the version this hit belongs to (from row metadata). Empty
	// for a legacy v1 row — the caller recovers the version through the
	// manifest (Manifest.VersionHashForChunk).
	EntryHash            string
	ChunkID              string
	Score                float32
	Text                 string
	Body                 string
	Breadcrumb           []string
	Depth                int
	IsSummary            bool
	IsAttachment         bool
	SourceAttachmentPath string
	ContentHash          string
	ModelFingerprint     string
}

// Index wraps a chromem-go persistent DB and a single collection.
type Index struct {
	db         *chromem.DB
	coll       *chromem.Collection
	published  *chromem.Collection
	manifest   *Manifest
	indexDir   string // root of the index storage tree (passed to Open)
	chromemDir string // sub-directory chromem-go writes its gob files into
	// dirty records whether a mutation touched the collection during this
	// session, so WriteStore bumps the store generation only for writes that
	// actually changed content — a no-op lazy-fill never invalidates readers'
	// cached snapshots.
	dirty bool
}

// Open opens or creates the persistent index under indexDir. The chromem
// store lives in indexDir/chromem; the manifest sidecar at
// indexDir/manifest.json (managed by callers via LoadManifest/Save).
//
// On a fresh directory, both subdirs are created. The collection is named
// CollectionName and is auto-created on first open.
//
// The load phase holds the store's shared lock so a reader never decodes
// half-written documents from a concurrent write session; the lock is
// released before Open returns — queries run on the in-memory copy. Open is
// the read-side entry point (the CLI finders); mutation goes through
// WriteStore, which acquires the exclusive lock before loading its snapshot.
func Open(indexDir string) (*Index, error) {
	if indexDir == "" {
		return nil, errors.New("index dir is required")
	}
	l := lockFile(indexDir)
	if err := ensureStoreDir(indexDir); err != nil {
		return nil, err
	}
	if _, err := l.TryRLockContext(context.Background(), lockRetryInterval); err != nil {
		return nil, fmt.Errorf("acquiring index read lock at %s: %w", indexDir, err)
	}
	defer func() { _ = l.Unlock() }()
	return loadStore(indexDir)
}

// loadStore opens or creates the chromem store under indexDir WITHOUT taking
// any lock. Callers must already hold the store lock (see Open, ReadStore,
// WriteStore) — loading a snapshot outside the lock is the open/load race the
// locked helpers close.
func loadStore(indexDir string) (*Index, error) {
	chromemDir := filepath.Join(indexDir, "chromem")
	if err := os.MkdirAll(chromemDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating index dir: %w", err)
	}
	db, err := chromem.NewPersistentDB(chromemDir, false)
	if err != nil {
		return nil, fmt.Errorf("open chromem db at %s: %w", chromemDir, err)
	}
	coll, err := db.GetOrCreateCollection(CollectionName, nil, embedFuncStub)
	if err != nil {
		return nil, fmt.Errorf("get/create collection %q: %w", CollectionName, err)
	}
	manifest, err := LoadManifest(indexDir)
	if err != nil {
		return nil, err
	}
	return &Index{db: db, coll: coll, indexDir: indexDir, chromemDir: chromemDir, manifest: manifest}, nil
}

// ensureStoreDir creates the store directory tree so the advisory lock file
// can be created before the snapshot is loaded.
func ensureStoreDir(indexDir string) error {
	if err := os.MkdirAll(filepath.Join(indexDir, "chromem"), 0o755); err != nil {
		return fmt.Errorf("creating index dir: %w", err)
	}
	return nil
}

// OpenInMemory returns a non-persistent index. Used by tests.
func OpenInMemory() *Index {
	db := chromem.NewDB()
	coll, _ := db.GetOrCreateCollection(CollectionName, nil, embedFuncStub)
	return &Index{db: db, coll: coll}
}

// embedFuncStub is the embedding function chromem-go requires when adding
// documents without pre-computed embeddings. We always supply embeddings
// at upsert time, so this stub is never called — wired only because
// chromem-go's API insists a collection be constructed with one. Calling
// it returns an error so any drift in our ingest path is loud.
func embedFuncStub(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("index does not compute embeddings — supply pre-computed Row.Embedding via UpsertEntry")
}

// Path returns the on-disk root of the index. Empty for in-memory indexes.
func (i *Index) Path() string { return i.indexDir }

// Count returns the total number of chunk rows in the index.
func (i *Index) Count() int { return i.coll.Count() }

// UpsertEntry replaces all rows for entryID with the given rows in a
// single transaction-shaped pass. Old chunk IDs (from the manifest) are
// deleted first; the new rows are added afterward. Caller is responsible
// for updating the manifest with the new chunk IDs.
//
// Pre-conditions:
//   - Each row's Embedding is populated (the index does not embed).
//   - row.EntryID == entryID (validated; mismatch is a programmer error).
//   - row.ChunkID is unique within rows.
func (i *Index) UpsertEntry(ctx context.Context, entryID string, oldChunkIDs []string, rows []Row) error {
	if entryID == "" {
		return errors.New("entryID is required")
	}
	dims := 0
	if i.indexDir != "" {
		manifest, err := LoadManifest(i.indexDir)
		if err != nil {
			return err
		}
		for _, state := range manifest.Entries {
			ids := state.AllChunkIDs()
			if len(ids) == 0 {
				continue
			}
			doc, err := i.coll.GetByID(ctx, ids[0])
			if err != nil {
				return err
			}
			dims = len(doc.Embedding)
			break
		}
	}
	for j, r := range rows {
		if r.EntryID != entryID {
			return fmt.Errorf("row %d: entry id %q does not match %q", j, r.EntryID, entryID)
		}
		if len(r.Embedding) == 0 {
			return fmt.Errorf("row %d (chunk %s): embedding is empty", j, r.ChunkID)
		}
		if dims == 0 {
			dims = len(r.Embedding)
		}
		if len(r.Embedding) != dims {
			return fmt.Errorf("inconsistent vector dimensions")
		}
		for _, v := range r.Embedding {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("non-finite vector")
			}
		}

	}

	if len(oldChunkIDs) > 0 {
		if err := i.coll.Delete(ctx, nil, nil, oldChunkIDs...); err != nil {
			return fmt.Errorf("delete old chunks for %s: %w", entryID, err)
		}
		i.dirty = true
		i.published = nil
	}

	if len(rows) == 0 {
		return nil
	}

	docs := make([]chromem.Document, 0, len(rows))
	for _, r := range rows {
		docs = append(docs, chromem.Document{
			ID:        r.ChunkID,
			Metadata:  rowMetadata(r),
			Embedding: r.Embedding,
			Content:   r.Text,
		})
	}
	if err := i.addDocuments(ctx, docs); err != nil {
		return fmt.Errorf("add chunks for %s: %w", entryID, err)
	}
	i.dirty = true
	i.published = nil
	return nil
}

// DeleteEntry removes all rows whose chunk IDs are listed. Called when an
// entry is removed from the graph or when reconciling the manifest.
func (i *Index) DeleteEntry(ctx context.Context, chunkIDs []string) error {
	if len(chunkIDs) == 0 {
		return nil
	}
	if err := i.coll.Delete(ctx, nil, nil, chunkIDs...); err != nil {
		return err
	}
	i.dirty = true
	i.published = nil
	return nil
}

// Query returns the top-N matches for the given query embedding. The
// nResults parameter is clamped to the collection's current count to
// avoid chromem-go's "n_results larger than collection" behavior.
func (i *Index) Query(ctx context.Context, embedding []float32, nResults int) ([]Hit, error) {
	if len(embedding) == 0 {
		return nil, errors.New("query embedding is empty")
	}
	collection, err := i.publishedCollection(ctx)
	if err != nil {
		return nil, err
	}
	count := collection.Count()
	if count == 0 {
		return nil, nil
	}
	nResults = min(nResults, count)
	if nResults <= 0 {
		return nil, nil
	}
	results, err := collection.QueryEmbedding(ctx, embedding, nResults, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("vector query: %w", err)
	}
	hits := make([]Hit, 0, len(results))
	for _, result := range results {
		hits = append(hits, hitFromResult(result))
	}

	return hits, nil
}

// rowMetadata serializes Row's metadata fields to chromem-go's
// map[string]string contract. Booleans become "true"/"false"; ints become
// decimal; the breadcrumb becomes ` > `-joined for human-readable
// chunk_path while the per-row Body is stored separately under a sidecar
// metadata key (we serialize it into Content via chromem-go).
func rowMetadata(r Row) map[string]string {
	m := map[string]string{
		MetaEntryID:          r.EntryID,
		MetaChunkPath:        strings.Join(r.Breadcrumb, " > "),
		MetaDepth:            strconv.Itoa(r.Depth),
		MetaContentHash:      r.ContentHash,
		MetaModelFingerprint: r.ModelFingerprint,
		MetaIsSummary:        boolStr(r.IsSummary),
		MetaIsAttachment:     boolStr(r.IsAttachment),
		// Body is stored alongside Text via a metadata key so the citation
		// renderer can extract a snippet without re-tokenizing the
		// preamble out of the embedded text.
		"body": r.Body,
	}
	// Only set for versioned (new) rows; a legacy v1 row leaves it absent, and
	// read-time freshness recovers its version through the manifest.
	if r.EntryHash != "" {
		m[MetaEntryHash] = r.EntryHash
	}
	if r.SourceAttachmentPath != "" {
		m[MetaSourceAttachmentPath] = r.SourceAttachmentPath
	}
	return m
}

func hitFromResult(r chromem.Result) Hit {
	depth, _ := strconv.Atoi(r.Metadata[MetaDepth])
	return Hit{
		EntryID:              r.Metadata[MetaEntryID],
		EntryHash:            r.Metadata[MetaEntryHash],
		ChunkID:              r.ID,
		Score:                r.Similarity,
		Text:                 r.Content,
		Body:                 r.Metadata["body"],
		Breadcrumb:           splitBreadcrumb(r.Metadata[MetaChunkPath]),
		Depth:                depth,
		IsSummary:            r.Metadata[MetaIsSummary] == "true",
		IsAttachment:         r.Metadata[MetaIsAttachment] == "true",
		SourceAttachmentPath: r.Metadata[MetaSourceAttachmentPath],
		ContentHash:          r.Metadata[MetaContentHash],
		ModelFingerprint:     r.Metadata[MetaModelFingerprint],
	}
}

func splitBreadcrumb(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, " > ")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
