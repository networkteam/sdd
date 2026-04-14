package model

import "sort"

// ShowTreeItem represents an entry at a specific depth in a show tree.
// Depth 0 is the primary entry. Depth 1+ entries are summary-only.
type ShowTreeItem struct {
	Entry          *Entry
	Depth          int
	Relations      []string // e.g. ["refs"], ["refs", "closes"], ["refd-by"]
	ShownAbove     bool     // already rendered earlier — "(see above)" marker
	ShownBelow     bool     // future primary — "(see below)" marker
	SummaryOnly    bool     // true for depth > 0
	TruncatedIDs []string // IDs of children hidden at max-depth boundary
}

// ShowTree holds the upstream and downstream chains for a single primary entry.
type ShowTree struct {
	Primary    *Entry
	Upstream   []ShowTreeItem
	Downstream []ShowTreeItem
}

// BuildShowTree constructs the upstream and downstream traversal trees for
// a primary entry, respecting max depth, cross-group dedup (rendered), and
// future-primary dedup (primaries). Both directions use per-direction visited
// sets. The rendered map is updated with newly-shown entries.
func (g *Graph) BuildShowTree(id string, maxDepth int, rendered, primaries map[string]bool) *ShowTree {
	e := g.ByID[id]
	if e == nil {
		return nil
	}

	// Upstream: expand primary's children (refs/closes/supersedes) directly.
	// The primary itself is rendered separately by the presenter.
	upVisited := make(map[string]bool)
	upVisited[id] = true // mark primary visited to prevent cycles back to it
	var upstream []ShowTreeItem
	for _, child := range upstreamChildren(e) {
		upstream = append(upstream, g.buildUpstream(child.id, 1, child.relations, maxDepth, upVisited, rendered, primaries)...)
	}

	// Downstream: expand primary's downstream children directly.
	downVisited := make(map[string]bool)
	downVisited[id] = true
	var downstream []ShowTreeItem
	for _, child := range g.downstreamChildren(id) {
		downstream = append(downstream, g.buildDownstream(child.id, 1, child.relations, maxDepth, downVisited, rendered, primaries)...)
	}

	// Mark items from this tree as rendered for cross-group dedup.
	markRendered(upstream, rendered)
	markRendered(downstream, rendered)
	rendered[id] = true

	return &ShowTree{
		Primary:    e,
		Upstream:   upstream,
		Downstream: downstream,
	}
}

func markRendered(items []ShowTreeItem, rendered map[string]bool) {
	for i := range items {
		if !items[i].ShownAbove && !items[i].ShownBelow {
			rendered[items[i].Entry.ID] = true
		}
	}
}

// childEdge represents an edge to a child entry with merged relations.
type childEdge struct {
	id        string
	relations []string
	order     int // insertion order for stable sort
}

// upstreamChildren merges refs, closes, supersedes edges for an entry
// into deduplicated children with combined relation labels.
func upstreamChildren(e *Entry) []childEdge {
	m := make(map[string]*childEdge)
	var order int

	add := func(id, relation string) {
		if ce, ok := m[id]; ok {
			ce.relations = append(ce.relations, relation)
		} else {
			m[id] = &childEdge{id: id, relations: []string{relation}, order: order}
			order++
		}
	}

	for _, ref := range e.Refs {
		add(ref, "refs")
	}
	for _, c := range e.Closes {
		add(c, "closes")
	}
	for _, s := range e.Supersedes {
		add(s, "supersedes")
	}

	result := make([]childEdge, 0, len(m))
	for _, ce := range m {
		result = append(result, *ce)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].order < result[j].order
	})
	return result
}

// downstreamChildren merges refd-by, closed-by, superseded-by edges for an
// entry into deduplicated children with combined relation labels, sorted by time.
func (g *Graph) downstreamChildren(id string) []childEdge {
	m := make(map[string]*childEdge)

	add := func(eid, relation string) {
		if ce, ok := m[eid]; ok {
			ce.relations = append(ce.relations, relation)
		} else {
			m[eid] = &childEdge{id: eid, relations: []string{relation}}
		}
	}

	for _, eid := range g.RefsTo[id] {
		add(eid, "refd-by")
	}
	for _, eid := range g.ClosedBy[id] {
		add(eid, "closed-by")
	}
	for _, eid := range g.SupersededBy[id] {
		add(eid, "superseded-by")
	}

	result := make([]childEdge, 0, len(m))
	for _, ce := range m {
		result = append(result, *ce)
	}
	sort.Slice(result, func(i, j int) bool {
		ei := g.ByID[result[i].id]
		ej := g.ByID[result[j].id]
		if ei == nil || ej == nil {
			return result[i].id < result[j].id
		}
		return ei.Time.Before(ej.Time)
	})
	return result
}

// buildUpstream walks the upstream reference chain in DFS pre-order with
// depth limit and summary-only rendering at depth > 0.
func (g *Graph) buildUpstream(id string, depth int, relations []string, maxDepth int, visited, rendered, primaries map[string]bool) []ShowTreeItem {
	e, ok := g.ByID[id]
	if !ok {
		return nil
	}

	item := ShowTreeItem{
		Entry:       e,
		Depth:       depth,
		Relations:   relations,
		SummaryOnly: depth > 0,
	}

	if rendered[id] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	if primaries[id] && depth > 0 {
		item.ShownBelow = true
		return []ShowTreeItem{item}
	}
	if visited[id] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	visited[id] = true

	children := upstreamChildren(e)
	if depth >= maxDepth && len(children) > 0 {
		item.TruncatedIDs = unvisitedIDs(children, visited, rendered)
		return []ShowTreeItem{item}
	}

	result := []ShowTreeItem{item}
	for _, child := range children {
		result = append(result, g.buildUpstream(child.id, depth+1, child.relations, maxDepth, visited, rendered, primaries)...)
	}
	return result
}

// buildDownstream walks the downstream graph in DFS pre-order with depth limit.
func (g *Graph) buildDownstream(id string, depth int, relations []string, maxDepth int, visited, rendered, primaries map[string]bool) []ShowTreeItem {
	e, ok := g.ByID[id]
	if !ok {
		return nil
	}

	item := ShowTreeItem{
		Entry:       e,
		Depth:       depth,
		Relations:   relations,
		SummaryOnly: true,
	}

	if rendered[id] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	if primaries[id] && depth > 0 {
		item.ShownBelow = true
		return []ShowTreeItem{item}
	}
	if visited[id] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	visited[id] = true

	children := g.downstreamChildren(id)
	if depth >= maxDepth && len(children) > 0 {
		item.TruncatedIDs = unvisitedIDs(children, visited, rendered)
		return []ShowTreeItem{item}
	}

	result := []ShowTreeItem{item}
	for _, child := range children {
		result = append(result, g.buildDownstream(child.id, depth+1, child.relations, maxDepth, visited, rendered, primaries)...)
	}
	return result
}

// unvisitedIDs returns IDs of children that would be new (not already visited/rendered).
func unvisitedIDs(children []childEdge, visited, rendered map[string]bool) []string {
	var ids []string
	for _, c := range children {
		if !visited[c.id] && !rendered[c.id] {
			ids = append(ids, c.id)
		}
	}
	return ids
}
