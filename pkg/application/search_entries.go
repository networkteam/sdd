package application

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/query"
)

type IndexSearchEntryCmd = command.IndexSearchEntryCmd
type SearchDiscoveryCursor = query.SearchDiscoveryCursor
type SearchEntryRequirement = query.SearchEntryRequirement

type DiscoverSearchEntriesQuery struct {
	// The caller owns the lease and releases it after consuming the iterator.
	Source *AcquiredSnapshot
	Cursor SearchDiscoveryCursor
}

// DiscoverSearchEntries hashes at most the current entry and never prepares
// chunks. Stop iteration to stop I/O. A returned error ends the sequence.
// Save each cursor atomically with enqueueing its missing descriptor.
func (r *ProjectRuntime) DiscoverSearchEntries(ctx context.Context, q DiscoverSearchEntriesQuery) iter.Seq2[SearchEntryRequirement, error] {
	return func(yield func(SearchEntryRequirement, error) bool) {
		if err := validateAcquiredSnapshot(q.Source, r.options.Project.ID, ""); err != nil {
			yield(SearchEntryRequirement{}, err)
			return
		}
		ns, err := r.indexNamespace()
		if err != nil {
			yield(SearchEntryRequirement{}, err)
			return
		}
		store, ok := r.options.SearchIndex.(SearchIndexEntryStore)
		if !ok {
			yield(SearchEntryRequirement{}, fmt.Errorf("sdd: entry publication capability is required"))
			return
		}
		finder := finders.SearchEntriesFinder{Graph: q.Source.Snapshot.graph, Revision: q.Source.Snapshot.Revision(), Namespace: ns, Attachments: graphStoreAttachmentReader{store: q.Source.Attachments}, Store: store, ExcludeEmbedded: r.options.ExcludeEmbeddedFromIndex}
		finder.Discover(ctx, q.Cursor)(yield)
	}
}

// IndexSearchEntry indexes exact retained source, never the current branch.
// Hosts authorize calls and retry failures. Already published work requires
// neither a source lease nor embedding, including after a lost acknowledgement.
func (r *ProjectRuntime) IndexSearchEntry(ctx context.Context, cmd IndexSearchEntryCmd) (err error) {
	store, err := r.entryStore(cmd.Entry)
	if err != nil {
		return err
	}
	published, err := store.EntryPublished(ctx, cmd.Entry.Version)
	if err != nil || published {
		return err
	}
	source, err := acquireReadSnapshot(ctx, r.options.Graph, r.options.Project.ID, SnapshotReadQuery{ExactRevision: cmd.Entry.SourceRevision})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, source.Release()) }()
	return r.indexSearchEntrySource(ctx, source, cmd)
}

func (r *ProjectRuntime) entryStore(entry SearchEntryDescriptor) (SearchIndexEntryStore, error) {
	ns, err := r.indexNamespace()
	if err != nil {
		return nil, err
	}
	if entry.SourceRevision == "" || entry.Version.Namespace != ns || entry.Version.EntryID == "" || entry.Version.EntryHash == "" {
		return nil, fmt.Errorf("sdd: entry descriptor does not match runtime configuration")
	}
	store, ok := r.options.SearchIndex.(SearchIndexEntryStore)
	if !ok {
		return nil, fmt.Errorf("sdd: entry publication capability is required")
	}
	return store, nil
}

func (r *ProjectRuntime) indexSearchEntrySource(ctx context.Context, source *AcquiredSnapshot, cmd IndexSearchEntryCmd) error {
	store, err := r.entryStore(cmd.Entry)
	if err != nil {
		return err
	}
	if err := validateAcquiredSnapshot(source, r.options.Project.ID, cmd.Entry.SourceRevision); err != nil {
		return err
	}
	entry := source.Snapshot.graph.ByID[cmd.Entry.Version.EntryID]
	if entry == nil || !chunking.IncludeEntry(entry, r.options.ExcludeEmbeddedFromIndex) {
		return fmt.Errorf("sdd: descriptor entry is absent or ineligible")
	}
	h := handlers.SearchEntryHandler{Store: store, Embedder: r.options.Embedder, Entry: entry, Attachments: graphStoreAttachmentReader{store: source.Attachments}}
	return h.Index(ctx, cmd)
}
