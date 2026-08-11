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
// A section that matched nothing renders nothing at all, header included: a
// title with no content under it claims a structure the result does not have,
// and surfaces that need to say "matched nothing" say it once for the whole
// result rather than once per empty section. The exception is a render whose
// empty state is itself the message (see rendersWhenEmpty).
//
// Render names are matched against the slice's known set; unknown names
// silently produce no output for that section. The finder validates the
// pipeline shape before reaching here, so an unknown name implies a
// programmer-side mistake (a layout produced by some path other than the
// parser+executor) rather than a user-facing error.
func RenderView(w io.Writer, result *query.ViewResult) {
	written := 0
	for _, section := range result.Sections {
		if (section.Data == nil || section.Data.Count() == 0) && !rendersWhenEmpty[section.Render] {
			continue
		}
		if written > 0 {
			fmt.Fprintln(w)
		}
		if section.Name != "" {
			fmt.Fprintf(w, "## %s\n\n", section.Name)
		}
		renderSection(w, result.Graph, section)
		written++
	}
}

// sectionHeadingLevel is the level RenderView writes a named section's header
// at; embedded content nests below it.
const sectionHeadingLevel = 2

// rendersWhenEmpty names the renders whose empty state is itself the message —
// as-wip-list reports that no work is in flight, which is what a reader of that
// section wanted to know. Every other render contributes nothing when it matched
// nothing, so its section is dropped whole.
var rendersWhenEmpty = map[string]bool{"as-wip-list": true}

func renderSection(w io.Writer, g *model.Graph, section query.SectionResult) {
	switch section.Render {
	case "as-bodies":
		bodies, ok := section.Data.(model.Bodies)
		if !ok {
			return
		}
		entryLevel := sectionHeadingLevel + 1
		if section.Name == "" {
			entryLevel = sectionHeadingLevel
		}
		renderAsBodies(w, bodies, entryLevel)
	case "as-list":
		flat, ok := section.Data.(model.FlatList)
		if !ok {
			return
		}
		renderAsList(w, g, flat, section.Brief)
	case "as-grouped":
		grouped, ok := section.Data.(model.Grouped)
		if !ok {
			return
		}
		renderAsGrouped(w, g, grouped, section.Brief)
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
		renderAsFocusBlock(w, g, block, section.Brief)
	case "as-participants-block":
		block, ok := section.Data.(model.ParticipantsBlock)
		if !ok {
			return
		}
		renderAsParticipantsBlock(w, g, block, section.Brief)
	case "as-wip-list":
		list, ok := section.Data.(model.WipList)
		if !ok {
			return
		}
		renderAsWipList(w, list)
	}
}
