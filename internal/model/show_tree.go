package model

import (
	"sort"

	"github.com/networkteam/sdd/pkg/application/types"
)

// ShowTreeItem represents an entry at a specific depth in a show tree.
// Depth 0 is the primary entry. Depth 1+ entries are summary-only.
type ShowTreeItem struct {
	Entry *Entry
	// CrossRepoID is the verbatim <repo-id>:<entry-id> when this node points
	// across the repo boundary. While the target's graph is not available
	// locally, Entry stays nil and the node renders unresolved; a resolved
	// remote node carries both — the remote Entry plus the prefixed ID the
	// renderer displays.
	CrossRepoID string
	Depth       int
	Relations   []string       // e.g. ["refs"], ["refs", "closes"], ["refd-by"]
	RefKind     RefKind        // kind of the refs edge linking parent to this entry (empty when no refs relation or no kind metadata)
	RefDesc     string         // desc of the refs edge (when present)
	ShownAbove  bool           // already rendered earlier — "(see above)" marker
	ShownBelow  bool           // future primary — "(see below)" marker
	SummaryOnly bool           // true for depth > 0
	Truncated   []TruncatedRef // children hidden at a frontier (depth, fan-out cap, or chain budget)
	// TruncatedReason names the frontier that hid Truncated when it is not
	// the depth limit ("fan-out cap", "chain budget"); empty means depth.
	TruncatedReason string
	Status          Status   // derived lifecycle status, computed at build so renderers stay pure
	SupersedePath   []string // resolved origin→head trail when superseded; nil otherwise
}

// NodeID is the identifier a renderer displays and dedup keys on: the
// prefixed cross-repo form when the node crosses the boundary, else the
// entry's own ID.
func (it ShowTreeItem) NodeID() string {
	if it.CrossRepoID != "" {
		return it.CrossRepoID
	}
	if it.Entry != nil {
		return it.Entry.ID
	}
	return ""
}

// TruncatedRef describes a child entry hidden at the depth boundary.
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
	Primary *Entry
	// PrimaryID is the display identity: bare for local and embedded
	// entries, repo-prefixed (<repo-id>:<entry-id>) for a member graph's
	// entry shown across the boundary.
	PrimaryID            string
	PrimaryStatus        Status
	PrimarySupersedePath []string
	PrimaryTopics        []TopicPath
	Upstream             []ShowTreeItem
	Downstream           []ShowTreeItem
	// UpstreamTruncated and DownstreamTruncated name the primary's own
	// children a chain budget or fan-out cap kept out of the direction
	// entirely — the direction-level honest frontier.
	UpstreamTruncated   []TruncatedRef
	DownstreamTruncated []TruncatedRef
}

// ShowTreeBudget is defined in pkg/application/types — the exported surface
// names it, so the definition lives in the cycle-free public leaf (s-tac-ah2).
type ShowTreeBudget = types.ShowTreeBudget

// treeWalk carries one direction's budget accounting through the recursion.
type treeWalk struct {
	budget ShowTreeBudget
	nodes  int
}

func (w *treeWalk) exhausted() bool {
	return w.budget.MaxNodes > 0 && w.nodes >= w.budget.MaxNodes
}

// childCap returns how many of n children may expand under the fan-out cap.
func (w *treeWalk) childCap(n int) int {
	if w.budget.MaxChildren > 0 && n > w.budget.MaxChildren {
		return w.budget.MaxChildren
	}
	return n
}

// BuildShowTree constructs the upstream and downstream traversal trees for a
// primary entry, each to its own depth, respecting cross-group dedup (rendered)
// and future-primary dedup (primaries). A depth of 0 skips that direction
// entirely. Both directions use per-direction visited sets. The rendered map is
// updated with newly-shown entries.
//
// The primary ID may be cross-repo (<repo-id>:<entry-id>): the tree is then
// built within the owning member graph, every node carrying that graph's
// repo prefix, and further cross-repo edges hop graphs the same way. Dedup
// keys are graph-qualified (nodeKeyFor) — the (repo-id, entry-id) pair in
// colon form — except embedded entries, which dedup by bare ID so exactly
// one copy ever surfaces.
func (g *Graph) BuildShowTree(id string, upDepth, downDepth int, rendered, primaries map[string]bool) *ShowTree {
	return g.BuildShowTreeBounded(id, upDepth, downDepth, ShowTreeBudget{}, rendered, primaries)
}

// BuildShowTreeBounded is BuildShowTree under a per-direction budget — the
// serve path's entry point, where chains must stay within their part cap.
func (g *Graph) BuildShowTreeBounded(id string, upDepth, downDepth int, budget ShowTreeBudget, rendered, primaries map[string]bool) *ShowTree {
	e, owner, ok := g.ResolveAcross(id)
	if !ok {
		return nil
	}
	key := owner.nodeKeyFor(e)

	// Upstream: expand primary's children (refs/closes/supersedes) directly.
	// The primary itself is rendered separately by the presenter.
	var upstream []ShowTreeItem
	var upstreamCut []TruncatedRef
	if upDepth > 0 {
		w := &treeWalk{budget: budget}
		upVisited := map[string]bool{key: true} // mark primary visited to prevent cycles back to it
		upstream, upstreamCut = owner.expandDirection(upstreamChildren(e), w, upVisited, rendered, func(child childEdge, visited map[string]bool) []ShowTreeItem {
			return owner.buildUpstream(child.id, 1, child.relations, child.refKind, child.refDesc, upDepth, w, visited, rendered, primaries)
		})
	}

	var downstream []ShowTreeItem
	var downstreamCut []TruncatedRef
	if downDepth > 0 {
		w := &treeWalk{budget: budget}
		downVisited := map[string]bool{key: true}
		downstream, downstreamCut = owner.expandDirection(owner.downstreamChildren(e.ID), w, downVisited, rendered, func(child childEdge, visited map[string]bool) []ShowTreeItem {
			return owner.buildDownstream(child.id, 1, child.relations, child.refKind, child.refDesc, downDepth, w, visited, rendered, primaries)
		})
	}

	// Mark items from this tree as rendered for cross-group dedup.
	markRendered(upstream, rendered)
	markRendered(downstream, rendered)
	rendered[key] = true

	status, supersedePath := owner.QualifiedItemStatus(e)
	return &ShowTree{
		Primary:              e,
		PrimaryID:            key,
		PrimaryStatus:        status,
		PrimarySupersedePath: supersedePath,
		PrimaryTopics:        owner.EffectiveTopics(e),
		Upstream:             upstream,
		Downstream:           downstream,
		UpstreamTruncated:    upstreamCut,
		DownstreamTruncated:  downstreamCut,
	}
}

// expandDirection walks the primary's children of one direction under the
// walk's budget: children past the fan-out cap or the node budget land in
// the direction-level truncated set instead of expanding.
func (g *Graph) expandDirection(children []childEdge, w *treeWalk, visited, rendered map[string]bool, expand func(childEdge, map[string]bool) []ShowTreeItem) ([]ShowTreeItem, []TruncatedRef) {
	var items []ShowTreeItem
	var cut []TruncatedRef
	limit := w.childCap(len(children))
	for i, child := range children {
		if i >= limit || w.exhausted() {
			cut = append(cut, g.unvisitedRefs(children[i:], visited, rendered)...)
			break
		}
		items = append(items, expand(child, visited)...)
	}
	return items, cut
}

func markRendered(items []ShowTreeItem, rendered map[string]bool) {
	for i := range items {
		if !items[i].ShownAbove && !items[i].ShownBelow {
			rendered[items[i].NodeID()] = true
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

// QualifiedItemStatus is itemStatus with the status target and supersede
// trail qualified for display outside the owning graph: a member graph's
// closing/superseding entries render repo-prefixed so the reader can place
// them. Head-walks themselves always ran within the owning graph — this
// only prefixes the resulting IDs.
func (g *Graph) QualifiedItemStatus(e *Entry) (Status, []string) {
	status, path := g.itemStatus(e)
	if g.repoPrefix == "" {
		return status, path
	}
	if status.By != "" {
		status.By = g.qualifyID(status.By)
	}
	for i, id := range path {
		path[i] = g.qualifyID(id)
	}
	return status, path
}

// buildUpstream walks the upstream reference chain in DFS pre-order with
// depth limit and summary-only rendering at depth > 0. The walk hops graphs
// at cross-repo edges: the edge resolves in the owning member graph and the
// recursion continues there, so remote chains traverse fully with each
// node qualified by its graph's repo prefix. An unresolvable cross-repo
// edge (repo not connected or entry absent) renders as an unresolved leaf.
func (g *Graph) buildUpstream(edgeID string, depth int, relations []string, refKind RefKind, refDesc string, maxDepth int, w *treeWalk, visited, rendered, primaries map[string]bool) []ShowTreeItem {
	e, owner, ok := g.ResolveAcross(edgeID)
	if !ok {
		if IsCrossRepoID(edgeID) {
			item := ShowTreeItem{
				CrossRepoID: edgeID,
				Depth:       depth,
				Relations:   relations,
				RefKind:     refKind,
				RefDesc:     refDesc,
				SummaryOnly: depth > 0,
			}
			if rendered[edgeID] || visited[edgeID] {
				item.ShownAbove = true
				return []ShowTreeItem{item}
			}
			visited[edgeID] = true
			return []ShowTreeItem{item}
		}
		return nil
	}
	key := owner.nodeKeyFor(e)

	item := ShowTreeItem{
		Entry:       e,
		Depth:       depth,
		Relations:   relations,
		RefKind:     refKind,
		RefDesc:     refDesc,
		SummaryOnly: depth > 0,
	}
	if key != e.ID {
		item.CrossRepoID = key
	}
	item.Status, item.SupersedePath = owner.QualifiedItemStatus(e)

	if rendered[key] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	if primaries[key] && depth > 0 {
		item.ShownBelow = true
		return []ShowTreeItem{item}
	}
	if visited[key] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	visited[key] = true
	w.nodes++

	children := upstreamChildren(e)
	if depth >= maxDepth && len(children) > 0 {
		item.Truncated = owner.unvisitedRefs(children, visited, rendered)
		return []ShowTreeItem{item}
	}

	limit := w.childCap(len(children))
	if limit < len(children) {
		item.Truncated = owner.unvisitedRefs(children[limit:], visited, rendered)
		item.TruncatedReason = "fan-out cap"
		children = children[:limit]
	}
	result := []ShowTreeItem{item}
	for i, child := range children {
		if w.exhausted() {
			result[0].Truncated = append(result[0].Truncated, owner.unvisitedRefs(children[i:], visited, rendered)...)
			result[0].TruncatedReason = "chain budget"
			break
		}
		result = append(result, owner.buildUpstream(child.id, depth+1, child.relations, child.refKind, child.refDesc, maxDepth, w, visited, rendered, primaries)...)
	}
	return result
}

// buildDownstream walks the downstream graph in DFS pre-order with depth
// limit. Downstream edges come from per-graph reverse indexes, which never
// hold cross-repo IDs (cross-graph backlinks are out of scope) — but a
// remote primary's within-graph downstream traverses its member graph the
// same way upstream does.
func (g *Graph) buildDownstream(edgeID string, depth int, relations []string, refKind RefKind, refDesc string, maxDepth int, w *treeWalk, visited, rendered, primaries map[string]bool) []ShowTreeItem {
	e, owner, ok := g.ResolveAcross(edgeID)
	if !ok {
		return nil
	}
	key := owner.nodeKeyFor(e)

	item := ShowTreeItem{
		Entry:       e,
		Depth:       depth,
		Relations:   relations,
		RefKind:     refKind,
		RefDesc:     refDesc,
		SummaryOnly: true,
	}
	if key != e.ID {
		item.CrossRepoID = key
	}
	item.Status, item.SupersedePath = owner.QualifiedItemStatus(e)

	if rendered[key] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	if primaries[key] && depth > 0 {
		item.ShownBelow = true
		return []ShowTreeItem{item}
	}
	if visited[key] {
		item.ShownAbove = true
		return []ShowTreeItem{item}
	}
	visited[key] = true
	w.nodes++

	children := owner.downstreamChildren(e.ID)
	if depth >= maxDepth && len(children) > 0 {
		item.Truncated = owner.unvisitedRefs(children, visited, rendered)
		return []ShowTreeItem{item}
	}

	limit := w.childCap(len(children))
	if limit < len(children) {
		item.Truncated = owner.unvisitedRefs(children[limit:], visited, rendered)
		item.TruncatedReason = "fan-out cap"
		children = children[:limit]
	}
	result := []ShowTreeItem{item}
	for i, child := range children {
		if w.exhausted() {
			result[0].Truncated = append(result[0].Truncated, owner.unvisitedRefs(children[i:], visited, rendered)...)
			result[0].TruncatedReason = "chain budget"
			break
		}
		result = append(result, owner.buildDownstream(child.id, depth+1, child.relations, child.refKind, child.refDesc, maxDepth, w, visited, rendered, primaries)...)
	}
	return result
}

// unvisitedRefs returns truncated refs for children not already visited/rendered.
// Child IDs are graph-qualified so a truncation line names targets a reader
// can follow from outside the owning graph.
func (g *Graph) unvisitedRefs(children []childEdge, visited, rendered map[string]bool) []TruncatedRef {
	var refs []TruncatedRef
	for _, c := range children {
		key := c.id
		var kind Kind
		if e, owner, ok := g.ResolveAcross(c.id); ok {
			key = owner.nodeKeyFor(e)
			kind = e.Kind
		}
		if !visited[key] && !rendered[key] {
			refs = append(refs, TruncatedRef{ID: key, Relations: c.relations, Kind: kind, RefKind: c.refKind, RefDesc: c.refDesc})
		}
	}
	return refs
}
