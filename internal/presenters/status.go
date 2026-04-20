package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderStatus writes the status view: top-line counts, then sections for
// contracts, plans, active decisions, open signals, and recent done signals.
// Each section is grouped by layer using LayerOrder.
func RenderStatus(w io.Writer, result *query.StatusResult) {
	g := result.Graph
	decisions := g.Filter(model.GraphFilter{Type: model.TypeDecision})
	signals := g.Filter(model.GraphFilter{Type: model.TypeSignal})
	fmt.Fprintf(w, "Graph: %d entries (%d decisions, %d signals)\n\n",
		len(g.Entries), len(decisions), len(signals))

	renderLayeredSection(w, g, "Contracts", result.Contracts)
	renderLayeredSection(w, g, "Aspirations", result.Aspirations)
	renderLayeredSection(w, g, "Plans", result.Plans)
	renderLayeredSection(w, g, "Active Decisions", result.Active)
	renderFlatSection(w, g, "Open Signals", result.Open)
	renderFlatSection(w, g, "Recent Done Signals", result.Recent)
}

func renderLayeredSection(w io.Writer, g *model.Graph, title string, entries []*model.Entry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "## %s\n\n", title)
	byLayer := GroupByLayer(entries)
	for _, layer := range LayerOrder() {
		group, ok := byLayer[layer]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "### %s\n", layer)
		for _, e := range group {
			EntryLine(w, e, g)
		}
		fmt.Fprintln(w)
	}
}

func renderFlatSection(w io.Writer, g *model.Graph, title string, entries []*model.Entry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "## %s\n\n", title)
	for _, e := range entries {
		EntryLine(w, e, g)
	}
	fmt.Fprintln(w)
}
