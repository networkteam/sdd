package finders

import (
	"github.com/networkteam/sdd/internal/query"
)

// WIPList loads every active WIP marker from the wip/ subdirectory of
// q.GraphDir. Returns an empty result (not an error) when no markers exist.
func (f *Finder) WIPList(q query.WIPListQuery) (*query.WIPListResult, error) {
	markers, err := f.LoadWIPMarkers(q.GraphDir)
	if err != nil {
		return nil, err
	}
	return &query.WIPListResult{Markers: markers}, nil
}
