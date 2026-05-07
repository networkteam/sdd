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
type FlatList struct {
	Entries []*Entry
}

// Shape implements SectionData.
func (FlatList) Shape() RenderShape { return ShapeFlatList }
