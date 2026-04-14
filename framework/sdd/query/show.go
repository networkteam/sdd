package query

import "github.com/networkteam/resonance/framework/sdd/model"

// ShowQuery captures intent to render a set of entries with their reference
// chains. Upstream refs and downstream entries are both included.
type ShowQuery struct {
	Graph    *model.Graph
	IDs      []string
	MaxDepth int // default 4; 0 means use default; -1 means unlimited
}

// DefaultMaxDepth is the default upstream/downstream expansion depth.
const DefaultMaxDepth = 4

// EffectiveMaxDepth returns the resolved max depth: default when 0,
// a very large number when -1 (unlimited), or the explicit value.
func (q ShowQuery) EffectiveMaxDepth() int {
	switch {
	case q.MaxDepth == 0:
		return DefaultMaxDepth
	case q.MaxDepth < 0:
		return 1000 // effectively unlimited
	default:
		return q.MaxDepth
	}
}

// ShowGroup is one primary's full tree: the primary entry, its upstream chain,
// and its downstream chain. Multiple groups are joined with separators.
type ShowGroup struct {
	Primary    *model.Entry
	Upstream   []model.ShowTreeItem
	Downstream []model.ShowTreeItem
}

// ShowResult is the structured output for a ShowQuery — one group per primary.
type ShowResult struct {
	Groups []ShowGroup
}
