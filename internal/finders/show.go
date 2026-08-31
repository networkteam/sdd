package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/query"
)

// Show resolves the entries named in q and returns groups with upstream and
// downstream chains. The heavy lifting (tree traversal, dedup, depth limiting)
// is delegated to model.Graph.BuildShowTree.
func (gf *GraphFinder) Show(q query.ShowQuery) (*query.ShowResult, error) {
	if gf.graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	// Bare IDs (short or unprefixed-full) resolve across the union of the
	// local graph and its declared dependencies, so a foreign entry ID handed
	// to `sdd show` or the `show` MCP tool resolves to its full prefixed form
	// before the tree build. Both surfaces share this one path.
	resolved, err := gf.graph.ResolveUnionIDs(q.IDs)
	if err != nil {
		return nil, err
	}

	rendered := make(map[string]bool)
	primaries := make(map[string]bool, len(resolved))
	for _, id := range resolved {
		// Dedup keys are display identities (repo-prefixed for cross-repo
		// primaries); an unresolvable ID fails below with the caller's text.
		if key, ok := gf.graph.DisplayID(id); ok {
			primaries[key] = true
		}
	}

	groups := make([]query.ShowGroup, 0, len(resolved))
	for _, id := range resolved {
		tree := gf.graph.BuildShowTreeBounded(id, q.UpDepth, q.DownDepth, q.Budget, rendered, primaries)
		if tree == nil {
			return nil, fmt.Errorf("entry not found: %s", id)
		}

		groups = append(groups, query.ShowGroup{
			Primary:              tree.Primary,
			PrimaryID:            tree.PrimaryID,
			PrimaryStatus:        tree.PrimaryStatus,
			PrimarySupersedePath: tree.PrimarySupersedePath,
			PrimaryTopics:        tree.PrimaryTopics,
			Upstream:             tree.Upstream,
			Downstream:           tree.Downstream,
			UpstreamTruncated:    tree.UpstreamTruncated,
			DownstreamTruncated:  tree.DownstreamTruncated,
		})
	}

	return &query.ShowResult{Graph: gf.graph, Groups: groups}, nil
}
