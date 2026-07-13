package query

import "github.com/networkteam/sdd/internal/model"

// ViewQuery captures intent to execute a layout pipeline against the graph.
// The Layout AST is produced upstream by ParseLayout — the finder consumes
// the parsed shape rather than the raw `--layout` string.
//
// WIPMarkers lets storage-neutral callers provide the current markers.
// GraphDir remains the filesystem fallback for legacy callers. Both can be
// empty when the layout does not use source(wip).
type ViewQuery struct {
	Graph      *model.Graph
	Layout     model.Layout
	GraphDir   string
	WIPMarkers []*model.WIPMarker
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
