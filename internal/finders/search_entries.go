package finders

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"strings"

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
		entries := slices.Clone(f.Graph.Entries)
		slices.SortFunc(entries, func(a, b *model.Entry) int { return strings.Compare(a.ID, b.ID) })
		start, _ := slices.BinarySearchFunc(entries, cursor.AfterEntryID, func(e *model.Entry, id string) int { return strings.Compare(e.ID, id) })
		for _, entry := range entries[start:] {
			if err := ctx.Err(); err != nil {
				fail(err)
				return
			}
			if entry.ID <= cursor.AfterEntryID || !chunking.IncludeEntry(entry, f.ExcludeEmbedded) {
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
