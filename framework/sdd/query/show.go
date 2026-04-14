package query

import "github.com/networkteam/resonance/framework/sdd/model"

// ShowQuery captures intent to render a set of entries with their reference
// chains. Upstream is always included; downstream requires opt-in.
type ShowQuery struct {
	Graph      *model.Graph
	IDs        []string
	MaxDepth   int  // depth limit for upstream/downstream expansion; 0 = no expansion
	Downstream bool // include downstream entries (refd-by, closed-by, superseded-by)
}

// DefaultMaxDepth is the default upstream/downstream expansion depth,
// applied by the CLI when no --max-depth flag is given.
const DefaultMaxDepth = 4

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
