package query

import "github.com/networkteam/sdd/internal/model"

// ViewQuery captures intent to execute a layout pipeline against the graph.
// The Layout AST is produced upstream by ParseLayout — the finder consumes
// the parsed shape rather than the raw `--layout` string.
//
// GraphDir lets the finder source from disk when a section uses `source(wip)`
// — the executor calls LoadWIPMarkers against this directory rather than
// receiving pre-resolved markers in the query slot. Mirrors the shape used
// by WIPListQuery; can be left empty when no section needs it (the executor
// errors at section-evaluation time if source(wip) appears without a
// configured GraphDir). The broader CQRS leak across read queries is
// captured separately in s-tac-m09 — slice 8 follows the existing peer
// shape rather than diverging.
type ViewQuery struct {
	Graph    *model.Graph
	Layout   model.Layout
	GraphDir string
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
}
