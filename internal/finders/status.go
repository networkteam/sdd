package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/query"
)

// Status assembles the per-section snapshots used by the status view.
// Pure read — every section comes from in-memory graph filters.
func (f *Finder) Status(q query.StatusQuery) (*query.StatusResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}
	n := q.RecentActions
	if n <= 0 {
		n = 5
	}
	return &query.StatusResult{
		Graph:       q.Graph,
		Contracts:   q.Graph.Contracts(),
		Aspirations: q.Graph.Aspirations(),
		Plans:       q.Graph.Plans(),
		Active:      q.Graph.ActiveDecisions(),
		Open:        q.Graph.OpenSignals(),
		Recent:      q.Graph.RecentActions(n),
	}, nil
}
