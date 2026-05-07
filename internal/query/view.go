package query

import "github.com/networkteam/sdd/internal/model"

// ViewQuery captures intent to execute a layout pipeline against the graph.
// The Layout AST is produced upstream by ParseLayout — the finder consumes
// the parsed shape rather than the raw `--layout` string.
type ViewQuery struct {
	Graph  *model.Graph
	Layout model.Layout
}

// ViewResult is the structured output of a ViewQuery: one SectionResult
// per section in the source layout, in the order they appeared.
type ViewResult struct {
	Graph    *model.Graph // needed by presenters for derived attributes (status, topics)
	Sections []SectionResult
}

// SectionResult is one section's render-ready data plus dispatch metadata.
// Render names the presenter that should consume Data; Data carries the
// shape-tagged result. The presenter validates that Data.Shape() matches
// what Render expects before rendering.
type SectionResult struct {
	Render string            // render function name (e.g. "as-list")
	Data   model.SectionData // shape-tagged data variant produced by the finder
}
