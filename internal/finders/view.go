package finders

import (
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// View executes the layout pipeline in q.Layout against q.Graph and returns
// one SectionResult per section in source order. Per the design, every
// section must terminate in a render function (e.g. as-list); render is
// always the pipeline's terminus.
//
// Slice 1 vocabulary is intentionally narrow — `active` (filter) and
// `as-list` (render). Unknown function names return an error listing the
// valid set so users (and future-slice tests) get a clear signal.
func (f *Finder) View(q query.ViewQuery) (*query.ViewResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	sections := make([]query.SectionResult, 0, len(q.Layout.Sections))
	for i, section := range q.Layout.Sections {
		sr, err := executeSection(q.Graph, section)
		if err != nil {
			return nil, fmt.Errorf("section %d: %w", i+1, err)
		}
		sections = append(sections, sr)
	}

	return &query.ViewResult{Graph: q.Graph, Sections: sections}, nil
}

// renderFunctions enumerates the function names that terminate a section.
// Slice 1 has only as-list; later slices add as-grouped, as-focus-block,
// as-participants-block, as-wip-list.
var renderFunctions = map[string]bool{
	"as-list": true,
}

// knownFunctions lists every function name the slice 1 executor recognizes.
// Used in the unknown-function error message so users see which names are
// currently available.
var knownFunctions = []string{"active", "as-list"}

// executeSection walks one section's pipeline left-to-right, accumulating
// filter intent and identifying the render terminator. Returns a
// SectionResult ready for the presenter to render.
func executeSection(g *model.Graph, section model.Section) (query.SectionResult, error) {
	if len(section.Functions) == 0 {
		// ParseLayout cannot produce empty sections, so this only fires
		// against programmatically built layouts. Defensive — keeps the
		// invariant explicit at the boundary.
		return query.SectionResult{}, fmt.Errorf("empty section")
	}

	var filter model.GraphFilter
	var renderName string

	for i, fn := range section.Functions {
		switch {
		case fn.Name == "active":
			filter.OpenOnly = true
		case renderFunctions[fn.Name]:
			if i != len(section.Functions)-1 {
				return query.SectionResult{}, fmt.Errorf(
					"render function %q must be the last function in a section, found at position %d of %d",
					fn.Name, i+1, len(section.Functions))
			}
			renderName = fn.Name
		default:
			return query.SectionResult{}, fmt.Errorf(
				"unknown function %q (known: %s)",
				fn.Name, strings.Join(knownFunctions, ", "))
		}
	}

	if renderName == "" {
		return query.SectionResult{}, fmt.Errorf(
			"section must end with a render function (one of: as-list)")
	}

	entries := g.Filter(filter)

	return query.SectionResult{
		Render: renderName,
		Data:   model.FlatList{Entries: entries},
	}, nil
}
