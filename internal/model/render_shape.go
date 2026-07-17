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
	// Count reports how many primary units the pipeline matched — entries,
	// focuses, actors, or WIP markers by shape; aggregating shapes like
	// as-counts report the entries aggregated, not the rows rendered. Zero
	// means the pipeline matched nothing, which lets callers distinguish an
	// empty result from a failure without re-deriving it from rendered text.
	Count() int
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
//
// SupersedePath is populated only when the target is superseded: the ordered
// supersession trail from the target (origin) to its live head (ResolveRef
// Path). The presenter renders the hops through to the head so a reader can
// connect a stale intermediate ID met elsewhere; it stays nil otherwise.
type RefExpansion struct {
	Kind          RefKind
	ID            string
	Status        Status
	Desc          string
	SupersedePath []string
	// UnresolvedRepo names the target repo of a cross-repo ref whose graph
	// is not available locally; the presenter renders it as
	// `[unresolved: repo <id>]` in place of a status segment.
	UnresolvedRepo string
}

// Shape implements SectionData.
func (FlatList) Shape() RenderShape { return ShapeFlatList }

// Count implements SectionData.
func (f FlatList) Count() int { return len(f.Entries) }
