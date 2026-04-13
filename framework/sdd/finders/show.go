package finders

import (
	"fmt"

	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// Show resolves the entries named in q and returns ordered groups of
// ShowItems suitable for hierarchical rendering. Each group corresponds to
// one requested primary ID. In downstream mode, the group contains the
// target plus its downstream entries (flat). In upstream mode (default),
// the group contains the primary plus its ref/closes/supersedes chain in
// pre-order, with deduplication across primaries.
func (f *Finder) Show(q query.ShowQuery) (*query.ShowResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	primaries := make(map[string]bool, len(q.IDs))
	for _, id := range q.IDs {
		primaries[id] = true
	}

	rendered := make(map[string]bool)
	groups := make([]query.ShowGroup, 0, len(q.IDs))

	for _, id := range q.IDs {
		if _, ok := q.Graph.ByID[id]; !ok {
			return nil, fmt.Errorf("entry not found: %s", id)
		}

		var items []query.ShowItem
		if q.Downstream {
			target := q.Graph.ByID[id]
			items = append(items, query.ShowItem{Entry: target, Depth: 0})
			rendered[target.ID] = true
			for _, e := range q.Graph.Downstream(id) {
				item := query.ShowItem{Entry: e, Depth: 1, Relation: "downstream"}
				if rendered[e.ID] {
					item.ShownAbove = true
				} else {
					rendered[e.ID] = true
				}
				items = append(items, item)
			}
		} else {
			visited := make(map[string]bool)
			items = buildShowTree(q.Graph, id, 0, "", visited, rendered, primaries)
			// Mark items rendered after the full tree is built — matches the
			// legacy single-pass deduplication (entries appearing later as refs
			// of the same primary become "(shown above)").
			for i := range items {
				e := items[i].Entry
				if items[i].ShownAbove || items[i].ShownBelow {
					continue
				}
				rendered[e.ID] = true
			}
		}

		groups = append(groups, query.ShowGroup{Items: items})
	}

	return &query.ShowResult{Groups: groups}, nil
}

// buildShowTree walks the upstream reference chain of an entry in pre-order,
// tracking depth. The root entry is at depth 0, its direct refs at depth 1,
// etc. Already-rendered entries appear with ShownAbove=true (no subtree
// traversal). Entries that will be rendered later as their own primary
// appear with ShownBelow=true (no subtree traversal).
func buildShowTree(g *model.Graph, id string, depth int, relation string, visited, rendered, primaries map[string]bool) []query.ShowItem {
	e, ok := g.ByID[id]
	if !ok {
		return nil
	}

	item := query.ShowItem{Entry: e, Depth: depth, Relation: relation}

	// Already shown earlier in another primary's section: heading + (shown above).
	if rendered[id] {
		item.ShownAbove = true
		return []query.ShowItem{item}
	}
	// Will be shown as its own primary later: heading + (shown below).
	if primaries[id] && depth > 0 {
		item.ShownBelow = true
		return []query.ShowItem{item}
	}
	// Already visited in this same tree walk: heading + (shown above), no recurse.
	if visited[id] {
		item.ShownAbove = true
		return []query.ShowItem{item}
	}
	visited[id] = true

	result := []query.ShowItem{item}
	for _, ref := range e.Refs {
		result = append(result, buildShowTree(g, ref, depth+1, "ref", visited, rendered, primaries)...)
	}
	for _, c := range e.Closes {
		result = append(result, buildShowTree(g, c, depth+1, "closes", visited, rendered, primaries)...)
	}
	for _, s := range e.Supersedes {
		result = append(result, buildShowTree(g, s, depth+1, "supersedes", visited, rendered, primaries)...)
	}
	return result
}
