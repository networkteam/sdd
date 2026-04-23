package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model"
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
		Graph:            q.Graph,
		LocalParticipant: f.localParticipant(),
		Language:         f.language(),
		Aspirations:      q.Graph.Aspirations(),
		Contracts:        q.Graph.Contracts(),
		Plans:            q.Graph.Plans(),
		Activities:       q.Graph.Activities(),
		Directives:       q.Graph.Directives(),
		Open:             q.Graph.OpenSignals(),
		Insights:         q.Graph.RecentInsights(nInsights),
		Recent:           q.Graph.RecentDone(nDone),
		Participants:     activeParticipantGroups(q.Graph),
	}, nil
}

// activeParticipantGroups materializes the Participants block for the
// status view per plan d-cpt-d34 AC 15. Each group couples one active
// actor head with the derived-active roles that cascade to its chain.
// Returns nil during grace (no active actors).
func activeParticipantGroups(g *model.Graph) []query.ParticipantGroup {
	active := g.ActiveActorHeads()
	if len(active) == 0 {
		return nil
	}
	roles := g.ActiveRoles()
	groups := make([]query.ParticipantGroup, 0, len(active))
	for _, a := range active {
		var bound []*model.Entry
		for _, r := range roles {
			chain := g.ResolveRoleChain(r)
			if chain != nil && chain.Head != nil && chain.Head.ID == a.ID {
				bound = append(bound, r)
			}
		}
		groups = append(groups, query.ParticipantGroup{Actor: a, Roles: bound})
	}
	return groups
}
