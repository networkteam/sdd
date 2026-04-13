package query

import "github.com/networkteam/resonance/framework/sdd/model"

// ShowQuery captures intent to render a set of entries with their reference
// chains (or downstream entries when Downstream is true).
type ShowQuery struct {
	Graph      *model.Graph
	IDs        []string
	Downstream bool
}

// ShowItem represents an entry at a specific depth in the show tree.
// The finder produces a slice of these per primary; the presenter renders
// them as nested markdown headings.
type ShowItem struct {
	Entry      *model.Entry
	Depth      int
	Relation   string // "", "ref", "closes", "supersedes", "downstream"
	ShownAbove bool   // already rendered earlier — heading + "(shown above)" only
	ShownBelow bool   // future primary — heading + "(shown below)" only
}

// ShowGroup is one primary's full tree (the primary plus its dependent items
// in pre-order). Multiple groups are joined with separators by the presenter.
type ShowGroup struct {
	Items []ShowItem
}

// ShowResult is the structured output for a ShowQuery — one group per primary.
type ShowResult struct {
	Groups []ShowGroup
}
