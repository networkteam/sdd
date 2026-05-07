package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderView writes one section per SectionResult, dispatching to the
// per-render helper based on the section's Render name. Sections are
// separated by a blank line so visual clusters are obvious; section
// headers from the `name(...)` modifier arrive in slice 5.
//
// Render names are matched against the slice's known set; unknown names
// silently produce no output for that section. The finder validates the
// pipeline shape before reaching here, so an unknown name implies a
// programmer-side mistake (a layout produced by some path other than the
// parser+executor) rather than a user-facing error.
func RenderView(w io.Writer, result *query.ViewResult) {
	for i, section := range result.Sections {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderSection(w, result.Graph, section)
	}
}

func renderSection(w io.Writer, g *model.Graph, section query.SectionResult) {
	switch section.Render {
	case "as-list":
		flat, ok := section.Data.(model.FlatList)
		if !ok {
			return
		}
		renderAsList(w, g, flat)
	case "as-grouped":
		grouped, ok := section.Data.(model.Grouped)
		if !ok {
			return
		}
		renderAsGrouped(w, g, grouped)
	}
}
