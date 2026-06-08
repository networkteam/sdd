package model

import "sort"

// ShowTreeItem represents an entry at a specific depth in a show tree.
// Depth 0 is the primary entry. Depth 1+ entries are summary-only.
type ShowTreeItem struct {
	Entry         *Entry
	Depth         int
	Relations     []string       // e.g. ["refs"], ["refs", "closes"], ["refd-by"]
	RefKind       RefKind        // kind of the refs edge linking parent to this entry (empty when no refs relation or no kind metadata)
	RefDesc       string         // desc of the refs edge (when present)
	ShownAbove    bool           // already rendered earlier — "(see above)" marker
	ShownBelow    bool           // future primary — "(see below)" marker
	SummaryOnly   bool           // true for depth > 0
	Truncated     []TruncatedRef // children hidden at max-depth boundary
	Status        Status         // derived lifecycle status, computed at build so renderers stay pure
	SupersedePath []string       // resolved origin→head trail when superseded; nil otherwise
}

// TruncatedRef describes a child entry hidden at the max-depth boundary.
type TruncatedRef struct {
	ID        string
	Relations []string
	Kind      Kind
	RefKind   RefKind // kind of the refs edge into this truncated entry
	RefDesc   string  // desc of the refs edge
}

// ShowTree holds the upstream and downstream chains for a single primary entry.
// Primary-derived attributes (status, supersede trail, effective topics) are
// computed during the build so presenters consume precomputed values and stay
// pure — no graph traversal at render time.
type ShowTree struct {
	Primary              *Entry
	PrimaryStatus        Status
	PrimarySupersedePath []string
	PrimaryTopics        []TopicPath
	Upstream             []ShowTreeItem
	Downstream           []ShowTreeItem
}

// BuildShowTree constructs the upstream and optionally downstream traversal
// trees for a primary entry, respecting max depth, cross-group dedup
// (rendered), and future-primary dedup (primaries). Both directions use
// per-direction visited sets. The rendered map is updated with newly-shown entries.
func (g *Graph) BuildShowTree(id string, maxDepth int, includeDownstream bool, rendered, primaries map[string]bool) *ShowTree {
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
		upstream = append(upstream, g.buildUpstream(child.id, 1, child.relations, child.refKind, child.refDesc, maxDepth, upVisited, rendered, primaries)...)
	}

	// Downstream: only when requested.
	var downstream []ShowTreeItem
	if includeDownstream {
		downVisited := make(map[string]bool)
		downVisited[id] = true
		for _, child := range g.downstreamChildren(id) {
			downstream = append(downstream, g.buildDownstream(child.id, 1, child.relations, child.refKind, child.refDesc, maxDepth, downVisited, rendered, primaries)...)
		}
	}

	// Mark items from this tree as rendered for cross-group dedup.
	markRendered(upstream, rendered)
	markRendered(downstream, rendered)
	rendered[id] = true

	status, supersedePath := g.itemStatus(e)
	return &ShowTree{
		Primary:              e,
		PrimaryStatus:        status,
		PrimarySupersedePath: supersedePath,
		PrimaryTopics:        g.EffectiveTopics(e),
		Upstream:             upstream,
		Downstream:           downstream,
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
// refKind / refDesc carry the per-ref metadata when one of the relations
// is "refs" or "refd-by"; for closes/supersedes (and their inverses) the
// fields stay empty because those relations are uniform by design.
type childEdge struct {
	id        string
	relations []string
	refKind   RefKind
	refDesc   string
	order     int // insertion order for stable sort
}

// upstreamChildren merges refs, closes, supersedes edges for an entry
// into deduplicated children with combined relation labels. The Ref
// metadata on the source entry flows through to the edge so renderers
// can show why each reference exists.
func upstreamChildren(e *Entry) []childEdge {
	m := make(map[string]*childEdge)
	var order int

	add := func(id, relation string, kind RefKind, desc string) {
		if ce, ok := m[id]; ok {
			ce.relations = append(ce.relations, relation)
			if kind != "" && ce.refKind == "" {
				ce.refKind = kind
				ce.refDesc = desc
			}
			return
		}
		m[id] = &childEdge{id: id, relations: []string{relation}, order: order, refKind: kind, refDesc: desc}
		order++
	}

	for _, ref := range e.Refs {
		add(ref.ID, "refs", ref.Kind, ref.Desc)
	}
	for _, c := range e.Closes {
		add(c, "closes", "", "")
	}
	for _, s := range e.Supersedes {
		add(s, "supersedes", "", "")
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
// entry into deduplicated children with combined relation labels, sorted
// by time. For refd-by edges, the kind/desc from the source entry's Ref
// pointing at id flows through so renderers can show why each downstream
// entry references this target.
func (g *Graph) downstreamChildren(id string) []childEdge {
	m := make(map[string]*childEdge)

	add := func(eid, relation string, kind RefKind, desc string) {
		if ce, ok := m[eid]; ok {
			ce.relations = append(ce.relations, relation)
			if kind != "" && ce.refKind == "" {
				ce.refKind = kind
				ce.refDesc = desc
			}
			return
		}
		m[eid] = &childEdge{id: eid, relations: []string{relation}, refKind: kind, refDesc: desc}
	}

	for _, eid := range g.RefsTo[id] {
		kind, desc := refMetaFrom(g.ByID[eid], id)
		add(eid, "refd-by", kind, desc)
	}
	for _, eid := range g.ClosedBy[id] {
		add(eid, "closed-by", "", "")
	}
	for _, eid := range g.SupersededBy[id] {
		add(eid, "superseded-by", "", "")
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

// refMetaFrom returns the kind and desc from source's Ref pointing at
// target.id, or empty values when source is nil or carries no such ref.
// Used to enrich downstream edges with the metadata that lives on the
// referring entry rather than the target.
func refMetaFrom(source *Entry, targetID string) (RefKind, string) {
	if source == nil {
		return "", ""
	}
	for _, r := range source.Refs {
		if r.ID == targetID {
			return r.Kind, r.Desc
		}
	}
	return "", ""
}

// itemStatus derives a tree node's lifecycle status and, when superseded, the
// resolved origin→head trail used for trail rendering. Computed during tree
// building so presenters consume precomputed values rather than deriving status
// themselves (keeps the renderers pure — no graph traversal at render time).
func (g *Graph) itemStatus(e *Entry) (Status, []string) {
	status := g.DerivedStatus(e)
	if status.Kind == StatusSupersededBy {
		return status, g.ResolveRef(e.ID).Path()
	}
	return status, nil
}

// buildUpstream walks the upstream reference chain in DFS pre-order with
// depth limit and summary-only rendering at depth > 0.
func (g *Graph) buildUpstream(id string, depth int, relations []string, refKind RefKind, refDesc string, maxDepth int, visited, rendered, primaries map[string]bool) []ShowTreeItem {
	e, ok := g.ByID[id]
	if !ok {
		return nil
	}

	item := ShowTreeItem{
		Entry:       e,
		Depth:       depth,
		Relations:   relations,
		RefKind:     refKind,
		RefDesc:     refDesc,
		SummaryOnly: depth > 0,
	}
	item.Status, item.SupersedePath = g.itemStatus(e)

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
		item.Truncated = g.unvisitedRefs(children, visited, rendered)
		return []ShowTreeItem{item}
	}

	result := []ShowTreeItem{item}
	for _, child := range children {
		result = append(result, g.buildUpstream(child.id, depth+1, child.relations, child.refKind, child.refDesc, maxDepth, visited, rendered, primaries)...)
	}
	return result
}

// buildDownstream walks the downstream graph in DFS pre-order with depth limit.
func (g *Graph) buildDownstream(id string, depth int, relations []string, refKind RefKind, refDesc string, maxDepth int, visited, rendered, primaries map[string]bool) []ShowTreeItem {
	e, ok := g.ByID[id]
	if !ok {
		return nil
	}

	item := ShowTreeItem{
		Entry:       e,
		Depth:       depth,
		Relations:   relations,
		RefKind:     refKind,
		RefDesc:     refDesc,
		SummaryOnly: true,
	}
	item.Status, item.SupersedePath = g.itemStatus(e)

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
		item.Truncated = g.unvisitedRefs(children, visited, rendered)
		return []ShowTreeItem{item}
	}

	result := []ShowTreeItem{item}
	for _, child := range children {
		result = append(result, g.buildDownstream(child.id, depth+1, child.relations, child.refKind, child.refDesc, maxDepth, visited, rendered, primaries)...)
	}
	return result
}

// unvisitedRefs returns truncated refs for children not already visited/rendered.
func (g *Graph) unvisitedRefs(children []childEdge, visited, rendered map[string]bool) []TruncatedRef {
	var refs []TruncatedRef
	for _, c := range children {
		if !visited[c.id] && !rendered[c.id] {
			ref := TruncatedRef{ID: c.id, Relations: c.relations, RefKind: c.refKind, RefDesc: c.refDesc}
			if e, ok := g.ByID[c.id]; ok {
				ref.Kind = e.Kind
			}
			refs = append(refs, ref)
		}
	}
	return refs
}
