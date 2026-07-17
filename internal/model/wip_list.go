package model

// ShapeWipList is the result shape produced by `source(wip)` terminated
// by `as-wip-list`. Carries the active WIP markers in the order they
// were loaded — chronological by ID per Finder.LoadWIPMarkers.
const ShapeWipList RenderShape = "wip-list"

// WipList is the as-wip-list-bound SectionData variant. Markers are
// referenced by pointer so the renderer can read every field without
// copying — markers are immutable in-memory structures shared across the
// section's lifetime.
type WipList struct {
	Markers []*WIPMarker
}

// Shape implements SectionData.
func (WipList) Shape() RenderShape { return ShapeWipList }

// Count implements SectionData: the number of WIP markers produced.
func (w WipList) Count() int { return len(w.Markers) }
