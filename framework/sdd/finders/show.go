package finders

import (
	"fmt"

	"github.com/networkteam/resonance/framework/sdd/query"
)

// Show resolves the entries named in q and returns groups with upstream and
// downstream chains. The heavy lifting (tree traversal, dedup, depth limiting)
// is delegated to model.Graph.BuildShowTree.
func (f *Finder) Show(q query.ShowQuery) (*query.ShowResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	maxDepth := q.MaxDepth

	resolved, err := q.Graph.ResolveIDs(q.IDs)
	if err != nil {
		return nil, err
	}

	rendered := make(map[string]bool)
	primaries := make(map[string]bool, len(resolved))
	for _, id := range resolved {
		primaries[id] = true
	}

	groups := make([]query.ShowGroup, 0, len(resolved))
	for _, id := range resolved {
		if _, ok := q.Graph.ByID[id]; !ok {
			return nil, fmt.Errorf("entry not found: %s", id)
		}

		tree := q.Graph.BuildShowTree(id, maxDepth, q.Downstream, rendered, primaries)

		groups = append(groups, query.ShowGroup{
			Primary:    tree.Primary,
			Upstream:   tree.Upstream,
			Downstream: tree.Downstream,
		})
	}

	return &query.ShowResult{Graph: q.Graph, Groups: groups}, nil
}
