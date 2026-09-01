package query

import (
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/application/types"
)

// ViewQuery captures intent to execute a layout pipeline. Pure intent: the
// Layout AST is produced upstream by ParseLayout — the finder consumes the
// parsed shape rather than the raw `--layout` string. The graph and any WIP
// markers the pipeline needs are held by the GraphFinder that runs the query,
// not carried here.
type ViewQuery struct {
	Layout model.Layout
	// Budget bounds the view's scaling parts at whole-unit boundaries on
	// the serve path. Zero values are unbounded — explicit pulls arrive
	// complete (d-tac-rzi).
	Budget ViewBudget
}

// ViewBudget is defined in pkg/application/types — the exported surface names
// it, so the definition lives in the cycle-free public leaf (s-tac-ah2).
type ViewBudget = types.ViewBudget

// ViewResult is the structured output of a ViewQuery: one SectionResult
// per section in the source layout, in the order they appeared.
type ViewResult struct {
	Graph    *model.Graph // needed by presenters for derived attributes (status, topics)
	Sections []SectionResult
}

// MatchedCount sums the primary units across every section — the total the
// pipeline produced. Zero means the layout matched nothing, which callers use
// to render an explicit empty result instead of a blank string.
func (r ViewResult) MatchedCount() int {
	total := 0
	for _, s := range r.Sections {
		if s.Data != nil {
			total += s.Data.Count()
		}
	}
	return total
}

// SectionResult is one section's render-ready data plus dispatch metadata.
// Render names the presenter that should consume Data; Data carries the
// shape-tagged result. The presenter validates that Data.Shape() matches
// what Render expects before rendering.
//
// Name carries the section header set by `name(string)`. Empty Name means
// the section renders without a `## <title>` header (the slice 5/6 default
// for as-list and as-grouped).
type SectionResult struct {
	Render string            // render function name (e.g. "as-list")
	Name   string            // section header set by `name(string)`; empty = no header
	Data   model.SectionData // shape-tagged data variant produced by the finder
	// Brief switches entry-line rendering to the compact form: identity
	// qualifiers plus the first summary sentence, no attribute segments.
	Brief bool
}
