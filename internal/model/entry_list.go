package model

import (
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/networkteam/sdd/pkg/application/types"
)

// EntryListFilter selects entries for a structured listing. Empty fields
// match all; values within one field are alternatives, fields combine with
// AND. Terms match summary and body (MatchesSearchTerms). Embedded
// base entries are left out unless IncludeEmbedded is set or IDs names them.
// IDs, when set, keeps the entries with these display IDs only.
type EntryListFilter struct {
	IDs             []string
	IncludeEmbedded bool
	Types           []EntryType
	Kinds           []Kind
	Layers          []Layer
	Statuses        []StatusKind
	Participants    []string
	CreatedAfter    time.Time
	CreatedBefore   time.Time
	Terms           []*regexp.Regexp
}

// ListMember is one graph contributing to a listing. Repo qualifies a
// connected repo's graph loaded standalone; empty for the local graph.
type ListMember struct {
	Graph *Graph
	Repo  RepoID
}

// ListedEntry is one entry of a listing with the graph that owns it.
type ListedEntry struct {
	Entry *Entry
	Owner *Graph
	Repo  RepoID
}

// EntryList is a filtered, time-ordered listing with facets over the whole
// filtered set.
type EntryList struct {
	Entries []ListedEntry
	Facets  types.EntryFacets
}

// ListEntries filters the members' entries, orders them by creation time
// (ties by ID) and counts facets. Included embedded entries surface once,
// from the first member. A non-nil empty IDs filter lists nothing.
func ListEntries(members []ListMember, f EntryListFilter, newestFirst bool) EntryList {
	list := EntryList{Facets: types.EntryFacets{
		Types: map[string]int{}, Kinds: map[string]int{}, Layers: map[string]int{}, Statuses: map[string]int{},
	}}
	var ids map[string]struct{}
	if f.IDs != nil {
		ids = make(map[string]struct{}, len(f.IDs))
		for _, id := range f.IDs {
			ids[id] = struct{}{}
		}
	}
	for i, m := range members {
		candidates := m.Graph.Entries
		if len(f.Participants) > 0 {
			candidates = FilterByParticipants(candidates, f.Participants)
		}
		for _, e := range candidates {
			if ids != nil {
				if _, ok := ids[m.Graph.qualifyIn(m.Repo, e.ID)]; !ok {
					continue
				}
			}
			if e.Embedded && (i > 0 || (!f.IncludeEmbedded && ids == nil)) {
				continue
			}
			status := m.Graph.DerivedStatus(e)
			if !f.matches(e, status.Kind) {
				continue
			}
			list.Entries = append(list.Entries, ListedEntry{Entry: e, Owner: m.Graph, Repo: m.Repo})
			list.Facets.Types[e.TypeLabel()]++
			list.Facets.Kinds[string(e.Kind)]++
			list.Facets.Layers[e.LayerLabel()]++
			list.Facets.Statuses[status.Kind.Label()]++
		}
	}
	sort.SliceStable(list.Entries, func(i, j int) bool {
		a, b := list.Entries[i].Entry, list.Entries[j].Entry
		if !a.Time.Equal(b.Time) {
			return a.Time.Before(b.Time) != newestFirst
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return list.Entries[i].Repo < list.Entries[j].Repo
	})
	return list
}

func (f EntryListFilter) matches(e *Entry, status StatusKind) bool {
	switch {
	case len(f.Types) > 0 && !slices.Contains(f.Types, e.Type),
		len(f.Kinds) > 0 && !slices.Contains(f.Kinds, e.Kind),
		len(f.Layers) > 0 && !slices.Contains(f.Layers, e.Layer),
		len(f.Statuses) > 0 && !slices.Contains(f.Statuses, status),
		!f.CreatedAfter.IsZero() && !e.Time.After(f.CreatedAfter),
		!f.CreatedBefore.IsZero() && !e.Time.Before(f.CreatedBefore):
		return false
	}
	return e.MatchesSearchTerms(f.Terms)
}

// Page returns the listing window at offset; a non-positive limit means the
// rest of the listing.
func (l EntryList) Page(offset, limit int) []ListedEntry {
	offset = max(offset, 0)
	if offset >= len(l.Entries) {
		return nil
	}
	end := len(l.Entries)
	if limit > 0 && limit < end-offset {
		end = offset + limit
	}
	return l.Entries[offset:end]
}
