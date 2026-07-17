package model

// ShapeGrouped marks results carrying named buckets of entries — produced
// by a section ending in `as-grouped` after a `group(by(<field>))`
// aggregation. The grouped shape is mutually exclusive with the flat
// shape (FlatList): a section produces one or the other, never both.
const ShapeGrouped RenderShape = "grouped"

// Group is one bucket inside a Grouped result. Key is the field value
// shared by every entry — e.g. for `group(by(kind))` over a decisions
// filter, Key would be "plan", "directive", and so on. Entries preserve
// the order they appeared in the pre-group input (no per-group ranking
// in slice 5; per-group sort is reserved for a later slice).
type Group struct {
	Key     string
	Entries []*Entry
}

// Grouped is the result shape produced by `group(by(<field>))`. Groups are
// ordered for stable rendering — slice 5 sorts alphabetically by Key,
// which is predictable but layer-blind. The `decisions`/`signals` macros
// in slice 6 can introduce field-aware ordering (e.g. plan, directive,
// activity, contract, aspiration) without changing this shape.
type Grouped struct {
	// Field names which entry attribute drove the grouping. Carried
	// through to renderers in case the section header benefits from it
	// (e.g. "Grouped by kind"); slice 5 doesn't render it but the value
	// is preserved end-to-end for slice 6's `name(...)` modifier.
	Field  string
	Groups []Group
}

// Shape implements SectionData.
func (Grouped) Shape() RenderShape { return ShapeGrouped }

// Count implements SectionData: total entries across all groups.
func (g Grouped) Count() int {
	n := 0
	for _, group := range g.Groups {
		n += len(group.Entries)
	}
	return n
}
