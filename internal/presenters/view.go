package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderView writes one section per SectionResult, dispatching to the
// per-render helper based on the section's Render name. Sections are
// separated by a blank line so visual clusters are obvious. A non-empty
// Section.Name renders as a `## <name>` header before the section body;
// empty Name omits the header (the as-list / as-grouped default).
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
		if section.Name != "" {
			fmt.Fprintf(w, "## %s\n\n", section.Name)
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
	case "as-counts":
		counts, ok := section.Data.(model.Counts)
		if !ok {
			return
		}
		renderAsCounts(w, counts)
	case "as-focus-block":
		block, ok := section.Data.(model.FocusBlock)
		if !ok {
			return
		}
		renderAsFocusBlock(w, g, block)
	case "as-participants-block":
		block, ok := section.Data.(model.ParticipantsBlock)
		if !ok {
			return
		}
		renderAsParticipantsBlock(w, g, block)
	case "as-wip-list":
		list, ok := section.Data.(model.WipList)
		if !ok {
			return
		}
		renderAsWipList(w, list)
	}
}
