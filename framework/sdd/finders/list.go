package finders

import (
	"fmt"

	"github.com/networkteam/resonance/framework/sdd/query"
)

// List returns entries matching q.Filter from q.Graph.
func (f *Finder) List(q query.ListQuery) (*query.ListResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}
	return &query.ListResult{Entries: q.Graph.Filter(q.Filter)}, nil
}
