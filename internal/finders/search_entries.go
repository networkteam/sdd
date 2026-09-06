package finders

import (
	"context"
	"fmt"
	"iter"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/application/types"
)

type EntryPublicationReader interface {
	EntryPublished(context.Context, types.SearchEntryVersion) (bool, error)
}

type SearchEntriesFinder struct {
	Graph           *model.Graph
	Revision        string
	Namespace       types.IndexNamespace
	Attachments     chunking.AttachmentReader
	Store           EntryPublicationReader
	ExcludeEmbedded bool
}

func (f SearchEntriesFinder) Discover(ctx context.Context, cursor query.SearchDiscoveryCursor) iter.Seq2[query.SearchEntryRequirement, error] {
	return func(yield func(query.SearchEntryRequirement, error) bool) {
		fail := func(err error) { yield(query.SearchEntryRequirement{}, err) }
		if err := ctx.Err(); err != nil {
			fail(err)
			return
		}
		if cursor != (query.SearchDiscoveryCursor{}) && (cursor.Revision != f.Revision || cursor.Namespace != f.Namespace) {
			fail(fmt.Errorf("sdd: discovery cursor does not match snapshot and index configuration"))
			return
		}
		for _, entry := range f.Graph.EntriesAfter(cursor.AfterEntryID) {
			if err := ctx.Err(); err != nil {
				fail(err)
				return
			}
			if !chunking.IncludeEntry(entry, f.ExcludeEmbedded) {
				continue
			}
			hash, err := chunking.EntryStateHash(ctx, entry, f.Attachments)
			if err != nil {
				fail(err)
				return
			}
			version := types.SearchEntryVersion{Namespace: f.Namespace, EntryID: entry.ID, EntryHash: hash}
			published, err := f.Store.EntryPublished(ctx, version)
			if err != nil {
				fail(err)
				return
			}
			item := query.SearchEntryRequirement{
				Entry: types.SearchEntryDescriptor{Version: version, SourceRevision: f.Revision}, Published: published,
				Cursor: query.SearchDiscoveryCursor{Revision: f.Revision, Namespace: f.Namespace, AfterEntryID: entry.ID},
			}
			if !yield(item, nil) {
				return
			}
		}
	}
}
