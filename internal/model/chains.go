package model

import "sort"

// chainGroups groups the entries selected by member into supersession
// chains: each group holds the member entries transitively linked by
// supersedes edges (walked among member entries only — a supersedes edge to
// or from a non-member never joins two groups), ordered oldest first. Group
// order is deterministic by each group's oldest entry ID; callers that need
// a different order (e.g. by head ID) re-sort. A cycle in supersedes edges
// terminates the root walk at the first revisited entry instead of looping.
func (g *Graph) chainGroups(member func(*Entry) bool) [][]*Entry {
	byRoot := make(map[string][]*Entry)

	rootOf := func(e *Entry) *Entry {
		visited := map[string]bool{e.ID: true}
		current := e
		for {
			var parent *Entry
			for _, id := range current.Supersedes {
				p, ok := g.ByID[id]
				if ok && member(p) && !visited[p.ID] {
					parent = p
					break
				}
			}
			if parent == nil {
				return current
			}
			visited[parent.ID] = true
			current = parent
		}
	}

	for _, e := range g.Entries {
		if !member(e) {
			continue
		}
		root := rootOf(e)
		byRoot[root.ID] = append(byRoot[root.ID], e)
	}

	groups := make([][]*Entry, 0, len(byRoot))
	for _, group := range byRoot {
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].Time.Before(group[j].Time)
		})
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i][0].ID < groups[j][0].ID
	})
	return groups
}
