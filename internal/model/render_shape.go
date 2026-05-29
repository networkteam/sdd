package model

// RenderShape is the data-shape contract between finders (which produce
// per-section results) and presenters (which render them). Each concrete
// SectionData variant declares the shape it carries so a presenter can
// validate before dispatching — e.g. as-grouped expects a grouped shape;
// receiving a flat-list is a render-shape mismatch.
type RenderShape string

const (
	// ShapeFlatList is an ordered sequence of entries — consumed by as-list.
	ShapeFlatList RenderShape = "flat-list"
)

// SectionData is one section's typed result. Variants implement Shape() to
// declare their render-side contract. Slice 1 has only FlatList; later
// slices add Grouped, FocusBlock, etc.
type SectionData interface {
	Shape() RenderShape
}

// FlatList is an ordered sequence of entries — the shape consumed by the
// as-list presenter. Used for top(N), topic(L), and any pipeline that ends
// in as-list.
//
// Scores is parallel to Entries when the section was ranked: Scores[i]
// is the rank score of Entries[i]. When unranked (no rank() in the
// pipeline, or by(date) which sorts without scoring), Scores is nil.
// The renderer detects ranked output via len(Scores) == len(Entries).
//
// RefExpansions is parallel to Entries when the section used expand(refs):
// RefExpansions[i] is the ordered list of resolved outgoing refs for
// Entries[i] (possibly empty for an entry with no surviving refs). When
// expand(refs) is absent, RefExpansions is nil. The renderer detects
// expanded output via len(RefExpansions) == len(Entries).
type FlatList struct {
	Entries       []*Entry
	Scores        []float64
	RefExpansions [][]RefExpansion
}

// RefExpansion is one resolved outgoing reference for expand(refs) output.
// The finder resolves each row from the loaded graph: Kind is the ref's
// stored semantic kind (the presenter renders it as the sub-line verb), ID
// is the referenced entry's full ID, Status is that entry's derived status,
// and Desc is the optional per-ref rationale. A ref whose target is missing
// from the graph (dangling — lint surfaces it elsewhere) carries a zero
// Status, which renders without a status segment.
type RefExpansion struct {
	Kind   RefKind
	ID     string
	Status Status
	Desc   string
}

// Shape implements SectionData.
func (FlatList) Shape() RenderShape { return ShapeFlatList }
