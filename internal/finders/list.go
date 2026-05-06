package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/query"
)

// List returns entries matching q.Filter from q.Graph. When q.Topic is set,
// the result is further narrowed via TopicFilter to entries whose effective
// topic set has the prefix.
func (f *Finder) List(q query.ListQuery) (*query.ListResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}
	entries := q.Graph.Filter(q.Filter)
	if !q.Topic.IsZero() {
		entries = TopicFilter{Prefix: q.Topic}.FilterEntries(q.Graph, entries)
	}
	return &query.ListResult{Graph: q.Graph, Entries: entries}, nil
}
