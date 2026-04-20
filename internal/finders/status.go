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
	nDone := q.RecentDone
	if nDone <= 0 {
		nDone = 10
	}
	nInsights := q.RecentInsights
	if nInsights <= 0 {
		nInsights = 10
	}
	return &query.StatusResult{
		Graph:       q.Graph,
		Aspirations: q.Graph.Aspirations(),
		Contracts:   q.Graph.Contracts(),
		Plans:       q.Graph.Plans(),
		Activities:  q.Graph.Activities(),
		Directives:  q.Graph.Directives(),
		Open:        q.Graph.OpenSignals(),
		Insights:    q.Graph.RecentInsights(nInsights),
		Recent:      q.Graph.RecentDone(nDone),
	}, nil
}
